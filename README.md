# Paso 30 · Dominio: vinculaciones y reglas de horas

> Rama `paso-a-paso` · paso 30 de 65 · anterior: **Paso 29 · Endpoints de espacios** · siguiente: **Paso 31 · Repositorio de vinculaciones**

---

## 1. Qué se hizo en este paso

Se modelaron las **vinculaciones** (una persona asignada a un curso o proyecto, con un rol y unas horas semanales) y las **reglas de dedicación horaria**, el núcleo de negocio del proyecto.

| Archivo | Cambio |
|---|---|
| `backend/internal/dominio/vinculacion.go` | **Nuevo.** Constantes de los límites, entidad `Vinculacion` con su vigencia, `ValidarHoras`, `LimiteMonitoriaMixta`, `ValidarCargaHoraria`, el tipo `ValidadorCarga`, `Rectificacion` y `FiltroVinculaciones`. |
| `backend/internal/dominio/reglas_test.go` | **Modificado.** Pruebas de todas las reglas de horas, incluido el ejemplo del enunciado. |

---

## 2. Por qué

El enunciado define reglas **bloqueantes** (RN-19): si se violan, la vinculación no se crea.

| Regla | Texto del enunciado |
|---|---|
| RN-14 / RF-05.1 | Máximo **22 horas** semanales como asistente graduado. |
| RN-15 / RF-05.2 | Máximo **3 monitorías** simultáneas. |
| RN-16 / RF-05.3 | Máximo **12 horas** semanales de monitoría. |
| RN-17 / RF-05.4 | Con monitoría **y** asistencia a la vez, las horas de monitoría no pueden superar el **40 %** de las horas de asistencia. |
| RN-18 / RF-05.5 | Ese 40 % se **redondea hacia arriba**: 22 h → 8,8 → **9 h**; 12 h → 4,8 → **5 h**. |

Son reglas puras (dado un conjunto de vinculaciones, ¿es válido?), así que van en el dominio y se prueban exhaustivamente sin base de datos.

---

## 3. Explicación detallada

### 3.1 Constantes

```go
const (
	MaxHorasAsistenteGraduado   = 22 // RN-14
	MaxMonitoriasSimultaneas    = 3  // RN-15
	MaxHorasMonitoria           = 12 // RN-16
	PorcentajeMaxMonitoriaMixta = 40 // RN-17: % de las horas de asistencia graduada
)
```
- Cada número del enunciado tiene un nombre y una referencia a su regla. Si la universidad cambiara un límite, se cambia en un solo lugar.

### 3.2 La entidad

```go
type Vinculacion struct {
	ID                 string
	UsuarioID          string
	UsuarioNombre      string
	UsuarioCorreo      string
	EspacioID          string
	EspacioNombre      string
	EspacioTipo        TipoEspacio
	EspacioEstado      EstadoEspacio
	EspacioFechaInicio time.Time
	EspacioFechaCierre time.Time
	ProfesorID         string
	ProfesorNombre     string
	Rol                Rol
	HorasSemanales     int
	CreadoEn           time.Time
	ActualizadoEn      time.Time
}
```
- Además de lo propio (rol y horas), trae datos de la persona, del espacio y del profesor, que el repositorio obtendrá con `JOIN` (paso 31). El **profesor responsable** de una vinculación es el del espacio (RN-11).

### 3.3 Vigencia

```go
func (v Vinculacion) InicioVigencia(zona *time.Location) time.Time {
	creada := Fecha(v.CreadoEn, zona)
	if creada.After(v.EspacioFechaInicio) {
		return creada
	}
	return v.EspacioFechaInicio
}

func (v Vinculacion) VigenteEnSemana(semanaInicio time.Time, zona *time.Location) bool {
	return seCruzan(v.InicioVigencia(zona), v.EspacioFechaCierre, InicioSemana(semanaInicio), FinSemana(semanaInicio))
}
```
- ¿Desde cuándo debe reportar una persona? Desde la fecha **más tardía** entre el inicio del espacio y el día en que se la vinculó. Si a Laura la vinculan el 16 de septiembre a un curso que empezó en agosto, no tiene sentido marcarle "sin reporte" las semanas de agosto.
- `VigenteEnSemana` reutiliza `seCruzan` (paso 25). Se usará en el seguimiento semanal (paso 41).

