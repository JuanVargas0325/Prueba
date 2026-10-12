package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
	"seguimiento/internal/servicio"
)

// --- usuarios, períodos, espacios y vinculaciones ------------------------------------

type servicioUsuariosFalso struct{ nuevo servicio.NuevoUsuario }

func (s *servicioUsuariosFalso) Crear(_ context.Context, _ auth.Actor, n servicio.NuevoUsuario) (dominio.Usuario, error) {
	s.nuevo = n
	return dominio.Usuario{ID: uuidA, Nombre: n.Nombre, Correo: n.Correo, Roles: n.Roles, PasswordHash: "secreto"}, nil
}

func (s *servicioUsuariosFalso) Listar(context.Context, auth.Actor, dominio.FiltroUsuarios) ([]dominio.Usuario, error) {
	return []dominio.Usuario{{ID: uuidA}}, nil
}

func (s *servicioUsuariosFalso) Obtener(context.Context, auth.Actor, string) (dominio.Usuario, error) {
	return dominio.Usuario{ID: uuidA}, nil
}

func (s *servicioUsuariosFalso) AsignarRoles(_ context.Context, _ auth.Actor, _ string, roles []dominio.Rol) (dominio.Usuario, error) {
	return dominio.Usuario{ID: uuidA, Roles: roles}, nil
}

func TestCrearUsuarioNoExponeElHash(t *testing.T) {
	srv := &servicioUsuariosFalso{}
	e := nuevoEntorno(t, func(d *Dependencias) { d.Usuarios = srv })
	w := e.peticion(http.MethodPost, "/api/v1/usuarios", e.token(t, dominio.RolAdministrador),
		`{"nombre":"Ana","correo":"ana@uniandes.edu.co","password":"Clave123*","roles":["monitor"]}`)
	esperarEstado(t, w, http.StatusCreated)
	if strings.Contains(w.Body.String(), "secreto") || strings.Contains(w.Body.String(), "password") {
		t.Errorf("la respuesta no debe incluir la contraseña ni su hash: %s", w.Body.String())
	}
	if srv.nuevo.Roles[0] != dominio.RolMonitor || srv.nuevo.Password != "Clave123*" {
		t.Errorf("datos recibidos inesperados: %+v", srv.nuevo)
	}
	esperarEstado(t, e.peticion(http.MethodPut, "/api/v1/usuarios/"+uuidA+"/roles", e.token(t, dominio.RolAdministrador), `{"roles":["profesor"]}`), http.StatusOK)
}
