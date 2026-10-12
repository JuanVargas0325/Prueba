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

type servicioPeriodosFalso struct {
	nuevo   servicio.NuevoPeriodo
	cambios servicio.CambiosPeriodo
}

func (s *servicioPeriodosFalso) Crear(_ context.Context, _ auth.Actor, n servicio.NuevoPeriodo) (dominio.PeriodoAcademico, error) {
	s.nuevo = n
	return dominio.PeriodoAcademico{ID: uuidA, Codigo: n.Codigo, FechaInicio: n.FechaInicio, FechaCierre: n.FechaCierre}, nil
}

func (s *servicioPeriodosFalso) Listar(context.Context, auth.Actor, dominio.FiltroPeriodos) ([]dominio.PeriodoAcademico, error) {
	return nil, nil
}

func (s *servicioPeriodosFalso) Obtener(context.Context, auth.Actor, string) (dominio.PeriodoAcademico, error) {
	return dominio.PeriodoAcademico{}, nil
}

func (s *servicioPeriodosFalso) Actualizar(_ context.Context, _ auth.Actor, _ string, c servicio.CambiosPeriodo) (dominio.PeriodoAcademico, error) {
	s.cambios = c
	return dominio.PeriodoAcademico{}, nil
}

func TestPeriodos(t *testing.T) {
	srv := &servicioPeriodosFalso{}
	e := nuevoEntorno(t, func(d *Dependencias) { d.Periodos = srv })
	admin := e.token(t, dominio.RolAdministrador)

	w := e.peticion(http.MethodPost, "/api/v1/periodos", admin, `{"codigo":"2026-20","fecha_inicio":"2026-08-01","fecha_cierre":"2026-12-15"}`)
	esperarEstado(t, w, http.StatusCreated)
	if !srv.nuevo.CreacionHabilitada || !strings.Contains(w.Body.String(), `"fecha_inicio":"2026-08-01"`) {
		t.Errorf("la ventana de creación debería quedar habilitada por defecto: %+v %s", srv.nuevo, w.Body.String())
	}
	esperarEstado(t, e.peticion(http.MethodPost, "/api/v1/periodos", admin, `{"codigo":"2026-20","fecha_inicio":"1/8/2026","fecha_cierre":"2026-12-15"}`), http.StatusBadRequest)

	esperarEstado(t, e.peticion(http.MethodPatch, "/api/v1/periodos/"+uuidA, admin, `{"creacion_habilitada":false}`), http.StatusOK)
	if srv.cambios.CreacionHabilitada == nil || *srv.cambios.CreacionHabilitada || srv.cambios.Estado != nil {
		t.Errorf("cambios inesperados: %+v", srv.cambios)
	}
	esperarEstado(t, e.peticion(http.MethodGet, "/api/v1/periodos", e.token(t, dominio.RolMonitor), ""), http.StatusOK)
}
