# Paso 34 · Dominio: tareas semanales

> Rama `paso-a-paso` · paso 34 de 65 · anterior: **Paso 33 · Endpoints de vinculaciones** · siguiente: **Paso 35 · Repositorio de tareas**

---

## 1. Qué se hizo en este paso

Se modeló la **tarea semanal**: el registro de una actividad que un monitor o asistente hace en una de sus vinculaciones durante una semana, con sus estados y las reglas de tiempo (semana activa, tardanza, inmutabilidad).

| Archivo | Cambio |
|---|---|
| `backend/internal/dominio/tarea.go` | **Nuevo.** `EstadoTarea`, entidad `Tarea`, `DatosTarea` con `Normalizar`, `Aplicar`, `ValidarSemanaDeRegistro`, `PuedeModificarse`, `FiltroTareas` y la entidad `Adjunto`. |
| `backend/internal/dominio/reglas_test.go` | **Modificado.** Pruebas de normalización, modificabilidad y semanas de registro. |

---

## 2. Por qué

| Requisito | Regla |
|---|---|
| RF-06.1 a RF-06.3 / RN-20 a RN-22 | Cada tarea pertenece a **una** vinculación y tiene título, descripción, estado, horas y observaciones. |
| RF-06.5 / RN-24 | Las horas son **enteras**. |
| RF-07 / RN-27 y RN-28 | Estados cerrados: **abierto**, **en desarrollo**, **finalizado**. |
| RF-06.7, RF-06.8, RF-08.2 / RN-30 | Editar y eliminar solo mientras la semana está **activa**. |
| RF-08.3 | Se puede registrar en semanas **pasadas**. |
| RF-08.4 / RN-32 | Registrar en una semana cerrada marca la tarea como **reporte tardío**. |
| RF-08.5 / RN-33 | Un reporte tardío **no se puede modificar ni eliminar**. |
| RN-34 | Las tareas de semanas cerradas no se modifican. |

Todas son reglas puras sobre fechas y datos: van en el dominio.

---

## 3. Explicación detallada

### 3.1 Estados

```go
type EstadoTarea string

const (
	TareaAbierta      EstadoTarea = "abierto"
	TareaEnDesarrollo EstadoTarea = "en_desarrollo"
	TareaFinalizada   EstadoTarea = "finalizado"
)

var EstadosTarea = []EstadoTarea{TareaAbierta, TareaEnDesarrollo, TareaFinalizada}

func (e EstadoTarea) Texto() string {
	if e == TareaEnDesarrollo {
		return "en desarrollo"
	}
	return string(e)
}
```
- Coinciden con el `CHECK` de la tabla `tareas` (paso 4). `EstadosTarea` sirve para recorrerlos (por ejemplo, contar tareas por estado en el consolidado, paso 41). `Texto` es la versión legible para el PDF.

### 3.2 La entidad

```go
type Tarea struct {
	ID               string
	VinculacionID    string
	UsuarioID        string
	UsuarioNombre    string
	EspacioID        string
	EspacioNombre    string
	EspacioTipo      TipoEspacio
	EspacioEstado    EstadoEspacio
	ProfesorID       string
	Rol              Rol
	SemanaInicio     time.Time // siempre un lunes
	Titulo           string
	Descripcion      string
	Estado           EstadoTarea
	HorasInvertidas  int // horas enteras (RN-24)
	Observaciones    string
	Tardia           bool // RN-32
	CantidadAdjuntos int
	Adjuntos         []Adjunto
	CreadoEn         time.Time
	ActualizadoEn    time.Time
}
```
- Una tarea se identifica por su vinculación; el resto (persona, espacio, profesor, rol) lo traerá el repositorio con `JOIN` (paso 35) para mostrarla y para decidir permisos.
- **La semana se guarda como su lunes** (`SemanaInicio`).
- `HorasInvertidas int`: las horas enteras (RN-24) se garantizan con el **tipo**.
- `Adjuntos` y `CantidadAdjuntos`: la entidad ya contempla la evidencia; la funcionalidad de adjuntos se completa en los pasos 38 a 40.

### 3.3 Datos editables y su validación

```go
type DatosTarea struct {
	Titulo          string
	Descripcion     string
	Estado          EstadoTarea
	HorasInvertidas int
	Observaciones   string
}

func (d DatosTarea) Normalizar() (DatosTarea, error) {
	var err error
	if d.Titulo, err = Texto("el título", d.Titulo, true, LongitudMaxTitulo); err != nil {
		return d, err
	}
	if d.Descripcion, err = Texto("la descripción", d.Descripcion, false, LongitudMaxDescripcion); err != nil { ... }
	if d.Observaciones, err = Texto("las observaciones", d.Observaciones, false, LongitudMaxObservaciones); err != nil { ... }
	if d.Estado == "" {
		d.Estado = TareaAbierta
	}
	if !d.Estado.EsValido() {
		return d, apperr.Validacion("estado inválido: use abierto, en_desarrollo o finalizado")
	}
	if d.HorasInvertidas < 0 || d.HorasInvertidas > MaxHorasPorTarea {
		return d, apperr.Validacion("las horas invertidas deben ser un número entero entre 0 y %d", MaxHorasPorTarea)
	}
	return d, nil
}
```
- Separar `DatosTarea` (lo que el usuario puede escribir) de `Tarea` (todo lo demás) deja claro **qué es editable**: la semana, la vinculación y la marca de tardanza no lo son.
- `if d.Titulo, err = Texto(...); err != nil`: asignación (no declaración) dentro del `if`.
- El título es obligatorio; si no se indica estado, la tarea queda **abierta**; las horas van de 0 a 168 (las horas de una semana).

