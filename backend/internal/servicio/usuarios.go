package servicio

import (
	"context"
	"log/slog"
	"slices"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

// RepositorioGestionUsuarios es lo que la gestión de usuarios necesita de la capa de datos.
type RepositorioGestionUsuarios interface {
	BuscarPorID(ctx context.Context, id string) (dominio.Usuario, error)
	Crear(ctx context.Context, u dominio.Usuario) (dominio.Usuario, error)
	Listar(ctx context.Context, f dominio.FiltroUsuarios) ([]dominio.Usuario, error)
	ReemplazarRoles(ctx context.Context, usuarioID string, roles []dominio.Rol) error
	TieneVinculacionesActivas(ctx context.Context, usuarioID string, rol dominio.Rol) (bool, error)
	TieneEspaciosActivos(ctx context.Context, usuarioID string) (bool, error)
}

// NuevoUsuario son los datos para crear un usuario (RF-02.1).
type NuevoUsuario struct {
	Nombre   string
	Correo   string
	Password string
	Roles    []dominio.Rol
}

// Usuarios implementa la administración de usuarios (RF-02, RF-15.2).
type Usuarios struct {
	repo RepositorioGestionUsuarios
	log  *slog.Logger
}

// NuevoUsuarios crea el servicio de usuarios.
func NuevoUsuarios(repo RepositorioGestionUsuarios, log *slog.Logger) *Usuarios {
	return &Usuarios{repo: repo, log: log}
}

// Crear registra un usuario con sus roles. Solo el administrador puede hacerlo.
func (s *Usuarios) Crear(ctx context.Context, actor auth.Actor, n NuevoUsuario) (dominio.Usuario, error) {
	if err := exigirRol(actor, dominio.RolAdministrador); err != nil {
		return dominio.Usuario{}, err
	}
	nombre, err := dominio.Texto("el nombre", n.Nombre, true, dominio.LongitudMaxNombre)
	if err != nil {
		return dominio.Usuario{}, err
	}
	correo, err := dominio.NormalizarCorreo(n.Correo)
	if err != nil {
		return dominio.Usuario{}, err
	}
	if err := dominio.ValidarPassword(n.Password); err != nil {
		return dominio.Usuario{}, err
	}
	roles, err := normalizarRoles(n.Roles)
	if err != nil {
		return dominio.Usuario{}, err
	}
	hash, err := auth.HashPassword(n.Password)
	if err != nil {
		return dominio.Usuario{}, apperr.Interno(err, "no fue posible crear el usuario")
	}
	u, err := s.repo.Crear(ctx, dominio.Usuario{Nombre: nombre, Correo: correo, PasswordHash: hash, Activo: true, Roles: roles})
	if err != nil {
		return u, err
	}
	s.log.Info("usuario creado", "usuario_id", u.ID, "roles", roles, "por", actor.UsuarioID)
	return u, nil
}

// Listar devuelve los usuarios del sistema (RF-02.3).
func (s *Usuarios) Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroUsuarios) ([]dominio.Usuario, error) {
	if err := exigirRol(actor, dominio.RolAdministrador); err != nil {
		return nil, err
	}
	if f.Rol != "" && !f.Rol.EsValido() {
		return nil, apperr.Validacion("rol inválido")
	}
	return s.repo.Listar(ctx, f)
}

// Obtener devuelve un usuario: el administrador ve a cualquiera, los demás solo a sí mismos.
func (s *Usuarios) Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.Usuario, error) {
	if !actor.EsAdministrador() && actor.UsuarioID != id {
		return dominio.Usuario{}, noEncontrado("usuario")
	}
	return s.repo.BuscarPorID(ctx, id)
}

// AsignarRoles reemplaza los roles de un usuario (RF-02.2). Impide dejar
// inconsistencias: quitar un rol que la persona usa en vinculaciones o espacios
// activos, o que el administrador se quite a sí mismo su rol.
func (s *Usuarios) AsignarRoles(ctx context.Context, actor auth.Actor, id string, roles []dominio.Rol) (dominio.Usuario, error) {
	if err := exigirRol(actor, dominio.RolAdministrador); err != nil {
		return dominio.Usuario{}, err
	}
	roles, err := normalizarRoles(roles)
	if err != nil {
		return dominio.Usuario{}, err
	}
	u, err := s.repo.BuscarPorID(ctx, id)
	if err != nil {
		return u, err
	}
	if id == actor.UsuarioID && !slices.Contains(roles, dominio.RolAdministrador) {
		return u, apperr.ReglaNegocio("no puede quitarse a sí mismo el rol de administrador")
	}
	for _, anterior := range u.Roles {
		if slices.Contains(roles, anterior) {
			continue
		}
		if err := s.verificarRetiroDeRol(ctx, id, anterior); err != nil {
			return u, err
		}
	}
	if err := s.repo.ReemplazarRoles(ctx, id, roles); err != nil {
		return u, err
	}
	s.log.Info("roles actualizados", "usuario_id", id, "roles", roles, "por", actor.UsuarioID)
	return s.repo.BuscarPorID(ctx, id)
}

func (s *Usuarios) verificarRetiroDeRol(ctx context.Context, usuarioID string, rol dominio.Rol) error {
	var enUso bool
	var err error
	switch {
	case rol.EsRolDeVinculacion():
		enUso, err = s.repo.TieneVinculacionesActivas(ctx, usuarioID, rol)
	case rol == dominio.RolProfesor:
		enUso, err = s.repo.TieneEspaciosActivos(ctx, usuarioID)
	}
	if err != nil {
		return err
	}
	if enUso {
		return apperr.ReglaNegocio("no se puede quitar el rol %s: la persona lo usa en cursos o proyectos activos", rol.Texto())
	}
	return nil
}

// normalizarRoles valida el catálogo, elimina duplicados y exige al menos un rol.
func normalizarRoles(roles []dominio.Rol) ([]dominio.Rol, error) {
	var resultado []dominio.Rol
	for _, r := range roles {
		if !r.EsValido() {
			return nil, apperr.Validacion("rol inválido: %q", r)
		}
		if !slices.Contains(resultado, r) {
			resultado = append(resultado, r)
		}
	}
	if len(resultado) == 0 {
		return nil, apperr.Validacion("el usuario debe tener al menos un rol")
	}
	slices.Sort(resultado)
	return resultado, nil
}
