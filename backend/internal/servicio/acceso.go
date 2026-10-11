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

// esProfesorResponsable indica si el actor es el profesor dado.
func esProfesorResponsable(actor auth.Actor, profesorID string) bool {
	return actor.TieneRol(dominio.RolProfesor) && actor.UsuarioID == profesorID
}

// puedeVerDeProfesor aplica RN-03 y RN-05: el profesor dueño o un administrador.
func puedeVerDeProfesor(actor auth.Actor, profesorID string) bool {
	return actor.EsAdministrador() || esProfesorResponsable(actor, profesorID)
}

// visiblePara devuelve el usuario por el que se deben filtrar los listados:
// vacío para el administrador (ve todo) y el propio actor para los demás.
func visiblePara(actor auth.Actor) string {
	if actor.EsAdministrador() {
		return ""
	}
	return actor.UsuarioID
}

// noEncontrado se usa en lugar de "prohibido" cuando el actor no debe saber
// siquiera que el recurso existe.
func noEncontrado(recurso string) error {
	return apperr.NoEncontrado("%s no encontrado", recurso)
}
