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

// ServicioVinculaciones es lo que el handler de vinculaciones necesita.
type ServicioVinculaciones interface {
	Crear(ctx context.Context, actor auth.Actor, n servicio.NuevaVinculacion) (dominio.Vinculacion, error)
	Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroVinculaciones) ([]dominio.Vinculacion, error)
	Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.Vinculacion, error)
	Rectificar(ctx context.Context, actor auth.Actor, id string, r servicio.SolicitudRectificacion) (dominio.Vinculacion, error)
	ListarRectificaciones(ctx context.Context, actor auth.Actor, id string) ([]dominio.Rectificacion, error)
}

// HandlerVinculaciones expone las vinculaciones de personas a cursos y proyectos (RF-04, RF-05).
type HandlerVinculaciones struct {
	servicio ServicioVinculaciones
	log      *slog.Logger
}

// Registrar declara las rutas del módulo.
func (h *HandlerVinculaciones) Registrar(g *gin.RouterGroup) {
	r := g.Group("/vinculaciones")
	r.GET("", h.Listar)
	r.GET("/:id", h.Obtener)
	r.POST("", RequiereRol(dominio.RolProfesor), h.Crear)
	r.PUT("/:id", RequiereRol(dominio.RolAdministrador), h.Rectificar)
	r.GET("/:id/rectificaciones", RequiereRol(dominio.RolProfesor, dominio.RolAdministrador), h.ListarRectificaciones)
}

type peticionVinculacion struct {
	EspacioID      string      `json:"espacio_id"`
	UsuarioID      string      `json:"usuario_id"`
	Correo         string      `json:"correo"`
	Rol            dominio.Rol `json:"rol"`
	HorasSemanales int         `json:"horas_semanales"`
}

type peticionRectificacion struct {
	Rol            *dominio.Rol `json:"rol"`
	HorasSemanales *int         `json:"horas_semanales"`
	Motivo         string       `json:"motivo"`
}

type rectificacionJSON struct {
	ID              string      `json:"id"`
	AdministradorID string      `json:"administrador_id"`
	RolAnterior     dominio.Rol `json:"rol_anterior"`
	RolNuevo        dominio.Rol `json:"rol_nuevo"`
	HorasAnteriores int         `json:"horas_anteriores"`
	HorasNuevas     int         `json:"horas_nuevas"`
	Motivo          string      `json:"motivo"`
	CreadoEn        time.Time   `json:"creado_en"`
}

func aRectificacionJSON(r dominio.Rectificacion) rectificacionJSON {
	return rectificacionJSON{ID: r.ID, AdministradorID: r.AdministradorID, RolAnterior: r.RolAnterior, RolNuevo: r.RolNuevo,
		HorasAnteriores: r.HorasAnteriores, HorasNuevas: r.HorasNuevas, Motivo: r.Motivo, CreadoEn: r.CreadoEn}
}

// Crear vincula a una persona (por usuario_id o correo) a un curso o proyecto
// del profesor, validando las reglas de horas (RF-04.1 a RF-04.3, RF-05).
func (h *HandlerVinculaciones) Crear(c *gin.Context) {
	var p peticionVinculacion
	if err := leerJSON(c, &p); err != nil {
		responderError(c, h.log, err)
		return
	}
	espacioID, err := validarUUIDOpcional("espacio_id", p.EspacioID)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	usuarioID, err := validarUUIDOpcional("usuario_id", p.UsuarioID)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	v, err := h.servicio.Crear(c.Request.Context(), ActorDe(c), servicio.NuevaVinculacion{
		EspacioID: espacioID, UsuarioID: usuarioID, Correo: p.Correo, Rol: p.Rol, HorasSemanales: p.HorasSemanales,
	})
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.Header("Location", "/api/v1/vinculaciones/"+v.ID)
	c.JSON(http.StatusCreated, aVinculacionJSON(v))
}

// Listar devuelve las vinculaciones visibles, con filtros opcionales espacio_id y usuario_id.
// Para un monitor o asistente es el listado unificado de sus cursos y proyectos (RF-11.1).
func (h *HandlerVinculaciones) Listar(c *gin.Context) {
	espacioID, err := consultaUUID(c, "espacio_id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	usuarioID, err := consultaUUID(c, "usuario_id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	vinculaciones, err := h.servicio.Listar(c.Request.Context(), ActorDe(c), dominio.FiltroVinculaciones{
		EspacioID: espacioID, UsuarioID: usuarioID,
		SoloEspaciosActivos: c.Query("solo_activas") == "true",
		Paginacion:          consultaPaginacion(c),
	})
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, convertir(vinculaciones, aVinculacionJSON))
}

// Obtener devuelve una vinculación.
func (h *HandlerVinculaciones) Obtener(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	v, err := h.servicio.Obtener(c.Request.Context(), ActorDe(c), id)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, aVinculacionJSON(v))
}

// Rectificar corrige el rol o las horas de una vinculación (RF-04.5, solo administrador).
func (h *HandlerVinculaciones) Rectificar(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	var p peticionRectificacion
	if err := leerJSON(c, &p); err != nil {
		responderError(c, h.log, err)
		return
	}
	v, err := h.servicio.Rectificar(c.Request.Context(), ActorDe(c), id, servicio.SolicitudRectificacion{
		Rol: p.Rol, HorasSemanales: p.HorasSemanales, Motivo: p.Motivo,
	})
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, aVinculacionJSON(v))
}

// ListarRectificaciones devuelve el historial de correcciones de la vinculación.
func (h *HandlerVinculaciones) ListarRectificaciones(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	rectificaciones, err := h.servicio.ListarRectificaciones(c.Request.Context(), ActorDe(c), id)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, convertir(rectificaciones, aRectificacionJSON))
}
