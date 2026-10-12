package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
	"seguimiento/internal/servicio"
)

// ServicioSeguimiento es lo que el handler de seguimiento necesita.
type ServicioSeguimiento interface {
	PersonasACargo(ctx context.Context, actor auth.Actor, f servicio.FiltroPersonas) ([]servicio.ResumenPersona, error)
	SemanasPersona(ctx context.Context, actor auth.Actor, personaID string, desde, hasta *time.Time) (servicio.DetallePersona, error)
	Consolidado(ctx context.Context, actor auth.Actor, personaID string) (servicio.ConsolidadoPersona, error)
}

// HandlerSeguimiento expone las vistas de seguimiento del profesor (RF-10) y
// el historial semanal del monitor o asistente (RF-11.3).
type HandlerSeguimiento struct {
	servicio ServicioSeguimiento
	log      *slog.Logger
}

// Registrar declara las rutas del módulo.
func (h *HandlerSeguimiento) Registrar(g *gin.RouterGroup) {
	r := g.Group("/seguimiento")
	r.GET("/personas", RequiereRol(dominio.RolProfesor, dominio.RolAdministrador), h.PersonasACargo)
	// La visibilidad por persona la decide el servicio: la propia persona, su
	// profesor (solo sus espacios) o el administrador.
	r.GET("/personas/:id/semanas", h.SemanasPersona)
	r.GET("/personas/:id/consolidado", h.Consolidado)
	r.GET("/mi-historial", h.MiHistorial)
}

// --- formatos ---------------------------------------------------------------------

type vinculacionResumenJSON struct {
	ID             string      `json:"id"`
	Espacio        espacioRef  `json:"espacio"`
	Rol            dominio.Rol `json:"rol"`
	HorasSemanales int         `json:"horas_semanales"`
}

func aVinculacionResumenJSON(v dominio.Vinculacion) vinculacionResumenJSON {
	return vinculacionResumenJSON{
		ID:             v.ID,
		Espacio:        espacioRef{ID: v.EspacioID, Nombre: v.EspacioNombre, Tipo: v.EspacioTipo, Estado: v.EspacioEstado},
		Rol:            v.Rol,
		HorasSemanales: v.HorasSemanales,
	}
}

type resumenSemanaJSON struct {
	SemanaInicio     string               `json:"semana_inicio"`
	SemanaFin        string               `json:"semana_fin"`
	Estado           dominio.EstadoSemana `json:"estado"`
	HorasContratadas int                  `json:"horas_contratadas"`
	HorasReportadas  int                  `json:"horas_reportadas"`
	Tareas           int                  `json:"tareas"`
}

func aResumenSemanaJSON(s *dominio.SemanaPersona) *resumenSemanaJSON {
	if s == nil {
		return nil
	}
	tareas := 0
	for _, d := range s.Vinculaciones {
		tareas += len(d.Tareas)
	}
	return &resumenSemanaJSON{SemanaInicio: fecha(s.SemanaInicio), SemanaFin: fecha(s.SemanaFin()), Estado: s.Estado,
		HorasContratadas: s.HorasContratadas, HorasReportadas: s.HorasReportadas, Tareas: tareas}
}

type personaACargoJSON struct {
	Usuario          personaRef               `json:"usuario"`
	HorasContratadas int                      `json:"horas_contratadas"`
	Vinculaciones    []vinculacionResumenJSON `json:"vinculaciones"`
	SemanaActual     *resumenSemanaJSON       `json:"semana_actual"`
	SemanaAnterior   *resumenSemanaJSON       `json:"semana_anterior"`
}

type detalleVinculacionSemanaJSON struct {
	Vinculacion     vinculacionResumenJSON `json:"vinculacion"`
	Estado          dominio.EstadoSemana   `json:"estado"`
	HorasReportadas int                    `json:"horas_reportadas"`
	TareasTardias   int                    `json:"tareas_tardias"`
	Tareas          []tareaResumenJSON     `json:"tareas"`
}

type semanaJSON struct {
	resumenSemanaJSON
	Vinculaciones []detalleVinculacionSemanaJSON `json:"vinculaciones"`
}

type detallePersonaJSON struct {
	Persona personaRef   `json:"persona"`
	Semanas []semanaJSON `json:"semanas"`
}

type consolidadoVinculacionJSON struct {
	Vinculacion          vinculacionResumenJSON      `json:"vinculacion"`
	SemanasTranscurridas int                         `json:"semanas_transcurridas"`
	SemanasConReporte    int                         `json:"semanas_con_reporte"`
	SemanasSinReporte    int                         `json:"semanas_sin_reporte"`
	TotalTareas          int                         `json:"total_tareas"`
	TotalHoras           int                         `json:"total_horas"`
	TareasTardias        int                         `json:"tareas_tardias"`
	TareasPorEstado      map[dominio.EstadoTarea]int `json:"tareas_por_estado"`
	PromedioHorasSemana  float64                     `json:"promedio_horas_semana"`
}

