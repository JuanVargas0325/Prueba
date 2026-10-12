// Package config carga la configuración de la aplicación desde variables de
// entorno. Ningún secreto se define en el código ni en el repositorio.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// Config es la configuración de la aplicación. En los pasos siguientes se le
// agregan la autenticación, Redis, los archivos y la IA.
type Config struct {
	Entorno     string // desarrollo | produccion
	PuertoHTTP  string
	BaseDatos   string // URL de conexión a PostgreSQL
	ZonaHoraria *time.Location
	// OrigenesCORS son los orígenes (esquema://dominio:puerto) desde los que un
	// frontend en el navegador puede llamar a la API.
	OrigenesCORS []string
}

// EsDesarrollo indica si la aplicación corre en modo desarrollo.
func (c Config) EsDesarrollo() bool { return c.Entorno == "desarrollo" }

// Cargar lee la configuración del entorno y valida los valores obligatorios.
func Cargar() (Config, error) {
	var errs []error
	c := Config{
		Entorno:      texto("APP_ENTORNO", "desarrollo"),
		PuertoHTTP:   texto("APP_PUERTO", "8080"),
		OrigenesCORS: lista("CORS_ORIGENES"),
	}

	zona, err := time.LoadLocation(texto("APP_ZONA_HORARIA", "America/Bogota"))
	if err != nil {
		errs = append(errs, fmt.Errorf("APP_ZONA_HORARIA: %w", err))
	}
	c.ZonaHoraria = zona

	c.BaseDatos = urlBaseDatos()
	if c.BaseDatos == "" {
		errs = append(errs, errors.New("defina DATABASE_URL o POSTGRES_USER, POSTGRES_PASSWORD y POSTGRES_DB"))
	}

	return c, errors.Join(errs...)
}

// urlBaseDatos construye la URL de PostgreSQL a partir de las mismas
// variables que usa la imagen oficial de postgres, salvo que se defina DATABASE_URL.
func urlBaseDatos() string {
	if v := strings.TrimSpace(os.Getenv("DATABASE_URL")); v != "" {
		return v
	}
	usuario, clave, bd := os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD"), os.Getenv("POSTGRES_DB")
	if usuario == "" || bd == "" {
		return ""
	}
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(usuario, clave),
		Host:     texto("POSTGRES_HOST", "localhost") + ":" + texto("POSTGRES_PUERTO", "5432"),
		Path:     "/" + bd,
		RawQuery: "sslmode=" + texto("POSTGRES_SSLMODE", "disable"),
	}
	return u.String()
}

func texto(clave, porDefecto string) string {
	if v := strings.TrimSpace(os.Getenv(clave)); v != "" {
		return v
	}
	return porDefecto
}

// lista lee una variable con valores separados por comas, sin espacios ni vacíos.
func lista(clave string) []string {
	var valores []string
	for _, v := range strings.Split(os.Getenv(clave), ",") {
		if v = strings.TrimSpace(v); v != "" {
			valores = append(valores, v)
		}
	}
	return valores
}
