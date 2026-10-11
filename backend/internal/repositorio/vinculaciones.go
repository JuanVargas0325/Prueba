package repositorio

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"seguimiento/internal/apperr"
	"seguimiento/internal/dominio"
)

// Vinculaciones gestiona la persistencia de las vinculaciones y sus rectificaciones.
type Vinculaciones struct {
	db *pgxpool.Pool
}

// NuevoVinculaciones crea el repositorio de vinculaciones.
func NuevoVinculaciones(db *pgxpool.Pool) *Vinculaciones { return &Vinculaciones{db: db} }

const selectVinculacion = `
	SELECT v.id, v.usuario_id, u.nombre, u.correo, v.espacio_id, e.nombre, e.tipo, e.estado,
	       e.fecha_inicio, e.fecha_cierre, e.profesor_id, p.nombre, v.rol, v.horas_semanales,
	       v.creado_en, v.actualizado_en
	FROM vinculaciones v
	JOIN usuarios u ON u.id = v.usuario_id
	JOIN espacios e ON e.id = v.espacio_id
	JOIN usuarios p ON p.id = e.profesor_id`

func escanearVinculacion(fila pgx.Row) (dominio.Vinculacion, error) {
	var v dominio.Vinculacion
	var tipo, estado, rol string
	err := fila.Scan(&v.ID, &v.UsuarioID, &v.UsuarioNombre, &v.UsuarioCorreo, &v.EspacioID, &v.EspacioNombre, &tipo, &estado,
		&v.EspacioFechaInicio, &v.EspacioFechaCierre, &v.ProfesorID, &v.ProfesorNombre, &rol, &v.HorasSemanales,
		&v.CreadoEn, &v.ActualizadoEn)
	v.EspacioTipo, v.EspacioEstado, v.Rol = dominio.TipoEspacio(tipo), dominio.EstadoEspacio(estado), dominio.Rol(rol)
	return v, err
}

