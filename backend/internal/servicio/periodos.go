package servicio

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

// RepositorioPeriodos es lo que la gestión de períodos necesita de la capa de datos.
type RepositorioPeriodos interface {
	Crear(ctx context.Context, p dominio.PeriodoAcademico) (dominio.PeriodoAcademico, error)
	Obtener(ctx context.Context, id string) (dominio.PeriodoAcademico, error)
	Listar(ctx context.Context, f dominio.FiltroPeriodos) ([]dominio.PeriodoAcademico, error)
	Actualizar(ctx context.Context, p dominio.PeriodoAcademico) (dominio.PeriodoAcademico, error)
}

// NuevoPeriodo son los datos para registrar un período académico.
type NuevoPeriodo struct {
	Codigo             string
	FechaInicio        time.Time
	FechaCierre        time.Time
	CreacionHabilitada bool
}

// CambiosPeriodo son los campos modificables de un período; nil = sin cambio.
type CambiosPeriodo struct {
	Estado             *dominio.EstadoPeriodo
	CreacionHabilitada *bool
}

// Periodos gestiona los períodos académicos (RF-03.2). Los administra el
// administrador; cualquier usuario autenticado puede consultarlos.
type Periodos struct {
	repo RepositorioPeriodos
	log  *slog.Logger
}

// NuevoPeriodos crea el servicio de períodos.
func NuevoPeriodos(repo RepositorioPeriodos, log *slog.Logger) *Periodos {
	return &Periodos{repo: repo, log: log}
}

// Crear registra un período académico activo.
func (s *Periodos) Crear(ctx context.Context, actor auth.Actor, n NuevoPeriodo) (dominio.PeriodoAcademico, error) {
	if err := exigirRol(actor, dominio.RolAdministrador); err != nil {
		return dominio.PeriodoAcademico{}, err
	}
	p := dominio.PeriodoAcademico{
		Codigo:             strings.TrimSpace(n.Codigo),
		FechaInicio:        n.FechaInicio,
		FechaCierre:        n.FechaCierre,
		Estado:             dominio.PeriodoActivo,
		CreacionHabilitada: n.CreacionHabilitada,
	}
	if err := dominio.ValidarCodigoPeriodo(p.Codigo); err != nil {
		return p, err
	}
	if err := p.ValidarFechas(); err != nil {
		return p, err
	}
	creado, err := s.repo.Crear(ctx, p)
	if err == nil {
		s.log.Info("período creado", "codigo", creado.Codigo, "por", actor.UsuarioID)
	}
	return creado, err
}

// Listar devuelve los períodos.
func (s *Periodos) Listar(ctx context.Context, _ auth.Actor, f dominio.FiltroPeriodos) ([]dominio.PeriodoAcademico, error) {
	if f.Estado != "" && !f.Estado.EsValido() {
		return nil, apperr.Validacion("estado inválido: use activo o cerrado")
	}
	return s.repo.Listar(ctx, f)
}

// Obtener devuelve un período.
func (s *Periodos) Obtener(ctx context.Context, _ auth.Actor, id string) (dominio.PeriodoAcademico, error) {
	return s.repo.Obtener(ctx, id)
}

// Actualizar cierra o reabre el período y abre o cierra la ventana de creación
// de cursos y proyectos. Un período cerrado deja de admitir creaciones.
func (s *Periodos) Actualizar(ctx context.Context, actor auth.Actor, id string, c CambiosPeriodo) (dominio.PeriodoAcademico, error) {
	if err := exigirRol(actor, dominio.RolAdministrador); err != nil {
		return dominio.PeriodoAcademico{}, err
	}
	if c.Estado == nil && c.CreacionHabilitada == nil {
		return dominio.PeriodoAcademico{}, apperr.Validacion("indique estado o creacion_habilitada")
	}
	p, err := s.repo.Obtener(ctx, id)
	if err != nil {
		return p, err
	}
	if c.Estado != nil {
		if !c.Estado.EsValido() {
			return p, apperr.Validacion("estado inválido: use activo o cerrado")
		}
		p.Estado = *c.Estado
	}
	if c.CreacionHabilitada != nil {
		p.CreacionHabilitada = *c.CreacionHabilitada
	}
	if p.Estado == dominio.PeriodoCerrado {
		p.CreacionHabilitada = false
	}
	actualizado, err := s.repo.Actualizar(ctx, p)
	if err == nil {
		s.log.Info("período actualizado", "codigo", actualizado.Codigo, "estado", actualizado.Estado,
			"creacion_habilitada", actualizado.CreacionHabilitada, "por", actor.UsuarioID)
	}
	return actualizado, err
}
