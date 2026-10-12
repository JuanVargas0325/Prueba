package dominio

import (
	"strings"
	"testing"
	"time"

	"seguimiento/internal/apperr"
)

func vinc(rol Rol, horas int) Vinculacion { return Vinculacion{Rol: rol, HorasSemanales: horas} }

func TestLimiteMonitoriaMixtaRedondeaHaciaArriba(t *testing.T) {
	casos := map[int]int{22: 9, 12: 5, 10: 4, 5: 2, 1: 1, 20: 8}
	for asistencia, esperado := range casos {
		if got := LimiteMonitoriaMixta(asistencia); got != esperado {
			t.Errorf("LimiteMonitoriaMixta(%d) = %d, se esperaba %d", asistencia, got, esperado)
		}
	}
}

func TestValidarCargaHoraria(t *testing.T) {
	casos := []struct {
		nombre string
		vs     []Vinculacion
		valido bool
	}{
		{"asistente con 22 h", []Vinculacion{vinc(RolAsistenteGraduado, 12), vinc(RolAsistenteGraduado, 10)}, true},
		{"asistente con 23 h (RN-14)", []Vinculacion{vinc(RolAsistenteGraduado, 12), vinc(RolAsistenteGraduado, 11)}, false},
		{"tres monitorías", []Vinculacion{vinc(RolMonitor, 4), vinc(RolMonitor, 4), vinc(RolMonitor, 4)}, true},
		{"cuatro monitorías (RN-15)", []Vinculacion{vinc(RolMonitor, 2), vinc(RolMonitor, 2), vinc(RolMonitor, 2), vinc(RolMonitor, 2)}, false},
		{"monitorías con 13 h (RN-16)", []Vinculacion{vinc(RolMonitor, 7), vinc(RolMonitor, 6)}, false},
		{"ejemplo del enunciado: 12 h asistencia + 6 h monitoría (RN-17)", []Vinculacion{vinc(RolAsistenteGraduado, 12), vinc(RolMonitor, 3), vinc(RolMonitor, 3)}, false},
		{"12 h asistencia + 5 h monitoría (redondeo RN-18)", []Vinculacion{vinc(RolAsistenteGraduado, 12), vinc(RolMonitor, 5)}, true},
		{"22 h asistencia + 9 h monitoría", []Vinculacion{vinc(RolAsistenteGraduado, 22), vinc(RolMonitor, 9)}, true},
		{"22 h asistencia + 10 h monitoría", []Vinculacion{vinc(RolAsistenteGraduado, 22), vinc(RolMonitor, 10)}, false},
		{"sin vinculaciones", nil, true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := ValidarCargaHoraria(c.vs)
			if c.valido && err != nil {
				t.Fatalf("debería ser válido: %v", err)
			}
			if !c.valido && !apperr.Es(err, apperr.TipoReglaNegocio) {
				t.Fatalf("debería violar una regla de negocio, se obtuvo %v", err)
			}
		})
	}
}

func TestValidarHoras(t *testing.T) {
	if ValidarHoras(0) == nil || ValidarHoras(-3) == nil || ValidarHoras(4) != nil {
		t.Error("solo se admiten horas positivas")
	}
}

func TestVigenciaDeLaVinculacion(t *testing.T) {
	v := Vinculacion{
		EspacioFechaInicio: fecha(t, "2026-08-03"),
		EspacioFechaCierre: fecha(t, "2026-12-04"),
		CreadoEn:           time.Date(2026, 9, 16, 15, 0, 0, 0, time.UTC),
	}
	if got := v.InicioVigencia(time.UTC).Format(FormatoFecha); got != "2026-09-16" {
		t.Errorf("la vigencia empieza al crear la vinculación, se obtuvo %s", got)
	}
	if v.VigenteEnSemana(fecha(t, "2026-09-07"), time.UTC) {
		t.Error("no debería estar vigente antes de crearse")
	}
	if !v.VigenteEnSemana(fecha(t, "2026-09-14"), time.UTC) || !v.VigenteEnSemana(fecha(t, "2026-11-30"), time.UTC) {
		t.Error("debería estar vigente en la semana de creación y en la del cierre")
	}
	if v.VigenteEnSemana(fecha(t, "2026-12-07"), time.UTC) {
		t.Error("no debería estar vigente después del cierre del espacio")
	}
}

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

