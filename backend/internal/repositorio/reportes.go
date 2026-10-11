package repositorio

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"seguimiento/internal/dominio"
)

// Reportes gestiona la persistencia de lotes y reportes semanales, y lee los
// datos de tareas y vinculaciones que alimentan cada reporte.
type Reportes struct {
	db *pgxpool.Pool
}

// NuevoReportes crea el repositorio de reportes.
func NuevoReportes(db *pgxpool.Pool) *Reportes { return &Reportes{db: db} }

// ---------------------------------------------------------------------------
// Lotes
// ---------------------------------------------------------------------------

const columnasLote = `id, profesor_id, semana_inicio, estado, total_reportes, reportes_completados,
	reportes_fallidos, error, creado_en, actualizado_en, finalizado_en`

func escanearLote(fila pgx.Row) (dominio.LoteReportes, error) {
	var l dominio.LoteReportes
	var estado string
	err := fila.Scan(&l.ID, &l.ProfesorID, &l.SemanaInicio, &estado, &l.TotalReportes, &l.ReportesCompletados,
		&l.ReportesFallidos, &l.Error, &l.CreadoEn, &l.ActualizadoEn, &l.FinalizadoEn)
	l.Estado = dominio.EstadoLote(estado)
	return l, err
}

// CrearLote registra un lote pendiente para el profesor y la semana dados.
func (r *Reportes) CrearLote(ctx context.Context, profesorID string, semana time.Time) (dominio.LoteReportes, error) {
	l, err := escanearLote(r.db.QueryRow(ctx,
		`INSERT INTO lotes_reportes (profesor_id, semana_inicio) VALUES ($1, $2) RETURNING `+columnasLote,
		profesorID, semana))
	return l, traducir(err, "lote de reportes")
}

// ExisteLoteActivo indica si el profesor ya tiene un lote en curso para la semana.
func (r *Reportes) ExisteLoteActivo(ctx context.Context, profesorID string, semana time.Time) (bool, error) {
	var existe bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM lotes_reportes
		 WHERE profesor_id = $1 AND semana_inicio = $2 AND estado IN ('pendiente', 'en_proceso'))`,
		profesorID, semana).Scan(&existe)
	return existe, traducir(err, "lotes de reportes")
}

// ObtenerLote devuelve el lote con el identificador dado.
func (r *Reportes) ObtenerLote(ctx context.Context, id string) (dominio.LoteReportes, error) {
	l, err := escanearLote(r.db.QueryRow(ctx, `SELECT `+columnasLote+` FROM lotes_reportes WHERE id = $1`, id))
	return l, traducir(err, "lote de reportes")
}

// ListarLotes devuelve los lotes que cumplen el filtro, del más reciente al más antiguo.
func (r *Reportes) ListarLotes(ctx context.Context, f dominio.FiltroLotes) ([]dominio.LoteReportes, error) {
	var w filtro
	if f.ProfesorID != "" {
		w.agregar("profesor_id = ?", f.ProfesorID)
	}
	if f.SemanaInicio != nil {
		w.agregar("semana_inicio = ?", *f.SemanaInicio)
	}
	sql := `SELECT ` + columnasLote + ` FROM lotes_reportes` + w.where() + ` ORDER BY creado_en DESC` + w.paginar(f.Paginacion)
	filas, err := r.db.Query(ctx, sql, w.args...)
	if err != nil {
		return nil, traducir(err, "lotes de reportes")
	}
	lotes, err := recolectar(filas, escanearLote)
	return lotes, traducir(err, "lotes de reportes")
}

// IniciarLote fija el total de reportes y pasa el lote a "en_proceso".
// Un lote sin personas queda completado de inmediato.
func (r *Reportes) IniciarLote(ctx context.Context, id string, total int) error {
	_, err := r.db.Exec(ctx,
		`UPDATE lotes_reportes SET
			total_reportes = $2,
			estado = CASE WHEN $2 = 0 THEN 'completado' ELSE 'en_proceso' END,
			finalizado_en = CASE WHEN $2 = 0 THEN now() ELSE NULL END,
			actualizado_en = now()
		 WHERE id = $1 AND estado IN ('pendiente', 'en_proceso')`, id, total)
	return traducir(err, "lote de reportes")
}

// MarcarLoteFallido registra que el lote no pudo procesarse.
func (r *Reportes) MarcarLoteFallido(ctx context.Context, id, motivo string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE lotes_reportes SET estado = 'fallido', error = $2, finalizado_en = now(), actualizado_en = now()
		 WHERE id = $1 AND estado IN ('pendiente', 'en_proceso')`, id, recortarTexto(motivo))
	return traducir(err, "lote de reportes")
}

