//go:build integracion

// Pruebas de integración de la capa de datos contra un PostgreSQL real.
// Se ejecutan con:
//
//	DATABASE_URL_PRUEBAS=postgres://usuario:clave@localhost:5432/bd_pruebas?sslmode=disable \
//	  go test -tags integracion ./internal/repositorio/...
//
// ¡Borran todos los datos de esa base de datos! Use una base exclusiva para pruebas.
package repositorio_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"seguimiento/internal/apperr"
	"seguimiento/internal/basedatos"
	"seguimiento/internal/dominio"
	"seguimiento/internal/repositorio"
)

var logPrueba = slog.New(slog.NewTextHandler(io.Discard, nil))

// conectar abre la base de pruebas, aplica las migraciones y la vacía.
func conectar(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL_PRUEBAS")
	if url == "" {
		t.Skip("DATABASE_URL_PRUEBAS no está definida")
	}
	ctx := context.Background()
	db, err := basedatos.Conectar(ctx, url, logPrueba)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := basedatos.Migrar(ctx, db, logPrueba); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `TRUNCATE usuarios, usuario_roles, periodos_academicos, espacios, vinculaciones,
		vinculacion_rectificaciones, tareas, tarea_adjuntos, lotes_reportes, reportes_semanales CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMigracionesSobreBaseVacia(t *testing.T) {
	db := conectar(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	// Dos veces: la segunda no debe aplicar nada ni fallar.
	for i := 0; i < 2; i++ {
		if err := basedatos.Migrar(ctx, db, logPrueba); err != nil {
			t.Fatalf("ejecución %d: %v", i+1, err)
		}
	}
	var aplicadas, tablas int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM migraciones_aplicadas`).Scan(&aplicadas); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&tablas); err != nil {
		t.Fatal(err)
	}
	if aplicadas != 1 || tablas != 11 {
		t.Errorf("se esperaba 1 migración y 11 tablas (10 del esquema + control), hay %d y %d", aplicadas, tablas)
	}
}

