package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ComprobadorSalud verifica una dependencia externa (base de datos, Redis).
type ComprobadorSalud func(ctx context.Context) error

// Dependencias agrupa los servicios que la API expone. Cada módulo que se
// construya (usuarios, períodos, espacios...) agrega aquí su servicio.
type Dependencias struct {
	Autenticacion ServicioAutenticacion

	Tokens ValidadorTokens
	Salud  map[string]ComprobadorSalud
	// OrigenesCORS son los orígenes de los frontends que pueden usar la API.
	OrigenesCORS []string
	Log          *slog.Logger
}

// modulo es un grupo de endpoints que declara sus propias rutas.
type modulo interface {
	Registrar(g *gin.RouterGroup)
}

// NuevoRouter construye el router HTTP con todas las rutas de la API.
// Para agregar un módulo: crear su handler con un método Registrar y añadirlo
// a la lista de módulos.
func NuevoRouter(d Dependencias) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), RegistrarPeticiones(d.Log), CORS(d.OrigenesCORS))
	// Detrás de Nginx la IP del cliente llega en X-Forwarded-For.
	_ = r.SetTrustedProxies([]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.1"})
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, RespuestaError{Error: "no_encontrado", Mensaje: "ruta no encontrada"})
	})
	r.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, RespuestaError{Error: "metodo_no_permitido", Mensaje: "método no permitido"})
	})
	r.HandleMethodNotAllowed = true

	r.GET("/salud", salud(d.Salud))

	api := r.Group("/api/v1")
	autenticacion := &HandlerAutenticacion{servicio: d.Autenticacion, log: d.Log}
	autenticacion.RegistrarPublicas(api)

	protegidas := api.Group("", Autenticar(d.Tokens))
	modulos := []modulo{
		autenticacion,
	}
	for _, m := range modulos {
		m.Registrar(protegidas)
	}
	return r
}

type estadoSalud struct {
	Estado      string            `json:"estado"`
	Componentes map[string]string `json:"componentes"`
}

// salud responde 200 si todas las dependencias están disponibles y 503 si no.
func salud(comprobadores map[string]ComprobadorSalud) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancelar := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancelar()
		respuesta := estadoSalud{Estado: "ok", Componentes: map[string]string{}}
		codigo := http.StatusOK
		for nombre, comprobar := range comprobadores {
			if err := comprobar(ctx); err != nil {
				respuesta.Componentes[nombre] = "error"
				respuesta.Estado = "degradado"
				codigo = http.StatusServiceUnavailable
				continue
			}
			respuesta.Componentes[nombre] = "ok"
		}
		c.JSON(codigo, respuesta)
	}
}