// ---------------------------------------------------------------------------
// Reportes
// ---------------------------------------------------------------------------

const selectReporte = `
	SELECT r.id, r.lote_id, r.profesor_id, r.usuario_id, u.nombre, r.semana_inicio, r.estado,
	       r.resumen, r.ruta_pdf, r.tamano_bytes, r.intentos, r.error, r.proveedor_ia, r.modelo_ia,
	       r.creado_en, r.actualizado_en, r.generado_en
	FROM reportes_semanales r
	JOIN usuarios u ON u.id = r.usuario_id`

func escanearReporte(fila pgx.Row) (dominio.ReporteSemanal, error) {
	var rep dominio.ReporteSemanal
	var estado string
	err := fila.Scan(&rep.ID, &rep.LoteID, &rep.ProfesorID, &rep.UsuarioID, &rep.Persona, &rep.SemanaInicio, &estado,
		&rep.Resumen, &rep.RutaPDF, &rep.TamanoBytes, &rep.Intentos, &rep.Error, &rep.ProveedorIA, &rep.ModeloIA,
		&rep.CreadoEn, &rep.ActualizadoEn, &rep.GeneradoEn)
	rep.Estado = dominio.EstadoReporte(estado)
	return rep, err
}

// ListarPersonasParaReporte devuelve los usuarios vinculados a cursos o
// proyectos del profesor que estuvieron vigentes durante la semana dada.
func (r *Reportes) ListarPersonasParaReporte(ctx context.Context, profesorID string, inicio, fin time.Time) ([]string, error) {
	filas, err := r.db.Query(ctx,
		`SELECT DISTINCT v.usuario_id::text
		 FROM vinculaciones v
		 JOIN espacios e ON e.id = v.espacio_id
		 WHERE e.profesor_id = $1 AND e.fecha_inicio <= $3 AND e.fecha_cierre >= $2
		 ORDER BY 1`, profesorID, inicio, fin)
	if err != nil {
		return nil, traducir(err, "personas para reporte")
	}
	ids, err := pgx.CollectRows(filas, pgx.RowTo[string])
	return ids, traducir(err, "personas para reporte")
}

// CrearReportesPendientes registra un reporte pendiente por persona (idempotente)
// y devuelve los identificadores de todos los reportes del lote.
func (r *Reportes) CrearReportesPendientes(ctx context.Context, loteID, profesorID string, semana time.Time, usuarioIDs []string) ([]string, error) {
	if len(usuarioIDs) > 0 {
		_, err := r.db.Exec(ctx,
			`INSERT INTO reportes_semanales (lote_id, profesor_id, usuario_id, semana_inicio)
			 SELECT $1, $2, u, $3 FROM unnest($4::uuid[]) AS u
			 ON CONFLICT (lote_id, usuario_id) DO NOTHING`,
			loteID, profesorID, semana, usuarioIDs)
		if err != nil {
			return nil, traducir(err, "reportes")
		}
	}
	filas, err := r.db.Query(ctx, `SELECT id::text FROM reportes_semanales WHERE lote_id = $1 ORDER BY creado_en, id`, loteID)
	if err != nil {
		return nil, traducir(err, "reportes")
	}
	ids, err := pgx.CollectRows(filas, pgx.RowTo[string])
	return ids, traducir(err, "reportes")
}

