// Package config carga la configuración de la aplicación desde variables de
// entorno. Ningún secreto se define en el código ni en el repositorio.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config es la configuración de la aplicación. En los pasos siguientes se le
// agregan la base de datos, la autenticación, Redis, los archivos y la IA.
type Config struct {
	Entorno     string // desarrollo | produccion
	PuertoHTTP  string
	ZonaHoraria *time.Location
}

// EsDesarrollo indica si la aplicación corre en modo desarrollo.
func (c Config) EsDesarrollo() bool { return c.Entorno == "desarrollo" }

// Cargar lee la configuración del entorno y valida los valores obligatorios.
func Cargar() (Config, error) {
	var errs []error
	c := Config{
		Entorno:    texto("APP_ENTORNO", "desarrollo"),
		PuertoHTTP: texto("APP_PUERTO", "8080"),
	}

	zona, err := time.LoadLocation(texto("APP_ZONA_HORARIA", "America/Bogota"))
	if err != nil {
		errs = append(errs, fmt.Errorf("APP_ZONA_HORARIA: %w", err))
	}
	c.ZonaHoraria = zona

	return c, errors.Join(errs...)
}

func texto(clave, porDefecto string) string {
	if v := strings.TrimSpace(os.Getenv(clave)); v != "" {
		return v
	}
	return porDefecto
}
