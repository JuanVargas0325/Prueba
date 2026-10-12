package almacenamiento

import (
	"context"
	"io"
	"strings"
	"testing"

	"seguimiento/internal/apperr"
)

func TestGuardarAbrirYEliminar(t *testing.T) {
	ctx := context.Background()
	l, err := NuevoLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	n, err := l.Guardar(ctx, "reportes/2026-10-05/abc.pdf", strings.NewReader("contenido"))
	if err != nil || n != int64(len("contenido")) {
		t.Fatalf("Guardar: n=%d err=%v", n, err)
	}

	r, err := l.Abrir(ctx, "reportes/2026-10-05/abc.pdf")
	if err != nil {
		t.Fatal(err)
	}
	leido, _ := io.ReadAll(r)
	_ = r.Close()
	if string(leido) != "contenido" {
		t.Fatalf("contenido leído = %q", leido)
	}

	if err := l.Eliminar(ctx, "reportes/2026-10-05/abc.pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Abrir(ctx, "reportes/2026-10-05/abc.pdf"); !apperr.Es(err, apperr.TipoNoEncontrado) {
		t.Fatalf("se esperaba no encontrado, se obtuvo %v", err)
	}
	if err := l.Eliminar(ctx, "reportes/2026-10-05/abc.pdf"); err != nil {
		t.Fatalf("eliminar un archivo inexistente no debería fallar: %v", err)
	}
}

func TestRutasFueraDeLaRaizSonRechazadas(t *testing.T) {
	l, err := NuevoLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, ruta := range []string{"../fuera.pdf", "a/../../fuera.pdf", "/etc/passwd", "", "."} {
		if _, err := l.Guardar(context.Background(), ruta, strings.NewReader("x")); !apperr.Es(err, apperr.TipoValidacion) {
			t.Errorf("la ruta %q debería rechazarse, err=%v", ruta, err)
		}
	}
}

func TestNuevoEligeElTipo(t *testing.T) {
	if _, err := Nuevo(Config{Tipo: TipoLocal, Ruta: t.TempDir()}); err != nil {
		t.Fatalf("local: %v", err)
	}
	if _, err := Nuevo(Config{Tipo: "nube"}); err == nil {
		t.Error("un tipo desconocido debería ser un error")
	}
}
