# Paso 45 · Dominio: lotes y reportes

> Rama `paso-a-paso` · paso 45 de 65 · anterior: **Paso 44 · Endpoints de seguimiento** · siguiente: **Paso 46 · Repositorio de reportes**

---

## 1. Qué se hizo en este paso

Comienza el último módulo funcional: los **reportes semanales con IA**. Este paso modela sus conceptos en el dominio.

| Archivo | Cambio |
|---|---|
| `backend/internal/dominio/reporte.go` | **Nuevo.** Estados y entidades de `LoteReportes` y `ReporteSemanal`, el resultado de una generación, los filtros y los **datos de entrada** del reporte (`DatosSemanaPersona`) con sus totales. |
| `backend/internal/dominio/semana_test.go` | **Modificado.** `TestTotalesDatosSemana`. |

---

## 2. Por qué

**RF-14** pide: generar los reportes **manualmente por lotes** (14.1), **uno por persona** (14.2), usando **solo** tareas, descripciones, observaciones y horas (14.3), en **PDF** (14.4), **almacenarlos** (14.5) para **descargarlos** después (14.6), **para una semana concreta** (14.7). Las reglas RN-41 a RN-46 agregan que el reporte **sintetiza, no evalúa**.

Generar un reporte con IA tarda segundos o más, y un profesor puede tener muchas personas a cargo. Por eso el diseño es **asíncrono**: la API registra un **lote** y responde de inmediato; un **worker** (paso 55) genera los reportes en segundo plano y va actualizando sus estados. Para eso hacen falta estados claros.

---

## 3. Explicación detallada

### 3.1 Estados del lote

```go
type EstadoLote string

const (
	LotePendiente            EstadoLote = "pendiente"
	LoteEnProceso            EstadoLote = "en_proceso"
	LoteCompletado           EstadoLote = "completado"
	LoteCompletadoConErrores EstadoLote = "completado_con_errores"
	LoteFallido              EstadoLote = "fallido"
)

func (e EstadoLote) EstaActivo() bool {
	return e == LotePendiente || e == LoteEnProceso
}
```

```
pendiente ──► en_proceso ──► completado               (todos bien)
    │              ├──────► completado_con_errores   (algunos fallaron)
    │              └──────► fallido                  (todos fallaron)
    └─────────────────────► fallido                  (no se pudo ni empezar)
```
- `EstaActivo`: un lote que todavía se procesa. Se usa para no lanzar dos lotes de la misma semana a la vez (paso 48) y para no reprocesar uno terminado (paso 54).

### 3.2 Estados del reporte

```go
type EstadoReporte string

const (
	ReportePendiente  EstadoReporte = "pendiente"
	ReporteEnProceso  EstadoReporte = "en_proceso"
	ReporteCompletado EstadoReporte = "completado"
	ReporteFallido    EstadoReporte = "fallido"
)

func (e EstadoReporte) EsFinal() bool {
	return e == ReporteCompletado || e == ReporteFallido
}
```
- `EsFinal`: un reporte terminado (bien o mal) no se vuelve a generar si la cola entrega el trabajo dos veces.

### 3.3 Entidades

```go
type LoteReportes struct {
	ID                  string
	ProfesorID          string
	SemanaInicio        time.Time
	Estado              EstadoLote
	TotalReportes       int
	ReportesCompletados int
	ReportesFallidos    int
	Error               string
	CreadoEn            time.Time
	ActualizadoEn       time.Time
	FinalizadoEn        *time.Time
}
```
- Los contadores permiten mostrar el avance ("7 de 10 listos").
- `FinalizadoEn *time.Time`: puntero porque **puede no existir** (un lote en proceso no tiene fecha de fin). En JSON será `null`.

```go
type ReporteSemanal struct {
	ID            string
	LoteID        string
	ProfesorID    string
	UsuarioID     string
	Persona       string // nombre de la persona reportada
	SemanaInicio  time.Time
	Estado        EstadoReporte
	Resumen       string
	RutaPDF       string
	TamanoBytes   int64
	Intentos      int
	Error         string
	ProveedorIA   string
	ModeloIA      string
	...
	GeneradoEn    *time.Time
}
```
- Guarda el **resumen** generado (para mostrarlo sin abrir el PDF), la **ruta del PDF** en el almacenamiento (paso 38), los **intentos** (la cola reintenta ante errores transitorios), el **error** si falló y **qué proveedor y modelo de IA** lo generaron (trazabilidad).

