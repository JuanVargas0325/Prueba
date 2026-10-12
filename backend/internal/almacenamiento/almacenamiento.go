package almacenamiento

import (
	"context"
	"fmt"
	"io"
)

// TipoLocal guarda los archivos en una carpeta del servidor (en Docker, un
// volumen compartido entre la API y el worker). Es el único tipo de la
// Entrega 1; en la Entrega 2 se agrega Cloud Storage.
const TipoLocal = "local"

// Config indica qué almacenamiento usar.
type Config struct {
	Tipo string // ALMACENAMIENTO_TIPO
	Ruta string // ALMACENAMIENTO_RUTA: carpeta raíz del almacenamiento local
}

// Archivos es lo que ofrece cualquier almacenamiento. Los servicios declaran
// su propia interfaz equivalente (servicio.Almacenamiento).
type Archivos interface {
	Guardar(ctx context.Context, ruta string, contenido io.Reader) (int64, error)
	Abrir(ctx context.Context, ruta string) (io.ReadCloser, error)
	Eliminar(ctx context.Context, ruta string) error
}

// Nuevo crea el almacenamiento configurado. Es el único lugar que cambia al
// agregar otro tipo: la API y el worker lo usan sin saber cuál es.
func Nuevo(c Config) (Archivos, error) {
	switch c.Tipo {
	case TipoLocal:
		local, err := NuevoLocal(c.Ruta)
		if err != nil {
			return nil, err
		}
		return local, nil
	default:
		return nil, fmt.Errorf("ALMACENAMIENTO_TIPO %q no es válido (use %q)", c.Tipo, TipoLocal)
	}
}
