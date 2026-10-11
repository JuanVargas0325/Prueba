package ia

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"seguimiento/internal/dominio"
)

func datosPrueba() dominio.DatosSemanaPersona {
	return dominio.DatosSemanaPersona{
		Persona:      "Ana Pérez",
		Correo:       "ana@uniandes.edu.co",
		SemanaInicio: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		Vinculaciones: []dominio.VinculacionSemana{
			{
				EspacioNombre:    "Desarrollo en la nube",
				EspacioTipo:      dominio.EspacioCurso,
				Rol:              dominio.RolMonitor,
				HorasContratadas: 6,
				Tareas: []dominio.TareaSemana{
					{Titulo: "Calificar taller 2", Descripcion: "Revisión de\n30 entregas", Estado: dominio.TareaFinalizada, Horas: 4},
					{Titulo: "Preparar laboratorio", Estado: dominio.TareaEnDesarrollo, Horas: 2, Observaciones: "Falta la guía", Tardia: true},
				},
			},
			{EspacioNombre: "Proyecto sin tareas", EspacioTipo: dominio.EspacioProyecto, Rol: dominio.RolAsistenteGraduado},
		},
	}
}

func TestMensajeUsuarioIncluyeSoloInsumosPermitidos(t *testing.T) {
	m := ConstruirMensajeUsuario(datosPrueba())

	for _, esperado := range []string{
		"2026-10-05", "2026-10-11", "Total de tareas: 2", "Total de horas reportadas: 6",
		"Calificar taller 2", "Revisión de 30 entregas", "en desarrollo", "Reporte tardío: sí",
		"Falta la guía", "<datos_semana>", "</datos_semana>",
	} {
		if !strings.Contains(m, esperado) {
			t.Errorf("el mensaje debería contener %q:\n%s", esperado, m)
		}
	}
	// Datos personales y horas contratadas no se envían al modelo.
	for _, prohibido := range []string{"Ana Pérez", "ana@uniandes.edu.co", "Proyecto sin tareas"} {
		if strings.Contains(m, prohibido) {
			t.Errorf("el mensaje no debería contener %q", prohibido)
		}
	}
}

func TestSimuladoEsDeterministico(t *testing.T) {
	s := NuevoSimulado()
	r1, err := s.Resumir(context.Background(), datosPrueba())
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := s.Resumir(context.Background(), datosPrueba())
	if r1 != r2 || !strings.Contains(r1, "2 tareas") || !strings.Contains(r1, "6 horas") {
		t.Fatalf("resumen simulado inesperado: %q", r1)
	}
}

func TestOpenAIEnviaPeticionYLeeRespuesta(t *testing.T) {
	var recibida peticionChat
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer clave-prueba" {
			t.Errorf("cabecera Authorization inesperada: %q", r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&recibida)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"  Resumen de prueba.  "}}]}`))
	}))
	defer srv.Close()

	o, err := NuevoOpenAI(Config{URLBase: srv.URL + "/v1/", APIKey: "clave-prueba", Modelo: "modelo-x", Timeout: 5 * time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	resumen, err := o.Resumir(context.Background(), datosPrueba())
	if err != nil {
		t.Fatal(err)
	}
	if resumen != "Resumen de prueba." {
		t.Errorf("resumen = %q", resumen)
	}
	if recibida.Model != "modelo-x" || len(recibida.Messages) != 2 || recibida.Messages[0].Role != "system" {
		t.Errorf("petición inesperada: %+v", recibida)
	}
	if recibida.Temperature != nil || recibida.MaxTokens != 0 {
		t.Error("temperatura y max_tokens deberían omitirse si no se configuran")
	}
}

func TestOpenAIClasificaErrores(t *testing.T) {
	casos := []struct {
		estado       int
		reintentable bool
	}{
		{http.StatusUnauthorized, false},
		{http.StatusBadRequest, false},
		{http.StatusTooManyRequests, true},
		{http.StatusServiceUnavailable, true},
	}
	for _, c := range casos {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(c.estado)
			_, _ = w.Write([]byte(`{"error":{"message":"detalle del error"}}`))
		}))
		o, _ := NuevoOpenAI(Config{URLBase: srv.URL, Modelo: "m", Timeout: 5 * time.Second}, nil)
		_, err := o.Resumir(context.Background(), datosPrueba())
		srv.Close()

		var ep *ErrorProveedor
		if !errors.As(err, &ep) {
			t.Fatalf("estado %d: se esperaba ErrorProveedor, se obtuvo %v", c.estado, err)
		}
		if ep.Estado != c.estado || ep.Mensaje != "detalle del error" || EsReintentable(err) != c.reintentable {
			t.Errorf("estado %d: error inesperado %+v", c.estado, ep)
		}
	}
}

func TestOpenAIRespuestaVaciaEsReintentable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"   "}}]}`))
	}))
	defer srv.Close()
	o, _ := NuevoOpenAI(Config{URLBase: srv.URL, Modelo: "m", Timeout: 5 * time.Second}, nil)
	_, err := o.Resumir(context.Background(), datosPrueba())
	if err == nil || !EsReintentable(err) {
		t.Fatalf("se esperaba un error reintentable, se obtuvo %v", err)
	}
}

func TestAnthropicEnviaPeticionYLeeRespuesta(t *testing.T) {
	var cuerpo map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("ruta inesperada: %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "clave-prueba" {
			t.Errorf("cabecera x-api-key inesperada")
		}
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5",
			"content":[{"type":"text","text":"Resumen de Claude."}],
			"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`))
	}))
	defer srv.Close()

	a, err := NuevoAnthropic(Config{URLBase: srv.URL, APIKey: "clave-prueba", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if a.Modelo() != ModeloAnthropicPorDefecto {
		t.Errorf("modelo por defecto = %s", a.Modelo())
	}
	resumen, err := a.Resumir(context.Background(), datosPrueba())
	if err != nil {
		t.Fatal(err)
	}
	if resumen != "Resumen de Claude." {
		t.Errorf("resumen = %q", resumen)
	}
	if cuerpo["model"] != ModeloAnthropicPorDefecto || cuerpo["system"] == nil {
		t.Errorf("petición inesperada: %v", cuerpo)
	}
}

func TestAnthropicErrorDeAutenticacionNoEsReintentable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	defer srv.Close()

	a, _ := NuevoAnthropic(Config{URLBase: srv.URL, APIKey: "mala", Timeout: 5 * time.Second})
	_, err := a.Resumir(context.Background(), datosPrueba())
	if err == nil || EsReintentable(err) || !EsErrorDeIA(err) {
		t.Fatalf("se esperaba un error de IA no reintentable, se obtuvo %v", err)
	}
}

func TestNuevoValidaConfiguracion(t *testing.T) {
	if r, err := Nuevo(Config{}); err != nil || r.Proveedor() != ProveedorSimulado {
		t.Errorf("sin proveedor debería usarse el simulado: %v", err)
	}
	if _, err := Nuevo(Config{Proveedor: "openai"}); err == nil {
		t.Error("openai sin modelo debería fallar")
	}
	if _, err := Nuevo(Config{Proveedor: "anthropic"}); err == nil {
		t.Error("anthropic sin API key debería fallar")
	}
	if _, err := Nuevo(Config{Proveedor: "otro"}); err == nil {
		t.Error("un proveedor desconocido debería fallar")
	}
	r, err := Nuevo(Config{Proveedor: "OpenAI", Modelo: "gemini-x"})
	if err != nil || r.Modelo() != "gemini-x" {
		t.Errorf("openai con modelo debería crearse: %v", err)
	}
}
