// Package dominio contiene las entidades, constantes y reglas puras del sistema.
// No depende de HTTP ni de la base de datos, por lo que sus reglas se prueban
// de forma aislada.
package dominio

import "time"

// Rol identifica el tipo de usuario dentro de la plataforma.
type Rol string

const (
	RolAdministrador     Rol = "administrador"
	RolProfesor          Rol = "profesor"
	RolMonitor           Rol = "monitor"
	RolAsistenteGraduado Rol = "asistente_graduado"
)

// RolesValidos lista todos los roles admitidos por el sistema.
var RolesValidos = []Rol{RolAdministrador, RolProfesor, RolMonitor, RolAsistenteGraduado}

// RolesOperativos son los roles de quienes registran tareas.
var RolesOperativos = []Rol{RolMonitor, RolAsistenteGraduado}

// EsValido indica si el rol pertenece al catálogo.
func (r Rol) EsValido() bool {
	for _, v := range RolesValidos {
		if r == v {
			return true
		}
	}
	return false
}

// EsRolDeVinculacion indica si el rol puede usarse en una vinculación (RN-02).
func (r Rol) EsRolDeVinculacion() bool {
	return r == RolMonitor || r == RolAsistenteGraduado
}

// Texto devuelve el rol en lenguaje natural.
func (r Rol) Texto() string {
	switch r {
	case RolAsistenteGraduado:
		return "asistente graduado"
	default:
		return string(r)
	}
}

// Usuario representa a una persona registrada en la plataforma.
// Una misma persona puede tener varios roles (RF-01.3, RN-01).
type Usuario struct {
	ID           string
	Nombre       string
	Correo       string
	PasswordHash string
	Activo       bool
	Roles        []Rol
	CreadoEn     time.Time
}

// TieneRol indica si el usuario tiene asignado el rol dado.
func (u Usuario) TieneRol(rol Rol) bool {
	for _, r := range u.Roles {
		if r == rol {
			return true
		}
	}
	return false
}

// FiltroUsuarios delimita la consulta de usuarios.
type FiltroUsuarios struct {
	Rol      Rol
	Busqueda string // coincide con nombre o correo
	Paginacion
}
