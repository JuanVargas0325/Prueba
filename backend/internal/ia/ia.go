// Package ia integra el modelo de lenguaje que redacta el resumen de los
// reportes semanales (RF-14). El proveedor se elige con variables de entorno,
// de modo que cambiar de modelo no requiere tocar el código.
package ia

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"seguimiento/internal/dominio"
)

// Proveedores admitidos en LLM_PROVEEDOR. Los proveedores reales se agregan
// en los pasos 52 (compatible con OpenAI) y 53 (Anthropic).
const (
	// ProveedorSimulado no llama a ningún modelo: genera un resumen
	// determinístico. Sirve para desarrollar y probar sin credenciales.
	ProveedorSimulado = "simulado"
)

// Resumidor redacta el resumen del trabajo semanal de una persona.
type Resumidor interface {
	Resumir(ctx context.Context, datos dominio.DatosSemanaPersona) (string, error)
	Proveedor() string
	Modelo() string
}

// Config agrupa los parámetros del proveedor de IA.
type Config struct {
	Proveedor   string
	URLBase     string
	APIKey      string
	Modelo      string
	MaxTokens   int      // 0 = valor por defecto del proveedor
	Temperatura *float64 // nil = valor por defecto del proveedor
	Timeout     time.Duration
}

// Nuevo crea el resumidor configurado.
func Nuevo(cfg Config) (Resumidor, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Minute
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Proveedor)) {
	case "", ProveedorSimulado:
		return NuevoSimulado(), nil
	default:
		return nil, fmt.Errorf("LLM_PROVEEDOR %q no soportado (use %s)", cfg.Proveedor, ProveedorSimulado)
	}
}

// ErrorProveedor es un error devuelto por el servicio del modelo.
// Reintentable indica si tiene sentido volver a intentar la llamada.
type ErrorProveedor struct {
	Proveedor    string
	Estado       int // código HTTP, 0 si no aplica
	Mensaje      string
	Reintentable bool
}

func (e *ErrorProveedor) Error() string {
	if e.Estado > 0 {
		return fmt.Sprintf("proveedor de IA %s respondió %d: %s", e.Proveedor, e.Estado, e.Mensaje)
	}
	return fmt.Sprintf("proveedor de IA %s: %s", e.Proveedor, e.Mensaje)
}

// EsReintentable indica si el error de IA es transitorio. Los errores que no
// provienen del proveedor (red, tiempo agotado) se consideran reintentables.
func EsReintentable(err error) bool {
	var ep *ErrorProveedor
	if errors.As(err, &ep) {
		return ep.Reintentable
	}
	return true
}

// EsErrorDeIA indica si el error proviene del proveedor de IA.
func EsErrorDeIA(err error) bool {
	var ep *ErrorProveedor
	return errors.As(err, &ep)
}
