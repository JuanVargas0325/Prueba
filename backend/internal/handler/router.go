// Package handler es la capa de presentación: expone la API REST.
package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ComprobadorSalud verifica una dependencia externa (base de datos, Redis).
type ComprobadorSalud func(ctx context.Context) error

// Dependencias agrupa lo que la API necesita. En los pasos siguientes se le
// agregan los servicios de cada módulo.
type Dependencias struct {
	Salud map[string]ComprobadorSalud
}

// NuevoRouter construye el router HTTP. Por ahora solo expone /salud.
func NuevoRouter(d Dependencias) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/salud", salud(d.Salud))
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