func fecha(t *testing.T, texto string) time.Time {
	t.Helper()
	f, err := dominio.ParsearFecha(texto)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// escenario crea un profesor, un período, dos espacios y dos personas.
type escenario struct {
	profesor, laura, andres dominio.Usuario
	periodo                 dominio.PeriodoAcademico
	curso, proyecto         dominio.Espacio
}

func crearEscenario(t *testing.T, db *pgxpool.Pool) escenario {
	t.Helper()
	ctx := context.Background()
	usuarios := repositorio.NuevoUsuarios(db)
	crear := func(nombre, correo string, roles ...dominio.Rol) dominio.Usuario {
		u, err := usuarios.Crear(ctx, dominio.Usuario{Nombre: nombre, Correo: correo, PasswordHash: "x", Activo: true, Roles: roles})
		if err != nil {
			t.Fatal(err)
		}
		return u
	}
	var e escenario
	e.profesor = crear("Carolina Rojas", "carolina@uniandes.edu.co", dominio.RolProfesor)
	e.laura = crear("Laura Martínez", "laura@uniandes.edu.co", dominio.RolMonitor, dominio.RolAsistenteGraduado)
	e.andres = crear("Andrés Gómez", "andres@uniandes.edu.co", dominio.RolMonitor)

	p, err := repositorio.NuevoPeriodos(db).Crear(ctx, dominio.PeriodoAcademico{Codigo: "2026-20",
		FechaInicio: fecha(t, "2026-08-01"), FechaCierre: fecha(t, "2026-12-15"), Estado: dominio.PeriodoActivo, CreacionHabilitada: true})
	if err != nil {
		t.Fatal(err)
	}
	e.periodo = p
	espacios := repositorio.NuevoEspacios(db)
	crearEspacio := func(tipo dominio.TipoEspacio, nombre string) dominio.Espacio {
		es, err := espacios.Crear(ctx, dominio.Espacio{Tipo: tipo, Nombre: nombre, PeriodoID: p.ID, ProfesorID: e.profesor.ID,
			FechaInicio: fecha(t, "2026-08-03"), FechaCierre: fecha(t, "2026-12-04")})
		if err != nil {
			t.Fatal(err)
		}
		return es
	}
	e.curso = crearEspacio(dominio.EspacioCurso, "Desarrollo en la nube")
	e.proyecto = crearEspacio(dominio.EspacioProyecto, "Analítica")
	return e
}

func validarReglas(simultaneas []dominio.Vinculacion, objetivo dominio.Vinculacion) error {
	return dominio.ValidarCargaHoraria(append(simultaneas, objetivo))
}

func vincular(t *testing.T, repo *repositorio.Vinculaciones, u dominio.Usuario, e dominio.Espacio, rol dominio.Rol, horas int) (dominio.Vinculacion, error) {
	t.Helper()
	return repo.Crear(context.Background(), dominio.Vinculacion{UsuarioID: u.ID, EspacioID: e.ID, Rol: rol, HorasSemanales: horas,
		EspacioEstado: e.Estado, EspacioFechaInicio: e.FechaInicio, EspacioFechaCierre: e.FechaCierre}, validarReglas)
}

func TestUsuarios(t *testing.T) {
	db := conectar(t)
	ctx := context.Background()
	e := crearEscenario(t, db)
	repo := repositorio.NuevoUsuarios(db)

	u, err := repo.BuscarPorCorreo(ctx, "LAURA@uniandes.edu.co")
	if err != nil || u.ID != e.laura.ID || len(u.Roles) != 2 {
		t.Fatalf("búsqueda por correo inesperada: %+v %v", u, err)
	}
	if _, err := repo.Crear(ctx, dominio.Usuario{Nombre: "x", Correo: "laura@uniandes.edu.co", PasswordHash: "x"}); !apperr.Es(err, apperr.TipoConflicto) {
		t.Errorf("un correo repetido debe dar conflicto: %v", err)
	}
	monitores, err := repo.Listar(ctx, dominio.FiltroUsuarios{Rol: dominio.RolMonitor, Busqueda: "ndr"})
	if err != nil || len(monitores) != 1 || monitores[0].ID != e.andres.ID {
		t.Errorf("filtro inesperado: %+v %v", monitores, err)
	}
	if err := repo.ReemplazarRoles(ctx, e.andres.ID, []dominio.Rol{dominio.RolProfesor}); err != nil {
		t.Fatal(err)
	}
	u, _ = repo.BuscarPorID(ctx, e.andres.ID)
	if len(u.Roles) != 1 || u.Roles[0] != dominio.RolProfesor {
		t.Errorf("roles inesperados: %v", u.Roles)
	}
	if _, err := repo.BuscarPorID(ctx, uuid.NewString()); !apperr.Es(err, apperr.TipoNoEncontrado) {
		t.Errorf("se esperaba no encontrado: %v", err)
	}
}

func TestEspaciosVisibilidadYCierre(t *testing.T) {
	db := conectar(t)
	ctx := context.Background()
	e := crearEscenario(t, db)
	espacios := repositorio.NuevoEspacios(db)
	if _, err := vincular(t, repositorio.NuevoVinculaciones(db), e.laura, e.curso, dominio.RolMonitor, 4); err != nil {
		t.Fatal(err)
	}

	delProfesor, _ := espacios.Listar(ctx, dominio.FiltroEspacios{VisiblePara: e.profesor.ID})
	deLaura, _ := espacios.Listar(ctx, dominio.FiltroEspacios{VisiblePara: e.laura.ID})
	deAndres, _ := espacios.Listar(ctx, dominio.FiltroEspacios{VisiblePara: e.andres.ID})
	if len(delProfesor) != 2 || len(deLaura) != 1 || deLaura[0].ID != e.curso.ID || len(deAndres) != 0 {
		t.Errorf("visibilidad inesperada: %d %d %d", len(delProfesor), len(deLaura), len(deAndres))
	}
	if deLaura[0].PeriodoCodigo != "2026-20" || deLaura[0].ProfesorNombre != "Carolina Rojas" {
		t.Errorf("faltan datos relacionados: %+v", deLaura[0])
	}

	cerrado, err := espacios.Cerrar(ctx, e.curso.ID)
	if err != nil || cerrado.Estado != dominio.EspacioCerrado {
		t.Fatalf("cierre inesperado: %+v %v", cerrado, err)
	}
	if _, err := espacios.Cerrar(ctx, e.curso.ID); !apperr.Es(err, apperr.TipoConflicto) {
		t.Errorf("cerrar dos veces debe dar conflicto: %v", err)
	}
}

func TestVinculacionesReglasYConcurrencia(t *testing.T) {
	db := conectar(t)
	ctx := context.Background()
	e := crearEscenario(t, db)
	repo := repositorio.NuevoVinculaciones(db)

	// Dos asistencias de 12 h creadas al mismo tiempo: juntas suman 24 > 22,
	// así que exactamente una debe fallar gracias al bloqueo de la fila del usuario.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, espacio := range []dominio.Espacio{e.curso, e.proyecto} {
		wg.Add(1)
		go func(i int, espacio dominio.Espacio) {
			defer wg.Done()
			_, errs[i] = vincular(t, repo, e.laura, espacio, dominio.RolAsistenteGraduado, 12)
		}(i, espacio)
	}
	wg.Wait()
	fallidas := 0
	for _, err := range errs {
		if err != nil {
			if !apperr.Es(err, apperr.TipoReglaNegocio) {
				t.Fatalf("error inesperado: %v", err)
			}
			fallidas++
		}
	}
	if fallidas != 1 {
		t.Fatalf("exactamente una vinculación debía rechazarse, se rechazaron %d", fallidas)
	}

	vs, err := repo.Listar(ctx, dominio.FiltroVinculaciones{UsuarioID: e.laura.ID, Paginacion: dominio.SinPaginar})
	if err != nil || len(vs) != 1 {
		t.Fatalf("debería existir una vinculación: %v %v", vs, err)
	}
	v := vs[0]

	// Duplicado del mismo rol en el mismo espacio.
	if _, err := vincular(t, repo, e.laura, espacioDe(e, v.EspacioID), dominio.RolAsistenteGraduado, 2); !apperr.Es(err, apperr.TipoConflicto) {
		t.Errorf("una vinculación repetida debe dar conflicto: %v", err)
	}

	// Rectificación: 12 → 10 h, queda registrada.
	corregida, err := repo.Rectificar(ctx, v.ID, v.Rol, 10, e.profesor.ID, "Ajuste de contrato", validarReglas)
	if err != nil || corregida.HorasSemanales != 10 {
		t.Fatalf("rectificación inesperada: %+v %v", corregida, err)
	}
	rectificaciones, err := repo.ListarRectificaciones(ctx, v.ID)
	if err != nil || len(rectificaciones) != 1 || rectificaciones[0].HorasAnteriores != 12 || rectificaciones[0].HorasNuevas != 10 {
		t.Errorf("historial inesperado: %+v %v", rectificaciones, err)
	}
	// Una rectificación que viola las reglas no se aplica.
	if _, err := repo.Rectificar(ctx, v.ID, v.Rol, 23, e.profesor.ID, "x", validarReglas); !apperr.Es(err, apperr.TipoReglaNegocio) {
		t.Errorf("se esperaba regla de negocio: %v", err)
	}
	actual, _ := repo.Obtener(ctx, v.ID)
	if actual.HorasSemanales != 10 {
		t.Errorf("la rectificación rechazada no debe cambiar las horas: %d", actual.HorasSemanales)
	}
}

