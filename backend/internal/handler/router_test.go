package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Utilidades compartidas por las pruebas de todos los módulos del handler.

var logPrueba = slog.New(slog.NewTextHandler(io.Discard, nil))

// entorno arma un router con las dependencias que cada prueba configure.
type entorno struct {
	router *gin.Engine
}

func nuevoEntorno(t *testing.T, configurar func(*Dependencias)) entorno {
	t.Helper()
	gin.SetMode(gin.TestMode)
	d := Dependencias{
		Salud: map[string]ComprobadorSalud{"postgres": func(context.Context) error { return nil }},
		Log:   logPrueba,
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

func TestRutaInexistenteYMetodoNoPermitido(t *testing.T) {
	e := nuevoEntorno(t, nil)
	esperarEstado(t, e.peticion(http.MethodGet, "/no-existe", ""), http.StatusNotFound)
	esperarEstado(t, e.peticion(http.MethodDelete, "/salud", ""), http.StatusMethodNotAllowed)
}

func TestCORS(t *testing.T) {
	e := nuevoEntorno(t, func(d *Dependencias) { d.OrigenesCORS = []string{"http://localhost:5173"} })
	conOrigen := func(metodo, origen string, cabeceras ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(metodo, "/salud", nil)
		req.Header.Set("Origin", origen)
		for i := 0; i+1 < len(cabeceras); i += 2 {
			req.Header.Set(cabeceras[i], cabeceras[i+1])
		}
		w := httptest.NewRecorder()
		e.router.ServeHTTP(w, req)
		return w
	}

	t.Run("origen permitido", func(t *testing.T) {
		w := conOrigen(http.MethodGet, "http://localhost:5173")
		esperarEstado(t, w, http.StatusOK)
		if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
			t.Errorf("cabeceras = %v", w.Header())
		}
	})
	t.Run("consulta previa", func(t *testing.T) {
		w := conOrigen(http.MethodOptions, "http://localhost:5173", "Access-Control-Request-Method", "POST")
		esperarEstado(t, w, http.StatusNoContent)
		if w.Header().Get("Access-Control-Allow-Headers") == "" {
			t.Errorf("faltan las cabeceras permitidas: %v", w.Header())
		}
	})
	t.Run("origen no permitido", func(t *testing.T) {
		w := conOrigen(http.MethodGet, "https://otro.com")
		esperarEstado(t, w, http.StatusOK)
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("no debería permitir un origen que no está configurado")
		}
	})
}