// ObtenerReporte devuelve el reporte con el identificador dado.
func (r *Reportes) ObtenerReporte(ctx context.Context, id string) (dominio.ReporteSemanal, error) {
	rep, err := escanearReporte(r.db.QueryRow(ctx, selectReporte+` WHERE r.id = $1`, id))
	return rep, traducir(err, "reporte")
}

// ListarReportes devuelve los reportes que cumplen el filtro.
func (r *Reportes) ListarReportes(ctx context.Context, f dominio.FiltroReportes) ([]dominio.ReporteSemanal, error) {
	var w filtro
	if f.ProfesorID != "" {
		w.agregar("r.profesor_id = ?", f.ProfesorID)
	}
	if f.UsuarioID != "" {
		w.agregar("r.usuario_id = ?", f.UsuarioID)
	}
	if f.LoteID != "" {
		w.agregar("r.lote_id = ?", f.LoteID)
	}
	if f.SemanaInicio != nil {
		w.agregar("r.semana_inicio = ?", *f.SemanaInicio)
	}
	sql := selectReporte + w.where() + ` ORDER BY r.semana_inicio DESC, lower(u.nombre), r.creado_en DESC` + w.paginar(f.Paginacion)
	filas, err := r.db.Query(ctx, sql, w.args...)
	if err != nil {
		return nil, traducir(err, "reportes")
	}
	reportes, err := recolectar(filas, escanearReporte)
	return reportes, traducir(err, "reportes")
}

