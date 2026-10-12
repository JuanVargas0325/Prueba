package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Utilidades compartidas por las pruebas de todos los módulos del handler.

// entorno arma un router con las dependencias que cada prueba configure.
type entorno struct {
	router *gin.Engine
}

func nuevoEntorno(t *testing.T, configurar func(*Dependencias)) entorno {
	t.Helper()
	gin.SetMode(gin.TestMode)
	d := Dependencias{
		Salud: map[string]ComprobadorSalud{"postgres": func(context.Context) error { return nil }},
	}
	if configurar != nil {
		configurar(&d)
	}
	return entorno{router: NuevoRouter(d)}
}

func (e entorno) peticion(metodo, ruta, cuerpo string) *httptest.ResponseRecorder {
	return e.peticionCon(metodo, ruta, "application/json", bytes.NewBufferString(cuerpo))
}

func (e entorno) peticionCon(metodo, ruta, tipo string, cuerpo io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(metodo, ruta, cuerpo)
	req.Header.Set("Content-Type", tipo)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func esperarEstado(t *testing.T, w *httptest.ResponseRecorder, estado int) {
	t.Helper()
	if w.Code != estado {
		t.Fatalf("estado = %d, se esperaba %d; cuerpo: %s", w.Code, estado, w.Body.String())
	}
}

// --- router -----------------------------------------------------------------------

func TestSaludResponde200(t *testing.T) {
	e := nuevoEntorno(t, nil)
	esperarEstado(t, e.peticion(http.MethodGet, "/salud", ""), http.StatusOK)
}

func TestSaludResponde503SiUnaDependenciaFalla(t *testing.T) {
	e := nuevoEntorno(t, func(d *Dependencias) {
		d.Salud = map[string]ComprobadorSalud{"redis": func(context.Context) error { return errors.New("caído") }}
	})
	esperarEstado(t, e.peticion(http.MethodGet, "/salud", ""), http.StatusServiceUnavailable)
}
