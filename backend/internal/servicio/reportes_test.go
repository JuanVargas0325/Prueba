package servicio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
	"seguimiento/internal/ia"
	"seguimiento/internal/pdf"
)

// --- dobles de prueba -------------------------------------------------------

type repoFalso struct {
	lotes          map[string]dominio.LoteReportes
	reportes       map[string]dominio.ReporteSemanal
	personas       []string
	datos          dominio.DatosSemanaPersona
	loteActivo     bool
	filtroLotes    dominio.FiltroLotes
	filtroReportes dominio.FiltroReportes
	completados    map[string]dominio.ResultadoReporte
	fallidos       map[string]string
	lotesFallidos  map[string]string
	enProceso      []string
	secuencia      int
}

func nuevoRepoFalso() *repoFalso {
	return &repoFalso{
		lotes:         map[string]dominio.LoteReportes{},
		reportes:      map[string]dominio.ReporteSemanal{},
		completados:   map[string]dominio.ResultadoReporte{},
		fallidos:      map[string]string{},
		lotesFallidos: map[string]string{},
	}
}

func (r *repoFalso) CrearLote(_ context.Context, profesorID string, semana time.Time) (dominio.LoteReportes, error) {
	r.secuencia++
	l := dominio.LoteReportes{ID: fmt.Sprintf("lote-%d", r.secuencia), ProfesorID: profesorID, SemanaInicio: semana, Estado: dominio.LotePendiente}
	r.lotes[l.ID] = l
	return l, nil
}
func (r *repoFalso) ExisteLoteActivo(context.Context, string, time.Time) (bool, error) {
	return r.loteActivo, nil
}
func (r *repoFalso) ObtenerLote(_ context.Context, id string) (dominio.LoteReportes, error) {
	l, ok := r.lotes[id]
	if !ok {
		return l, apperr.NoEncontrado("lote no encontrado")
	}
	return l, nil
}
func (r *repoFalso) ListarLotes(_ context.Context, f dominio.FiltroLotes) ([]dominio.LoteReportes, error) {
	r.filtroLotes = f
	return nil, nil
}
func (r *repoFalso) IniciarLote(_ context.Context, id string, total int) error {
	l := r.lotes[id]
	l.TotalReportes, l.Estado = total, dominio.LoteEnProceso
	r.lotes[id] = l
	return nil
}
func (r *repoFalso) MarcarLoteFallido(_ context.Context, id, motivo string) error {
	r.lotesFallidos[id] = motivo
	return nil
}
func (r *repoFalso) ListarPersonasParaReporte(context.Context, string, time.Time, time.Time) ([]string, error) {
	return r.personas, nil
}
func (r *repoFalso) CrearReportesPendientes(_ context.Context, loteID, profesorID string, semana time.Time, usuarios []string) ([]string, error) {
	var ids []string
	for _, u := range usuarios {
		id := "rep-" + u
		r.reportes[id] = dominio.ReporteSemanal{ID: id, LoteID: loteID, ProfesorID: profesorID, UsuarioID: u, SemanaInicio: semana, Estado: dominio.ReportePendiente}
		ids = append(ids, id)
	}
	return ids, nil
}
func (r *repoFalso) ObtenerReporte(_ context.Context, id string) (dominio.ReporteSemanal, error) {
	rep, ok := r.reportes[id]
	if !ok {
		return rep, apperr.NoEncontrado("reporte no encontrado")
	}
	return rep, nil
}
func (r *repoFalso) ListarReportes(_ context.Context, f dominio.FiltroReportes) ([]dominio.ReporteSemanal, error) {
	r.filtroReportes = f
	return nil, nil
}
func (r *repoFalso) MarcarReporteEnProceso(_ context.Context, id string) error {
	r.enProceso = append(r.enProceso, id)
	return nil
}
func (r *repoFalso) CompletarReporte(_ context.Context, id string, res dominio.ResultadoReporte) error {
	r.completados[id] = res
	return nil
}
func (r *repoFalso) FallarReporte(_ context.Context, id, motivo string) error {
	r.fallidos[id] = motivo
	return nil
}
func (r *repoFalso) ObtenerDatosSemana(context.Context, string, string, time.Time) (dominio.DatosSemanaPersona, error) {
	return r.datos, nil
}