func espacioDe(e escenario, id string) dominio.Espacio {
	if e.curso.ID == id {
		return e.curso
	}
	return e.proyecto
}

func TestTareasYAdjuntos(t *testing.T) {
	db := conectar(t)
	ctx := context.Background()
	e := crearEscenario(t, db)
	v, err := vincular(t, repositorio.NuevoVinculaciones(db), e.laura, e.curso, dominio.RolMonitor, 4)
	if err != nil {
		t.Fatal(err)
	}
	repo := repositorio.NuevoTareas(db)

	id := uuid.NewString()
	adjunto := dominio.Adjunto{ID: uuid.NewString(), TareaID: id, NombreOriginal: "evidencia.pdf",
		RutaAlmacenamiento: "adjuntos/" + id + "/a.pdf", TipoContenido: "application/pdf", TamanoBytes: 10}
	creada, err := repo.Crear(ctx, dominio.Tarea{ID: id, VinculacionID: v.ID, SemanaInicio: fecha(t, "2026-10-05"),
		Titulo: "Calificar", Estado: dominio.TareaAbierta, HorasInvertidas: 3}, []dominio.Adjunto{adjunto})
	if err != nil {
		t.Fatal(err)
	}
	if creada.UsuarioID != e.laura.ID || creada.ProfesorID != e.profesor.ID || creada.CantidadAdjuntos != 1 || len(creada.Adjuntos) != 1 {
		t.Errorf("tarea inesperada: %+v", creada)
	}

	// Una semana que no es lunes viola la restricción de la base de datos.
	_, err = repo.Crear(ctx, dominio.Tarea{ID: uuid.NewString(), VinculacionID: v.ID, SemanaInicio: fecha(t, "2026-10-07"),
		Titulo: "x", Estado: dominio.TareaAbierta}, nil)
	if !apperr.Es(err, apperr.TipoValidacion) {
		t.Errorf("semana_inicio debe ser lunes: %v", err)
	}

	visibles, _ := repo.Listar(ctx, dominio.FiltroTareas{VisiblePara: e.profesor.ID})
	ajenas, _ := repo.Listar(ctx, dominio.FiltroTareas{VisiblePara: e.andres.ID})
	if len(visibles) != 1 || len(ajenas) != 0 {
		t.Errorf("visibilidad inesperada: %d %d", len(visibles), len(ajenas))
	}

	creada.Titulo, creada.Estado = "Calificar taller", dominio.TareaFinalizada
	actualizada, err := repo.Actualizar(ctx, creada)
	if err != nil || actualizada.Titulo != "Calificar taller" || actualizada.Estado != dominio.TareaFinalizada {
		t.Errorf("actualización inesperada: %+v %v", actualizada, err)
	}

	extra := []dominio.Adjunto{}
	for i := 0; i < dominio.MaxAdjuntosPorTarea; i++ {
		extra = append(extra, dominio.Adjunto{ID: uuid.NewString(), TareaID: id, NombreOriginal: "x.pdf",
			RutaAlmacenamiento: "adjuntos/" + id + "/" + uuid.NewString(), TipoContenido: "application/pdf", TamanoBytes: 1})
	}
	if err := repo.AgregarAdjuntos(ctx, id, extra, dominio.MaxAdjuntosPorTarea); !apperr.Es(err, apperr.TipoReglaNegocio) {
		t.Errorf("se debe respetar el máximo de adjuntos: %v", err)
	}

	rutas, err := repo.Eliminar(ctx, id)
	if err != nil || len(rutas) != 1 || rutas[0] != adjunto.RutaAlmacenamiento {
		t.Errorf("eliminación inesperada: %v %v", rutas, err)
	}

	// Un reporte tardío no se puede modificar ni eliminar, ni siquiera en la base de datos.
	tardiaID := uuid.NewString()
	tardia, _ := repo.Crear(ctx, dominio.Tarea{ID: tardiaID, VinculacionID: v.ID, SemanaInicio: fecha(t, "2026-09-28"),
		Titulo: "Tardía", Estado: dominio.TareaAbierta, Tardia: true}, nil)
	if _, err := repo.Actualizar(ctx, tardia); !apperr.Es(err, apperr.TipoReglaNegocio) {
		t.Errorf("una tarea tardía no se actualiza: %v", err)
	}
	if _, err := repo.Eliminar(ctx, tardiaID); !apperr.Es(err, apperr.TipoReglaNegocio) {
		t.Errorf("una tarea tardía no se elimina: %v", err)
	}
}

