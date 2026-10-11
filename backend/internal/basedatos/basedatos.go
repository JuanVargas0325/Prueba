// Package basedatos gestiona la conexión a PostgreSQL y las migraciones del esquema.
package basedatos

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migraciones/*.sql
var archivosMigracion embed.FS

// Conectar abre el pool de conexiones y reintenta mientras la base de datos
// arranca (útil cuando los contenedores inician al mismo tiempo).
func Conectar(ctx context.Context, url string, log *slog.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("configuración de base de datos inválida: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creando pool de conexiones: %w", err)
	}

	const intentos = 15
	for i := 1; ; i++ {
		ctxPing, cancelar := context.WithTimeout(ctx, 3*time.Second)
		err = pool.Ping(ctxPing)
		cancelar()
		if err == nil {
			return pool, nil
		}
		if i == intentos {
			pool.Close()
			return nil, fmt.Errorf("no fue posible conectarse a PostgreSQL: %w", err)
		}
		log.Warn("esperando a PostgreSQL", "intento", i, "error", err)
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// claveBloqueoMigraciones evita que dos instancias apliquen migraciones a la vez.
const claveBloqueoMigraciones = 4204_2026

// Migrar aplica, en orden, los archivos de migraciones que aún no se han aplicado.
func Migrar(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("obteniendo conexión: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", claveBloqueoMigraciones); err != nil {
		return fmt.Errorf("bloqueando migraciones: %w", err)
	}
	defer conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", claveBloqueoMigraciones) //nolint:errcheck

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS migraciones_aplicadas (
		version     TEXT PRIMARY KEY,
		aplicada_en TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("creando tabla de migraciones: %w", err)
	}

	nombres, err := fs.Glob(archivosMigracion, "migraciones/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(nombres)

	for _, nombre := range nombres {
		version := strings.TrimSuffix(strings.TrimPrefix(nombre, "migraciones/"), ".sql")
		var aplicada bool
		if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM migraciones_aplicadas WHERE version = $1)", version).Scan(&aplicada); err != nil {
			return fmt.Errorf("consultando migración %s: %w", version, err)
		}
		if aplicada {
			continue
		}
		sql, err := archivosMigracion.ReadFile(nombre)
		if err != nil {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("aplicando migración %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO migraciones_aplicadas (version) VALUES ($1)", version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("registrando migración %s: %w", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("confirmando migración %s: %w", version, err)
		}
		log.Info("migración aplicada", "version", version)
	}
	return nil
}