type encoladorFalso struct {
	lotes, reportes []string
	err             error
}

func (e *encoladorFalso) EncolarLote(_ context.Context, id string) error {
	e.lotes = append(e.lotes, id)
	return e.err
}
func (e *encoladorFalso) EncolarReporte(_ context.Context, id string) error {
	e.reportes = append(e.reportes, id)
	return e.err
}

type almacenamientoFalso struct{ archivos map[string][]byte }

func (a *almacenamientoFalso) Guardar(_ context.Context, ruta string, r io.Reader) (int64, error) {
	b, _ := io.ReadAll(r)
	a.archivos[ruta] = b
	return int64(len(b)), nil
}
func (a *almacenamientoFalso) Abrir(_ context.Context, ruta string) (io.ReadCloser, error) {
	b, ok := a.archivos[ruta]
	if !ok {
		return nil, apperr.NoEncontrado("no existe")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}
func (a *almacenamientoFalso) Eliminar(_ context.Context, ruta string) error {
	delete(a.archivos, ruta)
	return nil
}

type resumidorFalso struct {
	llamadas int
	err      error
}

func (r *resumidorFalso) Resumir(context.Context, dominio.DatosSemanaPersona) (string, error) {
	r.llamadas++
	return "Resumen de prueba", r.err
}
func (r *resumidorFalso) Proveedor() string { return "falso" }
func (r *resumidorFalso) Modelo() string    { return "modelo-falso" }

type pdfFalso struct{ contenido pdf.Contenido }

func (p *pdfFalso) Generar(c pdf.Contenido) ([]byte, error) {
	p.contenido = c
	return []byte("%PDF-falso"), nil
}

var (
	logPrueba = slog.New(slog.NewTextHandler(io.Discard, nil))
	profesor  = auth.Actor{UsuarioID: "prof-1", Roles: []dominio.Rol{dominio.RolProfesor}}
	otroProf  = auth.Actor{UsuarioID: "prof-2", Roles: []dominio.Rol{dominio.RolProfesor}}
	admin     = auth.Actor{UsuarioID: "admin-1", Roles: []dominio.Rol{dominio.RolAdministrador}}
	monitor   = auth.Actor{UsuarioID: "mon-1", Roles: []dominio.Rol{dominio.RolMonitor}}
	// miércoles 14 de octubre de 2026
	ahoraPrueba = time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC)
)

func nuevoServicio() (*Reportes, *repoFalso, *encoladorFalso, *almacenamientoFalso) {
	repo, cola := nuevoRepoFalso(), &encoladorFalso{}
	alm := &almacenamientoFalso{archivos: map[string][]byte{}}
	s := NuevoReportes(repo, cola, alm, RelojFijo(ahoraPrueba, time.UTC), logPrueba)
	return s, repo, cola, alm
}

// --- SolicitarLote ----------------------------------------------------------

func TestSolicitarLoteNormalizaLaSemanaYEncola(t *testing.T) {
	s, repo, cola, _ := nuevoServicio()
	lote, err := s.SolicitarLote(context.Background(), profesor, "2026-10-08") // jueves
	if err != nil {
		t.Fatal(err)
	}
	if lote.SemanaInicio.Format(dominio.FormatoFecha) != "2026-10-05" {
		t.Errorf("la semana debería normalizarse al lunes, se obtuvo %s", lote.SemanaInicio.Format(dominio.FormatoFecha))
	}
	if lote.ProfesorID != "prof-1" || len(cola.lotes) != 1 || cola.lotes[0] != lote.ID {
		t.Errorf("el lote debería crearse para el profesor y encolarse: %+v %v", lote, cola.lotes)
	}
	if len(repo.lotesFallidos) != 0 {
		t.Error("no debería marcarse ningún lote como fallido")
	}
}

func TestSolicitarLoteValidaciones(t *testing.T) {
	casos := []struct {
		nombre     string
		actor      auth.Actor
		semana     string
		loteActivo bool
		tipo       apperr.Tipo
	}{
		{"solo profesores", monitor, "2026-10-05", false, apperr.TipoProhibido},
		{"administrador no genera", admin, "2026-10-05", false, apperr.TipoProhibido},
		{"fecha inválida", profesor, "05-10-2026", false, apperr.TipoValidacion},
		{"semana futura", profesor, "2026-10-19", false, apperr.TipoValidacion},
		{"lote en curso", profesor, "2026-10-05", true, apperr.TipoConflicto},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s, repo, _, _ := nuevoServicio()
			repo.loteActivo = c.loteActivo
			_, err := s.SolicitarLote(context.Background(), c.actor, c.semana)
			if !apperr.Es(err, c.tipo) {
				t.Fatalf("se esperaba error %s, se obtuvo %v", c.tipo, err)
			}
		})
	}
}