// MarcarReporteEnProceso registra el inicio de un intento de generación.
func (r *Reportes) MarcarReporteEnProceso(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE reportes_semanales SET estado = 'en_proceso', intentos = intentos + 1, actualizado_en = now()
		 WHERE id = $1 AND estado IN ('pendiente', 'en_proceso')`, id)
	return traducir(err, "reporte")
}

// CompletarReporte guarda el resultado del reporte y actualiza el avance del lote
// en la misma transacción.
func (r *Reportes) CompletarReporte(ctx context.Context, id string, res dominio.ResultadoReporte) error {
	return r.finalizarReporte(ctx,
		`UPDATE reportes_semanales SET estado = 'completado', resumen = $2, ruta_pdf = $3, tamano_bytes = $4,
			proveedor_ia = $5, modelo_ia = $6, error = '', generado_en = now(), actualizado_en = now()
		 WHERE id = $1 AND estado NOT IN ('completado', 'fallido')
		 RETURNING lote_id`,
		[]any{id, res.Resumen, res.RutaPDF, res.TamanoBytes, res.ProveedorIA, res.ModeloIA}, 1, 0)
}

// FallarReporte registra que el reporte no pudo generarse y actualiza el lote.
func (r *Reportes) FallarReporte(ctx context.Context, id, motivo string) error {
	return r.finalizarReporte(ctx,
		`UPDATE reportes_semanales SET estado = 'fallido', error = $2, actualizado_en = now()
		 WHERE id = $1 AND estado NOT IN ('completado', 'fallido')
		 RETURNING lote_id`,
		[]any{id, recortarTexto(motivo)}, 0, 1)
}

// finalizarReporte aplica el cambio de estado del reporte y, si ocurrió,
// incrementa los contadores del lote. Cuando todos los reportes terminaron,
// el lote pasa a su estado final. El bloqueo de fila del UPDATE serializa
// las actualizaciones concurrentes de varios workers sobre el mismo lote.
func (r *Reportes) finalizarReporte(ctx context.Context, sqlReporte string, args []any, completados, fallidos int) error {
	return enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		var loteID string
		err := tx.QueryRow(ctx, sqlReporte, args...).Scan(&loteID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // el reporte ya estaba en un estado final: nada que hacer
		}
		if err != nil {
			return traducir(err, "reporte")
		}
		_, err = tx.Exec(ctx,
			`UPDATE lotes_reportes l SET
				reportes_completados = l.reportes_completados + $2,
				reportes_fallidos    = l.reportes_fallidos + $3,
				estado = CASE
					WHEN l.estado = 'en_proceso'
					 AND l.reportes_completados + $2 + l.reportes_fallidos + $3 >= l.total_reportes THEN
						CASE
							WHEN l.reportes_fallidos + $3 = 0 THEN 'completado'
							WHEN l.reportes_completados + $2 = 0 THEN 'fallido'
							ELSE 'completado_con_errores'
						END
					ELSE l.estado
				END,
				finalizado_en = CASE
					WHEN l.estado = 'en_proceso'
					 AND l.reportes_completados + $2 + l.reportes_fallidos + $3 >= l.total_reportes THEN now()
					ELSE l.finalizado_en
				END,
				actualizado_en = now()
			 WHERE l.id = $1`, loteID, completados, fallidos)
		return traducir(err, "lote de reportes")
	})
}

// ---------------------------------------------------------------------------
// Datos de la semana
// ---------------------------------------------------------------------------

// ObtenerDatosSemana reúne las vinculaciones de la persona con el profesor y
// las tareas que registró durante la semana. Solo lee los insumos que
// permite RF-14.3 (tareas, descripciones, observaciones y horas).
func (r *Reportes) ObtenerDatosSemana(ctx context.Context, profesorID, usuarioID string, semana time.Time) (dominio.DatosSemanaPersona, error) {
	d := dominio.DatosSemanaPersona{UsuarioID: usuarioID, ProfesorID: profesorID, SemanaInicio: semana}
	fin := dominio.FinSemana(semana)

	err := r.db.QueryRow(ctx,
		`SELECT u.nombre, u.correo, p.nombre FROM usuarios u, usuarios p WHERE u.id = $1 AND p.id = $2`,
		usuarioID, profesorID).Scan(&d.Persona, &d.Correo, &d.Profesor)
	if err != nil {
		return d, traducir(err, "persona o profesor")
	}

	filas, err := r.db.Query(ctx,
		`SELECT v.id::text, e.nombre, e.tipo, v.rol, v.horas_semanales
		 FROM vinculaciones v
		 JOIN espacios e ON e.id = v.espacio_id
		 WHERE v.usuario_id = $1 AND e.profesor_id = $2 AND e.fecha_inicio <= $4 AND e.fecha_cierre >= $3
		 ORDER BY e.nombre, v.rol`, usuarioID, profesorID, semana, fin)
	if err != nil {
		return d, traducir(err, "vinculaciones")
	}
	type fila struct {
		id string
		v  dominio.VinculacionSemana
	}
	leidas, err := recolectar(filas, func(f pgx.Row) (fila, error) {
		var x fila
		var tipo, rol string
		err := f.Scan(&x.id, &x.v.EspacioNombre, &tipo, &rol, &x.v.HorasContratadas)
		x.v.EspacioTipo, x.v.Rol = dominio.TipoEspacio(tipo), dominio.Rol(rol)
		return x, err
	})
	if err != nil {
		return d, traducir(err, "vinculaciones")
	}
	if len(leidas) == 0 {
		return d, nil
	}
	indice := map[string]int{}
	ids := make([]string, 0, len(leidas))
	for i, x := range leidas {
		indice[x.id] = i
		ids = append(ids, x.id)
		d.Vinculaciones = append(d.Vinculaciones, x.v)
	}

	filas, err = r.db.Query(ctx,
		`SELECT vinculacion_id::text, titulo, descripcion, estado, horas_invertidas, observaciones, tardia
		 FROM tareas
		 WHERE vinculacion_id = ANY($1::uuid[]) AND semana_inicio = $2
		 ORDER BY creado_en, id`, ids, semana)
	if err != nil {
		return d, traducir(err, "tareas")
	}
	defer filas.Close()
	for filas.Next() {
		var vincID, estado string
		var t dominio.TareaSemana
		if err := filas.Scan(&vincID, &t.Titulo, &t.Descripcion, &estado, &t.Horas, &t.Observaciones, &t.Tardia); err != nil {
			return d, traducir(err, "tareas")
		}
		t.Estado = dominio.EstadoTarea(estado)
		i := indice[vincID]
		d.Vinculaciones[i].Tareas = append(d.Vinculaciones[i].Tareas, t)
	}
	return d, traducir(filas.Err(), "tareas")
}
