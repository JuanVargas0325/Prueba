# Paso 20 · Dominio: fechas y semanas

> Rama `paso-a-paso` · paso 20 de 65 · anterior: **Paso 19 · Endpoints de usuarios** · siguiente: **Paso 21 · Dominio: períodos académicos**

---

## 1. Qué se hizo en este paso

Se agregaron al dominio las funciones para trabajar con **fechas de calendario** y **semanas de lunes a domingo**, la unidad de tiempo de todo el sistema.

| Archivo | Cambio |
|---|---|
| `backend/internal/dominio/semana.go` | **Nuevo.** `FormatoFecha`, `Fecha`, `ParsearFecha`, `InicioSemana`, `FinSemana`, `SemanaActual` y `SemanaCerrada`. |
| `backend/internal/dominio/semana_test.go` | **Nuevo.** Pruebas con casos límite: domingo, cruce de año y cambio de día por zona horaria. |

---

## 2. Por qué

Casi todas las reglas del proyecto dependen de la semana:
- **RF-08.1 / RN-29:** la semana operativa va de **lunes a domingo**.
- **RF-08.2:** las tareas solo se editan mientras su semana está **activa**.
- **RF-08.4 / RN-32:** registrar una tarea en una semana ya cerrada la marca como **reporte tardío**.
- **RF-09:** una semana cerrada sin tareas queda **sin reporte**.
- **RF-14:** los reportes con IA son de una semana concreta.

Si cada módulo calculara las semanas a su manera, aparecerían diferencias sutiles (¿el domingo es fin o inicio de semana?, ¿en qué zona horaria?). Por eso se resuelve **una vez**, en el dominio, con pruebas de los casos difíciles.

---

## 3. Explicación detallada

### 3.1 Cómo se representan las fechas

```go
const FormatoFecha = "2006-01-02"
```
- En Go, los formatos de fecha se escriben **con una fecha de ejemplo fija**: el 2 de enero de 2006 a las 15:04:05 (1-2-3-4-5-6). `"2006-01-02"` significa "año de 4 dígitos, mes de 2, día de 2": el formato ISO `AAAA-MM-DD` que usa la API.

> Las fechas sin hora (`DATE` en PostgreSQL) se representan como `time.Time` **a medianoche UTC**. Así una fecha no cambia de día al pasar entre zonas horarias.

Un `time.Time` siempre tiene hora y zona. Si "12 de octubre" se guardara como medianoche de Bogotá y luego se convirtiera a UTC, quedaría "12 de octubre 05:00", y en otra zona podría verse como el 11. Al fijar todas las fechas a **medianoche UTC**, el día de calendario nunca se mueve.

### 3.2 Funciones

```go
func Fecha(t time.Time, zona *time.Location) time.Time {
	if zona != nil {
		t = t.In(zona)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
```
- Convierte un **instante** (por ejemplo, "ahora") en la **fecha de calendario** que le corresponde en la zona dada.
- Ejemplo de la prueba: el lunes 12 de octubre de 2026 a las 02:00 **UTC** todavía es **domingo 11** en Bogotá (UTC−5). `Fecha(instante, bogota)` devuelve 2026-10-11. Sin la zona horaria, un monitor que reporta el domingo a las 9 p. m. vería su tarea en la semana siguiente.

```go
func ParsearFecha(texto string) (time.Time, error) {
	return time.ParseInLocation(FormatoFecha, strings.TrimSpace(texto), time.UTC)
}
```
- Interpreta `"2026-10-08"` como medianoche UTC de ese día. Falla con otros formatos (`"14/10/2026"`).

```go
func InicioSemana(fecha time.Time) time.Time {
	d := time.Date(fecha.Year(), fecha.Month(), fecha.Day(), 0, 0, 0, 0, time.UTC)
	desplazamiento := (int(d.Weekday()) + 6) % 7 // lunes=0 ... domingo=6
	return d.AddDate(0, 0, -desplazamiento)
}
```
- Devuelve el **lunes** de la semana de cualquier fecha.
- `Weekday()` numera los días a la manera estadounidense: domingo = 0, lunes = 1, …, sábado = 6. Con `(día + 6) % 7` se reacomoda a lunes = 0, …, domingo = 6, que es exactamente cuántos días hay que retroceder.
  - jueves (4) → (4+6) % 7 = 3 → retrocede 3 días → lunes;
  - domingo (0) → (0+6) % 7 = 6 → retrocede 6 días → el lunes anterior.