func TestSolicitarLotePermiteLaSemanaEnCurso(t *testing.T) {
	s, _, _, _ := nuevoServicio()
	if _, err := s.SolicitarLote(context.Background(), profesor, "2026-10-14"); err != nil {
		t.Fatalf("la semana en curso debería permitirse: %v", err)
	}
}

func TestSolicitarLoteMarcaFallidoSiNoPuedeEncolar(t *testing.T) {
	s, repo, cola, _ := nuevoServicio()
	cola.err = errors.New("redis caído")
	_, err := s.SolicitarLote(context.Background(), profesor, "2026-10-05")
	if !apperr.Es(err, apperr.TipoInterno) {
		t.Fatalf("se esperaba error interno, se obtuvo %v", err)
	}
	if len(repo.lotesFallidos) != 1 {
		t.Error("el lote debería quedar marcado como fallido")
	}
}

// --- consultas y visibilidad -------------------------------------------------

func TestListarLotesAplicaVisibilidad(t *testing.T) {
	s, repo, _, _ := nuevoServicio()

	_, _ = s.ListarLotes(context.Background(), profesor, dominio.FiltroLotes{ProfesorID: "prof-2"})
	if repo.filtroLotes.ProfesorID != "prof-1" {
		t.Error("un profesor solo debe ver sus propios lotes")
	}

	_, _ = s.ListarLotes(context.Background(), admin, dominio.FiltroLotes{ProfesorID: "prof-2"})
	if repo.filtroLotes.ProfesorID != "prof-2" {
		t.Errorf("el administrador puede filtrar por cualquier profesor: %+v", repo.filtroLotes)
	}

	if _, err := s.ListarLotes(context.Background(), monitor, dominio.FiltroLotes{}); !apperr.Es(err, apperr.TipoProhibido) {
		t.Errorf("un monitor no puede listar lotes: %v", err)
	}
}

func TestListarReportesAplicaVisibilidad(t *testing.T) {
	s, repo, _, _ := nuevoServicio()
	_, _ = s.ListarReportes(context.Background(), profesor, dominio.FiltroReportes{ProfesorID: "prof-2"})
	if repo.filtroReportes.ProfesorID != "prof-1" {
		t.Error("un profesor solo debe ver sus propios reportes")
	}
	if _, err := s.ListarReportes(context.Background(), monitor, dominio.FiltroReportes{}); !apperr.Es(err, apperr.TipoProhibido) {
		t.Errorf("un monitor no puede listar reportes: %v", err)
	}
}

func TestReporteDeOtroProfesorNoEsVisible(t *testing.T) {
	s, repo, _, _ := nuevoServicio()
	repo.reportes["r1"] = dominio.ReporteSemanal{ID: "r1", ProfesorID: "prof-1"}
	repo.lotes["l1"] = dominio.LoteReportes{ID: "l1", ProfesorID: "prof-1"}

	if _, err := s.ObtenerReporte(context.Background(), otroProf, "r1"); !apperr.Es(err, apperr.TipoNoEncontrado) {
		t.Errorf("otro profesor no debería ver el reporte: %v", err)
	}
	if _, _, err := s.ObtenerLote(context.Background(), otroProf, "l1"); !apperr.Es(err, apperr.TipoNoEncontrado) {
		t.Errorf("otro profesor no debería ver el lote: %v", err)
	}
	if _, err := s.ObtenerReporte(context.Background(), profesor, "r1"); err != nil {
		t.Errorf("el profesor dueño debería ver el reporte: %v", err)
	}
	if _, err := s.ObtenerReporte(context.Background(), admin, "r1"); err != nil {
		t.Errorf("el administrador debería ver el reporte: %v", err)
	}
}

