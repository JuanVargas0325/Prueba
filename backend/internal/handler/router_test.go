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
	"time"

	"github.com/gin-gonic/gin"

	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

// Utilidades compartidas por las pruebas de todos los módulos del handler.

const secretoPrueba = "secreto-de-prueba-con-mas-de-32-caracteres"

var logPrueba = slog.New(slog.NewTextHandler(io.Discard, nil))

// entorno arma un router con los servicios falsos que cada prueba configure.
type entorno struct {
	router *gin.Engine
	tokens *auth.GestorTokens
}

func nuevoEntorno(t *testing.T, configurar func(*Dependencias)) entorno {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tokens := auth.NuevoGestorTokens(secretoPrueba, time.Hour)
	d := Dependencias{
		Tokens:              tokens,
		Salud:               map[string]ComprobadorSalud{"postgres": func(context.Context) error { return nil }},
		MaxCuerpoArchivosMB: 1,
		Log:                 logPrueba,
	}
	if configurar != nil {
		configurar(&d)
	}
	return entorno{router: NuevoRouter(d), tokens: tokens}
}

// token emite un token para el usuario "usuario-1" con los roles dados.
func (e entorno) token(t *testing.T, roles ...dominio.Rol) string {
	t.Helper()
	token, _, err := e.tokens.Generar(dominio.Usuario{ID: "usuario-1", Roles: roles})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func (e entorno) peticion(metodo, ruta, token, cuerpo string) *httptest.ResponseRecorder {
	return e.peticionCon(metodo, ruta, token, "application/json", bytes.NewBufferString(cuerpo))
}

func (e entorno) peticionCon(metodo, ruta, token, tipo string, cuerpo io.Reader) *httptest.ResponseRecorder {
	req := httptest.NewRequest(metodo, ruta, cuerpo)
	req.Header.Set("Content-Type", tipo)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
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

const (
	uuidA = "6f1c2a8e-1a7d-4b8e-9c55-0d6c5c1f7a01"
	uuidB = "0b8f3e52-6c1a-4f0e-8a3b-2a9f1d7c4e02"
)

// --- router -----------------------------------------------------------------------

func TestSaludResponde200(t *testing.T) {
	e := nuevoEntorno(t, nil)
	esperarEstado(t, e.peticion(http.MethodGet, "/salud", "", ""), http.StatusOK)
}

func TestSaludResponde503SiUnaDependenciaFalla(t *testing.T) {
	e := nuevoEntorno(t, func(d *Dependencias) {
		d.Salud = map[string]ComprobadorSalud{"redis": func(context.Context) error { return errors.New("caído") }}
	})
	esperarEstado(t, e.peticion(http.MethodGet, "/salud", "", ""), http.StatusServiceUnavailable)
}

func TestRutasProtegidasExigenToken(t *testing.T) {
	e := nuevoEntorno(t, nil)
	for _, ruta := range []string{"/api/v1/tareas", "/api/v1/usuarios", "/api/v1/auth/yo", "/api/v1/seguimiento/personas"} {
		if w := e.peticion(http.MethodGet, ruta, "", ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s sin token: estado = %d", ruta, w.Code)
		}
	}
	esperarEstado(t, e.peticion(http.MethodGet, "/api/v1/tareas", "token-invalido", ""), http.StatusUnauthorized)
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

func TestRutaInexistenteYMetodoNoPermitido(t *testing.T) {
	e := nuevoEntorno(t, nil)
	esperarEstado(t, e.peticion(http.MethodGet, "/no-existe", "", ""), http.StatusNotFound)
	esperarEstado(t, e.peticion(http.MethodDelete, "/salud", "", ""), http.StatusMethodNotAllowed)
}

func TestControlDeAccesoPorRol(t *testing.T) {
	e := nuevoEntorno(t, nil)
	monitor := e.token(t, dominio.RolMonitor)
	profesor := e.token(t, dominio.RolProfesor)
	casos := []struct {
		metodo, ruta, token string
	}{
		{http.MethodGet, "/api/v1/usuarios", monitor},                // solo administrador
		{http.MethodPost, "/api/v1/periodos", profesor},              // solo administrador
		{http.MethodPost, "/api/v1/espacios", monitor},               // solo profesor
		{http.MethodPost, "/api/v1/vinculaciones", monitor},          // solo profesor
		{http.MethodPut, "/api/v1/vinculaciones/" + uuidA, profesor}, // solo administrador
		{http.MethodPost, "/api/v1/tareas", profesor},                // solo operativos
		{http.MethodGet, "/api/v1/seguimiento/personas", monitor},    // profesor o administrador
	}
	for _, c := range casos {
		if w := e.peticion(c.metodo, c.ruta, c.token, `{}`); w.Code != http.StatusForbidden {
			t.Errorf("%s %s: estado = %d, se esperaba 403", c.metodo, c.ruta, w.Code)
		}
	}
}
