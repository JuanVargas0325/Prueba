package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

type servicioReportesFalso struct {
	semanaRecibida string
	errSolicitar   error
}

func (s *servicioReportesFalso) SolicitarLote(_ context.Context, actor auth.Actor, semana string) (dominio.LoteReportes, error) {
	s.semanaRecibida = semana
	if s.errSolicitar != nil {
		return dominio.LoteReportes{}, s.errSolicitar
	}
	return dominio.LoteReportes{ID: uuidA, ProfesorID: actor.UsuarioID, SemanaInicio: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), Estado: dominio.LotePendiente}, nil
}
func (s *servicioReportesFalso) ListarLotes(context.Context, auth.Actor, dominio.FiltroLotes) ([]dominio.LoteReportes, error) {
	return nil, nil
}
func (s *servicioReportesFalso) ObtenerLote(context.Context, auth.Actor, string) (dominio.LoteReportes, []dominio.ReporteSemanal, error) {
	return dominio.LoteReportes{ID: uuidA}, []dominio.ReporteSemanal{{ID: uuidB, Estado: dominio.ReporteCompletado}}, nil
}
func (s *servicioReportesFalso) ListarReportes(context.Context, auth.Actor, dominio.FiltroReportes) ([]dominio.ReporteSemanal, error) {
	return nil, nil
}
func (s *servicioReportesFalso) ObtenerReporte(context.Context, auth.Actor, string) (dominio.ReporteSemanal, error) {
	return dominio.ReporteSemanal{}, apperr.NoEncontrado("reporte no encontrado")
}
func (s *servicioReportesFalso) AbrirPDF(context.Context, auth.Actor, string) (io.ReadCloser, dominio.ReporteSemanal, error) {
	rep := dominio.ReporteSemanal{ID: uuidB, Persona: "José Peña", SemanaInicio: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), TamanoBytes: 8}
	return io.NopCloser(strings.NewReader("%PDF-1.3")), rep, nil
}

func entornoReportes(t *testing.T) (entorno, *servicioReportesFalso) {
	srv := &servicioReportesFalso{}
	return nuevoEntorno(t, func(d *Dependencias) { d.Reportes = srv }), srv
}

func TestSolicitarLoteResponde202(t *testing.T) {
	e, srv := entornoReportes(t)
	w := e.peticion(http.MethodPost, "/api/v1/reportes/lotes", e.token(t, dominio.RolProfesor), `{"semana":"2026-10-08"}`)
	esperarEstado(t, w, http.StatusAccepted)
	var lote loteJSON
	if err := json.Unmarshal(w.Body.Bytes(), &lote); err != nil {
		t.Fatal(err)
	}
	if lote.ID != uuidA || lote.SemanaInicio != "2026-10-05" || lote.SemanaFin != "2026-10-11" || srv.semanaRecibida != "2026-10-08" {
		t.Errorf("respuesta inesperada: %+v", lote)
	}
	if w.Header().Get("Location") != "/api/v1/reportes/lotes/"+uuidA {
		t.Errorf("Location = %q", w.Header().Get("Location"))
	}
}

func TestSolicitarLoteValidaElCuerpo(t *testing.T) {
	e, _ := entornoReportes(t)
	profesor := e.token(t, dominio.RolProfesor)
	for _, cuerpo := range []string{`{}`, ``, `{"semana":"2026-10-05","otro":1}`, `{"semana":5}`} {
		if w := e.peticion(http.MethodPost, "/api/v1/reportes/lotes", profesor, cuerpo); w.Code != http.StatusBadRequest {
			t.Errorf("cuerpo %q: estado = %d", cuerpo, w.Code)
		}
	}
}

func TestErroresDelServicioSeTraducen(t *testing.T) {
	e, srv := entornoReportes(t)
	profesor := e.token(t, dominio.RolProfesor)
	srv.errSolicitar = apperr.Conflicto("ya hay un lote en proceso")
	w := e.peticion(http.MethodPost, "/api/v1/reportes/lotes", profesor, `{"semana":"2026-10-05"}`)
	esperarEstado(t, w, http.StatusConflict)
	var r RespuestaError
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	if r.Error != "conflicto" || r.Mensaje != "ya hay un lote en proceso" {
		t.Errorf("error inesperado: %+v", r)
	}

	srv.errSolicitar = apperr.ReglaNegocio("regla")
	esperarEstado(t, e.peticion(http.MethodPost, "/api/v1/reportes/lotes", profesor, `{"semana":"2026-10-05"}`), http.StatusUnprocessableEntity)

	srv.errSolicitar = errors.New("detalle interno que no debe filtrarse")
	w = e.peticion(http.MethodPost, "/api/v1/reportes/lotes", profesor, `{"semana":"2026-10-05"}`)
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "detalle interno") {
		t.Errorf("los errores internos no deben exponer detalles: %d %s", w.Code, w.Body.String())
	}
}

func TestIdentificadoresInvalidosResponden400(t *testing.T) {
	e, _ := entornoReportes(t)
	profesor := e.token(t, dominio.RolProfesor)
	for _, ruta := range []string{"/api/v1/reportes/no-es-uuid", "/api/v1/reportes/lotes/123", "/api/v1/reportes?usuario_id=x", "/api/v1/reportes?semana=ayer"} {
		if w := e.peticion(http.MethodGet, ruta, profesor, ""); w.Code != http.StatusBadRequest {
			t.Errorf("%s: estado = %d", ruta, w.Code)
		}
	}
}

func TestObtenerLoteIncluyeURLDeDescarga(t *testing.T) {
	e, _ := entornoReportes(t)
	w := e.peticion(http.MethodGet, "/api/v1/reportes/lotes/"+uuidA, e.token(t, dominio.RolProfesor), "")
	esperarEstado(t, w, http.StatusOK)
	if !strings.Contains(w.Body.String(), "/api/v1/reportes/"+uuidB+"/pdf") {
		t.Errorf("falta la URL de descarga: %s", w.Body.String())
	}
}

func TestDescargarPDF(t *testing.T) {
	e, _ := entornoReportes(t)
	w := e.peticion(http.MethodGet, "/api/v1/reportes/"+uuidB+"/pdf", e.token(t, dominio.RolProfesor), "")
	esperarEstado(t, w, http.StatusOK)
	if w.Header().Get("Content-Type") != "application/pdf" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("cabeceras inesperadas: %v", w.Header())
	}
	if !strings.HasPrefix(w.Header().Get("Content-Disposition"), `attachment; filename="reporte-jose-pena-2026-10-05.pdf"`) {
		t.Errorf("Content-Disposition = %q", w.Header().Get("Content-Disposition"))
	}
	if w.Body.String() != "%PDF-1.3" {
		t.Errorf("cuerpo = %q", w.Body.String())
	}
}