```go
type ResultadoReporte struct {
	Resumen     string
	RutaPDF     string
	TamanoBytes int64
	ProveedorIA string
	ModeloIA    string
}
```
- Lo que se guarda cuando un reporte se genera con éxito, agrupado en un solo valor.

### 3.4 Los datos de entrada del reporte

```go
// TareaSemana es la información de una tarea que alimenta el reporte.
// Solo contiene los insumos permitidos por RF-14.3 / RN-43.
type TareaSemana struct {
	Titulo        string
	Descripcion   string
	Estado        EstadoTarea
	Horas         int
	Observaciones string
	Tardia        bool
}

type VinculacionSemana struct {
	EspacioNombre    string
	EspacioTipo      TipoEspacio
	Rol              Rol
	HorasContratadas int
	Tareas           []TareaSemana
}

type DatosSemanaPersona struct {
	UsuarioID     string
	Persona       string
	Correo        string
	ProfesorID    string
	Profesor      string
	SemanaInicio  time.Time
	Vinculaciones []VinculacionSemana
}
```
- **Tipos propios, no `dominio.Tarea`:** `TareaSemana` tiene **exactamente** los insumos que permite RF-14.3. Así es imposible que, por descuido, se le envíe a la IA un dato que no corresponde (por ejemplo, IDs internos). El diseño de los tipos **hace cumplir el requisito**.
- `Persona`, `Correo` y `Profesor` se usan en el encabezado del **PDF**, pero **no se envían al modelo** (paso 51).

```go
func (d DatosSemanaPersona) SemanaFin() time.Time { return FinSemana(d.SemanaInicio) }
func (d DatosSemanaPersona) TotalTareas() int { ... }
func (d DatosSemanaPersona) TotalHoras() int { ... }
func (v VinculacionSemana) HorasReportadas() int { ... }
```
- Totales calculados que usarán el prompt, el modo simulado y el PDF.

### 3.5 Filtros

`FiltroLotes` (por profesor y semana) y `FiltroReportes` (por profesor, persona, lote y semana), ambos con `Paginacion`.

### 3.6 Prueba

`TestTotalesDatosSemana`: dos vinculaciones con tareas de 2, 3 y 4 h → 3 tareas, 9 horas y la semana termina el domingo 11/10.

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Máquina de estados | Valores posibles y transiciones permitidas, documentadas en el tipo. |
| Puntero para "puede no existir" | `*time.Time` (en JSON, `null`). |
| Tipos que hacen cumplir requisitos | `TareaSemana` solo tiene los campos permitidos para la IA. |
| Procesamiento asíncrono | Registrar el trabajo, responder de inmediato y procesar después. |

---

## 5. Cómo comprobarlo

```bash
cd backend
go test ./internal/dominio/ -v -run TestTotalesDatosSemana
```

---

## 6. Pasos para lograr esta versión desde el paso 44

1. Crear `backend/internal/dominio/reporte.go` con los estados (y `EstaActivo`, `EsFinal`), `LoteReportes`, `ReporteSemanal`, `ResultadoReporte`, `FiltroLotes`, `FiltroReportes`, `TareaSemana`, `VinculacionSemana`, `DatosSemanaPersona` y los métodos de totales.
2. En `semana_test.go`: `TestTotalesDatosSemana`.
3. `make pruebas lint`
4. Commit: `git add . && git commit -m "Paso 45: dominio de lotes y reportes"`

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-14.1 Ejecución por lotes | `LoteReportes` con estados y contadores. |
| RF-14.2 Un reporte por persona | `ReporteSemanal`. |
| RF-14.3 / RN-43 Solo insumos permitidos | `TareaSemana`. |
| RF-14.5 / RF-14.6 Almacenar y descargar | `RutaPDF`. |
| RF-14.7 Una semana concreta | `SemanaInicio`. |