func TestEspacioFechasYEstado(t *testing.T) {
	p := PeriodoAcademico{Codigo: "2026-20", FechaInicio: fecha(t, "2026-08-01"), FechaCierre: fecha(t, "2026-12-15")}
	e := Espacio{Nombre: "Nube", Tipo: EspacioCurso, FechaInicio: fecha(t, "2026-08-03"), FechaCierre: fecha(t, "2026-12-04"), Estado: EspacioActivo}
	if err := e.ValidarFechasEnPeriodo(p); err != nil {
		t.Fatal(err)
	}
	fuera := e
	fuera.FechaCierre = fecha(t, "2027-01-10")
	if fuera.ValidarFechasEnPeriodo(p) == nil {
		t.Error("las fechas deben estar dentro del período")
	}
	invertido := e
	invertido.FechaCierre = fecha(t, "2026-08-01")
	if invertido.ValidarFechasEnPeriodo(p) == nil {
		t.Error("el cierre no puede ser anterior al inicio")
	}
	if !e.IncluyeSemana(fecha(t, "2026-08-05")) || e.IncluyeSemana(fecha(t, "2026-12-09")) {
		t.Error("cálculo de semanas del espacio incorrecto")
	}
	e.Estado = EspacioCerrado
	if !apperr.Es(e.PermiteRegistros(), apperr.TipoReglaNegocio) {
		t.Error("un espacio cerrado no admite registros (RN-09)")
	}
}

func TestDatosTareaNormalizar(t *testing.T) {
	d, err := DatosTarea{Titulo: "  Calificar  ", HorasInvertidas: 3}.Normalizar()
	if err != nil || d.Titulo != "Calificar" || d.Estado != TareaAbierta {
		t.Fatalf("normalización inesperada: %+v %v", d, err)
	}
	invalidos := []DatosTarea{
		{Titulo: "", HorasInvertidas: 1},
		{Titulo: "x", Estado: "cerrado"},
		{Titulo: "x", HorasInvertidas: -1},
		{Titulo: "x", HorasInvertidas: MaxHorasPorTarea + 1},
		{Titulo: strings.Repeat("x", LongitudMaxTitulo+1)},
	}
	for _, d := range invalidos {
		if _, err := d.Normalizar(); !apperr.Es(err, apperr.TipoValidacion) {
			t.Errorf("%+v debería ser inválido", d)
		}
	}
}

func TestTareaPuedeModificarse(t *testing.T) {
	ahora := time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC) // miércoles
	actual := Tarea{SemanaInicio: fecha(t, "2026-10-12")}
	if err := actual.PuedeModificarse(ahora, time.UTC); err != nil {
		t.Errorf("la tarea de la semana activa se puede modificar: %v", err)
	}
	pasada := Tarea{SemanaInicio: fecha(t, "2026-10-05")}
	if !apperr.Es(pasada.PuedeModificarse(ahora, time.UTC), apperr.TipoReglaNegocio) {
		t.Error("la tarea de una semana cerrada no se puede modificar (RN-34)")
	}
	tardia := Tarea{SemanaInicio: fecha(t, "2026-10-05"), Tardia: true}
	if err := tardia.PuedeModificarse(ahora, time.UTC); !apperr.Es(err, apperr.TipoReglaNegocio) || !strings.Contains(err.Error(), "tardíos") {
		t.Errorf("un reporte tardío no se puede modificar (RN-33): %v", err)
	}
}

func TestValidarSemanaDeRegistro(t *testing.T) {
	ahora := time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC)
	if ValidarSemanaDeRegistro(fecha(t, "2026-10-18"), ahora, time.UTC) != nil {
		t.Error("el domingo de la semana actual es válido")
	}
	if ValidarSemanaDeRegistro(fecha(t, "2026-09-01"), ahora, time.UTC) != nil {
		t.Error("las semanas pasadas se admiten como reporte tardío (RF-08.3)")
	}
	if !apperr.Es(ValidarSemanaDeRegistro(fecha(t, "2026-10-19"), ahora, time.UTC), apperr.TipoValidacion) {
		t.Error("las semanas futuras no se admiten")
	}
}

func TestValidarArchivoAdjunto(t *testing.T) {
	nombre, ext, tipo, err := ValidarArchivoAdjunto(`C:\fakepath\..\Informe Final.PDF`)
	if err != nil || nombre != "Informe Final.PDF" || ext != ".pdf" || tipo != "application/pdf" {
		t.Fatalf("resultado inesperado: %q %q %q %v", nombre, ext, tipo, err)
	}
	for _, malo := range []string{"virus.exe", "pagina.html", "sin-extension", ""} {
		if _, _, _, err := ValidarArchivoAdjunto(malo); !apperr.Es(err, apperr.TipoValidacion) {
			t.Errorf("%q debería rechazarse", malo)
		}
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
