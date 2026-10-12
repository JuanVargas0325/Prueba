package handler

import (
	"time"

	"seguimiento/internal/dominio"
)

// Formatos JSON compartidos por varios módulos. Las entidades del dominio no
// llevan etiquetas JSON: la forma de la API se define solo en esta capa.

type usuarioJSON struct {
	ID       string    `json:"id"`
	Nombre   string    `json:"nombre"`
	Correo   string    `json:"correo"`
	Activo   bool      `json:"activo"`
	Roles    []string  `json:"roles"`
	CreadoEn time.Time `json:"creado_en"`
}

func aUsuarioJSON(u dominio.Usuario) usuarioJSON {
	return usuarioJSON{ID: u.ID, Nombre: u.Nombre, Correo: u.Correo, Activo: u.Activo, Roles: rolesTexto(u.Roles), CreadoEn: u.CreadoEn}
}