- `%` es el resto de la división. `AddDate(0, 0, -n)` resta n días.
- Toda semana se identifica por **su lunes**. Por eso la tabla `tareas` exige que `semana_inicio` sea lunes (paso 4).

```go
func FinSemana(fecha time.Time) time.Time {
	return InicioSemana(fecha).AddDate(0, 0, 6)
}

func SemanaActual(ahora time.Time, zona *time.Location) time.Time {
	return InicioSemana(Fecha(ahora, zona))
}

func SemanaCerrada(semanaInicio, ahora time.Time, zona *time.Location) bool {
	return InicioSemana(semanaInicio).Before(SemanaActual(ahora, zona))
}
```
- `FinSemana`: el domingo (lunes + 6 días).
- `SemanaActual`: el lunes de la semana en curso **según la zona horaria**.
- `SemanaCerrada`: una semana está cerrada si su lunes es anterior al lunes de la semana actual. Es la base de la tardanza (RN-32) y de la inmutabilidad de las tareas (RN-34).
- Las funciones reciben `ahora` como parámetro en lugar de llamar a `time.Now()`: así las pruebas pueden fijar el "hoy" que quieran. En el paso 27 se agregará un **reloj inyectable** con la misma idea para los servicios.

### 3.3 Pruebas (`semana_test.go`)

```go
func fecha(t *testing.T, texto string) time.Time { ... }
```
- Auxiliar para escribir fechas legibles en las pruebas: `fecha(t, "2026-10-05")`. Se reutilizará en las pruebas de reglas.

| Prueba | Casos |
|---|---|
| `TestInicioSemanaDevuelveLunes` | Lunes → el mismo; jueves → lunes; **domingo → el lunes anterior**; **1 de enero de 2026 (jueves) → 29 de diciembre de 2025** (cruce de año). |
| `TestFinSemanaDevuelveDomingo` | Miércoles 7 → domingo 11. |
| `TestFechaUsaLaZonaHoraria` | 02:00 UTC del lunes 12 es domingo 11 en Bogotá, y la semana actual es la del 5. |
| `TestSemanaCerrada` | La semana en curso no está cerrada; la anterior sí. |
| `TestParsearFechaRechazaFormatoInvalido` | `14/10/2026` es inválido. |

Las pruebas usan un **mapa entrada → resultado esperado** y un ciclo: es el estilo de *pruebas por tabla*, muy común en Go.

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Formato de fechas | Se escribe con la fecha de referencia `2006-01-02 15:04:05`. |
| `time.Date(...)` | Construye una fecha con año, mes, día, hora y zona. |
| `t.In(zona)` | El mismo instante visto en otra zona horaria. |
| `AddDate(años, meses, días)` | Suma o resta tiempo de calendario. |
| `Before` / `After` | Comparación de instantes. |
| `%` | Resto de la división entera. |
| Pruebas por tabla | Mapa o lista de casos recorrida en un ciclo. |

---

## 5. Cómo comprobarlo

```bash
cd backend
go test ./internal/dominio/ -v -run 'Semana|Fecha'
```

---

## 6. Pasos para lograr esta versión desde el paso 19

1. Crear `backend/internal/dominio/semana.go` con `FormatoFecha`, el comentario sobre fechas a medianoche UTC y las seis funciones.
2. Crear `backend/internal/dominio/semana_test.go` con el auxiliar `fecha` y las cinco pruebas.
3. `make pruebas lint`
4. Commit: `git add . && git commit -m "Paso 20: dominio de fechas y semanas"`

---

## 7. Preparación para las próximas entregas

- **Todo depende de `APP_ZONA_HORARIA`, no de la zona del servidor.** Las máquinas de la nube suelen estar en UTC; gracias a esto, las semanas se siguen calculando en hora de Bogotá aunque la API corra en otra región.
- La futura notificación de los viernes (fuera del alcance de la Entrega 1) podrá usar estas mismas funciones para saber qué semana está por cerrar.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-08.1 / RN-29 Semana de lunes a domingo | `InicioSemana`, `FinSemana`. |
| RF-08.2 Semana activa | `SemanaActual` con zona horaria. |
| RN-32 Tardanza | `SemanaCerrada` (se usará en el paso 34). |
