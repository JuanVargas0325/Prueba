package repositorio

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"seguimiento/internal/apperr"
	"seguimiento/internal/dominio"
)

// Usuarios gestiona la persistencia de usuarios y sus roles.
type Usuarios struct {
	db *pgxpool.Pool
}

// NuevoUsuarios crea el repositorio de usuarios.
func NuevoUsuarios(db *pgxpool.Pool) *Usuarios { return &Usuarios{db: db} }

const selectUsuario = `
	SELECT u.id, u.nombre, u.correo, u.password_hash, u.activo, u.creado_en,
	       COALESCE(array_agg(r.rol ORDER BY r.rol) FILTER (WHERE r.rol IS NOT NULL), '{}') AS roles
	FROM usuarios u
	LEFT JOIN usuario_roles r ON r.usuario_id = u.id`

func escanearUsuario(fila pgx.Row) (dominio.Usuario, error) {
	var u dominio.Usuario
	var roles []string
	if err := fila.Scan(&u.ID, &u.Nombre, &u.Correo, &u.PasswordHash, &u.Activo, &u.CreadoEn, &roles); err != nil {
		return u, err
	}
	for _, r := range roles {
		u.Roles = append(u.Roles, dominio.Rol(r))
	}
	return u, nil
}

// BuscarPorCorreo devuelve el usuario con el correo dado (sin distinguir mayúsculas).
func (r *Usuarios) BuscarPorCorreo(ctx context.Context, correo string) (dominio.Usuario, error) {
	u, err := escanearUsuario(r.db.QueryRow(ctx,
		selectUsuario+` WHERE lower(u.correo) = lower($1) GROUP BY u.id`, strings.TrimSpace(correo)))
	return u, traducir(err, recursoUsuario)
}

// BuscarPorID devuelve el usuario con el identificador dado.
func (r *Usuarios) BuscarPorID(ctx context.Context, id string) (dominio.Usuario, error) {
	u, err := escanearUsuario(r.db.QueryRow(ctx, selectUsuario+` WHERE u.id = $1 GROUP BY u.id`, id))
	return u, traducir(err, recursoUsuario)
}

// Listar devuelve los usuarios que cumplen el filtro, ordenados por nombre.
func (r *Usuarios) Listar(ctx context.Context, f dominio.FiltroUsuarios) ([]dominio.Usuario, error) {
	var w filtro
	if f.Rol != "" {
		w.agregar(`EXISTS (SELECT 1 FROM usuario_roles x WHERE x.usuario_id = u.id AND x.rol = ?)`, string(f.Rol))
	}
	if b := strings.TrimSpace(f.Busqueda); b != "" {
		w.agregar(`(u.nombre ILIKE ? OR u.correo ILIKE ?)`, patronBusqueda(b))
	}
	sql := selectUsuario + w.where() + ` GROUP BY u.id ORDER BY lower(u.nombre), u.correo` + w.paginar(f.Paginacion)
	filas, err := r.db.Query(ctx, sql, w.args...)
	if err != nil {
		return nil, traducir(err, recursoUsuarios)
	}
	usuarios, err := recolectar(filas, escanearUsuario)
	return usuarios, traducir(err, recursoUsuarios)
}

// Crear inserta un usuario con sus roles y devuelve el usuario creado.
func (r *Usuarios) Crear(ctx context.Context, u dominio.Usuario) (dominio.Usuario, error) {
	err := enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO usuarios (nombre, correo, password_hash, activo) VALUES ($1, $2, $3, $4)
			 RETURNING id, creado_en`,
			u.Nombre, u.Correo, u.PasswordHash, u.Activo,
		).Scan(&u.ID, &u.CreadoEn)
		if esViolacionUnica(err) {
			return apperr.Conflicto("ya existe un usuario con el correo %s", u.Correo)
		}
		if err != nil {
			return traducir(err, recursoUsuario)
		}
		return insertarRoles(ctx, tx, u.ID, u.Roles)
	})
	return u, err
}

// ReemplazarRoles sustituye los roles del usuario por los dados (RF-02.2).
func (r *Usuarios) ReemplazarRoles(ctx context.Context, usuarioID string, roles []dominio.Rol) error {
	return enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM usuario_roles WHERE usuario_id = $1`, usuarioID); err != nil {
			return traducir(err, "roles")
		}
		if _, err := tx.Exec(ctx, `UPDATE usuarios SET actualizado_en = now() WHERE id = $1`, usuarioID); err != nil {
			return traducir(err, recursoUsuario)
		}
		return insertarRoles(ctx, tx, usuarioID, roles)
	})
}

func insertarRoles(ctx context.Context, tx pgx.Tx, usuarioID string, roles []dominio.Rol) error {
	for _, rol := range roles {
		if _, err := tx.Exec(ctx,
			`INSERT INTO usuario_roles (usuario_id, rol) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			usuarioID, string(rol)); err != nil {
			return traducir(err, "rol")
		}
	}
	return nil
}

// TieneVinculacionesActivas indica si el usuario tiene vinculaciones con el rol
// dado en cursos o proyectos que no se han cerrado.
func (r *Usuarios) TieneVinculacionesActivas(ctx context.Context, usuarioID string, rol dominio.Rol) (bool, error) {
	var existe bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM vinculaciones v JOIN espacios e ON e.id = v.espacio_id
		 WHERE v.usuario_id = $1 AND v.rol = $2 AND e.estado = 'activo')`,
		usuarioID, string(rol)).Scan(&existe)
	return existe, traducir(err, recursoVinculaciones)
}

// TieneEspaciosActivos indica si el usuario es profesor de algún espacio no cerrado.
func (r *Usuarios) TieneEspaciosActivos(ctx context.Context, usuarioID string) (bool, error) {
	var existe bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM espacios WHERE profesor_id = $1 AND estado = 'activo')`,
		usuarioID).Scan(&existe)
	return existe, traducir(err, "espacios")
}
