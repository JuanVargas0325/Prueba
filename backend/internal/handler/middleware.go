package handler

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

const claveActor = "actor"

// ValidadorTokens valida tokens de acceso.
type ValidadorTokens interface {
	Validar(token string) (auth.Actor, error)
}

// Autenticar exige un token Bearer válido y deja el actor en el contexto.
func Autenticar(tokens ValidadorTokens) gin.HandlerFunc {
	return func(c *gin.Context) {
		cabecera := c.GetHeader("Authorization")
		token, ok := strings.CutPrefix(cabecera, "Bearer ")
		if !ok || strings.TrimSpace(token) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, RespuestaError{Error: "no_autenticado", Mensaje: "se requiere un token de acceso"})
			return
		}
		actor, err := tokens.Validar(strings.TrimSpace(token))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, RespuestaError{Error: "no_autenticado", Mensaje: "token inválido o expirado"})
			return
		}
		c.Set(claveActor, actor)
		c.Next()
	}
}

// RequiereRol permite continuar solo si el actor tiene alguno de los roles (RF-01.2).
func RequiereRol(roles ...dominio.Rol) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !ActorDe(c).TieneAlgunRol(roles...) {
			c.AbortWithStatusJSON(http.StatusForbidden, RespuestaError{Error: "prohibido", Mensaje: "no tiene permiso para esta operación"})
			return
		}
		c.Next()
	}
}

// ActorDe devuelve el actor autenticado de la petición.
func ActorDe(c *gin.Context) auth.Actor {
	if v, ok := c.Get(claveActor); ok {
		if actor, ok := v.(auth.Actor); ok {
			return actor
		}
	}
	return auth.Actor{}
}

// RegistrarPeticiones escribe una línea de log por petición.
func RegistrarPeticiones(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		inicio := time.Now()
		c.Next()
		if c.Request.URL.Path == "/salud" {
			return
		}
		log.Info("petición",
			"metodo", c.Request.Method,
			"ruta", c.Request.URL.Path,
			"estado", c.Writer.Status(),
			"duracion_ms", time.Since(inicio).Milliseconds(),
			"ip", c.ClientIP(),
		)
	}
}

// CORS permite que un frontend servido desde otro origen (otro dominio o
// puerto, por ejemplo un bucket o http://localhost:5173) llame a la API desde
// el navegador. Solo se aceptan los orígenes configurados; "*" acepta cualquiera.
// Las peticiones sin cabecera Origin (Postman, curl, otro servidor) no cambian.
func CORS(origenes []string) gin.HandlerFunc {
	permitidos := map[string]bool{}
	for _, o := range origenes {
		permitidos[o] = true
	}
	return func(c *gin.Context) {
		origen := c.GetHeader("Origin")
		if origen == "" || (!permitidos["*"] && !permitidos[origen]) {
			c.Next()
			return
		}
		cabeceras := c.Writer.Header()
		cabeceras.Set("Access-Control-Allow-Origin", origen)
		cabeceras.Add("Vary", "Origin")
		// Consulta previa (preflight): el navegador pregunta si puede enviar la petición real.
		if c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != "" {
			cabeceras.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
			cabeceras.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			cabeceras.Set("Access-Control-Max-Age", "600")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		// Permite que el frontend lea el nombre de los archivos descargados.
		cabeceras.Set("Access-Control-Expose-Headers", "Content-Disposition, Location")
		c.Next()
	}
}