```go
func (t *Tarea) Aplicar(d DatosTarea) {
	t.Titulo, t.Descripcion, t.Estado = d.Titulo, d.Descripcion, d.Estado
	t.HorasInvertidas, t.Observaciones = d.HorasInvertidas, d.Observaciones
}
```
- Receptor **puntero** (`*Tarea`): modifica la tarea original. Copia solo los campos editables.

### 3.4 Reglas de tiempo

```go
func ValidarSemanaDeRegistro(semanaInicio, ahora time.Time, zona *time.Location) error {
	if InicioSemana(semanaInicio).After(SemanaActual(ahora, zona)) {
		return apperr.Validacion("no se pueden registrar tareas en semanas futuras")
	}
	return nil
}
```
- Se permiten la semana actual y las **pasadas** (RF-08.3); las futuras no.

```go
func (t Tarea) PuedeModificarse(ahora time.Time, zona *time.Location) error {
	if t.Tardia {
		return apperr.ReglaNegocio("los reportes tardíos no pueden modificarse ni eliminarse")
	}
	if SemanaCerrada(t.SemanaInicio, ahora, zona) {
		return apperr.ReglaNegocio("la semana del %s ya cerró: sus tareas no pueden modificarse ni eliminarse",
			InicioSemana(t.SemanaInicio).Format(FormatoFecha))
	}
	return nil
}
```
- **RN-33:** una tarea tardía **nunca** se modifica (aunque su semana fuera la actual, caso que no ocurre en la práctica pero la regla lo cubre).
- **RN-30 / RN-34:** si la semana ya cerró, tampoco.
- Ambas son reglas de negocio (422). La marca `Tardia` la pone el servicio al crear (paso 36) con `SemanaCerrada` (paso 20).

### 3.5 Filtro y adjuntos

```go
type FiltroTareas struct {
	VisiblePara   string
	UsuarioID     string
	EspacioID     string
	VinculacionID string
	SemanaInicio  *time.Time
	Paginacion
}
```
- `SemanaInicio *time.Time`: puntero porque es **opcional** (sin semana = todas).

```go
type Adjunto struct {
	ID                 string
	TareaID            string
	NombreOriginal     string
	RutaAlmacenamiento string
	TipoContenido      string
	TamanoBytes        int64
	CreadoEn           time.Time
}
```
- Los **metadatos** de un archivo de evidencia (tabla `tarea_adjuntos`). El archivo en sí se guardará fuera de la base de datos (paso 38). `int64` porque un tamaño en bytes puede superar el rango de un `int` de 32 bits.

### 3.6 Pruebas

El "ahora" de las pruebas es el **miércoles 14 de octubre de 2026** (semana del 12 al 18).

| Prueba | Casos |
|---|---|
| `TestDatosTareaNormalizar` | `"  Calificar  "` → `"Calificar"` y estado abierto por defecto. Inválidos: título vacío, estado `cerrado`, horas −1, horas 169 y título de 201 caracteres. |
| `TestTareaPuedeModificarse` | Semana actual ✔; semana pasada ✘ (RN-34); tardía ✘ con el mensaje de "tardíos" (RN-33). |
| `TestValidarSemanaDeRegistro` | El domingo 18 (semana actual) ✔; una semana pasada ✔ (RF-08.3); el lunes 19 (semana futura) ✘. |

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Asignación en el `if` | `if x, err = f(); err != nil` (sin `:=`, porque `x` ya existe). |
| Asignación múltiple de campos | `t.A, t.B = d.A, d.B`. |
| Receptor puntero para modificar | `func (t *Tarea) Aplicar(...)`. |
| Puntero como opcional | `*time.Time` en un filtro. |
| `int64` | Entero de 64 bits para tamaños en bytes. |

---

## 5. Cómo comprobarlo

```bash
cd backend
go test ./internal/dominio/ -v -run 'Tarea|SemanaDeRegistro'
```

---

## 6. Pasos para lograr esta versión desde el paso 33

1. Crear `backend/internal/dominio/tarea.go` con las constantes de límites (`LongitudMaxTitulo`, `LongitudMaxDescripcion`, `MaxHorasPorTarea`), `EstadoTarea` con sus constantes, `EstadosTarea`, `EsValido`, `Texto`, `Tarea`, `DatosTarea`, `Normalizar`, `Aplicar`, `ValidarSemanaDeRegistro`, `PuedeModificarse`, `FiltroTareas` y `Adjunto`.
2. En `reglas_test.go`, después de `TestEspacioFechasYEstado`: `TestDatosTareaNormalizar`, `TestTareaPuedeModificarse` y `TestValidarSemanaDeRegistro`.
3. `make pruebas lint`
4. Commit: `git add . && git commit -m "Paso 34: dominio de tareas semanales"`

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-06.3 / RN-22 Información mínima | `DatosTarea` y `Normalizar`. |
| RF-06.5 / RN-24 Horas enteras | `HorasInvertidas int` y rango 0–168. |
| RF-07 / RN-27 y RN-28 Estados | `EstadoTarea`. |
| RF-08.2 / RN-30 Semana activa | `PuedeModificarse`. |
| RF-08.3 Semanas anteriores | `ValidarSemanaDeRegistro`. |
| RF-08.5 / RN-33 Tardía inmutable | `PuedeModificarse`. |
| RN-34 Semanas cerradas | `PuedeModificarse`. |
