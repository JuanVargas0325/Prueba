// Package repositorio implementa la capa de acceso a datos sobre PostgreSQL.
// Cada repositorio encapsula las consultas SQL de un agregado del dominio y
// traduce los errores de la base de datos a errores de aplicación (apperr).
package repositorio

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"seguimiento/internal/apperr"
	"seguimiento/internal/dominio"
)

// consultor es lo común entre el pool de conexiones y una transacción, para
// reutilizar las mismas consultas dentro y fuera de transacciones.
type consultor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Nombres de los recursos que aparecen en los mensajes de error de la capa de datos.
const (
	recursoUsuario       = "usuario"
	recursoUsuarios      = "usuarios"
	recursoPeriodo       = "período"
	recursoPeriodos      = "períodos"
	recursoEspacio       = "curso o proyecto"
	recursoEspacios      = "cursos y proyectos"
	recursoVinculacion   = "vinculación"
	recursoVinculaciones = "vinculaciones"
	recursoTarea         = "tarea"
	recursoTareas        = "tareas"
	recursoAdjunto       = "adjunto"
	recursoAdjuntos      = "adjuntos"
)

// enTransaccion ejecuta fn dentro de una transacción: confirma si fn termina
// sin error y revierte en cualquier otro caso.
func enTransaccion(ctx context.Context, db *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciando transacción: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no hace nada si ya se confirmó

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmando transacción: %w", err)
	}
	return nil
}

// Códigos de error de PostgreSQL que se traducen a errores de aplicación.
const (
	codigoUnico         = "23505"
	codigoLlaveForanea  = "23503"
	codigoRestriccion   = "23514"
	codigoTextoInvalido = "22P02"
)

// traducir convierte errores de la base de datos en errores de aplicación.
// recurso se usa en el mensaje de "no encontrado" (por ejemplo "tarea").
func traducir(err error, recurso string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.NoEncontrado("%s no encontrado", recurso)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case codigoUnico:
			return apperr.Conflicto("%s ya existe", recurso)
		case codigoLlaveForanea:
			return apperr.Validacion("%s hace referencia a un registro que no existe", recurso)
		case codigoRestriccion, codigoTextoInvalido:
			return apperr.Validacion("los datos de %s no son válidos", recurso)
		}
	}
	return fmt.Errorf("%s: %w", recurso, err)
}

// esViolacionUnica indica si el error es una violación de unicidad.
func esViolacionUnica(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codigoUnico
}

// filtro construye cláusulas WHERE con parámetros numerados ($1, $2...).
type filtro struct {
	condiciones []string
	args        []any
}

// agregar añade una condición. Cada "?" de la expresión se reemplaza por el
// mismo parámetro, así una expresión puede usar el valor varias veces.
func (f *filtro) agregar(expresion string, valor any) {
	f.args = append(f.args, valor)
	f.condiciones = append(f.condiciones, strings.ReplaceAll(expresion, "?", fmt.Sprintf("$%d", len(f.args))))
}

// agregarFija añade una condición sin parámetros.
func (f *filtro) agregarFija(expresion string) {
	f.condiciones = append(f.condiciones, expresion)
}

// where devuelve la cláusula WHERE (vacía si no hay condiciones).
func (f *filtro) where() string {
	if len(f.condiciones) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(f.condiciones, " AND ")
}

// paginar devuelve la cláusula LIMIT/OFFSET normalizada, o nada si se pidieron
// todos los resultados (dominio.SinPaginar).
func (f *filtro) paginar(p dominio.Paginacion) string {
	if p == dominio.SinPaginar {
		return ""
	}
	p = p.Normalizar()
	f.args = append(f.args, p.Limite, p.Desplazamiento)
	return fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(f.args)-1, len(f.args))
}

// patronBusqueda escapa los comodines de LIKE y envuelve el texto con %.
func patronBusqueda(texto string) string {
	escapado := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(texto)
	return "%" + escapado + "%"
}

// recortarTexto limita los mensajes que se guardan en la base de datos.
func recortarTexto(s string) string {
	const maximo = 1000
	if r := []rune(s); len(r) > maximo {
		return string(r[:maximo])
	}
	return s
}

// recolectar lee todas las filas con la función de escaneo dada.
func recolectar[T any](filas pgx.Rows, escanear func(pgx.Row) (T, error)) ([]T, error) {
	defer filas.Close()
	resultado := []T{}
	for filas.Next() {
		v, err := escanear(filas)
		if err != nil {
			return nil, err
		}
		resultado = append(resultado, v)
	}
	return resultado, filas.Err()
}
