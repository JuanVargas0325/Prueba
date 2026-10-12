package dominio

import (
	"time"

	"seguimiento/internal/apperr"
)

// Límites de las tareas.
const (
	LongitudMaxTitulo      = 200
	LongitudMaxDescripcion = 4000
	MaxHorasPorTarea       = 168 // horas de una semana completa
)

// EstadoTarea es el catálogo cerrado de estados de una tarea (RN-27, RN-28).
type EstadoTarea string

const (
	TareaAbierta      EstadoTarea = "abierto"
	TareaEnDesarrollo EstadoTarea = "en_desarrollo"
	TareaFinalizada   EstadoTarea = "finalizado"
)

// EstadosTarea lista los estados válidos.
var EstadosTarea = []EstadoTarea{TareaAbierta, TareaEnDesarrollo, TareaFinalizada}

// EsValido indica si el estado pertenece al catálogo.
func (e EstadoTarea) EsValido() bool {
	return e == TareaAbierta || e == TareaEnDesarrollo || e == TareaFinalizada
}

// Texto devuelve el estado en lenguaje natural.
func (e EstadoTarea) Texto() string {
	if e == TareaEnDesarrollo {
		return "en desarrollo"
	}
	return string(e)
}

// Tarea es el registro de una actividad semanal asociada a una única
// vinculación (RF-06, RN-21). Incluye datos de la vinculación para las consultas.
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

// DatosTarea son los campos que el usuario puede escribir en una tarea.
type DatosTarea struct {
	Titulo          string
	Descripcion     string
	Estado          EstadoTarea
	HorasInvertidas int
	Observaciones   string
}

// Normalizar limpia y valida los datos de la tarea (RN-22, RN-24, RN-27).
func (d DatosTarea) Normalizar() (DatosTarea, error) {
	var err error
	if d.Titulo, err = Texto("el título", d.Titulo, true, LongitudMaxTitulo); err != nil {
		return d, err
	}
	if d.Descripcion, err = Texto("la descripción", d.Descripcion, false, LongitudMaxDescripcion); err != nil {
		return d, err
	}
	if d.Observaciones, err = Texto("las observaciones", d.Observaciones, false, LongitudMaxObservaciones); err != nil {
		return d, err
	}
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

// Aplicar copia los datos editables sobre la tarea.
func (t *Tarea) Aplicar(d DatosTarea) {
	t.Titulo, t.Descripcion, t.Estado = d.Titulo, d.Descripcion, d.Estado
	t.HorasInvertidas, t.Observaciones = d.HorasInvertidas, d.Observaciones
}

// ValidarSemanaDeRegistro impide registrar tareas en semanas futuras.
// Las semanas pasadas sí se admiten, como reporte tardío (RF-08.3).
func ValidarSemanaDeRegistro(semanaInicio, ahora time.Time, zona *time.Location) error {
	if InicioSemana(semanaInicio).After(SemanaActual(ahora, zona)) {
		return apperr.Validacion("no se pueden registrar tareas en semanas futuras")
	}
	return nil
}

// PuedeModificarse aplica RN-30, RN-33 y RN-34: una tarea solo se edita o
// elimina mientras su semana está activa, y un reporte tardío nunca.
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

// FiltroTareas delimita la consulta de tareas.
type FiltroTareas struct {
	// VisiblePara limita el resultado a las tareas propias o de los espacios
	// del profesor. Vacío = sin restricción (administrador).
	VisiblePara   string
	UsuarioID     string
	EspacioID     string
	VinculacionID string
	SemanaInicio  *time.Time
	Paginacion
}

// Adjunto es un archivo de evidencia asociado a una tarea (RF-06.4, RN-23).
// El archivo se guarda en el almacenamiento; aquí solo están sus metadatos.
type Adjunto struct {
	ID                 string
	TareaID            string
	NombreOriginal     string
	RutaAlmacenamiento string
	TipoContenido      string
	TamanoBytes        int64
	CreadoEn           time.Time
}
