// Comando api: servidor HTTP de la plataforma de seguimiento.
package main

import (
	"log/slog"
	"os"
	_ "time/tzdata" // incluye las zonas horarias en el binario

	"seguimiento/internal/config"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("servicio", "api")
	if err := ejecutar(log); err != nil {
		log.Error("la API terminó con error", "error", err)
		os.Exit(1)
	}
}

// ejecutar arma la aplicación. En los siguientes pasos se agregan el servidor
// HTTP y la base de datos.
func ejecutar(log *slog.Logger) error {
	cfg, err := config.Cargar()
	if err != nil {
		return err
	}
	log.Info("configuración cargada", "entorno", cfg.Entorno, "puerto", cfg.PuertoHTTP, "zona_horaria", cfg.ZonaHoraria.String())
	return nil
}
