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

// ServicioEspacios es lo que el handler de cursos y proyectos necesita.
type ServicioEspacios interface {
	Crear(ctx context.Context, actor auth.Actor, n servicio.NuevoEspacio) (dominio.Espacio, error)
	Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroEspacios) ([]dominio.Espacio, error)
	Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.Espacio, error)
	Cerrar(ctx context.Context, actor auth.Actor, id string) (dominio.Espacio, error)
}

// HandlerEspacios expone los cursos y proyectos (RF-03).
type HandlerEspacios struct {
	servicio ServicioEspacios
	log      *slog.Logger
}

// Registrar declara las rutas del módulo.
func (h *HandlerEspacios) Registrar(g *gin.RouterGroup) {
	r := g.Group("/espacios")
	r.GET("", h.Listar)
	r.GET("/:id", h.Obtener)
	r.POST("", RequiereRol(dominio.RolProfesor), h.Crear)
	r.POST("/:id/cerrar", RequiereRol(dominio.RolProfesor, dominio.RolAdministrador), h.Cerrar)
}

type espacioJSON struct {
	ID            string                `json:"id"`
	Tipo          dominio.TipoEspacio   `json:"tipo"`
	Nombre        string                `json:"nombre"`
	Periodo       periodoRef            `json:"periodo"`
	Profesor      personaRef            `json:"profesor"`
	FechaInicio   string                `json:"fecha_inicio"`
	FechaCierre   string                `json:"fecha_cierre"`
	Observaciones string                `json:"observaciones"`
	Estado        dominio.EstadoEspacio `json:"estado"`
	SoloLectura   bool                  `json:"solo_lectura"`
	CreadoEn      time.Time             `json:"creado_en"`
}

type periodoRef struct {
	ID     string `json:"id"`
	Codigo string `json:"codigo"`
}

func aEspacioJSON(e dominio.Espacio) espacioJSON {
	return espacioJSON{
		ID: e.ID, Tipo: e.Tipo, Nombre: e.Nombre,
		Periodo:     periodoRef{ID: e.PeriodoID, Codigo: e.PeriodoCodigo},
		Profesor:    personaRef{ID: e.ProfesorID, Nombre: e.ProfesorNombre},
		FechaInicio: fecha(e.FechaInicio), FechaCierre: fecha(e.FechaCierre),
		Observaciones: e.Observaciones, Estado: e.Estado,
		SoloLectura: e.Estado == dominio.EspacioCerrado, CreadoEn: e.CreadoEn,
	}
}

type peticionEspacio struct {
	Tipo          dominio.TipoEspacio `json:"tipo"`
	Nombre        string              `json:"nombre"`
	PeriodoID     string              `json:"periodo_id"`
	FechaInicio   string              `json:"fecha_inicio"`
	FechaCierre   string              `json:"fecha_cierre"`
	Observaciones string              `json:"observaciones"`
}

// Crear registra un curso o proyecto del profesor autenticado (RF-03.1 a RF-03.3).
func (h *HandlerEspacios) Crear(c *gin.Context) {
	var p peticionEspacio
	if err := leerJSON(c, &p); err != nil {
		responderError(c, h.log, err)
		return
	}
	periodoID, err := validarUUIDOpcional("periodo_id", p.PeriodoID)
	if err != nil {
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
	e, err := h.servicio.Crear(c.Request.Context(), ActorDe(c), servicio.NuevoEspacio{
		Tipo: p.Tipo, Nombre: p.Nombre, PeriodoID: periodoID,
		FechaInicio: inicio, FechaCierre: cierre, Observaciones: p.Observaciones,
	})
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.Header("Location", "/api/v1/espacios/"+e.ID)
	c.JSON(http.StatusCreated, aEspacioJSON(e))
}

// Listar devuelve los cursos y proyectos visibles (RF-03.4, RF-03.6, RF-11.1),
// con filtros opcionales periodo_id, estado y tipo.
func (h *HandlerEspacios) Listar(c *gin.Context) {
	periodoID, err := consultaUUID(c, "periodo_id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	espacios, err := h.servicio.Listar(c.Request.Context(), ActorDe(c), dominio.FiltroEspacios{
		PeriodoID:  periodoID,
		Estado:     dominio.EstadoEspacio(c.Query("estado")),
		Tipo:       dominio.TipoEspacio(c.Query("tipo")),
		Paginacion: consultaPaginacion(c),
	})
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, convertir(espacios, aEspacioJSON))
}

// Obtener devuelve un curso o proyecto.
func (h *HandlerEspacios) Obtener(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	e, err := h.servicio.Obtener(c.Request.Context(), ActorDe(c), id)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, aEspacioJSON(e))
}

// Cerrar pasa el curso o proyecto a cerrado (RF-03.5).
func (h *HandlerEspacios) Cerrar(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	e, err := h.servicio.Cerrar(c.Request.Context(), ActorDe(c), id)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, aEspacioJSON(e))
}
