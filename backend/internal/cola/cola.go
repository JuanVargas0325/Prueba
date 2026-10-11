// Package cola integra Asynq (sobre Redis) para el procesamiento asíncrono de
// la generación de reportes. La API encola trabajos y el worker los ejecuta.
package cola

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
)

// Tipos de tarea y cola usados por el módulo de reportes.
const (
	TipoProcesarLote   = "reportes:procesar_lote"
	TipoGenerarReporte = "reportes:generar_reporte"
	ColaReportes       = "reportes"
)

// Opciones controla los reintentos y tiempos máximos de las tareas.
type Opciones struct {
	MaxReintentos  int
	TimeoutReporte time.Duration
}

type cargaLote struct {
	LoteID string `json:"lote_id"`
}

type cargaReporte struct {
	ReporteID string `json:"reporte_id"`
}

// Cliente encola trabajos. Implementa servicio.Encolador.
type Cliente struct {
	asynq *asynq.Client
	op    Opciones
}

// NuevoCliente crea el cliente de la cola.
func NuevoCliente(redis asynq.RedisClientOpt, op Opciones) *Cliente {
	return &Cliente{asynq: asynq.NewClient(redis), op: op}
}

// Cerrar libera la conexión con Redis.
func (c *Cliente) Cerrar() error { return c.asynq.Close() }

// Ping verifica la conexión con Redis (se usa en /salud).
func (c *Cliente) Ping() error { return c.asynq.Ping() }

// EncolarLote publica el trabajo que reparte un lote en reportes individuales.
func (c *Cliente) EncolarLote(ctx context.Context, loteID string) error {
	return c.encolar(ctx, TipoProcesarLote, cargaLote{LoteID: loteID}, "lote:"+loteID, time.Minute)
}

// EncolarReporte publica el trabajo que genera el PDF de una persona.
func (c *Cliente) EncolarReporte(ctx context.Context, reporteID string) error {
	return c.encolar(ctx, TipoGenerarReporte, cargaReporte{ReporteID: reporteID}, "reporte:"+reporteID, c.op.TimeoutReporte)
}

func (c *Cliente) encolar(ctx context.Context, tipo string, carga any, id string, timeout time.Duration) error {
	datos, err := json.Marshal(carga)
	if err != nil {
		return err
	}
	_, err = c.asynq.EnqueueContext(ctx, asynq.NewTask(tipo, datos),
		asynq.Queue(ColaReportes),
		asynq.MaxRetry(c.op.MaxReintentos),
		asynq.Timeout(timeout),
		// El identificador evita encolar dos veces el mismo trabajo si una tarea se reintenta.
		asynq.TaskID(id),
		asynq.Retention(24*time.Hour),
	)
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil // ya estaba en la cola
	}
	if err != nil {
		return fmt.Errorf("encolando %s: %w", tipo, err)
	}
	return nil
}

// Procesador es lo que el worker ejecuta para cada tipo de tarea.
// Lo implementa servicio.GeneradorReportes.
type Procesador interface {
	ProcesarLote(ctx context.Context, loteID string) error
	GenerarReporte(ctx context.Context, reporteID string) error
	RegistrarFalloLote(ctx context.Context, loteID string, causa error) error
	RegistrarFalloReporte(ctx context.Context, reporteID string, causa error) error
}

// NuevoMux registra los manejadores de cada tipo de tarea.
// esPermanente decide si un error no debe reintentarse.
func NuevoMux(p Procesador, esPermanente func(error) bool, log *slog.Logger) *asynq.ServeMux {
	mux := asynq.NewServeMux()

	mux.HandleFunc(TipoProcesarLote, func(ctx context.Context, t *asynq.Task) error {
		var carga cargaLote
		if err := json.Unmarshal(t.Payload(), &carga); err != nil || carga.LoteID == "" {
			return fmt.Errorf("carga inválida: %w", asynq.SkipRetry)
		}
		return ejecutar(ctx, log, "lote_id", carga.LoteID, esPermanente,
			func(ctx context.Context) error { return p.ProcesarLote(ctx, carga.LoteID) },
			func(ctx context.Context, err error) error { return p.RegistrarFalloLote(ctx, carga.LoteID, err) })
	})

	mux.HandleFunc(TipoGenerarReporte, func(ctx context.Context, t *asynq.Task) error {
		var carga cargaReporte
		if err := json.Unmarshal(t.Payload(), &carga); err != nil || carga.ReporteID == "" {
			return fmt.Errorf("carga inválida: %w", asynq.SkipRetry)
		}
		return ejecutar(ctx, log, "reporte_id", carga.ReporteID, esPermanente,
			func(ctx context.Context) error { return p.GenerarReporte(ctx, carga.ReporteID) },
			func(ctx context.Context, err error) error { return p.RegistrarFalloReporte(ctx, carga.ReporteID, err) })
	})

	return mux
}

// ejecutar corre el trabajo y decide qué hacer con un error: si es permanente
// o es el último intento, registra el fallo y evita más reintentos; si no,
// devuelve el error para que Asynq reintente con espera exponencial.
func ejecutar(ctx context.Context, log *slog.Logger, campo, id string, esPermanente func(error) bool,
	trabajo func(context.Context) error, registrarFallo func(context.Context, error) error) error {

	err := trabajo(ctx)
	if err == nil {
		return nil
	}

	reintento, _ := asynq.GetRetryCount(ctx)
	maximo, _ := asynq.GetMaxRetry(ctx)
	if !esPermanente(err) && reintento < maximo {
		log.Warn("trabajo fallido, se reintentará", campo, id, "intento", reintento+1, "error", err)
		return err
	}

	log.Error("trabajo fallido definitivamente", campo, id, "intento", reintento+1, "error", err)
	// Se usa un contexto nuevo: el del trabajo puede haber expirado.
	ctxFallo, cancelar := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancelar()
	if errFallo := registrarFallo(ctxFallo, err); errFallo != nil {
		log.Error("no fue posible registrar el fallo", campo, id, "error", errFallo)
	}
	return fmt.Errorf("%w: %w", err, asynq.SkipRetry)
}
