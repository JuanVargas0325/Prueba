package servicio

import (
	"context"
	"strings"
	"testing"
	"time"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

// Dobles en memoria de los repositorios de los módulos de gestión.

func esperarTipo(t *testing.T, err error, tipo apperr.Tipo) {
	t.Helper()
	if !apperr.Es(err, tipo) {
		t.Fatalf("se esperaba un error %s, se obtuvo %v", tipo, err)
	}
}

func fechaP(t *testing.T, texto string) time.Time {
	t.Helper()
	f, err := dominio.ParsearFecha(texto)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// --- usuarios ------------------------------------------------------------------------

type repoUsuariosFalso struct {
	usuarios       map[string]dominio.Usuario
	rolesEnUso     map[dominio.Rol]bool
	espaciosActivo bool
}

func nuevoRepoUsuarios() *repoUsuariosFalso {
	return &repoUsuariosFalso{usuarios: map[string]dominio.Usuario{}, rolesEnUso: map[dominio.Rol]bool{}}
}

func (r *repoUsuariosFalso) BuscarPorID(_ context.Context, id string) (dominio.Usuario, error) {
	u, ok := r.usuarios[id]
	if !ok {
		return u, apperr.NoEncontrado("usuario no encontrado")
	}
	return u, nil
}

func (r *repoUsuariosFalso) BuscarPorCorreo(_ context.Context, correo string) (dominio.Usuario, error) {
	for _, u := range r.usuarios {
		if u.Correo == strings.ToLower(correo) {
			return u, nil
		}
	}
	return dominio.Usuario{}, apperr.NoEncontrado("usuario no encontrado")
}

func (r *repoUsuariosFalso) Crear(_ context.Context, u dominio.Usuario) (dominio.Usuario, error) {
	u.ID = "nuevo-" + u.Correo
	r.usuarios[u.ID] = u
	return u, nil
}

func (r *repoUsuariosFalso) Listar(context.Context, dominio.FiltroUsuarios) ([]dominio.Usuario, error) {
	return nil, nil
}

func (r *repoUsuariosFalso) ReemplazarRoles(_ context.Context, id string, roles []dominio.Rol) error {
	u := r.usuarios[id]
	u.Roles = roles
	r.usuarios[id] = u
	return nil
}

func (r *repoUsuariosFalso) TieneVinculacionesActivas(_ context.Context, _ string, rol dominio.Rol) (bool, error) {
	return r.rolesEnUso[rol], nil
}

func (r *repoUsuariosFalso) TieneEspaciosActivos(context.Context, string) (bool, error) {
	return r.espaciosActivo, nil
}

func TestCrearUsuario(t *testing.T) {
	repo := nuevoRepoUsuarios()
	s := NuevoUsuarios(repo, logPrueba)
	ctx := context.Background()

	u, err := s.Crear(ctx, admin, NuevoUsuario{Nombre: " Ana ", Correo: "ANA@uniandes.edu.co", Password: "Clave123*",
		Roles: []dominio.Rol{dominio.RolMonitor, dominio.RolMonitor, dominio.RolAsistenteGraduado}})
	if err != nil {
		t.Fatal(err)
	}
	if u.Nombre != "Ana" || u.Correo != "ana@uniandes.edu.co" || len(u.Roles) != 2 || u.PasswordHash == "Clave123*" || !u.Activo {
		t.Errorf("usuario inesperado: %+v", u)
	}

	casos := []struct {
		actor auth.Actor
		n     NuevoUsuario
		tipo  apperr.Tipo
	}{
		{profesor, NuevoUsuario{Nombre: "x", Correo: "x@x.co", Password: "Clave123*", Roles: []dominio.Rol{dominio.RolMonitor}}, apperr.TipoProhibido},
		{admin, NuevoUsuario{Nombre: "", Correo: "x@x.co", Password: "Clave123*", Roles: []dominio.Rol{dominio.RolMonitor}}, apperr.TipoValidacion},
		{admin, NuevoUsuario{Nombre: "x", Correo: "no-es-correo", Password: "Clave123*", Roles: []dominio.Rol{dominio.RolMonitor}}, apperr.TipoValidacion},
		{admin, NuevoUsuario{Nombre: "x", Correo: "x@x.co", Password: "corta", Roles: []dominio.Rol{dominio.RolMonitor}}, apperr.TipoValidacion},
		{admin, NuevoUsuario{Nombre: "x", Correo: "x@x.co", Password: "Clave123*", Roles: []dominio.Rol{"rector"}}, apperr.TipoValidacion},
		{admin, NuevoUsuario{Nombre: "x", Correo: "x@x.co", Password: "Clave123*"}, apperr.TipoValidacion},
	}
	for _, c := range casos {
		_, err := s.Crear(ctx, c.actor, c.n)
		esperarTipo(t, err, c.tipo)
	}
}

func TestAsignarRoles(t *testing.T) {
	repo := nuevoRepoUsuarios()
	repo.usuarios["admin-1"] = dominio.Usuario{ID: "admin-1", Roles: []dominio.Rol{dominio.RolAdministrador}}
	repo.usuarios["u1"] = dominio.Usuario{ID: "u1", Roles: []dominio.Rol{dominio.RolMonitor, dominio.RolProfesor}}
	s := NuevoUsuarios(repo, logPrueba)
	ctx := context.Background()

	_, err := s.AsignarRoles(ctx, admin, "admin-1", []dominio.Rol{dominio.RolProfesor})
	esperarTipo(t, err, apperr.TipoReglaNegocio) // no puede quitarse su propio rol de administrador

	repo.rolesEnUso[dominio.RolMonitor] = true
	_, err = s.AsignarRoles(ctx, admin, "u1", []dominio.Rol{dominio.RolProfesor})
	esperarTipo(t, err, apperr.TipoReglaNegocio) // el rol monitor está en uso

	repo.rolesEnUso[dominio.RolMonitor] = false
	u, err := s.AsignarRoles(ctx, admin, "u1", []dominio.Rol{dominio.RolProfesor, dominio.RolAsistenteGraduado})
	if err != nil || !u.TieneRol(dominio.RolAsistenteGraduado) || u.TieneRol(dominio.RolMonitor) {
		t.Fatalf("roles inesperados: %+v %v", u, err)
	}

	repo.espaciosActivo = true
	_, err = s.AsignarRoles(ctx, admin, "u1", []dominio.Rol{dominio.RolAsistenteGraduado})
	esperarTipo(t, err, apperr.TipoReglaNegocio) // el profesor tiene espacios activos

	_, err = s.Obtener(ctx, monitor, "u1")
	esperarTipo(t, err, apperr.TipoNoEncontrado) // un usuario solo se ve a sí mismo
}

// --- períodos -------------------------------------------------------------------------

type repoPeriodosFalso struct {
	periodos map[string]dominio.PeriodoAcademico
}

func (r *repoPeriodosFalso) Crear(_ context.Context, p dominio.PeriodoAcademico) (dominio.PeriodoAcademico, error) {
	p.ID = "p-" + p.Codigo
	r.periodos[p.ID] = p
	return p, nil
}

func (r *repoPeriodosFalso) Obtener(_ context.Context, id string) (dominio.PeriodoAcademico, error) {
	p, ok := r.periodos[id]
	if !ok {
		return p, apperr.NoEncontrado("período no encontrado")
	}
	return p, nil
}

func (r *repoPeriodosFalso) Listar(context.Context, dominio.FiltroPeriodos) ([]dominio.PeriodoAcademico, error) {
	return nil, nil
}

func (r *repoPeriodosFalso) Actualizar(_ context.Context, p dominio.PeriodoAcademico) (dominio.PeriodoAcademico, error) {
	r.periodos[p.ID] = p
	return p, nil
}

func TestPeriodos(t *testing.T) {
	repo := &repoPeriodosFalso{periodos: map[string]dominio.PeriodoAcademico{}}
	s := NuevoPeriodos(repo, logPrueba)
	ctx := context.Background()

	p, err := s.Crear(ctx, admin, NuevoPeriodo{Codigo: "2026-20", FechaInicio: fechaP(t, "2026-08-01"), FechaCierre: fechaP(t, "2026-12-15"), CreacionHabilitada: true})
	if err != nil || p.Estado != dominio.PeriodoActivo {
		t.Fatalf("período inesperado: %+v %v", p, err)
	}
	_, err = s.Crear(ctx, profesor, NuevoPeriodo{Codigo: "2026-20"})
	esperarTipo(t, err, apperr.TipoProhibido)
	_, err = s.Crear(ctx, admin, NuevoPeriodo{Codigo: "2026", FechaInicio: fechaP(t, "2026-08-01"), FechaCierre: fechaP(t, "2026-12-15")})
	esperarTipo(t, err, apperr.TipoValidacion)
	_, err = s.Crear(ctx, admin, NuevoPeriodo{Codigo: "2026-10", FechaInicio: fechaP(t, "2026-08-01"), FechaCierre: fechaP(t, "2026-07-01")})
	esperarTipo(t, err, apperr.TipoValidacion)

	cerrado := dominio.PeriodoCerrado
	actualizado, err := s.Actualizar(ctx, admin, p.ID, CambiosPeriodo{Estado: &cerrado})
	if err != nil || actualizado.CreacionHabilitada {
		t.Errorf("cerrar el período debe deshabilitar la creación: %+v %v", actualizado, err)
	}
	_, err = s.Actualizar(ctx, admin, p.ID, CambiosPeriodo{})
	esperarTipo(t, err, apperr.TipoValidacion)
}

// --- espacios ---------------------------------------------------------------------------

type repoEspaciosFalso struct {
	espacios     map[string]dominio.Espacio
	participante bool
	filtro       dominio.FiltroEspacios
}

func (r *repoEspaciosFalso) Crear(_ context.Context, e dominio.Espacio) (dominio.Espacio, error) {
	e.ID = "e-nuevo"
	r.espacios[e.ID] = e
	return e, nil
}

func (r *repoEspaciosFalso) Obtener(_ context.Context, id string) (dominio.Espacio, error) {
	e, ok := r.espacios[id]
	if !ok {
		return e, apperr.NoEncontrado("curso o proyecto no encontrado")
	}
	return e, nil
}

func (r *repoEspaciosFalso) Listar(_ context.Context, f dominio.FiltroEspacios) ([]dominio.Espacio, error) {
	r.filtro = f
	return nil, nil
}

func (r *repoEspaciosFalso) Cerrar(_ context.Context, id string) (dominio.Espacio, error) {
	e := r.espacios[id]
	e.Estado = dominio.EspacioCerrado
	return e, nil
}

func (r *repoEspaciosFalso) EsParticipante(context.Context, string, string) (bool, error) {
	return r.participante, nil
}

func entornoEspacios(t *testing.T) (*Espacios, *repoEspaciosFalso, *repoPeriodosFalso) {
	periodos := &repoPeriodosFalso{periodos: map[string]dominio.PeriodoAcademico{
		"p1": {ID: "p1", Codigo: "2026-20", Estado: dominio.PeriodoActivo, CreacionHabilitada: true,
			FechaInicio: fechaP(t, "2026-08-01"), FechaCierre: fechaP(t, "2026-12-15")},
	}}
	repo := &repoEspaciosFalso{espacios: map[string]dominio.Espacio{
		"e1": {ID: "e1", ProfesorID: "prof-1", Tipo: dominio.EspacioCurso, Nombre: "Nube", Estado: dominio.EspacioActivo},
	}}
	return NuevoEspacios(repo, periodos, RelojFijo(ahoraPrueba, time.UTC), logPrueba), repo, periodos
}

func TestCrearEspacio(t *testing.T) {
	s, _, periodos := entornoEspacios(t)
	ctx := context.Background()
	n := NuevoEspacio{Tipo: dominio.EspacioCurso, Nombre: "Nube", PeriodoID: "p1",
		FechaInicio: fechaP(t, "2026-08-03"), FechaCierre: fechaP(t, "2026-12-04")}

	e, err := s.Crear(ctx, profesor, n)
	if err != nil || e.ProfesorID != "prof-1" || e.Estado != dominio.EspacioActivo {
		t.Fatalf("espacio inesperado: %+v %v", e, err)
	}

	_, err = s.Crear(ctx, monitor, n)
	esperarTipo(t, err, apperr.TipoProhibido)

	fuera := n
	fuera.FechaCierre = fechaP(t, "2027-01-30")
	_, err = s.Crear(ctx, profesor, fuera)
	esperarTipo(t, err, apperr.TipoValidacion)

	inexistente := n
	inexistente.PeriodoID = "otro"
	_, err = s.Crear(ctx, profesor, inexistente)
	esperarTipo(t, err, apperr.TipoValidacion)

	p := periodos.periodos["p1"]
	p.CreacionHabilitada = false
	periodos.periodos["p1"] = p
	_, err = s.Crear(ctx, profesor, n)
	esperarTipo(t, err, apperr.TipoReglaNegocio) // RF-03.2: período no habilitado
}

func TestVisibilidadYCierreDeEspacios(t *testing.T) {
	s, repo, _ := entornoEspacios(t)
	ctx := context.Background()

	_, _ = s.Listar(ctx, monitor, dominio.FiltroEspacios{})
	if repo.filtro.VisiblePara != "mon-1" {
		t.Error("un monitor solo lista los espacios en los que participa")
	}
	_, _ = s.Listar(ctx, admin, dominio.FiltroEspacios{})
	if repo.filtro.VisiblePara != "" {
		t.Error("el administrador lista todos los espacios")
	}

	_, err := s.Obtener(ctx, monitor, "e1")
	esperarTipo(t, err, apperr.TipoNoEncontrado)
	repo.participante = true
	if _, err := s.Obtener(ctx, monitor, "e1"); err != nil {
		t.Errorf("un participante puede ver el espacio: %v", err)
	}
	_, err = s.Cerrar(ctx, monitor, "e1")
	esperarTipo(t, err, apperr.TipoProhibido)

	_, err = s.Cerrar(ctx, otroProf, "e1")
	esperarTipo(t, err, apperr.TipoProhibido) // participa (repo.participante) pero no es el responsable
	if e, err := s.Cerrar(ctx, profesor, "e1"); err != nil || e.Estado != dominio.EspacioCerrado {
		t.Errorf("el profesor responsable puede cerrar el espacio: %+v %v", e, err)
	}
}

// --- vinculaciones -------------------------------------------------------------------------

type repoVinculacionesFalso struct {
	simultaneas   []dominio.Vinculacion
	vinculaciones map[string]dominio.Vinculacion
	creada        dominio.Vinculacion
	rectificada   struct {
		rol    dominio.Rol
		horas  int
		motivo string
	}
	filtro dominio.FiltroVinculaciones
}

func (r *repoVinculacionesFalso) Crear(_ context.Context, v dominio.Vinculacion, validar dominio.ValidadorCarga) (dominio.Vinculacion, error) {
	if err := validar(r.simultaneas, v); err != nil {
		return dominio.Vinculacion{}, err
	}
	v.ID = "v-nueva"
	r.creada = v
	return v, nil
}

func (r *repoVinculacionesFalso) Rectificar(_ context.Context, id string, rol dominio.Rol, horas int, _, motivo string, validar dominio.ValidadorCarga) (dominio.Vinculacion, error) {
	v := r.vinculaciones[id]
	v.Rol, v.HorasSemanales = rol, horas
	if err := validar(r.simultaneas, v); err != nil {
		return dominio.Vinculacion{}, err
	}
	r.rectificada.rol, r.rectificada.horas, r.rectificada.motivo = rol, horas, motivo
	return v, nil
}

func (r *repoVinculacionesFalso) Obtener(_ context.Context, id string) (dominio.Vinculacion, error) {
	v, ok := r.vinculaciones[id]
	if !ok {
		return v, apperr.NoEncontrado("vinculación no encontrada")
	}
	return v, nil
}

func (r *repoVinculacionesFalso) Listar(_ context.Context, f dominio.FiltroVinculaciones) ([]dominio.Vinculacion, error) {
	r.filtro = f
	var resultado []dominio.Vinculacion
	for _, v := range r.vinculaciones {
		if (f.UsuarioID == "" || v.UsuarioID == f.UsuarioID) && (f.ProfesorID == "" || v.ProfesorID == f.ProfesorID) {
			resultado = append(resultado, v)
		}
	}
	return resultado, nil
}

func (r *repoVinculacionesFalso) ListarRectificaciones(context.Context, string) ([]dominio.Rectificacion, error) {
	return nil, nil
}

func entornoVinculaciones(t *testing.T) (*Vinculaciones, *repoVinculacionesFalso) {
	usuarios := nuevoRepoUsuarios()
	usuarios.usuarios["u1"] = dominio.Usuario{ID: "u1", Correo: "laura@uniandes.edu.co", Activo: true,
		Roles: []dominio.Rol{dominio.RolMonitor, dominio.RolAsistenteGraduado}}
	usuarios.usuarios["u2"] = dominio.Usuario{ID: "u2", Correo: "solo-monitor@uniandes.edu.co", Activo: true, Roles: []dominio.Rol{dominio.RolMonitor}}
	espacios := &repoEspaciosFalso{espacios: map[string]dominio.Espacio{
		"e1": {ID: "e1", ProfesorID: "prof-1", Tipo: dominio.EspacioCurso, Estado: dominio.EspacioActivo,
			FechaInicio: fechaP(t, "2026-08-03"), FechaCierre: fechaP(t, "2026-12-04")},
		"cerrado": {ID: "cerrado", ProfesorID: "prof-1", Tipo: dominio.EspacioCurso, Estado: dominio.EspacioCerrado},
	}}
	repo := &repoVinculacionesFalso{vinculaciones: map[string]dominio.Vinculacion{}}
	return NuevoVinculaciones(repo, usuarios, espacios, logPrueba), repo
}

func TestCrearVinculacion(t *testing.T) {
	s, repo := entornoVinculaciones(t)
	ctx := context.Background()

	v, err := s.Crear(ctx, profesor, NuevaVinculacion{EspacioID: "e1", Correo: "Laura@uniandes.edu.co", Rol: dominio.RolMonitor, HorasSemanales: 4})
	if err != nil || v.UsuarioID != "u1" || repo.creada.EspacioFechaCierre.IsZero() {
		t.Fatalf("vinculación inesperada: %+v %v", v, err)
	}

	casos := []struct {
		nombre string
		actor  auth.Actor
		n      NuevaVinculacion
		tipo   apperr.Tipo
	}{
		{"solo profesores", monitor, NuevaVinculacion{EspacioID: "e1", UsuarioID: "u1", Rol: dominio.RolMonitor, HorasSemanales: 2}, apperr.TipoProhibido},
		{"profesor no responsable", otroProf, NuevaVinculacion{EspacioID: "e1", UsuarioID: "u1", Rol: dominio.RolMonitor, HorasSemanales: 2}, apperr.TipoProhibido},
		{"espacio cerrado (RN-09)", profesor, NuevaVinculacion{EspacioID: "cerrado", UsuarioID: "u1", Rol: dominio.RolMonitor, HorasSemanales: 2}, apperr.TipoReglaNegocio},
		{"espacio inexistente", profesor, NuevaVinculacion{EspacioID: "x", UsuarioID: "u1", Rol: dominio.RolMonitor, HorasSemanales: 2}, apperr.TipoValidacion},
		{"rol inválido", profesor, NuevaVinculacion{EspacioID: "e1", UsuarioID: "u1", Rol: dominio.RolProfesor, HorasSemanales: 2}, apperr.TipoValidacion},
		{"horas en cero", profesor, NuevaVinculacion{EspacioID: "e1", UsuarioID: "u1", Rol: dominio.RolMonitor}, apperr.TipoValidacion},
		{"persona sin el rol", profesor, NuevaVinculacion{EspacioID: "e1", UsuarioID: "u2", Rol: dominio.RolAsistenteGraduado, HorasSemanales: 2}, apperr.TipoReglaNegocio},
		{"persona inexistente", profesor, NuevaVinculacion{EspacioID: "e1", Correo: "nadie@x.co", Rol: dominio.RolMonitor, HorasSemanales: 2}, apperr.TipoValidacion},
		{"sin persona", profesor, NuevaVinculacion{EspacioID: "e1", Rol: dominio.RolMonitor, HorasSemanales: 2}, apperr.TipoValidacion},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := s.Crear(ctx, c.actor, c.n)
			esperarTipo(t, err, c.tipo)
		})
	}
}

