package dominio

import (
	"strings"
	"testing"

	"seguimiento/internal/apperr"
)

func TestPeriodoPermiteCrearEspacios(t *testing.T) {
	p := PeriodoAcademico{Codigo: "2026-20", Estado: PeriodoActivo, CreacionHabilitada: true,
		FechaInicio: fecha(t, "2026-08-01"), FechaCierre: fecha(t, "2026-12-15")}
	if err := p.PermiteCrearEspacios(fecha(t, "2026-08-10")); err != nil {
		t.Fatal(err)
	}
	cerrado := p
	cerrado.Estado = PeriodoCerrado
	inhabilitado := p
	inhabilitado.CreacionHabilitada = false
	for nombre, err := range map[string]error{
		"cerrado":      cerrado.PermiteCrearEspacios(fecha(t, "2026-08-10")),
		"inhabilitado": inhabilitado.PermiteCrearEspacios(fecha(t, "2026-08-10")),
		"terminado":    p.PermiteCrearEspacios(fecha(t, "2026-12-16")),
	} {
		if !apperr.Es(err, apperr.TipoReglaNegocio) {
			t.Errorf("%s: se esperaba regla de negocio, se obtuvo %v", nombre, err)
		}
	}
	if (PeriodoAcademico{FechaInicio: fecha(t, "2026-08-01"), FechaCierre: fecha(t, "2026-08-01")}).ValidarFechas() == nil {
		t.Error("el cierre debe ser posterior al inicio")
	}
}

func TestCodigoPeriodo(t *testing.T) {
	if ValidarCodigoPeriodo("2026-20") != nil || ValidarCodigoPeriodo("2026-2") == nil || ValidarCodigoPeriodo("20262") == nil {
		t.Error("validación de código de período incorrecta")
	}
}

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
