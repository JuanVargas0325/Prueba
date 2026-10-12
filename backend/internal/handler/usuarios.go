package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
	"seguimiento/internal/servicio"
)

// ServicioUsuarios es lo que el handler de usuarios necesita.
type ServicioUsuarios interface {
	Crear(ctx context.Context, actor auth.Actor, n servicio.NuevoUsuario) (dominio.Usuario, error)
	Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroUsuarios) ([]dominio.Usuario, error)
	Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.Usuario, error)
	AsignarRoles(ctx context.Context, actor auth.Actor, id string, roles []dominio.Rol) (dominio.Usuario, error)
}

// HandlerUsuarios expone la administración de usuarios (RF-02, RF-15.2).
type HandlerUsuarios struct {
	servicio ServicioUsuarios
	log      *slog.Logger
}

// Registrar declara las rutas del módulo. Todas son del administrador.
func (h *HandlerUsuarios) Registrar(g *gin.RouterGroup) {
	r := g.Group("/usuarios", RequiereRol(dominio.RolAdministrador))
	r.POST("", h.Crear)
	r.GET("", h.Listar)
	r.GET("/:id", h.Obtener)
	r.PUT("/:id/roles", h.AsignarRoles)
}

type peticionUsuario struct {
	Nombre   string        `json:"nombre"`
	Correo   string        `json:"correo"`
	Password string        `json:"password"`
	Roles    []dominio.Rol `json:"roles"`
}

type peticionRoles struct {
	Roles []dominio.Rol `json:"roles"`
}

// Crear registra un usuario (RF-02.1).
func (h *HandlerUsuarios) Crear(c *gin.Context) {
	var p peticionUsuario
	if err := leerJSON(c, &p); err != nil {
		responderError(c, h.log, err)
		return
	}
	u, err := h.servicio.Crear(c.Request.Context(), ActorDe(c), servicio.NuevoUsuario{
		Nombre: p.Nombre, Correo: p.Correo, Password: p.Password, Roles: p.Roles,
	})
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.Header("Location", "/api/v1/usuarios/"+u.ID)
	c.JSON(http.StatusCreated, aUsuarioJSON(u))
}

// Listar devuelve los usuarios (RF-02.3), con filtros opcionales rol y busqueda.
func (h *HandlerUsuarios) Listar(c *gin.Context) {
	usuarios, err := h.servicio.Listar(c.Request.Context(), ActorDe(c), dominio.FiltroUsuarios{
		Rol:        dominio.Rol(c.Query("rol")),
		Busqueda:   c.Query("busqueda"),
		Paginacion: consultaPaginacion(c),
	})
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, convertir(usuarios, aUsuarioJSON))
}

// Obtener devuelve un usuario.
func (h *HandlerUsuarios) Obtener(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	u, err := h.servicio.Obtener(c.Request.Context(), ActorDe(c), id)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, aUsuarioJSON(u))
}

// AsignarRoles reemplaza los roles del usuario (RF-02.2).
func (h *HandlerUsuarios) AsignarRoles(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	var p peticionRoles
	if err := leerJSON(c, &p); err != nil {
		responderError(c, h.log, err)
		return
	}
	u, err := h.servicio.AsignarRoles(c.Request.Context(), ActorDe(c), id, p.Roles)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, aUsuarioJSON(u))
}