// Crear registra la vinculación si el validador la aprueba. La validación y
// la inserción ocurren en una transacción que bloquea la fila del usuario, de
// modo que dos vinculaciones simultáneas no puedan superar juntas los límites.
func (r *Vinculaciones) Crear(ctx context.Context, v dominio.Vinculacion, validar dominio.ValidadorCarga) (dominio.Vinculacion, error) {
	var id string
	err := enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		if err := bloquearUsuario(ctx, tx, v.UsuarioID); err != nil {
			return err
		}
		// La duplicidad se informa antes que las reglas de horas: es la causa real del rechazo.
		var existe bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM vinculaciones WHERE usuario_id = $1 AND espacio_id = $2 AND rol = $3)`,
			v.UsuarioID, v.EspacioID, string(v.Rol)).Scan(&existe); err != nil {
			return traducir(err, "vinculación")
		}
		if existe {
			return apperr.Conflicto("la persona ya está vinculada como %s en este curso o proyecto", v.Rol.Texto())
		}
		simultaneas, err := listarSimultaneas(ctx, tx, v.UsuarioID, v.EspacioFechaInicio, v.EspacioFechaCierre, "")
		if err != nil {
			return err
		}
		if err := validar(simultaneas, v); err != nil {
			return err
		}
		err = tx.QueryRow(ctx,
			`INSERT INTO vinculaciones (usuario_id, espacio_id, rol, horas_semanales) VALUES ($1, $2, $3, $4) RETURNING id`,
			v.UsuarioID, v.EspacioID, string(v.Rol), v.HorasSemanales).Scan(&id)
		if esViolacionUnica(err) {
			return apperr.Conflicto("la persona ya está vinculada como %s en este curso o proyecto", v.Rol.Texto())
		}
		return traducir(err, "vinculación")
	})
	if err != nil {
		return dominio.Vinculacion{}, err
	}
	return r.Obtener(ctx, id)
}

// Rectificar corrige el rol o las horas de una vinculación (RF-04.5) y deja
// registro de la corrección, todo en la misma transacción.
func (r *Vinculaciones) Rectificar(ctx context.Context, id string, rol dominio.Rol, horas int,
	administradorID, motivo string, validar dominio.ValidadorCarga) (dominio.Vinculacion, error) {

	err := enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		actual, err := escanearVinculacion(tx.QueryRow(ctx, selectVinculacion+` WHERE v.id = $1 FOR UPDATE OF v`, id))
		if err != nil {
			return traducir(err, "vinculación")
		}
		if err := bloquearUsuario(ctx, tx, actual.UsuarioID); err != nil {
			return err
		}
		simultaneas, err := listarSimultaneas(ctx, tx, actual.UsuarioID, actual.EspacioFechaInicio, actual.EspacioFechaCierre, id)
		if err != nil {
			return err
		}
		corregida := actual
		corregida.Rol, corregida.HorasSemanales = rol, horas
		if err := validar(simultaneas, corregida); err != nil {
			return err
		}

		_, err = tx.Exec(ctx,
			`UPDATE vinculaciones SET rol = $2, horas_semanales = $3, actualizado_en = now() WHERE id = $1`,
			id, string(rol), horas)
		if esViolacionUnica(err) {
			return apperr.Conflicto("la persona ya tiene otra vinculación como %s en este curso o proyecto", rol.Texto())
		}
		if err != nil {
			return traducir(err, "vinculación")
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO vinculacion_rectificaciones
			   (vinculacion_id, administrador_id, rol_anterior, rol_nuevo, horas_anteriores, horas_nuevas, motivo)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			id, administradorID, string(actual.Rol), string(rol), actual.HorasSemanales, horas, recortarTexto(motivo))
		return traducir(err, "rectificación")
	})
	if err != nil {
		return dominio.Vinculacion{}, err
	}
	return r.Obtener(ctx, id)
}

// Obtener devuelve la vinculación con el identificador dado.
func (r *Vinculaciones) Obtener(ctx context.Context, id string) (dominio.Vinculacion, error) {
	v, err := escanearVinculacion(r.db.QueryRow(ctx, selectVinculacion+` WHERE v.id = $1`, id))
	return v, traducir(err, "vinculación")
}

// Listar devuelve las vinculaciones que cumplen el filtro.
func (r *Vinculaciones) Listar(ctx context.Context, f dominio.FiltroVinculaciones) ([]dominio.Vinculacion, error) {
	var w filtro
	if f.VisiblePara != "" {
		w.agregar("(e.profesor_id = ? OR v.usuario_id = ?)", f.VisiblePara)
	}
	if f.ProfesorID != "" {
		w.agregar("e.profesor_id = ?", f.ProfesorID)
	}
	if f.UsuarioID != "" {
		w.agregar("v.usuario_id = ?", f.UsuarioID)
	}
	if f.EspacioID != "" {
		w.agregar("v.espacio_id = ?", f.EspacioID)
	}
	if f.SoloEspaciosActivos {
		w.agregarFija("e.estado = 'activo'")
	}
	sql := selectVinculacion + w.where() + ` ORDER BY lower(u.nombre), lower(e.nombre), v.rol` + w.paginar(f.Paginacion)
	filas, err := r.db.Query(ctx, sql, w.args...)
	if err != nil {
		return nil, traducir(err, "vinculaciones")
	}
	vinculaciones, err := recolectar(filas, escanearVinculacion)
	return vinculaciones, traducir(err, "vinculaciones")
}

// ListarRectificaciones devuelve las correcciones de una vinculación, de la más reciente a la más antigua.
func (r *Vinculaciones) ListarRectificaciones(ctx context.Context, vinculacionID string) ([]dominio.Rectificacion, error) {
	filas, err := r.db.Query(ctx,
		`SELECT id, vinculacion_id, administrador_id, rol_anterior, rol_nuevo, horas_anteriores, horas_nuevas, motivo, creado_en
		 FROM vinculacion_rectificaciones WHERE vinculacion_id = $1 ORDER BY creado_en DESC`, vinculacionID)
	if err != nil {
		return nil, traducir(err, "rectificaciones")
	}
	rectificaciones, err := recolectar(filas, func(fila pgx.Row) (dominio.Rectificacion, error) {
		var rc dominio.Rectificacion
		var anterior, nuevo string
		err := fila.Scan(&rc.ID, &rc.VinculacionID, &rc.AdministradorID, &anterior, &nuevo,
			&rc.HorasAnteriores, &rc.HorasNuevas, &rc.Motivo, &rc.CreadoEn)
		rc.RolAnterior, rc.RolNuevo = dominio.Rol(anterior), dominio.Rol(nuevo)
		return rc, err
	})
	return rectificaciones, traducir(err, "rectificaciones")
}

// bloquearUsuario serializa las operaciones que validan la carga horaria de una persona.
func bloquearUsuario(ctx context.Context, tx pgx.Tx, usuarioID string) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM usuarios WHERE id = $1 FOR UPDATE`, usuarioID).Scan(&id)
	return traducir(err, "usuario")
}

// listarSimultaneas devuelve las vinculaciones de la persona en espacios activos
// cuyas fechas se cruzan con [inicio, cierre], excepto la indicada.
func listarSimultaneas(ctx context.Context, c consultor, usuarioID string, inicio, cierre time.Time, excepto string) ([]dominio.Vinculacion, error) {
	var w filtro
	w.agregar("v.usuario_id = ?", usuarioID)
	w.agregarFija("e.estado = 'activo'")
	w.agregar("e.fecha_inicio <= ?", cierre)
	w.agregar("e.fecha_cierre >= ?", inicio)
	if excepto != "" {
		w.agregar("v.id <> ?", excepto)
	}
	filas, err := c.Query(ctx, selectVinculacion+w.where(), w.args...)
	if err != nil {
		return nil, traducir(err, "vinculaciones")
	}
	vinculaciones, err := recolectar(filas, escanearVinculacion)
	return vinculaciones, traducir(err, "vinculaciones")
}
