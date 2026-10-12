package servicio

import (
	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

// Reglas de visibilidad compartidas por los servicios (RF-12, RN-03 a RN-05).
// La capa de presentación ya filtra por rol; aquí se verifica la pertenencia
// de cada dato, para que la regla se cumpla aunque cambie la capa HTTP.

// exigirRol devuelve un error si el actor no tiene ninguno de los roles.
func exigirRol(actor auth.Actor, roles ...dominio.Rol) error {
	if !actor.TieneAlgunRol(roles...) {
		return apperr.Prohibido("no tiene permiso para esta operación")
	}
	return nil
}

// noEncontrado se usa en lugar de "prohibido" cuando el actor no debe saber
// siquiera que el recurso existe.
func noEncontrado(recurso string) error {
	return apperr.NoEncontrado("%s no encontrado", recurso)
}
