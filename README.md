# Paso 27 · Reloj inyectable

> Rama `paso-a-paso` · paso 27 de 65 · anterior: **Paso 26 · Repositorio de espacios** · siguiente: **Paso 28 · Servicio de espacios**

---

## 1. Qué se hizo en este paso

Se creó el tipo `Reloj` en la capa de servicio: la **única** fuente de "qué hora es" para los servicios, siempre en la zona horaria de la aplicación.

| Archivo | Cambio |
|---|---|
| `backend/internal/servicio/reloj.go` | **Nuevo.** `Reloj`, `NuevoReloj`, `RelojFijo` y los métodos `Ahora`, `Zona`, `Hoy` y `SemanaActual`. |

---

## 2. Por qué

Muchas reglas dependen del momento actual:
- ¿El período ya terminó? (crear espacios, paso 28)
- ¿La semana de esta tarea sigue activa? (editar tareas, paso 36)
- ¿Esta tarea es tardía? (paso 36)
- ¿Cuál es la semana en curso? (seguimiento, paso 43; reportes, paso 48)

Si cada servicio llamara a `time.Now()`:
1. **No se podría probar** una regla como "una tarea de la semana pasada es tardía" sin esperar a que pase una semana, o las pruebas cambiarían de resultado según el día en que se ejecuten.
2. Cada uno tendría que recordar convertir a la zona de Bogotá.

Con un `Reloj` que se **inyecta** (se pasa como dependencia), en producción se usa la hora real y en las pruebas una hora fija, por ejemplo "miércoles 14 de octubre de 2026 a las 3 p. m.".

---

## 3. Explicación detallada de `reloj.go`

```go
type Reloj struct {
	zona  *time.Location
	ahora func() time.Time
}
```
- Guarda la zona horaria y una **función** que devuelve la hora (la misma técnica del gestor de tokens del paso 12).

```go
func NuevoReloj(zona *time.Location) Reloj {
	if zona == nil {
		zona = time.UTC
	}
	return Reloj{zona: zona, ahora: time.Now}
}
```
- Reloj real: `ahora` es `time.Now`. Si no se indica zona, usa UTC (nunca queda `nil`, para que los métodos no fallen).
- Se devuelve **por valor** (`Reloj`, no `*Reloj`): es pequeño e inmutable, así que copiarlo es seguro y barato.

```go
func RelojFijo(instante time.Time, zona *time.Location) Reloj {
	r := NuevoReloj(zona)
	r.ahora = func() time.Time { return instante }
	return r
}
```
- Reloj de prueba: siempre devuelve el mismo instante. La función anónima "captura" `instante` (closure).

```go
func (r Reloj) Ahora() time.Time { return r.ahora() }
func (r Reloj) Zona() *time.Location { return r.zona }
func (r Reloj) Hoy() time.Time { return dominio.Fecha(r.ahora(), r.zona) }
func (r Reloj) SemanaActual() time.Time { return dominio.SemanaActual(r.ahora(), r.zona) }
```
- `Hoy` y `SemanaActual` combinan la hora actual con las funciones de fechas del dominio (paso 20), ya con la zona correcta. Los servicios piden directamente lo que necesitan: `s.reloj.Hoy()`, `s.reloj.SemanaActual()`.

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Inyección de dependencias | El servicio recibe el reloj en lugar de crearlo; las pruebas pasan uno fijo. |
| Valor vs. puntero | Tipos pequeños e inmutables se pasan por valor. |
| Closure | `func() time.Time { return instante }` recuerda `instante`. |

---

## 5. Cómo comprobarlo

```bash
make lint
```

El reloj se usará y probará a través de los servicios desde el paso 28 (`RelojFijo(ahoraPrueba, time.UTC)` en las pruebas).

---

## 6. Pasos para lograr esta versión desde el paso 26

1. Crear `backend/internal/servicio/reloj.go` con `Reloj`, `NuevoReloj`, `RelojFijo`, `Ahora`, `Zona`, `Hoy` y `SemanaActual`.
2. `make lint`
3. Commit: `git add . && git commit -m "Paso 27: reloj inyectable"`

---

## 7. Preparación para las próximas entregas

- **La zona de negocio no depende del servidor.** En la nube las máquinas suelen estar en UTC; el `Reloj` siempre razona en `APP_ZONA_HORARIA`.
- Si en el futuro se necesitara "simular" otra fecha en un ambiente de pruebas (por ejemplo, para la sustentación), bastaría con un reloj desplazado, sin tocar los servicios.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-08.1 / RF-08.2 Semana activa | `SemanaActual` con la zona horaria de la aplicación. |
| RNF-05 Mantenibilidad | Reglas de tiempo probables con una fecha fija. |
