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

// ServicioPeriodos es lo que el handler de períodos necesita.
type ServicioPeriodos interface {
	Crear(ctx context.Context, actor auth.Actor, n servicio.NuevoPeriodo) (dominio.PeriodoAcademico, error)
	Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroPeriodos) ([]dominio.PeriodoAcademico, error)
	Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.PeriodoAcademico, error)
	Actualizar(ctx context.Context, actor auth.Actor, id string, c servicio.CambiosPeriodo) (dominio.PeriodoAcademico, error)
}

// HandlerPeriodos expone los períodos académicos (RF-03.2).
type HandlerPeriodos struct {
	servicio ServicioPeriodos
	log      *slog.Logger
}

// Registrar declara las rutas del módulo.
func (h *HandlerPeriodos) Registrar(g *gin.RouterGroup) {
	r := g.Group("/periodos")
	r.GET("", h.Listar)
	r.GET("/:id", h.Obtener)
	r.POST("", RequiereRol(dominio.RolAdministrador), h.Crear)
	r.PATCH("/:id", RequiereRol(dominio.RolAdministrador), h.Actualizar)
}

type periodoJSON struct {
	ID                 string                `json:"id"`
	Codigo             string                `json:"codigo"`
	FechaInicio        string                `json:"fecha_inicio"`
	FechaCierre        string                `json:"fecha_cierre"`
	Estado             dominio.EstadoPeriodo `json:"estado"`
	CreacionHabilitada bool                  `json:"creacion_habilitada"`
	CreadoEn           time.Time             `json:"creado_en"`
}

func aPeriodoJSON(p dominio.PeriodoAcademico) periodoJSON {
	return periodoJSON{ID: p.ID, Codigo: p.Codigo, FechaInicio: fecha(p.FechaInicio), FechaCierre: fecha(p.FechaCierre),
		Estado: p.Estado, CreacionHabilitada: p.CreacionHabilitada, CreadoEn: p.CreadoEn}
}

type peticionPeriodo struct {
	Codigo             string `json:"codigo"`
	FechaInicio        string `json:"fecha_inicio"`
	FechaCierre        string `json:"fecha_cierre"`
	CreacionHabilitada *bool  `json:"creacion_habilitada"`
}

type peticionCambiosPeriodo struct {
	Estado             *dominio.EstadoPeriodo `json:"estado"`
	CreacionHabilitada *bool                  `json:"creacion_habilitada"`
}

// Crear registra un período. La ventana de creación queda habilitada salvo que se indique lo contrario.
func (h *HandlerPeriodos) Crear(c *gin.Context) {
	var p peticionPeriodo
	if err := leerJSON(c, &p); err != nil {
		responderError(c, h.log, err)
		return
	}
	inicio, err := parsearFecha("fecha_inicio", p.FechaInicio)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	cierre, err := parsearFecha("fecha_cierre", p.FechaCierre)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	habilitada := p.CreacionHabilitada == nil || *p.CreacionHabilitada
	periodo, err := h.servicio.Crear(c.Request.Context(), ActorDe(c), servicio.NuevoPeriodo{
		Codigo: p.Codigo, FechaInicio: inicio, FechaCierre: cierre, CreacionHabilitada: habilitada,
	})
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.Header("Location", "/api/v1/periodos/"+periodo.ID)
	c.JSON(http.StatusCreated, aPeriodoJSON(periodo))
}

// Listar devuelve los períodos, con filtro opcional de estado.
func (h *HandlerPeriodos) Listar(c *gin.Context) {
	periodos, err := h.servicio.Listar(c.Request.Context(), ActorDe(c), dominio.FiltroPeriodos{
		Estado: dominio.EstadoPeriodo(c.Query("estado")), Paginacion: consultaPaginacion(c),
	})
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, convertir(periodos, aPeriodoJSON))
}

// Obtener devuelve un período.
func (h *HandlerPeriodos) Obtener(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	p, err := h.servicio.Obtener(c.Request.Context(), ActorDe(c), id)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, aPeriodoJSON(p))
}

// Actualizar cambia el estado del período o su ventana de creación.
func (h *HandlerPeriodos) Actualizar(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	var p peticionCambiosPeriodo
	if err := leerJSON(c, &p); err != nil {
		responderError(c, h.log, err)
		return
	}
	periodo, err := h.servicio.Actualizar(c.Request.Context(), ActorDe(c), id, servicio.CambiosPeriodo{
		Estado: p.Estado, CreacionHabilitada: p.CreacionHabilitada,
	})
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, aPeriodoJSON(periodo))
}
