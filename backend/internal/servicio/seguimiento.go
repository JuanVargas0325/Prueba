package servicio

import (
	"context"
	"time"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

// ListadorVinculaciones permite consultar vinculaciones con filtros.
type ListadorVinculaciones interface {
	Listar(ctx context.Context, f dominio.FiltroVinculaciones) ([]dominio.Vinculacion, error)
}

// ListadorTareasPorVinculacion permite consultar las tareas de varias vinculaciones.
type ListadorTareasPorVinculacion interface {
	ListarPorVinculaciones(ctx context.Context, vinculacionIDs []string, desde, hasta *time.Time) ([]dominio.Tarea, error)
}

// BuscadorUsuarios permite consultar un usuario por su identificador.
type BuscadorUsuarios interface {
	BuscarPorID(ctx context.Context, id string) (dominio.Usuario, error)
}

// FiltroPersonas delimita el listado de personas a cargo.
type FiltroPersonas struct {
	EspacioID  string
	ProfesorID string // solo lo usa el administrador
}

// ResumenPersona es una fila de la vista principal del profesor (RF-10.1):
// la persona, sus vinculaciones y cómo va su reporte en la semana en curso y en la anterior.
type ResumenPersona struct {
	dominio.PersonaACargo
	SemanaActual   *dominio.SemanaPersona
	SemanaAnterior *dominio.SemanaPersona
}

// DetallePersona es el detalle semanal del trabajo de una persona (RF-10.3).
type DetallePersona struct {
	Persona dominio.Usuario
	Semanas []dominio.SemanaPersona
}

// ConsolidadoPersona es el resumen de todo el trabajo de una persona (RF-10.4).
type ConsolidadoPersona struct {
	Persona       dominio.Usuario
	Vinculaciones []dominio.ConsolidadoVinculacion
}

// Seguimiento arma las vistas del profesor y el historial del usuario
// operativo (RF-09, RF-10, RF-11.3).
type Seguimiento struct {
	vinculaciones ListadorVinculaciones
	tareas        ListadorTareasPorVinculacion
	usuarios      BuscadorUsuarios
	reloj         Reloj
}

// NuevoSeguimiento crea el servicio de seguimiento.
func NuevoSeguimiento(vinculaciones ListadorVinculaciones, tareas ListadorTareasPorVinculacion, usuarios BuscadorUsuarios, reloj Reloj) *Seguimiento {
	return &Seguimiento{vinculaciones: vinculaciones, tareas: tareas, usuarios: usuarios, reloj: reloj}
}

// PersonasACargo lista los monitores y asistentes del profesor con sus
// vinculaciones y el estado de su reporte: qué registró esta semana, cuánto
// tiempo consignó y si dejó la semana anterior sin reporte.
func (s *Seguimiento) PersonasACargo(ctx context.Context, actor auth.Actor, f FiltroPersonas) ([]ResumenPersona, error) {
	filtro := dominio.FiltroVinculaciones{EspacioID: f.EspacioID, Paginacion: dominio.SinPaginar}
	switch {
	case actor.EsAdministrador():
		filtro.ProfesorID = f.ProfesorID
	case actor.TieneRol(dominio.RolProfesor):
		filtro.ProfesorID = actor.UsuarioID // RN-03
	default:
		return nil, apperr.Prohibido("solo profesores y administradores consultan personas a cargo")
	}

	vinculaciones, err := s.vinculaciones.Listar(ctx, filtro)
	if err != nil {
		return nil, err
	}
	actual := s.reloj.SemanaActual()
	anterior := actual.AddDate(0, 0, -7)
	tareas, err := s.tareas.ListarPorVinculaciones(ctx, idsDe(vinculaciones), &anterior, &actual)
	if err != nil {
		return nil, err
	}

	personas := dominio.AgruparPorPersona(vinculaciones)
	resultado := make([]ResumenPersona, 0, len(personas))
	for _, p := range personas {
		r := ResumenPersona{PersonaACargo: p}
		for _, semana := range dominio.ConstruirSemanas(p.Vinculaciones, tareas, anterior, actual, s.reloj.Ahora(), s.reloj.Zona()) {
			semana := semana
			if semana.SemanaInicio.Equal(actual) {
				r.SemanaActual = &semana
			} else {
				r.SemanaAnterior = &semana
			}
		}
		resultado = append(resultado, r)
	}
	return resultado, nil
}

// SemanasPersona devuelve el detalle semana a semana de una persona entre
// desde y hasta (por defecto, las últimas 8 semanas), con sus tareas y el
// estado "sin_reporte" en las semanas cerradas sin registros (RN-35 a RN-37).
func (s *Seguimiento) SemanasPersona(ctx context.Context, actor auth.Actor, personaID string, desde, hasta *time.Time) (DetallePersona, error) {
	inicio, fin, err := s.rangoSemanas(desde, hasta)
	if err != nil {
		return DetallePersona{}, err
	}
	persona, vinculaciones, err := s.vinculacionesVisibles(ctx, actor, personaID)
	if err != nil {
		return DetallePersona{}, err
	}
	tareas, err := s.tareas.ListarPorVinculaciones(ctx, idsDe(vinculaciones), &inicio, &fin)
	if err != nil {
		return DetallePersona{}, err
	}
	return DetallePersona{
		Persona: persona,
		Semanas: dominio.ConstruirSemanas(vinculaciones, tareas, inicio, fin, s.reloj.Ahora(), s.reloj.Zona()),
	}, nil
}

// Consolidado devuelve el resumen acumulado del trabajo de la persona por vinculación.
func (s *Seguimiento) Consolidado(ctx context.Context, actor auth.Actor, personaID string) (ConsolidadoPersona, error) {
	persona, vinculaciones, err := s.vinculacionesVisibles(ctx, actor, personaID)
	if err != nil {
		return ConsolidadoPersona{}, err
	}
	tareas, err := s.tareas.ListarPorVinculaciones(ctx, idsDe(vinculaciones), nil, nil)
	if err != nil {
		return ConsolidadoPersona{}, err
	}
	return ConsolidadoPersona{
		Persona:       persona,
		Vinculaciones: dominio.ConstruirConsolidado(vinculaciones, tareas, s.reloj.Ahora(), s.reloj.Zona()),
	}, nil
}

// vinculacionesVisibles aplica RF-12: la propia persona y el administrador
// ven todas sus vinculaciones; un profesor solo las de sus espacios.
func (s *Seguimiento) vinculacionesVisibles(ctx context.Context, actor auth.Actor, personaID string) (dominio.Usuario, []dominio.Vinculacion, error) {
	filtro := dominio.FiltroVinculaciones{UsuarioID: personaID, Paginacion: dominio.SinPaginar}
	propio := actor.UsuarioID == personaID
	switch {
	case propio || actor.EsAdministrador():
	case actor.TieneRol(dominio.RolProfesor):
		filtro.ProfesorID = actor.UsuarioID
	default:
		return dominio.Usuario{}, nil, noEncontrado("persona")
	}

	persona, err := s.usuarios.BuscarPorID(ctx, personaID)
	if err != nil {
		return persona, nil, err
	}
	vinculaciones, err := s.vinculaciones.Listar(ctx, filtro)
	if err != nil {
		return persona, nil, err
	}
	if len(vinculaciones) == 0 && !propio && !actor.EsAdministrador() {
		return dominio.Usuario{}, nil, noEncontrado("persona")
	}
	return persona, vinculaciones, nil
}

// rangoSemanas normaliza el rango de consulta: lunes a lunes, hasta la semana
// actual como máximo y sin superar MaxSemanasConsulta semanas.
func (s *Seguimiento) rangoSemanas(desde, hasta *time.Time) (time.Time, time.Time, error) {
	actual := s.reloj.SemanaActual()
	fin := actual
	if hasta != nil && dominio.InicioSemana(*hasta).Before(actual) {
		fin = dominio.InicioSemana(*hasta)
	}
	inicio := fin.AddDate(0, 0, -7*7) // 8 semanas por defecto
	if desde != nil {
		inicio = dominio.InicioSemana(*desde)
	}
	if inicio.After(fin) {
		return inicio, fin, apperr.Validacion("desde no puede ser posterior a hasta")
	}
	if fin.Sub(inicio) > time.Duration(dominio.MaxSemanasConsulta-1)*7*24*time.Hour {
		return inicio, fin, apperr.Validacion("el rango máximo de consulta es de %d semanas", dominio.MaxSemanasConsulta)
	}
	return inicio, fin, nil
}

func idsDe(vinculaciones []dominio.Vinculacion) []string {
	ids := make([]string, 0, len(vinculaciones))
	for _, v := range vinculaciones {
		ids = append(ids, v.ID)
	}
	return ids
}