// loteDePrueba es un lote con dos reportes pendientes: Laura (con una tarea
// en la semana) y Andrés (sin tareas).
type loteDePrueba struct {
	repo     *repositorio.Reportes
	esc      escenario
	semana   time.Time
	lote     dominio.LoteReportes
	personas []string
	reportes []string
}

func prepararLote(t *testing.T) loteDePrueba {
	t.Helper()
	db := conectar(t)
	ctx := context.Background()
	l := loteDePrueba{repo: repositorio.NuevoReportes(db), esc: crearEscenario(t, db), semana: fecha(t, "2026-10-05")}

	vinculaciones := repositorio.NuevoVinculaciones(db)
	vLaura, err := vincular(t, vinculaciones, l.esc.laura, l.esc.curso, dominio.RolMonitor, 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vincular(t, vinculaciones, l.esc.andres, l.esc.proyecto, dominio.RolMonitor, 6); err != nil {
		t.Fatal(err)
	}
	_, err = repositorio.NuevoTareas(db).Crear(ctx, dominio.Tarea{ID: uuid.NewString(), VinculacionID: vLaura.ID, SemanaInicio: l.semana,
		Titulo: "Calificar", Descripcion: "Taller 2", Estado: dominio.TareaFinalizada, HorasInvertidas: 3}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if l.lote, err = l.repo.CrearLote(ctx, l.esc.profesor.ID, l.semana); err != nil {
		t.Fatal(err)
	}
	if l.personas, err = l.repo.ListarPersonasParaReporte(ctx, l.esc.profesor.ID, l.semana, dominio.FinSemana(l.semana)); err != nil {
		t.Fatal(err)
	}
	if l.reportes, err = l.repo.CrearReportesPendientes(ctx, l.lote.ID, l.esc.profesor.ID, l.semana, l.personas); err != nil {
		t.Fatal(err)
	}
	if err := l.repo.IniciarLote(ctx, l.lote.ID, len(l.reportes)); err != nil {
		t.Fatal(err)
	}
	return l
}

func TestCreacionDeLoteDeReportes(t *testing.T) {
	l := prepararLote(t)
	ctx := context.Background()

	if len(l.personas) != 2 || len(l.reportes) != 2 {
		t.Fatalf("se esperaban 2 personas y 2 reportes: %v %v", l.personas, l.reportes)
	}
	if activo, _ := l.repo.ExisteLoteActivo(ctx, l.esc.profesor.ID, l.semana); !activo {
		t.Error("el lote en proceso debe considerarse activo")
	}
	// Idempotente: repetirlo no duplica.
	if otra, _ := l.repo.CrearReportesPendientes(ctx, l.lote.ID, l.esc.profesor.ID, l.semana, l.personas); len(otra) != 2 {
		t.Errorf("crear reportes dos veces no debe duplicarlos: %d", len(otra))
	}
	datos, err := l.repo.ObtenerDatosSemana(ctx, l.esc.profesor.ID, l.esc.laura.ID, l.semana)
	if err != nil || datos.TotalTareas() != 1 || datos.TotalHoras() != 3 || datos.Persona != "Laura Martínez" || datos.Profesor != "Carolina Rojas" {
		t.Errorf("datos de la semana inesperados: %+v %v", datos, err)
	}
	lista, err := l.repo.ListarReportes(ctx, dominio.FiltroReportes{LoteID: l.lote.ID, Paginacion: dominio.SinPaginar})
	if err != nil || len(lista) != 2 {
		t.Errorf("listado inesperado: %v %v", lista, err)
	}
}

func TestFinalizacionDeReportes(t *testing.T) {
	l := prepararLote(t)
	ctx := context.Background()

	if err := l.repo.MarcarReporteEnProceso(ctx, l.reportes[0]); err != nil {
		t.Fatal(err)
	}
	if err := l.repo.CompletarReporte(ctx, l.reportes[0], dominio.ResultadoReporte{Resumen: "ok", RutaPDF: "r.pdf", TamanoBytes: 10}); err != nil {
		t.Fatal(err)
	}
	// Completar dos veces no cuenta dos veces.
	_ = l.repo.CompletarReporte(ctx, l.reportes[0], dominio.ResultadoReporte{Resumen: "ok"})
	if err := l.repo.FallarReporte(ctx, l.reportes[1], "clave inválida"); err != nil {
		t.Fatal(err)
	}

	final, _ := l.repo.ObtenerLote(ctx, l.lote.ID)
	if final.Estado != dominio.LoteCompletadoConErrores || final.ReportesCompletados != 1 || final.ReportesFallidos != 1 || final.FinalizadoEn == nil {
		t.Errorf("estado final del lote inesperado: %+v", final)
	}
	rep, _ := l.repo.ObtenerReporte(ctx, l.reportes[0])
	if rep.Estado != dominio.ReporteCompletado || rep.Intentos != 1 || rep.RutaPDF != "r.pdf" {
		t.Errorf("reporte inesperado: %+v", rep)
	}
}