### 3.4 Reglas de horas

```go
func ValidarHoras(horas int) error {
	if horas <= 0 {
		return apperr.Validacion("las horas semanales contratadas deben ser mayores que cero")
	}
	return nil
}
```

```go
func LimiteMonitoriaMixta(horasAsistencia int) int {
	return (horasAsistencia*PorcentajeMaxMonitoriaMixta + 99) / 100
}
```
- **Redondeo hacia arriba solo con enteros:** en Go, la división entera **trunca** (`88 / 100 = 0`). Sumar `99` antes de dividir convierte el truncamiento en redondeo hacia arriba:
  - 22 h: `(22·40 + 99) / 100 = (880 + 99) / 100 = 979 / 100 = 9` ✔ (8,8 → 9);
  - 12 h: `(480 + 99) / 100 = 5` ✔ (4,8 → 5);
  - 10 h: `(400 + 99) / 100 = 4` ✔ (exacto, no sube).
- Se evitan los números decimales (`float64`), que pueden dar sorpresas como `0.1 + 0.2 = 0.30000000000000004`.

```go
func ValidarCargaHoraria(vinculaciones []Vinculacion) error {
	horasAsistencia, horasMonitoria, monitorias := 0, 0, 0
	for _, v := range vinculaciones {
		switch v.Rol {
		case RolAsistenteGraduado:
			horasAsistencia += v.HorasSemanales
		case RolMonitor:
			horasMonitoria += v.HorasSemanales
			monitorias++
		}
	}

	if horasAsistencia > MaxHorasAsistenteGraduado {
		return apperr.ReglaNegocio("la persona superaría el máximo de %d horas semanales como asistente graduado (quedaría con %d)", ...)
	}
	if monitorias > MaxMonitoriasSimultaneas { ... }
	if horasMonitoria > MaxHorasMonitoria { ... }
	if horasAsistencia > 0 && monitorias > 0 {
		if limite := LimiteMonitoriaMixta(horasAsistencia); horasMonitoria > limite {
			return apperr.ReglaNegocio("con %d horas como asistente graduado, las horas de monitoría no pueden superar %d (40 %% redondeado hacia arriba); quedaría con %d", ...)
		}
	}
	return nil
}
```
- Recibe **todas** las vinculaciones simultáneas de una persona **incluida la que se quiere crear**, suma por rol y verifica las cuatro reglas en orden.
- Los mensajes dicen **cuánto quedaría**, para que el profesor entienda por qué se rechazó.
- `%%` en un formato produce un `%` literal.
- `if limite := ...; cond`: el `if` con inicialización (paso 0) mantiene `limite` dentro del bloque.
- Todas son **422** (regla de negocio), no errores de validación.

### 3.5 `ValidadorCarga`: la regla viaja hasta la transacción

```go
type ValidadorCarga func(simultaneas []Vinculacion, objetivo Vinculacion) error
```
- Un **tipo función**. Hay un problema de concurrencia: si dos profesores vinculan a la misma persona **al mismo tiempo**, cada uno podría ver "tiene 12 h, agregar 10 h está bien" y entre los dos dejarla con 32 h.
- La solución (paso 31) es validar **dentro de una transacción que bloquea a la persona**. Pero la regla es del dominio, no de la base de datos. Con este tipo, el servicio le **pasa la regla como función** al repositorio, que la ejecuta en el momento exacto, con el usuario bloqueado.

### 3.6 Rectificaciones y filtro

