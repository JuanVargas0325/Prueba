// Package apperr define los errores de aplicación que las capas de servicio y
// datos devuelven. La capa de presentación los traduce a códigos HTTP.
package apperr

import (
	"errors"
	"fmt"
)

// Tipo clasifica el error para que la capa HTTP elija el código de estado.
type Tipo string

const (
	TipoValidacion    Tipo = "validacion"     // 400
	TipoNoAutenticado Tipo = "no_autenticado" // 401
	TipoProhibido     Tipo = "prohibido"      // 403
	TipoNoEncontrado  Tipo = "no_encontrado"  // 404
	TipoConflicto     Tipo = "conflicto"      // 409
	TipoReglaNegocio  Tipo = "regla_negocio"  // 422
	TipoInterno       Tipo = "interno"        // 500
)

// Error es un error de aplicación con un tipo y un mensaje apto para el cliente.
type Error struct {
	Tipo    Tipo
	Mensaje string
	Causa   error
}

func (e *Error) Error() string {
	if e.Causa != nil {
		return fmt.Sprintf("%s: %s: %v", e.Tipo, e.Mensaje, e.Causa)
	}
	return fmt.Sprintf("%s: %s", e.Tipo, e.Mensaje)
}

func (e *Error) Unwrap() error { return e.Causa }

func nuevo(tipo Tipo, formato string, args ...any) *Error {
	return &Error{Tipo: tipo, Mensaje: fmt.Sprintf(formato, args...)}
}

func Validacion(formato string, args ...any) *Error {
	return nuevo(TipoValidacion, formato, args...)
}

func NoAutenticado(formato string, args ...any) *Error {
	return nuevo(TipoNoAutenticado, formato, args...)
}

func Prohibido(formato string, args ...any) *Error {
	return nuevo(TipoProhibido, formato, args...)
}

func NoEncontrado(formato string, args ...any) *Error {
	return nuevo(TipoNoEncontrado, formato, args...)
}

func Conflicto(formato string, args ...any) *Error {
	return nuevo(TipoConflicto, formato, args...)
}

// ReglaNegocio indica que la operación viola una regla del dominio
// (por ejemplo, superar las 22 horas de asistencia graduada).
func ReglaNegocio(formato string, args ...any) *Error {
	return nuevo(TipoReglaNegocio, formato, args...)
}

// Interno envuelve un error inesperado. El mensaje no expone detalles internos.
func Interno(causa error, formato string, args ...any) *Error {
	e := nuevo(TipoInterno, formato, args...)
	e.Causa = causa
	return e
}

// TipoDe devuelve el tipo del error de aplicación, o TipoInterno si no lo es.
func TipoDe(err error) Tipo {
	var e *Error
	if errors.As(err, &e) {
		return e.Tipo
	}
	return TipoInterno
}

// Es indica si el error es un error de aplicación del tipo dado.
func Es(err error, tipo Tipo) bool {
	var e *Error
	return errors.As(err, &e) && e.Tipo == tipo
}