type consolidadoPersonaJSON struct {
	Persona       personaRef                   `json:"persona"`
	Vinculaciones []consolidadoVinculacionJSON `json:"vinculaciones"`
}

func aSemanaJSON(s dominio.SemanaPersona) semanaJSON {
	j := semanaJSON{resumenSemanaJSON: *aResumenSemanaJSON(&s)}
	for _, d := range s.Vinculaciones {
		j.Vinculaciones = append(j.Vinculaciones, detalleVinculacionSemanaJSON{
			Vinculacion: aVinculacionResumenJSON(d.Vinculacion), Estado: d.Estado,
			HorasReportadas: d.HorasReportadas, TareasTardias: d.TareasTardias,
			Tareas: convertir(d.Tareas, aTareaResumenJSON),
		})
	}
	return j
}

// --- endpoints -----------------------------------------------------------------------

// PersonasACargo es la vista principal del profesor (RF-10.1, RF-10.2): sus
// monitores y asistentes, sus vinculaciones, lo que reportaron esta semana y
// si la semana anterior quedó sin reporte. Filtros: espacio_id y, para el
// administrador, profesor_id.
func (h *HandlerSeguimiento) PersonasACargo(c *gin.Context) {
	espacioID, err := consultaUUID(c, "espacio_id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	profesorID, err := consultaUUID(c, "profesor_id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	personas, err := h.servicio.PersonasACargo(c.Request.Context(), ActorDe(c), servicio.FiltroPersonas{EspacioID: espacioID, ProfesorID: profesorID})
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, convertir(personas, func(p servicio.ResumenPersona) personaACargoJSON {
		return personaACargoJSON{
			Usuario:          personaRef{ID: p.UsuarioID, Nombre: p.Nombre, Correo: p.Correo},
			HorasContratadas: p.HorasContratadas,
			Vinculaciones:    convertir(p.Vinculaciones, aVinculacionResumenJSON),
			SemanaActual:     aResumenSemanaJSON(p.SemanaActual),
			SemanaAnterior:   aResumenSemanaJSON(p.SemanaAnterior),
		}
	}))
}

// SemanasPersona devuelve el detalle semanal de una persona (RF-10.3, RF-10.6,
// RF-09), con rango opcional desde/hasta (AAAA-MM-DD).
func (h *HandlerSeguimiento) SemanasPersona(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	h.responderSemanas(c, id)
}

// MiHistorial devuelve el historial semanal del usuario autenticado (RF-11.3).
func (h *HandlerSeguimiento) MiHistorial(c *gin.Context) {
	h.responderSemanas(c, ActorDe(c).UsuarioID)
}

func (h *HandlerSeguimiento) responderSemanas(c *gin.Context, personaID string) {
	desde, err := consultaFecha(c, "desde")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	hasta, err := consultaFecha(c, "hasta")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	d, err := h.servicio.SemanasPersona(c.Request.Context(), ActorDe(c), personaID, desde, hasta)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, detallePersonaJSON{
		Persona: personaRef{ID: d.Persona.ID, Nombre: d.Persona.Nombre, Correo: d.Persona.Correo},
		Semanas: convertir(d.Semanas, aSemanaJSON),
	})
}

// Consolidado devuelve el resumen acumulado del trabajo de una persona (RF-10.4).
func (h *HandlerSeguimiento) Consolidado(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	cp, err := h.servicio.Consolidado(c.Request.Context(), ActorDe(c), id)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, consolidadoPersonaJSON{
		Persona: personaRef{ID: cp.Persona.ID, Nombre: cp.Persona.Nombre, Correo: cp.Persona.Correo},
		Vinculaciones: convertir(cp.Vinculaciones, func(v dominio.ConsolidadoVinculacion) consolidadoVinculacionJSON {
			return consolidadoVinculacionJSON{
				Vinculacion: aVinculacionResumenJSON(v.Vinculacion), SemanasTranscurridas: v.SemanasTranscurridas,
				SemanasConReporte: v.SemanasConReporte, SemanasSinReporte: v.SemanasSinReporte,
				TotalTareas: v.TotalTareas, TotalHoras: v.TotalHoras, TareasTardias: v.TareasTardias,
				TareasPorEstado: v.TareasPorEstado, PromedioHorasSemana: v.PromedioHorasSemana,
			}
		}),
	})
}
