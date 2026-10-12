// Package handler es la capa de presentación: expone la API REST, valida la
// entrada, aplica autenticación y autorización por rol, y traduce los
// resultados y errores del servicio a respuestas HTTP.
//
// Cada módulo (usuarios, tareas, reportes...) está en su propio archivo con:
// la interfaz del servicio que necesita, los formatos JSON de entrada y salida,
// los métodos que atienden cada endpoint y un método Registrar con sus rutas.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"seguimiento/internal/apperr"
	"seguimiento/internal/dominio"
)

// RespuestaError es el formato de todos los errores de la API.
type RespuestaError struct {
	Error   string `json:"error"`
	Mensaje string `json:"mensaje"`
}

var estadosPorTipo = map[apperr.Tipo]int{
	apperr.TipoValidacion:    http.StatusBadRequest,
	apperr.TipoNoAutenticado: http.StatusUnauthorized,
	apperr.TipoProhibido:     http.StatusForbidden,
	apperr.TipoNoEncontrado:  http.StatusNotFound,
	apperr.TipoConflicto:     http.StatusConflict,
	apperr.TipoReglaNegocio:  http.StatusUnprocessableEntity,
}

// responderError traduce un error de aplicación al código HTTP adecuado.
// Los errores inesperados se registran y se responden sin detalles internos.
func responderError(c *gin.Context, log *slog.Logger, err error) {
	var e *apperr.Error
	if errors.As(err, &e) {
		if estado, ok := estadosPorTipo[e.Tipo]; ok {
			c.AbortWithStatusJSON(estado, RespuestaError{Error: string(e.Tipo), Mensaje: e.Mensaje})
			return
		}
	}
	var demasiadoGrande *http.MaxBytesError
	if errors.As(err, &demasiadoGrande) {
		c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, RespuestaError{
			Error: "demasiado_grande", Mensaje: "la petición supera el tamaño máximo permitido"})
		return
	}
	log.Error("error interno", "ruta", c.FullPath(), "metodo", c.Request.Method, "error", err)
	mensaje := "ocurrió un error inesperado"
	if e != nil && e.Tipo == apperr.TipoInterno {
		mensaje = e.Mensaje
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, RespuestaError{Error: string(apperr.TipoInterno), Mensaje: mensaje})
}

// --- lectura de la entrada --------------------------------------------------

const maxCuerpoJSON = 1 << 20 // 1 MB

// leerJSON decodifica el cuerpo y rechaza campos desconocidos, para que los
// errores de escritura del cliente no pasen desapercibidos.
func leerJSON(c *gin.Context, destino any) error {
	return decodificarJSON(io.LimitReader(c.Request.Body, maxCuerpoJSON), destino)
}

func decodificarJSON(r io.Reader, destino any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	err := dec.Decode(destino)
	if err == nil {
		return nil
	}
	var tipo *json.UnmarshalTypeError
	switch {
	case errors.Is(err, io.EOF):
		return apperr.Validacion("el cuerpo de la petición está vacío")
	case errors.As(err, &tipo):
		return apperr.Validacion("el campo %q tiene un tipo inválido (se esperaba %s)", tipo.Field, tipo.Type.String())
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		return apperr.Validacion("campo desconocido %s", strings.TrimPrefix(err.Error(), "json: unknown field "))
	default:
		return apperr.Validacion("el cuerpo no es un JSON válido")
	}
}

// --- formatos de salida comunes ------------------------------------------------

// rolesTexto convierte los roles a cadenas para el JSON.
func rolesTexto(roles []dominio.Rol) []string {
	resultado := make([]string, 0, len(roles))
	for _, r := range roles {
		resultado = append(resultado, string(r))
	}
	return resultado
}
