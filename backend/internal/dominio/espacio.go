package dominio

import (
	"regexp"
	"time"

	"seguimiento/internal/apperr"
)

// EstadoPeriodo indica si un período académico está vigente.
type EstadoPeriodo string

const (
	PeriodoActivo  EstadoPeriodo = "activo"
	PeriodoCerrado EstadoPeriodo = "cerrado"
)

// EsValido indica si el estado pertenece al catálogo.
func (e EstadoPeriodo) EsValido() bool { return e == PeriodoActivo || e == PeriodoCerrado }

// PeriodoAcademico agrupa los cursos y proyectos de un semestre (RF-03.2).
type PeriodoAcademico struct {
	ID          string
	Codigo      string // por ejemplo 2026-20
	FechaInicio time.Time
	FechaCierre time.Time
	Estado      EstadoPeriodo
	// CreacionHabilitada abre o cierra la ventana en la que los profesores
	// pueden crear cursos y proyectos (RN-07).
	CreacionHabilitada bool
	CreadoEn           time.Time
}

var formatoCodigoPeriodo = regexp.MustCompile(`^\d{4}-\d{2}$`)

// ValidarCodigoPeriodo verifica el formato AAAA-NN del código del período.
func ValidarCodigoPeriodo(codigo string) error {
	if !formatoCodigoPeriodo.MatchString(codigo) {
		return apperr.Validacion("el código del período debe tener formato AAAA-NN, por ejemplo 2026-20")
	}
	return nil
}

// ValidarFechas verifica que el período termine después de iniciar.
func (p PeriodoAcademico) ValidarFechas() error {
	if !p.FechaCierre.After(p.FechaInicio) {
		return apperr.Validacion("la fecha de cierre del período debe ser posterior a la de inicio")
	}
	return nil
}

// PermiteCrearEspacios aplica RF-03.2 y RN-07: solo se crean cursos y
// proyectos en períodos activos, habilitados para creación y no terminados.
func (p PeriodoAcademico) PermiteCrearEspacios(hoy time.Time) error {
	switch {
	case p.Estado != PeriodoActivo:
		return apperr.ReglaNegocio("el período %s está cerrado", p.Codigo)
	case !p.CreacionHabilitada:
		return apperr.ReglaNegocio("el período %s no está habilitado para crear cursos o proyectos", p.Codigo)
	case hoy.After(p.FechaCierre):
		return apperr.ReglaNegocio("el período %s ya terminó", p.Codigo)
	}
	return nil
}

// FiltroPeriodos delimita la consulta de períodos.
type FiltroPeriodos struct {
	Estado EstadoPeriodo
	Paginacion
}

// TipoEspacio distingue cursos de proyectos (RN-06).
type TipoEspacio string

const (
	EspacioCurso    TipoEspacio = "curso"
	EspacioProyecto TipoEspacio = "proyecto"
)

// EsValido indica si el tipo pertenece al catálogo.
func (t TipoEspacio) EsValido() bool { return t == EspacioCurso || t == EspacioProyecto }

// EstadoEspacio indica si un curso o proyecto admite nuevos registros (RN-08).
type EstadoEspacio string

const (
	EspacioActivo  EstadoEspacio = "activo"
	EspacioCerrado EstadoEspacio = "cerrado"
)

// EsValido indica si el estado pertenece al catálogo.
func (e EstadoEspacio) EsValido() bool { return e == EspacioActivo || e == EspacioCerrado }

// Espacio es un curso o proyecto creado por un profesor (RF-03).
type Espacio struct {
	ID             string
	Tipo           TipoEspacio
	Nombre         string
	PeriodoID      string
	PeriodoCodigo  string
	ProfesorID     string
	ProfesorNombre string
	FechaInicio    time.Time
	FechaCierre    time.Time
	Observaciones  string
	Estado         EstadoEspacio
	CreadoEn       time.Time
}

// ValidarFechasEnPeriodo verifica que el espacio quede dentro de su período.
func (e Espacio) ValidarFechasEnPeriodo(p PeriodoAcademico) error {
	if e.FechaCierre.Before(e.FechaInicio) {
		return apperr.Validacion("la fecha de cierre no puede ser anterior a la de inicio")
	}
	if e.FechaInicio.Before(p.FechaInicio) || e.FechaCierre.After(p.FechaCierre) {
		return apperr.Validacion("las fechas deben estar dentro del período %s (%s a %s)",
			p.Codigo, p.FechaInicio.Format(FormatoFecha), p.FechaCierre.Format(FormatoFecha))
	}
	return nil
}

// PermiteRegistros aplica RN-09: un espacio cerrado solo admite consultas.
func (e Espacio) PermiteRegistros() error {
	if e.Estado == EspacioCerrado {
		return apperr.ReglaNegocio("el %s %q está cerrado y solo admite consultas", e.Tipo, e.Nombre)
	}
	return nil
}

// IncluyeSemana indica si la semana se cruza con las fechas del espacio.
func (e Espacio) IncluyeSemana(semanaInicio time.Time) bool {
	return seCruzan(e.FechaInicio, e.FechaCierre, InicioSemana(semanaInicio), FinSemana(semanaInicio))
}

// FiltroEspacios delimita la consulta de cursos y proyectos.
type FiltroEspacios struct {
	// VisiblePara limita el resultado a los espacios del profesor o en los que
	// el usuario participa. Vacío = sin restricción (administrador).
	VisiblePara string
	PeriodoID   string
	Estado      EstadoEspacio
	Tipo        TipoEspacio
	Paginacion
}

// seCruzan indica si los intervalos de fechas [a1, a2] y [b1, b2] comparten algún día.
func seCruzan(a1, a2, b1, b2 time.Time) bool {
	return !a1.After(b2) && !b1.After(a2)
}