func TestAbrirPDF(t *testing.T) {
	s, repo, _, alm := nuevoServicio()
	repo.reportes["pendiente"] = dominio.ReporteSemanal{ID: "pendiente", ProfesorID: "prof-1", Estado: dominio.ReporteEnProceso}
	repo.reportes["listo"] = dominio.ReporteSemanal{ID: "listo", ProfesorID: "prof-1", Estado: dominio.ReporteCompletado, RutaPDF: "reportes/x.pdf"}
	alm.archivos["reportes/x.pdf"] = []byte("%PDF")

	if _, _, err := s.AbrirPDF(context.Background(), profesor, "pendiente"); !apperr.Es(err, apperr.TipoConflicto) {
		t.Errorf("un reporte sin terminar no se puede descargar: %v", err)
	}
	r, _, err := s.AbrirPDF(context.Background(), profesor, "listo")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	if b, _ := io.ReadAll(r); string(b) != "%PDF" {
		t.Errorf("contenido inesperado: %q", b)
	}
}

// --- GeneradorReportes -------------------------------------------------------

func nuevoGenerador() (*GeneradorReportes, *repoFalso, *encoladorFalso, *almacenamientoFalso, *resumidorFalso, *pdfFalso) {
	repo, cola := nuevoRepoFalso(), &encoladorFalso{}
	alm := &almacenamientoFalso{archivos: map[string][]byte{}}
	res, gen := &resumidorFalso{}, &pdfFalso{}
	g := NuevoGeneradorReportes(repo, cola, alm, res, gen, RelojFijo(ahoraPrueba, time.UTC), logPrueba)
	return g, repo, cola, alm, res, gen
}

func semanaPrueba() time.Time { return time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC) }

func TestProcesarLoteCreaYEncolaUnReportePorPersona(t *testing.T) {
	g, repo, cola, _, _, _ := nuevoGenerador()
	repo.lotes["l1"] = dominio.LoteReportes{ID: "l1", ProfesorID: "prof-1", SemanaInicio: semanaPrueba(), Estado: dominio.LotePendiente}
	repo.personas = []string{"u1", "u2", "u3"}

	if err := g.ProcesarLote(context.Background(), "l1"); err != nil {
		t.Fatal(err)
	}
	if repo.lotes["l1"].TotalReportes != 3 || repo.lotes["l1"].Estado != dominio.LoteEnProceso {
		t.Errorf("lote inesperado: %+v", repo.lotes["l1"])
	}
	if len(cola.reportes) != 3 {
		t.Errorf("se esperaban 3 reportes encolados, hay %d", len(cola.reportes))
	}
}

func TestProcesarLoteOmiteLotesTerminados(t *testing.T) {
	g, repo, cola, _, _, _ := nuevoGenerador()
	repo.lotes["l1"] = dominio.LoteReportes{ID: "l1", Estado: dominio.LoteCompletado}
	repo.personas = []string{"u1"}
	if err := g.ProcesarLote(context.Background(), "l1"); err != nil {
		t.Fatal(err)
	}
	if len(cola.reportes) != 0 {
		t.Error("un lote terminado no debería volver a procesarse")
	}
}

func TestProcesarLoteMarcaFallidoElReporteQueNoSePudoEncolar(t *testing.T) {
	g, repo, cola, _, _, _ := nuevoGenerador()
	repo.lotes["l1"] = dominio.LoteReportes{ID: "l1", ProfesorID: "prof-1", SemanaInicio: semanaPrueba(), Estado: dominio.LotePendiente}
	repo.personas = []string{"u1"}
	cola.err = errors.New("redis caído")
	if err := g.ProcesarLote(context.Background(), "l1"); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.fallidos["rep-u1"]; !ok {
		t.Error("el reporte que no se pudo encolar debería quedar fallido")
	}
}

func datosConTareas() dominio.DatosSemanaPersona {
	return dominio.DatosSemanaPersona{
		Persona: "Ana", SemanaInicio: semanaPrueba(),
		Vinculaciones: []dominio.VinculacionSemana{{
			EspacioNombre: "Curso", Rol: dominio.RolMonitor, HorasContratadas: 4,
			Tareas: []dominio.TareaSemana{{Titulo: "Tarea", Estado: dominio.TareaFinalizada, Horas: 3}},
		}},
	}
}

