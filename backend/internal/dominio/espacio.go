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
