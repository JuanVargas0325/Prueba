// Package almacenamiento guarda los archivos binarios (PDF de reportes y
// adjuntos de tareas). En la Entrega 1 se usa un volumen de Docker montado en
// el contenedor; en entregas posteriores puede añadirse una implementación
// sobre Cloud Storage que cumpla la misma interfaz del servicio.
package almacenamiento

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"seguimiento/internal/apperr"
)

// Local guarda archivos en un directorio del sistema de archivos.
type Local struct {
	raiz string
}

// NuevoLocal crea el almacenamiento y su directorio raíz si no existe.
func NuevoLocal(raiz string) (*Local, error) {
	abs, err := filepath.Abs(raiz)
	if err != nil {
		return nil, fmt.Errorf("ruta de almacenamiento inválida: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("creando directorio de almacenamiento: %w", err)
	}
	return &Local{raiz: abs}, nil
}

// rutaSegura convierte una ruta relativa (con "/") en una ruta absoluta dentro
// de la raíz y rechaza intentos de salir de ella (por ejemplo "../../etc").
func (l *Local) rutaSegura(ruta string) (string, error) {
	limpia := filepath.Clean(filepath.FromSlash(strings.TrimSpace(ruta)))
	if limpia == "." || filepath.IsAbs(limpia) || limpia == ".." ||
		strings.HasPrefix(limpia, ".."+string(filepath.Separator)) {
		return "", apperr.Validacion("ruta de archivo inválida")
	}
	return filepath.Join(l.raiz, limpia), nil
}

// Guardar escribe el contenido de forma atómica (archivo temporal + rename)
// y devuelve la cantidad de bytes escritos.
func (l *Local) Guardar(_ context.Context, ruta string, contenido io.Reader) (int64, error) {
	destino, err := l.rutaSegura(ruta)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(destino), 0o750); err != nil {
		return 0, fmt.Errorf("creando directorio: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(destino), ".tmp-*")
	if err != nil {
		return 0, fmt.Errorf("creando archivo temporal: %w", err)
	}
	// Si algo falla antes del rename, el temporal se borra; después del rename ya no existe.
	defer func() { _ = os.Remove(tmp.Name()) }()

	n, err := io.Copy(tmp, contenido)
	if err != nil {
		_ = tmp.Close()
		return 0, fmt.Errorf("escribiendo archivo: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("cerrando archivo: %w", err)
	}
	if err := os.Rename(tmp.Name(), destino); err != nil {
		return 0, fmt.Errorf("moviendo archivo a su destino: %w", err)
	}
	return n, nil
}

// Abrir devuelve un lector del archivo. El llamador debe cerrarlo.
func (l *Local) Abrir(_ context.Context, ruta string) (io.ReadCloser, error) {
	origen, err := l.rutaSegura(ruta)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(origen)
	if errors.Is(err, os.ErrNotExist) {
		return nil, apperr.NoEncontrado("el archivo no existe")
	}
	if err != nil {
		return nil, fmt.Errorf("abriendo archivo: %w", err)
	}
	return f, nil
}

// Eliminar borra el archivo. No falla si el archivo ya no existe.
func (l *Local) Eliminar(_ context.Context, ruta string) error {
	objetivo, err := l.rutaSegura(ruta)
	if err != nil {
		return err
	}
	if err := os.Remove(objetivo); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("eliminando archivo: %w", err)
	}
	return nil
}
