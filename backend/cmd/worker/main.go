// Comando worker: procesa en segundo plano la generación de reportes con IA
// (tareas de Asynq sobre Redis).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	_ "time/tzdata" // incluye las zonas horarias en el binario

	"github.com/hibiken/asynq"

	"seguimiento/internal/almacenamiento"
	"seguimiento/internal/basedatos"
	"seguimiento/internal/cola"
	"seguimiento/internal/config"
	"seguimiento/internal/ia"
	"seguimiento/internal/pdf"
	"seguimiento/internal/repositorio"
	"seguimiento/internal/servicio"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("servicio", "worker")
	if err := ejecutar(log); err != nil {
		log.Error("el worker terminó con error", "error", err)
		os.Exit(1)
	}
}

func ejecutar(log *slog.Logger) error {
	cfg, err := config.Cargar()
	if err != nil {
		return err
	}

	ctx, detener := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer detener()

	db, err := basedatos.Conectar(ctx, cfg.BaseDatos, log)
	if err != nil {
		return err
	}
	defer db.Close()

	almacen, err := almacenamiento.Nuevo(cfg.Almacenamiento)
	if err != nil {
		return err
	}

	resumidor, err := ia.Nuevo(cfg.LLM)
	if err != nil {
		return err
	}
	log.Info("proveedor de IA configurado", "proveedor", resumidor.Proveedor(), "modelo", resumidor.Modelo())

	redis := asynq.RedisClientOpt{Addr: cfg.Redis.Direccion, Password: cfg.Redis.Password, DB: cfg.Redis.DB}
	clienteCola := cola.NuevoCliente(redis, cola.Opciones{MaxReintentos: cfg.Worker.MaxReintentos, TimeoutReporte: cfg.Worker.TimeoutReporte})
	defer func() {
		if err := clienteCola.Cerrar(); err != nil {
			log.Warn("cerrando el cliente de la cola", "error", err)
		}
	}()

	generador := servicio.NuevoGeneradorReportes(
		repositorio.NuevoReportes(db),
		clienteCola,
		almacen,
		resumidor,
		pdf.NuevoGenerador(cfg.ZonaHoraria),
		servicio.NuevoReloj(cfg.ZonaHoraria),
		log,
	)

	servidor := asynq.NewServer(redis, asynq.Config{
		Concurrency: cfg.Worker.Concurrencia,
		Queues:      map[string]int{cola.ColaReportes: 1},
		Logger:      nuevoLoggerAsynq(log),
	})
	if err := servidor.Start(cola.NuevoMux(generador, servicio.EsErrorPermanente, log)); err != nil {
		return err
	}
	log.Info("worker iniciado", "concurrencia", cfg.Worker.Concurrencia, "max_reintentos", cfg.Worker.MaxReintentos)

	<-ctx.Done()
	log.Info("deteniendo el worker")
	servidor.Shutdown()
	return nil
}

// loggerAsynq adapta slog a la interfaz de logs de Asynq.
type loggerAsynq struct{ log *slog.Logger }

func nuevoLoggerAsynq(log *slog.Logger) loggerAsynq {
	return loggerAsynq{log: log.With("componente", "asynq")}
}

func (l loggerAsynq) Debug(args ...any) { l.log.Debug(fmt.Sprint(args...)) }
func (l loggerAsynq) Info(args ...any)  { l.log.Info(fmt.Sprint(args...)) }
func (l loggerAsynq) Warn(args ...any)  { l.log.Warn(fmt.Sprint(args...)) }
func (l loggerAsynq) Error(args ...any) { l.log.Error(fmt.Sprint(args...)) }
func (l loggerAsynq) Fatal(args ...any) {
	l.log.Error(fmt.Sprint(args...))
	os.Exit(1)
}
