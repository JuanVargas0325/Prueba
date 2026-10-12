// Comando api: servidor HTTP de la plataforma de seguimiento.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // incluye las zonas horarias en el binario

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"

	"seguimiento/internal/almacenamiento"
	"seguimiento/internal/auth"
	"seguimiento/internal/basedatos"
	"seguimiento/internal/cola"
	"seguimiento/internal/config"
	"seguimiento/internal/dominio"
	"seguimiento/internal/handler"
	"seguimiento/internal/repositorio"
	"seguimiento/internal/servicio"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("servicio", "api")
	if err := ejecutar(log); err != nil {
		log.Error("la API terminó con error", "error", err)
		os.Exit(1)
	}
}

func ejecutar(log *slog.Logger) error {
	cfg, err := config.Cargar()
	if err != nil {
		return err
	}
	if !cfg.EsDesarrollo() {
		gin.SetMode(gin.ReleaseMode)
	}

	ctx, detener := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer detener()

	// Capa de datos
	db, err := basedatos.Conectar(ctx, cfg.BaseDatos, log)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := basedatos.Migrar(ctx, db, log); err != nil {
		return err
	}
	var (
		usuarios      = repositorio.NuevoUsuarios(db)
		periodos      = repositorio.NuevoPeriodos(db)
		espacios      = repositorio.NuevoEspacios(db)
		vinculaciones = repositorio.NuevoVinculaciones(db)
		tareas        = repositorio.NuevoTareas(db)
		reportes      = repositorio.NuevoReportes(db)
	)

	// Infraestructura
	almacen, err := almacenamiento.Nuevo(cfg.Almacenamiento)
	if err != nil {
		return err
	}
	clienteCola := cola.NuevoCliente(
		asynq.RedisClientOpt{Addr: cfg.Redis.Direccion, Password: cfg.Redis.Password, DB: cfg.Redis.DB},
		cola.Opciones{MaxReintentos: cfg.Worker.MaxReintentos, TimeoutReporte: cfg.Worker.TimeoutReporte},
	)
	defer func() {
		if err := clienteCola.Cerrar(); err != nil {
			log.Warn("cerrando el cliente de la cola", "error", err)
		}
	}()

	// Capa de servicio
	reloj := servicio.NuevoReloj(cfg.ZonaHoraria)
	tokens := auth.NuevoGestorTokens(cfg.JWT.Secreto, cfg.JWT.Expiracion)
	servicioAuth := servicio.NuevaAutenticacion(usuarios, tokens, log)
	if err := servicioAuth.AsegurarAdministrador(ctx, cfg.Admin.Nombre, cfg.Admin.Correo, cfg.Admin.Password); err != nil {
		return err
	}
	maxBytesAdjunto := int64(cfg.MaxAdjuntoMB) << 20

	// Capa de presentación
	router := handler.NuevoRouter(handler.Dependencias{
		Autenticacion: servicioAuth,
		Usuarios:      servicio.NuevoUsuarios(usuarios, log),
		Periodos:      servicio.NuevoPeriodos(periodos, log),
		Espacios:      servicio.NuevoEspacios(espacios, periodos, reloj, log),
		Vinculaciones: servicio.NuevoVinculaciones(vinculaciones, usuarios, espacios, log),
		Tareas:        servicio.NuevoTareas(tareas, vinculaciones, almacen, reloj, maxBytesAdjunto, log),
		Seguimiento:   servicio.NuevoSeguimiento(vinculaciones, tareas, usuarios, reloj),
		Reportes:      servicio.NuevoReportes(reportes, clienteCola, almacen, reloj, log),
		Tokens:        tokens,
		// Todos los adjuntos de una tarea más un margen para los demás campos.
		MaxCuerpoArchivosMB: cfg.MaxAdjuntoMB*dominio.MaxAdjuntosPorTarea + 1,
		OrigenesCORS:        cfg.OrigenesCORS,
		Log:                 log,
		Salud: map[string]handler.ComprobadorSalud{
			"postgres": db.Ping,
			"redis":    func(context.Context) error { return clienteCola.Ping() },
		},
	})

	servidor := &http.Server{
		Addr:              ":" + cfg.PuertoHTTP,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	errServidor := make(chan error, 1)
	go func() {
		log.Info("API escuchando", "puerto", cfg.PuertoHTTP, "entorno", cfg.Entorno)
		errServidor <- servidor.ListenAndServe()
	}()

	select {
	case err := <-errServidor:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("deteniendo la API")
	}

	ctxCierre, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	return servidor.Shutdown(ctxCierre)
}
