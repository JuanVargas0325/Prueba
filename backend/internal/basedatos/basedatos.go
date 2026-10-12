// Package basedatos gestiona la conexión a PostgreSQL.
package basedatos

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