func TestGenerarReporteCompleto(t *testing.T) {
	g, repo, _, alm, res, gen := nuevoGenerador()
	repo.reportes["r1"] = dominio.ReporteSemanal{ID: "r1", ProfesorID: "prof-1", UsuarioID: "u1", SemanaInicio: semanaPrueba(), Estado: dominio.ReportePendiente}
	repo.datos = datosConTareas()

	if err := g.GenerarReporte(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
	if res.llamadas != 1 || gen.contenido.Resumen != "Resumen de prueba" {
		t.Errorf("el resumen de IA debería usarse en el PDF: llamadas=%d contenido=%+v", res.llamadas, gen.contenido)
	}
	ruta := "reportes/2026-10-05/r1.pdf"
	if string(alm.archivos[ruta]) != "%PDF-falso" {
		t.Errorf("el PDF debería guardarse en %s", ruta)
	}
	resultado, ok := repo.completados["r1"]
	if !ok || resultado.RutaPDF != ruta || resultado.TamanoBytes != int64(len("%PDF-falso")) ||
		resultado.ProveedorIA != "falso" || resultado.ModeloIA != "modelo-falso" {
		t.Errorf("resultado inesperado: %+v", resultado)
	}
	if len(repo.enProceso) != 1 {
		t.Error("el reporte debería marcarse en proceso antes de generarse")
	}
}

func TestGenerarReporteSinTareasNoConsultaLaIA(t *testing.T) {
	g, repo, _, _, res, gen := nuevoGenerador()
	repo.reportes["r1"] = dominio.ReporteSemanal{ID: "r1", SemanaInicio: semanaPrueba(), Estado: dominio.ReportePendiente}
	repo.datos = dominio.DatosSemanaPersona{SemanaInicio: semanaPrueba()}

	if err := g.GenerarReporte(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
	if res.llamadas != 0 {
		t.Error("sin tareas no debería llamarse al modelo de IA")
	}
	if gen.contenido.Resumen != ResumenSinTareas || repo.completados["r1"].ProveedorIA != "" {
		t.Errorf("resultado inesperado: %+v", repo.completados["r1"])
	}
}

func TestGenerarReporteOmiteReportesFinalizados(t *testing.T) {
	g, repo, _, _, res, _ := nuevoGenerador()
	repo.reportes["r1"] = dominio.ReporteSemanal{ID: "r1", Estado: dominio.ReporteCompletado}
	if err := g.GenerarReporte(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
	if res.llamadas != 0 || len(repo.completados) != 0 {
		t.Error("un reporte finalizado no debería regenerarse")
	}
}

func TestGenerarReportePropagaElErrorDeIA(t *testing.T) {
	g, repo, _, _, res, _ := nuevoGenerador()
	repo.reportes["r1"] = dominio.ReporteSemanal{ID: "r1", SemanaInicio: semanaPrueba(), Estado: dominio.ReportePendiente}
	repo.datos = datosConTareas()
	res.err = &ia.ErrorProveedor{Proveedor: "falso", Estado: 401, Mensaje: "clave inválida"}

	err := g.GenerarReporte(context.Background(), "r1")
	if err == nil || !EsErrorPermanente(err) {
		t.Fatalf("un 401 del proveedor debería ser un error permanente: %v", err)
	}
	if len(repo.completados) != 0 {
		t.Error("el reporte no debería completarse")
	}

	if err := g.RegistrarFalloReporte(context.Background(), "r1", err); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(repo.fallidos["r1"], "clave inválida") {
		t.Errorf("la causa del fallo debería registrarse: %q", repo.fallidos["r1"])
	}
}

func TestEsErrorPermanente(t *testing.T) {
	casos := []struct {
		err        error
		permanente bool
	}{
		{apperr.NoEncontrado("x"), true},
		{apperr.Validacion("x"), true},
		{&ia.ErrorProveedor{Estado: 400}, true},
		{&ia.ErrorProveedor{Estado: 429, Reintentable: true}, false},
		{fmt.Errorf("envuelto: %w", &ia.ErrorProveedor{Estado: 503, Reintentable: true}), false},
		{errors.New("timeout de red"), false},
	}
	for _, c := range casos {
		if got := EsErrorPermanente(c.err); got != c.permanente {
			t.Errorf("EsErrorPermanente(%v) = %v, se esperaba %v", c.err, got, c.permanente)
		}
	}
}
