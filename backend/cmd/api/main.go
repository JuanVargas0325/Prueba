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

	"seguimiento/internal/config"
	"seguimiento/internal/handler"
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

	// Capa de presentación
	router := handler.NuevoRouter(handler.Dependencias{})

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
