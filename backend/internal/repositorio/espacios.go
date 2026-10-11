package repositorio

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"seguimiento/internal/apperr"
	"seguimiento/internal/dominio"
)

// Espacios gestiona la persistencia de cursos y proyectos.
type Espacios struct {
	db *pgxpool.Pool
}

// NuevoEspacios crea el repositorio de espacios.
func NuevoEspacios(db *pgxpool.Pool) *Espacios { return &Espacios{db: db} }

const selectEspacio = `
	SELECT e.id, e.tipo, e.nombre, e.periodo_id, p.codigo, e.profesor_id, u.nombre,
	       e.fecha_inicio, e.fecha_cierre, e.observaciones, e.estado, e.creado_en
	FROM espacios e
	JOIN periodos_academicos p ON p.id = e.periodo_id
	JOIN usuarios u ON u.id = e.profesor_id`

func escanearEspacio(fila pgx.Row) (dominio.Espacio, error) {
	var e dominio.Espacio
	var tipo, estado string
	err := fila.Scan(&e.ID, &tipo, &e.Nombre, &e.PeriodoID, &e.PeriodoCodigo, &e.ProfesorID, &e.ProfesorNombre,
		&e.FechaInicio, &e.FechaCierre, &e.Observaciones, &estado, &e.CreadoEn)
	e.Tipo, e.Estado = dominio.TipoEspacio(tipo), dominio.EstadoEspacio(estado)
	return e, err
}

// Crear registra un curso o proyecto y lo devuelve con sus datos relacionados.
func (r *Espacios) Crear(ctx context.Context, e dominio.Espacio) (dominio.Espacio, error) {
	var id string
	err := r.db.QueryRow(ctx,
		`INSERT INTO espacios (tipo, nombre, periodo_id, profesor_id, fecha_inicio, fecha_cierre, observaciones)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		string(e.Tipo), e.Nombre, e.PeriodoID, e.ProfesorID, e.FechaInicio, e.FechaCierre, e.Observaciones,
	).Scan(&id)
	if err != nil {
		return e, traducir(err, "curso o proyecto")
	}
	return r.Obtener(ctx, id)
}

// Obtener devuelve el espacio con el identificador dado.
func (r *Espacios) Obtener(ctx context.Context, id string) (dominio.Espacio, error) {
	e, err := escanearEspacio(r.db.QueryRow(ctx, selectEspacio+` WHERE e.id = $1`, id))
	return e, traducir(err, "curso o proyecto")
}

// Listar devuelve los espacios que cumplen el filtro.
func (r *Espacios) Listar(ctx context.Context, f dominio.FiltroEspacios) ([]dominio.Espacio, error) {
	var w filtro
	if f.VisiblePara != "" {
		w.agregar(`(e.profesor_id = ? OR EXISTS (SELECT 1 FROM vinculaciones v WHERE v.espacio_id = e.id AND v.usuario_id = ?))`, f.VisiblePara)
	}
	if f.PeriodoID != "" {
		w.agregar("e.periodo_id = ?", f.PeriodoID)
	}
	if f.Estado != "" {
		w.agregar("e.estado = ?", string(f.Estado))
	}
	if f.Tipo != "" {
		w.agregar("e.tipo = ?", string(f.Tipo))
	}
	sql := selectEspacio + w.where() + ` ORDER BY e.estado, lower(e.nombre)` + w.paginar(f.Paginacion)
	filas, err := r.db.Query(ctx, sql, w.args...)
	if err != nil {
		return nil, traducir(err, "cursos y proyectos")
	}
	espacios, err := recolectar(filas, escanearEspacio)
	return espacios, traducir(err, "cursos y proyectos")
}

// Cerrar pasa el espacio a estado cerrado (RF-03.5).
func (r *Espacios) Cerrar(ctx context.Context, id string) (dominio.Espacio, error) {
	etiqueta, err := r.db.Exec(ctx,
		`UPDATE espacios SET estado = 'cerrado', actualizado_en = now() WHERE id = $1 AND estado = 'activo'`, id)
	if err != nil {
		return dominio.Espacio{}, traducir(err, "curso o proyecto")
	}
	if etiqueta.RowsAffected() == 0 {
		return dominio.Espacio{}, apperr.Conflicto("el curso o proyecto ya está cerrado")
	}
	return r.Obtener(ctx, id)
}

// EsParticipante indica si el usuario tiene alguna vinculación en el espacio.
func (r *Espacios) EsParticipante(ctx context.Context, espacioID, usuarioID string) (bool, error) {
	var existe bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM vinculaciones WHERE espacio_id = $1 AND usuario_id = $2)`,
		espacioID, usuarioID).Scan(&existe)
	return existe, traducir(err, "vinculación")
}