```go
type Rectificacion struct {
	ID              string
	VinculacionID   string
	AdministradorID string
	RolAnterior     Rol
	RolNuevo        Rol
	HorasAnteriores int
	HorasNuevas     int
	Motivo          string
	CreadoEn        time.Time
}
```
- **RF-04.4 / RN-12:** las horas no se modifican; **RF-04.5:** el administrador puede **rectificar** un error, con motivo y dejando registro (tabla `vinculacion_rectificaciones`, paso 4).

```go
type FiltroVinculaciones struct {
	VisiblePara string
	ProfesorID string
	UsuarioID  string
	EspacioID  string
	SoloEspaciosActivos bool
	Paginacion
}
```
- `VisiblePara`: mismo mecanismo de visibilidad de los espacios. `ProfesorID`: limitar a los espacios de un profesor (lo usará el seguimiento, paso 43).

### 3.7 Pruebas

```go
func vinc(rol Rol, horas int) Vinculacion { return Vinculacion{Rol: rol, HorasSemanales: horas} }
```

| Prueba | Casos |
|---|---|
| `TestLimiteMonitoriaMixtaRedondeaHaciaArriba` | 22→9, 12→5, 10→4, 5→2, 1→1, 20→8. |
| `TestValidarCargaHoraria` (10 subpruebas con `t.Run`) | 22 h de asistencia ✔; 23 h ✘ (RN-14); 3 monitorías ✔; 4 ✘ (RN-15); 13 h de monitoría ✘ (RN-16); **ejemplo del enunciado: 12 h de asistencia + 3 h + 3 h de monitoría ✘** (6 > 5, RN-17); 12 + 5 ✔ (redondeo RN-18); 22 + 9 ✔; 22 + 10 ✘; sin vinculaciones ✔. |
| `TestValidarHoras` | 0 y −3 inválidos; 4 válido. |
| `TestVigenciaDeLaVinculacion` | Creada el 16/09 en un curso de agosto: empieza el 16/09; no vigente la semana del 07/09; vigente la del 14/09 y la del cierre; no vigente después del cierre. |

`t.Run(nombre, func...)` crea **subpruebas** con nombre; en la salida se ve cuál de los 10 casos falló.

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| División entera | `/` entre enteros trunca; `(a + b - 1) / b` redondea hacia arriba. |
| `%%` | `%` literal en un formato. |
| Tipo función como estrategia | `ValidadorCarga`: una regla que se entrega a otra capa para ejecutarla en el momento justo. |
| Subpruebas | `t.Run(nombre, f)`. |

---

## 5. Cómo comprobarlo

```bash
cd backend
go test ./internal/dominio/ -v -run 'Carga|Monitoria|Horas|Vigencia'
```

---

## 6. Pasos para lograr esta versión desde el paso 29

1. Crear `backend/internal/dominio/vinculacion.go` con las constantes, `Vinculacion`, `InicioVigencia`, `VigenteEnSemana`, `ValidarHoras`, `LimiteMonitoriaMixta`, `ValidarCargaHoraria`, `ValidadorCarga`, `Rectificacion` y `FiltroVinculaciones`.
2. En `reglas_test.go`, al inicio: `vinc`, `TestLimiteMonitoriaMixtaRedondeaHaciaArriba`, `TestValidarCargaHoraria`, `TestValidarHoras` y `TestVigenciaDeLaVinculacion` (agregar el import `time`).
3. `make pruebas lint`
4. Commit: `git add . && git commit -m "Paso 30: dominio de vinculaciones y reglas de horas"`

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-04.1 a RF-04.3 Vinculación con rol y horas | Entidad `Vinculacion`. |
| RF-04.5 Rectificación | Entidad `Rectificacion`. |
| RF-05.1 a RF-05.5 / RN-14 a RN-18 | `ValidarCargaHoraria` y `LimiteMonitoriaMixta`, con el ejemplo del enunciado probado. |
| RF-05.6 / RN-19 Reglas bloqueantes | Errores 422 y `ValidadorCarga` para aplicarlas de forma atómica (paso 31). |
