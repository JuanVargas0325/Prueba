// Comando api: servidor HTTP de la plataforma de seguimiento.
package main

import (
	"log/slog"
	"os"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("servicio", "api")
	if err := ejecutar(log); err != nil {
		log.Error("la API terminó con error", "error", err)
		os.Exit(1)
	}
}

// ejecutar arma la aplicación. En los siguientes pasos se agregan la
// configuración, la base de datos y el servidor HTTP.
func ejecutar(log *slog.Logger) error {
	log.Info("API iniciada")
	return nil
}
