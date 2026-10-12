// Package config carga la configuración de la aplicación desde variables de
// entorno. Ningún secreto se define en el código ni en el repositorio.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"seguimiento/internal/almacenamiento"
	"seguimiento/internal/ia"
)

// Config es la configuración compartida por la API y el worker.
type Config struct {
	Entorno     string // desarrollo | produccion
	PuertoHTTP  string
	BaseDatos   string // URL de conexión a PostgreSQL
	ZonaHoraria *time.Location
	// OrigenesCORS son los orígenes (esquema://dominio:puerto) desde los que un
	// frontend en el navegador puede llamar a la API.
	OrigenesCORS []string

	Redis Redis
	JWT   JWT
	Admin AdminInicial

	Almacenamiento almacenamiento.Config
	MaxAdjuntoMB   int

	LLM    ia.Config
	Worker Worker
}

type Redis struct {
	Direccion string
	Password  string
	DB        int
}

type JWT struct {
	Secreto    string
	Expiracion time.Duration
}

// AdminInicial es el administrador que se crea al arrancar si aún no existe.
type AdminInicial struct {
	Nombre   string
	Correo   string
	Password string
}

type Worker struct {
	Concurrencia   int
	MaxReintentos  int
	TimeoutReporte time.Duration
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

	c.Redis = Redis{
		Direccion: texto("REDIS_DIRECCION", "localhost:6379"),
		Password:  os.Getenv("REDIS_PASSWORD"),
		DB:        entero("REDIS_DB", 0, &errs),
	}

	c.JWT = JWT{
		Secreto:    os.Getenv("JWT_SECRETO"),
		Expiracion: duracion("JWT_EXPIRACION", 8*time.Hour, &errs),
	}
	if len(c.JWT.Secreto) < 32 {
		errs = append(errs, errors.New("JWT_SECRETO es obligatorio y debe tener al menos 32 caracteres"))
	}

	c.Admin = AdminInicial{
		Nombre:   texto("ADMIN_NOMBRE", "Administrador"),
		Correo:   strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_CORREO"))),
		Password: os.Getenv("ADMIN_PASSWORD"),
	}

	c.Almacenamiento = almacenamiento.Config{
		Tipo: texto("ALMACENAMIENTO_TIPO", almacenamiento.TipoLocal),
		Ruta: texto("ALMACENAMIENTO_RUTA", "/data"),
	}
	c.MaxAdjuntoMB = entero("ADJUNTO_MAX_MB", 10, &errs)

	c.LLM = ia.Config{
		Proveedor:   strings.ToLower(texto("LLM_PROVEEDOR", ia.ProveedorSimulado)),
		URLBase:     strings.TrimSpace(os.Getenv("LLM_URL_BASE")),
		APIKey:      strings.TrimSpace(os.Getenv("LLM_API_KEY")),
		Modelo:      strings.TrimSpace(os.Getenv("LLM_MODELO")),
		MaxTokens:   entero("LLM_MAX_TOKENS", 0, &errs),
		Temperatura: decimalOpcional("LLM_TEMPERATURA", &errs),
		Timeout:     duracion("LLM_TIMEOUT", 2*time.Minute, &errs),
	}

	c.Worker = Worker{
		Concurrencia:   entero("WORKER_CONCURRENCIA", 2, &errs),
		MaxReintentos:  entero("REPORTE_MAX_REINTENTOS", 3, &errs),
		TimeoutReporte: duracion("REPORTE_TIMEOUT", 5*time.Minute, &errs),
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

func entero(clave string, porDefecto int, errs *[]error) int {
	v := strings.TrimSpace(os.Getenv(clave))
	if v == "" {
		return porDefecto
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s debe ser un número entero: %w", clave, err))
		return porDefecto
	}
	return n
}

func duracion(clave string, porDefecto time.Duration, errs *[]error) time.Duration {
	v := strings.TrimSpace(os.Getenv(clave))
	if v == "" {
		return porDefecto
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s debe ser una duración como 30s o 2m: %w", clave, err))
		return porDefecto
	}
	return d
}

func decimalOpcional(clave string, errs *[]error) *float64 {
	v := strings.TrimSpace(os.Getenv(clave))
	if v == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s debe ser un número decimal: %w", clave, err))
		return nil
	}
	return &f
}
