package repositorio

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"seguimiento/internal/apperr"
	"seguimiento/internal/dominio"
)

// Tareas gestiona la persistencia de las tareas semanales y sus adjuntos.
type Tareas struct {
	db *pgxpool.Pool
}

// NuevoTareas crea el repositorio de tareas.
func NuevoTareas(db *pgxpool.Pool) *Tareas { return &Tareas{db: db} }

const selectTarea = `
	SELECT t.id, t.vinculacion_id, v.usuario_id, u.nombre, v.espacio_id, e.nombre, e.tipo, e.estado, e.profesor_id, v.rol,
	       t.semana_inicio, t.titulo, t.descripcion, t.estado, t.horas_invertidas, t.observaciones, t.tardia,
	       (SELECT count(*) FROM tarea_adjuntos a WHERE a.tarea_id = t.id),
	       t.creado_en, t.actualizado_en
	FROM tareas t
	JOIN vinculaciones v ON v.id = t.vinculacion_id
	JOIN usuarios u ON u.id = v.usuario_id
	JOIN espacios e ON e.id = v.espacio_id`

func escanearTarea(fila pgx.Row) (dominio.Tarea, error) {
	var t dominio.Tarea
	var tipo, estadoEspacio, rol, estado string
	err := fila.Scan(&t.ID, &t.VinculacionID, &t.UsuarioID, &t.UsuarioNombre, &t.EspacioID, &t.EspacioNombre, &tipo, &estadoEspacio,
		&t.ProfesorID, &rol, &t.SemanaInicio, &t.Titulo, &t.Descripcion, &estado, &t.HorasInvertidas,
		&t.Observaciones, &t.Tardia, &t.CantidadAdjuntos, &t.CreadoEn, &t.ActualizadoEn)
	t.EspacioTipo, t.EspacioEstado = dominio.TipoEspacio(tipo), dominio.EstadoEspacio(estadoEspacio)
	t.Rol, t.Estado = dominio.Rol(rol), dominio.EstadoTarea(estado)
	return t, err
}

const columnasAdjunto = `id, tarea_id, nombre_original, ruta_almacenamiento, tipo_contenido, tamano_bytes, creado_en`

func escanearAdjunto(fila pgx.Row) (dominio.Adjunto, error) {
	var a dominio.Adjunto
	err := fila.Scan(&a.ID, &a.TareaID, &a.NombreOriginal, &a.RutaAlmacenamiento, &a.TipoContenido, &a.TamanoBytes, &a.CreadoEn)
	return a, err
}

// Crear registra la tarea (con el ID que trae) y sus adjuntos en una transacción.
func (r *Tareas) Crear(ctx context.Context, t dominio.Tarea, adjuntos []dominio.Adjunto) (dominio.Tarea, error) {
	err := enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO tareas (id, vinculacion_id, semana_inicio, titulo, descripcion, estado, horas_invertidas, observaciones, tardia)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			t.ID, t.VinculacionID, t.SemanaInicio, t.Titulo, t.Descripcion, string(t.Estado), t.HorasInvertidas, t.Observaciones, t.Tardia)
		if err != nil {
			return traducir(err, "tarea")
		}
		return insertarAdjuntos(ctx, tx, adjuntos)
	})
	if err != nil {
		return dominio.Tarea{}, err
	}
	return r.Obtener(ctx, t.ID)
}

// Obtener devuelve la tarea con sus adjuntos.
func (r *Tareas) Obtener(ctx context.Context, id string) (dominio.Tarea, error) {
	t, err := escanearTarea(r.db.QueryRow(ctx, selectTarea+` WHERE t.id = $1`, id))
	if err != nil {
		return t, traducir(err, "tarea")
	}
	filas, err := r.db.Query(ctx, `SELECT `+columnasAdjunto+` FROM tarea_adjuntos WHERE tarea_id = $1 ORDER BY creado_en`, id)
	if err != nil {
		return t, traducir(err, "adjuntos")
	}
	t.Adjuntos, err = recolectar(filas, escanearAdjunto)
	return t, traducir(err, "adjuntos")
}

// Listar devuelve las tareas que cumplen el filtro, de la semana más reciente a la más antigua.
func (r *Tareas) Listar(ctx context.Context, f dominio.FiltroTareas) ([]dominio.Tarea, error) {
	var w filtro
	if f.VisiblePara != "" {
		w.agregar("(e.profesor_id = ? OR v.usuario_id = ?)", f.VisiblePara)
	}
	if f.UsuarioID != "" {
		w.agregar("v.usuario_id = ?", f.UsuarioID)
	}
	if f.EspacioID != "" {
		w.agregar("v.espacio_id = ?", f.EspacioID)
	}
	if f.VinculacionID != "" {
		w.agregar("t.vinculacion_id = ?", f.VinculacionID)
	}
	if f.SemanaInicio != nil {
		w.agregar("t.semana_inicio = ?", *f.SemanaInicio)
	}
	sql := selectTarea + w.where() + ` ORDER BY t.semana_inicio DESC, t.creado_en DESC` + w.paginar(f.Paginacion)
	filas, err := r.db.Query(ctx, sql, w.args...)
	if err != nil {
		return nil, traducir(err, "tareas")
	}
	tareas, err := recolectar(filas, escanearTarea)
	return tareas, traducir(err, "tareas")
}

