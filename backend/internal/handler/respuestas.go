// Package handler es la capa de presentación: expone la API REST, valida la
// entrada, aplica autenticación y autorización por rol, y traduce los
// resultados y errores del servicio a respuestas HTTP.
//
// Cada módulo (usuarios, tareas, reportes...) está en su propio archivo con:
// la interfaz del servicio que necesita, los formatos JSON de entrada y salida,
// los métodos que atienden cada endpoint y un método Registrar con sus rutas.
package handler

// RespuestaError es el formato de todos los errores de la API.
type RespuestaError struct {
	Error   string `json:"error"`
	Mensaje string `json:"mensaje"`
}
