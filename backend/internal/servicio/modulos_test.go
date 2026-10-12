package servicio

import (
	"context"
	"strings"
	"testing"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

// Dobles en memoria de los repositorios de los módulos de gestión.

func esperarTipo(t *testing.T, err error, tipo apperr.Tipo) {
	t.Helper()
	if !apperr.Es(err, tipo) {
		t.Fatalf("se esperaba un error %s, se obtuvo %v", tipo, err)
	}
}

// --- usuarios ------------------------------------------------------------------------

type repoUsuariosFalso struct {
	usuarios       map[string]dominio.Usuario
	rolesEnUso     map[dominio.Rol]bool
	espaciosActivo bool
}

func nuevoRepoUsuarios() *repoUsuariosFalso {
	return &repoUsuariosFalso{usuarios: map[string]dominio.Usuario{}, rolesEnUso: map[dominio.Rol]bool{}}
}

func (r *repoUsuariosFalso) BuscarPorID(_ context.Context, id string) (dominio.Usuario, error) {
	u, ok := r.usuarios[id]
	if !ok {
		return u, apperr.NoEncontrado("usuario no encontrado")
	}
	return u, nil
}

func (r *repoUsuariosFalso) BuscarPorCorreo(_ context.Context, correo string) (dominio.Usuario, error) {
	for _, u := range r.usuarios {
		if u.Correo == strings.ToLower(correo) {
			return u, nil
		}
	}
	return dominio.Usuario{}, apperr.NoEncontrado("usuario no encontrado")
}

func (r *repoUsuariosFalso) Crear(_ context.Context, u dominio.Usuario) (dominio.Usuario, error) {
	u.ID = "nuevo-" + u.Correo
	r.usuarios[u.ID] = u
	return u, nil
}

func (r *repoUsuariosFalso) Listar(context.Context, dominio.FiltroUsuarios) ([]dominio.Usuario, error) {
	return nil, nil
}

func (r *repoUsuariosFalso) ReemplazarRoles(_ context.Context, id string, roles []dominio.Rol) error {
	u := r.usuarios[id]
	u.Roles = roles
	r.usuarios[id] = u
	return nil
}

func (r *repoUsuariosFalso) TieneVinculacionesActivas(_ context.Context, _ string, rol dominio.Rol) (bool, error) {
	return r.rolesEnUso[rol], nil
}

func (r *repoUsuariosFalso) TieneEspaciosActivos(context.Context, string) (bool, error) {
	return r.espaciosActivo, nil
}

func TestCrearUsuario(t *testing.T) {
	repo := nuevoRepoUsuarios()
	s := NuevoUsuarios(repo, logPrueba)
	ctx := context.Background()

	u, err := s.Crear(ctx, admin, NuevoUsuario{Nombre: " Ana ", Correo: "ANA@uniandes.edu.co", Password: "Clave123*",
		Roles: []dominio.Rol{dominio.RolMonitor, dominio.RolMonitor, dominio.RolAsistenteGraduado}})
	if err != nil {
		t.Fatal(err)
	}
	if u.Nombre != "Ana" || u.Correo != "ana@uniandes.edu.co" || len(u.Roles) != 2 || u.PasswordHash == "Clave123*" || !u.Activo {
		t.Errorf("usuario inesperado: %+v", u)
	}

	casos := []struct {
		actor auth.Actor
		n     NuevoUsuario
		tipo  apperr.Tipo
	}{
		{profesor, NuevoUsuario{Nombre: "x", Correo: "x@x.co", Password: "Clave123*", Roles: []dominio.Rol{dominio.RolMonitor}}, apperr.TipoProhibido},
		{admin, NuevoUsuario{Nombre: "", Correo: "x@x.co", Password: "Clave123*", Roles: []dominio.Rol{dominio.RolMonitor}}, apperr.TipoValidacion},
		{admin, NuevoUsuario{Nombre: "x", Correo: "no-es-correo", Password: "Clave123*", Roles: []dominio.Rol{dominio.RolMonitor}}, apperr.TipoValidacion},
		{admin, NuevoUsuario{Nombre: "x", Correo: "x@x.co", Password: "corta", Roles: []dominio.Rol{dominio.RolMonitor}}, apperr.TipoValidacion},
		{admin, NuevoUsuario{Nombre: "x", Correo: "x@x.co", Password: "Clave123*", Roles: []dominio.Rol{"rector"}}, apperr.TipoValidacion},
		{admin, NuevoUsuario{Nombre: "x", Correo: "x@x.co", Password: "Clave123*"}, apperr.TipoValidacion},
	}
	for _, c := range casos {
		_, err := s.Crear(ctx, c.actor, c.n)
		esperarTipo(t, err, c.tipo)
	}
}

func TestAsignarRoles(t *testing.T) {
	repo := nuevoRepoUsuarios()
	repo.usuarios["admin-1"] = dominio.Usuario{ID: "admin-1", Roles: []dominio.Rol{dominio.RolAdministrador}}
	repo.usuarios["u1"] = dominio.Usuario{ID: "u1", Roles: []dominio.Rol{dominio.RolMonitor, dominio.RolProfesor}}
	s := NuevoUsuarios(repo, logPrueba)
	ctx := context.Background()

	_, err := s.AsignarRoles(ctx, admin, "admin-1", []dominio.Rol{dominio.RolProfesor})
	esperarTipo(t, err, apperr.TipoReglaNegocio) // no puede quitarse su propio rol de administrador

	repo.rolesEnUso[dominio.RolMonitor] = true
	_, err = s.AsignarRoles(ctx, admin, "u1", []dominio.Rol{dominio.RolProfesor})
	esperarTipo(t, err, apperr.TipoReglaNegocio) // el rol monitor está en uso

	repo.rolesEnUso[dominio.RolMonitor] = false
	u, err := s.AsignarRoles(ctx, admin, "u1", []dominio.Rol{dominio.RolProfesor, dominio.RolAsistenteGraduado})
	if err != nil || !u.TieneRol(dominio.RolAsistenteGraduado) || u.TieneRol(dominio.RolMonitor) {
		t.Fatalf("roles inesperados: %+v %v", u, err)
	}

	repo.espaciosActivo = true
	_, err = s.AsignarRoles(ctx, admin, "u1", []dominio.Rol{dominio.RolAsistenteGraduado})
	esperarTipo(t, err, apperr.TipoReglaNegocio) // el profesor tiene espacios activos

	_, err = s.Obtener(ctx, monitor, "u1")
	esperarTipo(t, err, apperr.TipoNoEncontrado) // un usuario solo se ve a sí mismo
}
