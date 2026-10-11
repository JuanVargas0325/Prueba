package cola

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/hibiken/asynq"
)

var logPrueba = slog.New(slog.NewTextHandler(io.Discard, nil))

type procesadorFalso struct {
	errReporte    error
	reportes      []string
	fallosReporte []string
	lotes         []string
}

func (p *procesadorFalso) ProcesarLote(_ context.Context, id string) error {
	p.lotes = append(p.lotes, id)
	return nil
}
func (p *procesadorFalso) GenerarReporte(_ context.Context, id string) error {
	p.reportes = append(p.reportes, id)
	return p.errReporte
}
func (p *procesadorFalso) RegistrarFalloLote(context.Context, string, error) error { return nil }
func (p *procesadorFalso) RegistrarFalloReporte(_ context.Context, id string, _ error) error {
	p.fallosReporte = append(p.fallosReporte, id)
	return nil
}

func tarea(tipo, carga string) *asynq.Task { return asynq.NewTask(tipo, []byte(carga)) }

func TestMuxEnrutaCadaTipoDeTarea(t *testing.T) {
	p := &procesadorFalso{}
	mux := NuevoMux(p, func(error) bool { return false }, logPrueba)

	if err := mux.ProcessTask(context.Background(), tarea(TipoProcesarLote, `{"lote_id":"l1"}`)); err != nil {
		t.Fatal(err)
	}
	if err := mux.ProcessTask(context.Background(), tarea(TipoGenerarReporte, `{"reporte_id":"r1"}`)); err != nil {
		t.Fatal(err)
	}
	if len(p.lotes) != 1 || p.lotes[0] != "l1" || len(p.reportes) != 1 || p.reportes[0] != "r1" {
		t.Errorf("enrutamiento inesperado: %+v", p)
	}
}

func TestCargaInvalidaNoSeReintenta(t *testing.T) {
	mux := NuevoMux(&procesadorFalso{}, func(error) bool { return false }, logPrueba)
	err := mux.ProcessTask(context.Background(), tarea(TipoGenerarReporte, `{}`))
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("una carga inválida no debería reintentarse: %v", err)
	}
}

func TestErrorPermanenteRegistraElFalloSinReintentar(t *testing.T) {
	p := &procesadorFalso{errReporte: errors.New("credenciales inválidas")}
	mux := NuevoMux(p, func(error) bool { return true }, logPrueba)

	err := mux.ProcessTask(context.Background(), tarea(TipoGenerarReporte, `{"reporte_id":"r1"}`))
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("un error permanente no debería reintentarse: %v", err)
	}
	if len(p.fallosReporte) != 1 {
		t.Error("el fallo debería registrarse")
	}
}

func TestErrorTransitorioEnElUltimoIntentoRegistraElFallo(t *testing.T) {
	// Fuera del servidor de Asynq no hay metadatos de reintento: se interpreta
	// como el último intento (reintento 0 de máximo 0).
	p := &procesadorFalso{errReporte: errors.New("timeout")}
	mux := NuevoMux(p, func(error) bool { return false }, logPrueba)

	err := mux.ProcessTask(context.Background(), tarea(TipoGenerarReporte, `{"reporte_id":"r1"}`))
	if !errors.Is(err, asynq.SkipRetry) || len(p.fallosReporte) != 1 {
		t.Fatalf("en el último intento el fallo debería registrarse: err=%v fallos=%v", err, p.fallosReporte)
	}
}
