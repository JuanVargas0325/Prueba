package dominio

import (
	"strings"
	"testing"
)

func TestNormalizarCorreoYPassword(t *testing.T) {
	if c, err := NormalizarCorreo("  Ana.Perez@Uniandes.EDU.co "); err != nil || c != "ana.perez@uniandes.edu.co" {
		t.Errorf("correo normalizado = %q, %v", c, err)
	}
	for _, malo := range []string{"", "sin-arroba", "Ana <ana@x.co>", "a@"} {
		if _, err := NormalizarCorreo(malo); err == nil {
			t.Errorf("%q debería rechazarse", malo)
		}
	}
	if ValidarPassword("corta") == nil || ValidarPassword("suficiente") != nil || ValidarPassword(strings.Repeat("x", 73)) == nil {
		t.Error("política de contraseñas incorrecta")
	}
}

func TestPaginacion(t *testing.T) {
	if p := (Paginacion{}).Normalizar(); p.Limite != LimitePorDefecto {
		t.Errorf("límite por defecto = %d", p.Limite)
	}
	if p := (Paginacion{Limite: 5000, Desplazamiento: -4}).Normalizar(); p.Limite != LimiteMaximo || p.Desplazamiento != 0 {
		t.Errorf("paginación inesperada: %+v", p)
	}
}