// ListarPorVinculaciones devuelve todas las tareas de las vinculaciones dadas,
// opcionalmente limitadas a las semanas entre desde y hasta (ambas incluidas).
func (r *Tareas) ListarPorVinculaciones(ctx context.Context, vinculacionIDs []string, desde, hasta *time.Time) ([]dominio.Tarea, error) {
	if len(vinculacionIDs) == 0 {
		return []dominio.Tarea{}, nil
	}
	var w filtro
	w.agregar("t.vinculacion_id = ANY(?::uuid[])", vinculacionIDs)
	if desde != nil {
		w.agregar("t.semana_inicio >= ?", *desde)
	}
	if hasta != nil {
		w.agregar("t.semana_inicio <= ?", *hasta)
	}
	filas, err := r.db.Query(ctx, selectTarea+w.where()+` ORDER BY t.semana_inicio DESC, t.creado_en`, w.args...)
	if err != nil {
		return nil, traducir(err, "tareas")
	}
	tareas, err := recolectar(filas, escanearTarea)
	return tareas, traducir(err, "tareas")
}

// Actualizar guarda los campos editables. Como defensa adicional a la regla
// del servicio, nunca modifica un reporte tardío (RN-33).
func (r *Tareas) Actualizar(ctx context.Context, t dominio.Tarea) (dominio.Tarea, error) {
	etiqueta, err := r.db.Exec(ctx,
		`UPDATE tareas SET titulo = $2, descripcion = $3, estado = $4, horas_invertidas = $5, observaciones = $6,
		        actualizado_en = now()
		 WHERE id = $1 AND tardia = FALSE`,
		t.ID, t.Titulo, t.Descripcion, string(t.Estado), t.HorasInvertidas, t.Observaciones)
	if err != nil {
		return t, traducir(err, "tarea")
	}
	if etiqueta.RowsAffected() == 0 {
		return t, apperr.ReglaNegocio("la tarea no se puede modificar")
	}
	return r.Obtener(ctx, t.ID)
}

// Eliminar borra la tarea y sus adjuntos, y devuelve las rutas de los archivos
// para que el servicio los elimine del almacenamiento.
func (r *Tareas) Eliminar(ctx context.Context, id string) ([]string, error) {
	var rutas []string
	err := enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `SELECT ruta_almacenamiento FROM tarea_adjuntos WHERE tarea_id = $1`, id)
		if err != nil {
			return traducir(err, "adjuntos")
		}
		rutas, err = pgx.CollectRows(filas, pgx.RowTo[string])
		if err != nil {
			return traducir(err, "adjuntos")
		}
		etiqueta, err := tx.Exec(ctx, `DELETE FROM tareas WHERE id = $1 AND tardia = FALSE`, id)
		if err != nil {
			return traducir(err, "tarea")
		}
		if etiqueta.RowsAffected() == 0 {
			return apperr.ReglaNegocio("la tarea no se puede eliminar")
		}
		return nil
	})
	return rutas, err
}

// AgregarAdjuntos registra adjuntos de una tarea sin superar el máximo permitido.
func (r *Tareas) AgregarAdjuntos(ctx context.Context, tareaID string, adjuntos []dominio.Adjunto, maximo int) error {
	return enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, `SELECT id FROM tareas WHERE id = $1 FOR UPDATE`, tareaID).Scan(&id); err != nil {
			return traducir(err, "tarea")
		}
		var actuales int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM tarea_adjuntos WHERE tarea_id = $1`, tareaID).Scan(&actuales); err != nil {
			return traducir(err, "adjuntos")
		}
		if actuales+len(adjuntos) > maximo {
			return apperr.ReglaNegocio("una tarea admite máximo %d adjuntos (ya tiene %d)", maximo, actuales)
		}
		return insertarAdjuntos(ctx, tx, adjuntos)
	})
}

// ObtenerAdjunto devuelve un adjunto de la tarea.
func (r *Tareas) ObtenerAdjunto(ctx context.Context, tareaID, adjuntoID string) (dominio.Adjunto, error) {
	a, err := escanearAdjunto(r.db.QueryRow(ctx,
		`SELECT `+columnasAdjunto+` FROM tarea_adjuntos WHERE id = $1 AND tarea_id = $2`, adjuntoID, tareaID))
	return a, traducir(err, "adjunto")
}

// EliminarAdjunto borra el registro del adjunto y devuelve la ruta del archivo.
func (r *Tareas) EliminarAdjunto(ctx context.Context, tareaID, adjuntoID string) (string, error) {
	var ruta string
	err := r.db.QueryRow(ctx,
		`DELETE FROM tarea_adjuntos WHERE id = $1 AND tarea_id = $2 RETURNING ruta_almacenamiento`,
		adjuntoID, tareaID).Scan(&ruta)
	return ruta, traducir(err, "adjunto")
}

func insertarAdjuntos(ctx context.Context, tx pgx.Tx, adjuntos []dominio.Adjunto) error {
	for _, a := range adjuntos {
		_, err := tx.Exec(ctx,
			`INSERT INTO tarea_adjuntos (id, tarea_id, nombre_original, ruta_almacenamiento, tipo_contenido, tamano_bytes)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			a.ID, a.TareaID, a.NombreOriginal, a.RutaAlmacenamiento, a.TipoContenido, a.TamanoBytes)
		if err != nil {
			return traducir(err, "adjunto")
		}
	}
	return nil
}
