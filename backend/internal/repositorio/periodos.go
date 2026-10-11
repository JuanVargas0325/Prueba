package repositorio

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"seguimiento/internal/apperr"
	"seguimiento/internal/dominio"
)

// Periodos gestiona la persistencia de los períodos académicos.
type Periodos struct {
	db *pgxpool.Pool
}

// NuevoPeriodos crea el repositorio de períodos.
func NuevoPeriodos(db *pgxpool.Pool) *Periodos { return &Periodos{db: db} }

const columnasPeriodo = `id, codigo, fecha_inicio, fecha_cierre, estado, creacion_habilitada, creado_en`

func escanearPeriodo(fila pgx.Row) (dominio.PeriodoAcademico, error) {
	var p dominio.PeriodoAcademico
	var estado string
	err := fila.Scan(&p.ID, &p.Codigo, &p.FechaInicio, &p.FechaCierre, &estado, &p.CreacionHabilitada, &p.CreadoEn)
	p.Estado = dominio.EstadoPeriodo(estado)
	return p, err
}

// Crear registra un período académico.
func (r *Periodos) Crear(ctx context.Context, p dominio.PeriodoAcademico) (dominio.PeriodoAcademico, error) {
	creado, err := escanearPeriodo(r.db.QueryRow(ctx,
		`INSERT INTO periodos_academicos (codigo, fecha_inicio, fecha_cierre, estado, creacion_habilitada)
		 VALUES ($1, $2, $3, $4, $5) RETURNING `+columnasPeriodo,
		p.Codigo, p.FechaInicio, p.FechaCierre, string(p.Estado), p.CreacionHabilitada))
	if esViolacionUnica(err) {
		return creado, apperr.Conflicto("ya existe el período %s", p.Codigo)
	}
	return creado, traducir(err, "período")
}

// Obtener devuelve el período con el identificador dado.
func (r *Periodos) Obtener(ctx context.Context, id string) (dominio.PeriodoAcademico, error) {
	p, err := escanearPeriodo(r.db.QueryRow(ctx, `SELECT `+columnasPeriodo+` FROM periodos_academicos WHERE id = $1`, id))
	return p, traducir(err, "período")
}

// Listar devuelve los períodos, del más reciente al más antiguo.
func (r *Periodos) Listar(ctx context.Context, f dominio.FiltroPeriodos) ([]dominio.PeriodoAcademico, error) {
	var w filtro
	if f.Estado != "" {
		w.agregar("estado = ?", string(f.Estado))
	}
	sql := `SELECT ` + columnasPeriodo + ` FROM periodos_academicos` + w.where() +
		` ORDER BY fecha_inicio DESC` + w.paginar(f.Paginacion)
	filas, err := r.db.Query(ctx, sql, w.args...)
	if err != nil {
		return nil, traducir(err, "períodos")
	}
	periodos, err := recolectar(filas, escanearPeriodo)
	return periodos, traducir(err, "períodos")
}

// Actualizar guarda el estado y la ventana de creación del período.
func (r *Periodos) Actualizar(ctx context.Context, p dominio.PeriodoAcademico) (dominio.PeriodoAcademico, error) {
	actualizado, err := escanearPeriodo(r.db.QueryRow(ctx,
		`UPDATE periodos_academicos SET estado = $2, creacion_habilitada = $3
		 WHERE id = $1 RETURNING `+columnasPeriodo,
		p.ID, string(p.Estado), p.CreacionHabilitada))
	return actualizado, traducir(err, "período")
}
