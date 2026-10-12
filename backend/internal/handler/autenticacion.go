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

// ServicioAutenticacion es lo que el handler de autenticación necesita.
type ServicioAutenticacion interface {
	IniciarSesion(ctx context.Context, correo, password string) (servicio.Sesion, error)
	UsuarioActual(ctx context.Context, actor auth.Actor) (dominio.Usuario, error)
}

// HandlerAutenticacion expone el inicio de sesión (RF-01.1).
type HandlerAutenticacion struct {
	servicio ServicioAutenticacion
	log      *slog.Logger
}

type peticionLogin struct {
	Correo   string `json:"correo"`
	Password string `json:"password"`
}

type respuestaLogin struct {
	Token    string      `json:"token"`
	ExpiraEn time.Time   `json:"expira_en"`
	Usuario  usuarioJSON `json:"usuario"`
}

// RegistrarPublicas declara las rutas que no requieren token.
func (h *HandlerAutenticacion) RegistrarPublicas(g *gin.RouterGroup) {
	g.POST("/auth/login", h.Login)
}

// Registrar declara las rutas protegidas.
func (h *HandlerAutenticacion) Registrar(g *gin.RouterGroup) {
	g.GET("/auth/yo", h.Yo)
}

// Login valida las credenciales y devuelve un token JWT.
func (h *HandlerAutenticacion) Login(c *gin.Context) {
	var p peticionLogin
	if err := leerJSON(c, &p); err != nil {
		responderError(c, h.log, err)
		return
	}
	sesion, err := h.servicio.IniciarSesion(c.Request.Context(), p.Correo, p.Password)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, respuestaLogin{Token: sesion.Token, ExpiraEn: sesion.ExpiraEn, Usuario: aUsuarioJSON(sesion.Usuario)})
}

// Yo devuelve el usuario autenticado.
func (h *HandlerAutenticacion) Yo(c *gin.Context) {
	u, err := h.servicio.UsuarioActual(c.Request.Context(), ActorDe(c))
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, aUsuarioJSON(u))
}
