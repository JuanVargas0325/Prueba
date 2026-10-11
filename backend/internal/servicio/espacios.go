package servicio

import (
	"context"
	"log/slog"
	"time"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

// RepositorioEspacios es lo que la gestión de cursos y proyectos necesita de la capa de datos.
type RepositorioEspacios interface {
	Crear(ctx context.Context, e dominio.Espacio) (dominio.Espacio, error)
	Obtener(ctx context.Context, id string) (dominio.Espacio, error)
	Listar(ctx context.Context, f dominio.FiltroEspacios) ([]dominio.Espacio, error)
	Cerrar(ctx context.Context, id string) (dominio.Espacio, error)
	EsParticipante(ctx context.Context, espacioID, usuarioID string) (bool, error)
}

// LectorPeriodos permite consultar un período académico.
type LectorPeriodos interface {
	Obtener(ctx context.Context, id string) (dominio.PeriodoAcademico, error)
}

// NuevoEspacio son los datos para crear un curso o proyecto (RF-03.3).
type NuevoEspacio struct {
	Tipo          dominio.TipoEspacio
	Nombre        string
	PeriodoID     string
	FechaInicio   time.Time
	FechaCierre   time.Time
	Observaciones string
}

// Espacios gestiona los cursos y proyectos (RF-03, RN-06 a RN-09).
type Espacios struct {
	repo     RepositorioEspacios
	periodos LectorPeriodos
	reloj    Reloj
	log      *slog.Logger
}

// NuevoEspacios crea el servicio de cursos y proyectos.
func NuevoEspacios(repo RepositorioEspacios, periodos LectorPeriodos, reloj Reloj, log *slog.Logger) *Espacios {
	return &Espacios{repo: repo, periodos: periodos, reloj: reloj, log: log}
}

// Crear registra un curso o proyecto del profesor autenticado, solo dentro de
// un período habilitado para creación (RF-03.1, RF-03.2, RN-07).
func (s *Espacios) Crear(ctx context.Context, actor auth.Actor, n NuevoEspacio) (dominio.Espacio, error) {
	if err := exigirRol(actor, dominio.RolProfesor); err != nil {
		return dominio.Espacio{}, err
	}
	if !n.Tipo.EsValido() {
		return dominio.Espacio{}, apperr.Validacion("tipo inválido: use curso o proyecto")
	}
	nombre, err := dominio.Texto("el nombre", n.Nombre, true, dominio.LongitudMaxNombre)
	if err != nil {
		return dominio.Espacio{}, err
	}
	observaciones, err := dominio.Texto("las observaciones", n.Observaciones, false, dominio.LongitudMaxObservaciones)
	if err != nil {
		return dominio.Espacio{}, err
	}
	periodo, err := s.periodos.Obtener(ctx, n.PeriodoID)
	if apperr.Es(err, apperr.TipoNoEncontrado) {
		return dominio.Espacio{}, apperr.Validacion("el período académico indicado no existe")
	}
	if err != nil {
		return dominio.Espacio{}, err
	}
	if err := periodo.PermiteCrearEspacios(s.reloj.Hoy()); err != nil {
		return dominio.Espacio{}, err
	}
	e := dominio.Espacio{
		Tipo: n.Tipo, Nombre: nombre, PeriodoID: periodo.ID, ProfesorID: actor.UsuarioID,
		FechaInicio: n.FechaInicio, FechaCierre: n.FechaCierre, Observaciones: observaciones,
		Estado: dominio.EspacioActivo,
	}
	if err := e.ValidarFechasEnPeriodo(periodo); err != nil {
		return dominio.Espacio{}, err
	}
	creado, err := s.repo.Crear(ctx, e)
	if err == nil {
		s.log.Info("espacio creado", "espacio_id", creado.ID, "tipo", creado.Tipo, "profesor_id", actor.UsuarioID)
	}
	return creado, err
}

// Listar devuelve los espacios visibles: el administrador ve todos; los demás,
// los que dirigen como profesor y aquellos en los que participan (RF-03.4, RF-11.1).
func (s *Espacios) Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroEspacios) ([]dominio.Espacio, error) {
	if f.Estado != "" && !f.Estado.EsValido() {
		return nil, apperr.Validacion("estado inválido: use activo o cerrado")
	}
	if f.Tipo != "" && !f.Tipo.EsValido() {
		return nil, apperr.Validacion("tipo inválido: use curso o proyecto")
	}
	f.VisiblePara = visiblePara(actor)
	return s.repo.Listar(ctx, f)
}

// Obtener devuelve un espacio si el actor puede verlo. Los espacios cerrados se
// consultan en modo de solo lectura (RF-03.6).
func (s *Espacios) Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.Espacio, error) {
	e, err := s.repo.Obtener(ctx, id)
	if err != nil {
		return e, err
	}
	if puedeVerDeProfesor(actor, e.ProfesorID) {
		return e, nil
	}
	participa, err := s.repo.EsParticipante(ctx, id, actor.UsuarioID)
	if err != nil {
		return dominio.Espacio{}, err
	}
	if !participa {
		return dominio.Espacio{}, noEncontrado("curso o proyecto")
	}
	return e, nil
}

// Cerrar pasa un espacio a cerrado; desde entonces no admite nuevos registros
// (RF-03.5, RF-03.7). Lo hace el profesor responsable o el administrador.
func (s *Espacios) Cerrar(ctx context.Context, actor auth.Actor, id string) (dominio.Espacio, error) {
	e, err := s.Obtener(ctx, actor, id)
	if err != nil {
		return e, err
	}
	if !puedeVerDeProfesor(actor, e.ProfesorID) {
		return dominio.Espacio{}, apperr.Prohibido("solo el profesor responsable puede cerrar el %s", e.Tipo)
	}
	cerrado, err := s.repo.Cerrar(ctx, id)
	if err == nil {
		s.log.Info("espacio cerrado", "espacio_id", id, "por", actor.UsuarioID)
	}
	return cerrado, err
}