func TestCrearVinculacionAplicaLasReglasDeHoras(t *testing.T) {
	s, repo := entornoVinculaciones(t)
	ctx := context.Background()
	// Ejemplo del enunciado: 12 h de asistencia y 3 h de monitoría; otra monitoría de 3 h supera el 40 % (5 h).
	repo.simultaneas = []dominio.Vinculacion{
		{Rol: dominio.RolAsistenteGraduado, HorasSemanales: 12, EspacioEstado: dominio.EspacioActivo},
		{Rol: dominio.RolMonitor, HorasSemanales: 3, EspacioEstado: dominio.EspacioActivo},
	}
	_, err := s.Crear(ctx, profesor, NuevaVinculacion{EspacioID: "e1", UsuarioID: "u1", Rol: dominio.RolMonitor, HorasSemanales: 3})
	esperarTipo(t, err, apperr.TipoReglaNegocio)

	if _, err := s.Crear(ctx, profesor, NuevaVinculacion{EspacioID: "e1", UsuarioID: "u1", Rol: dominio.RolMonitor, HorasSemanales: 2}); err != nil {
		t.Errorf("3 + 2 = 5 h de monitoría está permitido: %v", err)
	}
}

func TestRectificarVinculacion(t *testing.T) {
	s, repo := entornoVinculaciones(t)
	ctx := context.Background()
	repo.vinculaciones["v1"] = dominio.Vinculacion{ID: "v1", UsuarioID: "u2", ProfesorID: "prof-1", Rol: dominio.RolMonitor,
		HorasSemanales: 4, EspacioEstado: dominio.EspacioActivo}
	seis, trece := 6, 13
	asistente := dominio.RolAsistenteGraduado

	_, err := s.Rectificar(ctx, profesor, "v1", SolicitudRectificacion{HorasSemanales: &seis, Motivo: "x"})
	esperarTipo(t, err, apperr.TipoProhibido) // RF-04.4: solo el administrador
	_, err = s.Rectificar(ctx, admin, "v1", SolicitudRectificacion{HorasSemanales: &seis})
	esperarTipo(t, err, apperr.TipoValidacion) // motivo obligatorio
	_, err = s.Rectificar(ctx, admin, "v1", SolicitudRectificacion{Motivo: "sin cambios"})
	esperarTipo(t, err, apperr.TipoValidacion)
	_, err = s.Rectificar(ctx, admin, "v1", SolicitudRectificacion{HorasSemanales: &trece, Motivo: "x"})
	esperarTipo(t, err, apperr.TipoReglaNegocio) // 13 h de monitoría
	_, err = s.Rectificar(ctx, admin, "v1", SolicitudRectificacion{Rol: &asistente, Motivo: "x"})
	esperarTipo(t, err, apperr.TipoReglaNegocio) // la persona no tiene el rol

	if _, err := s.Rectificar(ctx, admin, "v1", SolicitudRectificacion{HorasSemanales: &seis, Motivo: "Error de digitación"}); err != nil {
		t.Fatal(err)
	}
	if repo.rectificada.horas != 6 || repo.rectificada.motivo != "Error de digitación" {
		t.Errorf("rectificación inesperada: %+v", repo.rectificada)
	}
}

func TestVisibilidadDeVinculaciones(t *testing.T) {
	s, repo := entornoVinculaciones(t)
	ctx := context.Background()
	repo.vinculaciones["v1"] = dominio.Vinculacion{ID: "v1", UsuarioID: "u1", ProfesorID: "prof-1"}

	_, _ = s.Listar(ctx, monitor, dominio.FiltroVinculaciones{})
	if repo.filtro.VisiblePara != "mon-1" {
		t.Error("los listados deben filtrarse por el actor")
	}
	_, err := s.Obtener(ctx, otroProf, "v1")
	esperarTipo(t, err, apperr.TipoNoEncontrado)
	if _, err := s.Obtener(ctx, auth.Actor{UsuarioID: "u1", Roles: []dominio.Rol{dominio.RolMonitor}}, "v1"); err != nil {
		t.Errorf("la persona vinculada puede ver su vinculación: %v", err)
	}
}
