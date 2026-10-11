package dominio

import "time"

// EstadoLote describe el avance de un lote de generación de reportes (RF-14.1).
type EstadoLote string

const (
	LotePendiente            EstadoLote = "pendiente"
	LoteEnProceso            EstadoLote = "en_proceso"
	LoteCompletado           EstadoLote = "completado"
	LoteCompletadoConErrores EstadoLote = "completado_con_errores"
	LoteFallido              EstadoLote = "fallido"
)

// EstaActivo indica si el lote todavía se está procesando.
func (e EstadoLote) EstaActivo() bool {
	return e == LotePendiente || e == LoteEnProceso
}

// EstadoReporte describe el avance de la generación de un reporte individual.
type EstadoReporte string

const (
	ReportePendiente  EstadoReporte = "pendiente"
	ReporteEnProceso  EstadoReporte = "en_proceso"
	ReporteCompletado EstadoReporte = "completado"
	ReporteFallido    EstadoReporte = "fallido"
)

// EsFinal indica si el reporte ya no cambiará de estado.
func (e EstadoReporte) EsFinal() bool {
	return e == ReporteCompletado || e == ReporteFallido
}

// LoteReportes es una ejecución manual, por lotes, de la generación de reportes
// de una semana para todas las personas vinculadas a un profesor (RF-14.1, RN-41).
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

// ReporteSemanal es el reporte PDF de una persona para una semana (RF-14.2, RN-42).
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
	CreadoEn      time.Time
	ActualizadoEn time.Time
	GeneradoEn    *time.Time
}

// ResultadoReporte agrupa lo que se persiste cuando un reporte se genera con éxito.
type ResultadoReporte struct {
	Resumen     string
	RutaPDF     string
	TamanoBytes int64
	ProveedorIA string
	ModeloIA    string
}

// FiltroLotes delimita la consulta de lotes.
type FiltroLotes struct {
	ProfesorID   string
	SemanaInicio *time.Time
	Paginacion
}

// FiltroReportes delimita la consulta de reportes.
type FiltroReportes struct {
	ProfesorID   string
	UsuarioID    string
	LoteID       string
	SemanaInicio *time.Time
	Paginacion
}

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

// VinculacionSemana agrupa las tareas de una vinculación durante la semana.
type VinculacionSemana struct {
	EspacioNombre    string
	EspacioTipo      TipoEspacio
	Rol              Rol
	HorasContratadas int
	Tareas           []TareaSemana
}

// DatosSemanaPersona reúne la información necesaria para generar el reporte
// semanal de una persona frente a un profesor.
type DatosSemanaPersona struct {
	UsuarioID     string
	Persona       string
	Correo        string
	ProfesorID    string
	Profesor      string
	SemanaInicio  time.Time
	Vinculaciones []VinculacionSemana
}

// SemanaFin devuelve el domingo de la semana reportada.
func (d DatosSemanaPersona) SemanaFin() time.Time {
	return FinSemana(d.SemanaInicio)
}

// TotalTareas cuenta las tareas registradas en la semana.
func (d DatosSemanaPersona) TotalTareas() int {
	total := 0
	for _, v := range d.Vinculaciones {
		total += len(v.Tareas)
	}
	return total
}

// TotalHoras suma las horas reportadas en la semana.
func (d DatosSemanaPersona) TotalHoras() int {
	total := 0
	for _, v := range d.Vinculaciones {
		total += v.HorasReportadas()
	}
	return total
}

// HorasReportadas suma las horas de las tareas de la vinculación.
func (v VinculacionSemana) HorasReportadas() int {
	total := 0
	for _, t := range v.Tareas {
		total += t.Horas
	}
	return total
}
