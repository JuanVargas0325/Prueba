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

type personaRef struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	Correo string `json:"correo,omitempty"`
}

type espacioRef struct {
	ID          string                `json:"id"`
	Nombre      string                `json:"nombre"`
	Tipo        dominio.TipoEspacio   `json:"tipo"`
	Estado      dominio.EstadoEspacio `json:"estado,omitempty"`
	FechaInicio string                `json:"fecha_inicio,omitempty"`
	FechaCierre string                `json:"fecha_cierre,omitempty"`
}

type vinculacionJSON struct {
	ID             string      `json:"id"`
	Usuario        personaRef  `json:"usuario"`
	Espacio        espacioRef  `json:"espacio"`
	Profesor       personaRef  `json:"profesor"`
	Rol            dominio.Rol `json:"rol"`
	HorasSemanales int         `json:"horas_semanales"`
	CreadoEn       time.Time   `json:"creado_en"`
	ActualizadoEn  time.Time   `json:"actualizado_en"`
}

func aVinculacionJSON(v dominio.Vinculacion) vinculacionJSON {
	return vinculacionJSON{
		ID:      v.ID,
		Usuario: personaRef{ID: v.UsuarioID, Nombre: v.UsuarioNombre, Correo: v.UsuarioCorreo},
		Espacio: espacioRef{
			ID: v.EspacioID, Nombre: v.EspacioNombre, Tipo: v.EspacioTipo, Estado: v.EspacioEstado,
			FechaInicio: fecha(v.EspacioFechaInicio), FechaCierre: fecha(v.EspacioFechaCierre),
		},
		Profesor:       personaRef{ID: v.ProfesorID, Nombre: v.ProfesorNombre},
		Rol:            v.Rol,
		HorasSemanales: v.HorasSemanales,
		CreadoEn:       v.CreadoEn,
		ActualizadoEn:  v.ActualizadoEn,
	}
}
