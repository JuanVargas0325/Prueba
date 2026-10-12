package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
	"seguimiento/internal/servicio"
)

// --- tareas -----------------------------------------------------------------------

type servicioTareasFalso struct {
	nueva       servicio.NuevaTarea
	archivos    map[string]string // nombre → contenido
	actualizada dominio.DatosTarea
	filtro      dominio.FiltroTareas
	eliminada   string
}

func tareaFalsa() dominio.Tarea {
	return dominio.Tarea{ID: uuidA, VinculacionID: uuidB, UsuarioID: "usuario-1", Titulo: "Tarea",
		SemanaInicio: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), Estado: dominio.TareaAbierta,
		Adjuntos: []dominio.Adjunto{{ID: uuidB, NombreOriginal: "evidencia.pdf", TipoContenido: "application/pdf", TamanoBytes: 4}}}
}

func (s *servicioTareasFalso) Crear(_ context.Context, _ auth.Actor, n servicio.NuevaTarea, archivos []servicio.ArchivoAdjunto) (dominio.Tarea, error) {
	s.nueva = n
	s.archivos = map[string]string{}
	for _, a := range archivos {
		b, _ := io.ReadAll(a.Contenido)
		s.archivos[a.Nombre] = string(b)
	}
	return tareaFalsa(), nil
}
func (s *servicioTareasFalso) Listar(_ context.Context, _ auth.Actor, f dominio.FiltroTareas) ([]dominio.Tarea, error) {
	s.filtro = f
	return []dominio.Tarea{tareaFalsa()}, nil
}
func (s *servicioTareasFalso) Obtener(context.Context, auth.Actor, string) (dominio.Tarea, error) {
	return tareaFalsa(), nil
}
func (s *servicioTareasFalso) Actualizar(_ context.Context, _ auth.Actor, _ string, d dominio.DatosTarea) (dominio.Tarea, error) {
	s.actualizada = d
	return tareaFalsa(), nil
}
func (s *servicioTareasFalso) Eliminar(_ context.Context, _ auth.Actor, id string) error {
	s.eliminada = id
	return nil
}
func (s *servicioTareasFalso) AgregarAdjuntos(_ context.Context, _ auth.Actor, _ string, archivos []servicio.ArchivoAdjunto) (dominio.Tarea, error) {
	if len(archivos) == 0 {
		return dominio.Tarea{}, apperr.Validacion("sin archivos")
	}
	return tareaFalsa(), nil
}
func (s *servicioTareasFalso) AbrirAdjunto(context.Context, auth.Actor, string, string) (io.ReadCloser, dominio.Adjunto, error) {
	return io.NopCloser(strings.NewReader("%PDF")), tareaFalsa().Adjuntos[0], nil
}
func (s *servicioTareasFalso) EliminarAdjunto(context.Context, auth.Actor, string, string) error {
	return nil
}
func (s *servicioTareasFalso) Modificable(auth.Actor, dominio.Tarea) bool { return true }

func entornoTareas(t *testing.T) (entorno, *servicioTareasFalso) {
	srv := &servicioTareasFalso{}
	return nuevoEntorno(t, func(d *Dependencias) { d.Tareas = srv }), srv
}

func TestCrearTareaConJSON(t *testing.T) {
	e, srv := entornoTareas(t)
	w := e.peticion(http.MethodPost, "/api/v1/tareas", e.token(t, dominio.RolMonitor),
		`{"vinculacion_id":"`+uuidB+`","semana":"2026-10-07","titulo":"Calificar","estado":"en_desarrollo","horas_invertidas":3}`)
	esperarEstado(t, w, http.StatusCreated)
	if srv.nueva.VinculacionID != uuidB || srv.nueva.Semana.Format(dominio.FormatoFecha) != "2026-10-07" ||
		srv.nueva.Datos.HorasInvertidas != 3 || srv.nueva.Datos.Estado != dominio.TareaEnDesarrollo {
		t.Errorf("datos recibidos inesperados: %+v", srv.nueva)
	}
	var j tareaJSON
	_ = json.Unmarshal(w.Body.Bytes(), &j)
	if j.SemanaInicio != "2026-10-05" || j.SemanaFin != "2026-10-11" || !j.Modificable ||
		j.Adjuntos[0].URLDescarga != "/api/v1/tareas/"+uuidA+"/adjuntos/"+uuidB {
		t.Errorf("respuesta inesperada: %+v", j)
	}
}

func TestCrearTareaSinSemanaUsaLaSemanaEnCurso(t *testing.T) {
	e, srv := entornoTareas(t)
	esperarEstado(t, e.peticion(http.MethodPost, "/api/v1/tareas", e.token(t, dominio.RolAsistenteGraduado),
		`{"vinculacion_id":"`+uuidB+`","titulo":"x"}`), http.StatusCreated)
	if !srv.nueva.Semana.IsZero() {
		t.Error("sin semana, el handler debe dejar que el servicio use la semana en curso")
	}
}

func TestCrearTareaRechazaHorasNoEnterasYCamposDesconocidos(t *testing.T) {
	e, _ := entornoTareas(t)
	monitor := e.token(t, dominio.RolMonitor)
	for _, cuerpo := range []string{
		`{"vinculacion_id":"` + uuidB + `","titulo":"x","horas_invertidas":2.5}`, // RN-24: horas enteras
		`{"vinculacion_id":"` + uuidB + `","titulo":"x","prioridad":"alta"}`,
		`{"vinculacion_id":"no-uuid","titulo":"x"}`,
		`{"vinculacion_id":"` + uuidB + `","titulo":"x","semana":"07/10/2026"}`,
	} {
		if w := e.peticion(http.MethodPost, "/api/v1/tareas", monitor, cuerpo); w.Code != http.StatusBadRequest {
			t.Errorf("cuerpo %s: estado = %d", cuerpo, w.Code)
		}
	}
}

func formulario(t *testing.T, datos string, archivos map[string]string) (string, *bytes.Buffer) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	if datos != "" {
		_ = mw.WriteField("datos", datos)
	}
	for nombre, contenido := range archivos {
		fw, err := mw.CreateFormFile(campoAdjuntos, nombre)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write([]byte(contenido))
	}
	_ = mw.Close()
	return mw.FormDataContentType(), &b
}

func TestCrearTareaConAdjuntosMultipart(t *testing.T) {
	e, srv := entornoTareas(t)
	tipo, cuerpo := formulario(t, `{"vinculacion_id":"`+uuidB+`","titulo":"Con evidencia","horas_invertidas":1}`,
		map[string]string{"evidencia.pdf": "%PDF-contenido", "foto.png": "png"})
	w := e.peticionCon(http.MethodPost, "/api/v1/tareas", e.token(t, dominio.RolMonitor), tipo, cuerpo)
	esperarEstado(t, w, http.StatusCreated)
	if srv.nueva.Datos.Titulo != "Con evidencia" || srv.archivos["evidencia.pdf"] != "%PDF-contenido" || len(srv.archivos) != 2 {
		t.Errorf("el servicio debería recibir datos y archivos: %+v %v", srv.nueva, srv.archivos)
	}
}

func TestCrearTareaMultipartSinDatosEsInvalida(t *testing.T) {
	e, _ := entornoTareas(t)
	tipo, cuerpo := formulario(t, "", map[string]string{"a.pdf": "x"})
	esperarEstado(t, e.peticionCon(http.MethodPost, "/api/v1/tareas", e.token(t, dominio.RolMonitor), tipo, cuerpo), http.StatusBadRequest)
}

func TestPeticionDemasiadoGrandeResponde413(t *testing.T) {
	e, _ := entornoTareas(t) // límite de 1 MB en el entorno de prueba
	tipo, cuerpo := formulario(t, `{"vinculacion_id":"`+uuidB+`","titulo":"x"}`, map[string]string{"grande.pdf": strings.Repeat("x", 2<<20)})
	esperarEstado(t, e.peticionCon(http.MethodPost, "/api/v1/tareas", e.token(t, dominio.RolMonitor), tipo, cuerpo), http.StatusRequestEntityTooLarge)
}

func TestListarActualizarYEliminarTareas(t *testing.T) {
	e, srv := entornoTareas(t)
	monitor := e.token(t, dominio.RolMonitor)

	esperarEstado(t, e.peticion(http.MethodGet, "/api/v1/tareas?semana=2026-10-08&vinculacion_id="+uuidB+"&limite=5", monitor, ""), http.StatusOK)
	if srv.filtro.SemanaInicio.Format(dominio.FormatoFecha) != "2026-10-05" || srv.filtro.VinculacionID != uuidB || srv.filtro.Limite != 5 {
		t.Errorf("filtro inesperado: %+v", srv.filtro)
	}

	esperarEstado(t, e.peticion(http.MethodPut, "/api/v1/tareas/"+uuidA, monitor, `{"titulo":"Nuevo","estado":"finalizado","horas_invertidas":4}`), http.StatusOK)
	if srv.actualizada.Titulo != "Nuevo" || srv.actualizada.Estado != dominio.TareaFinalizada {
		t.Errorf("datos de actualización inesperados: %+v", srv.actualizada)
	}

	esperarEstado(t, e.peticion(http.MethodDelete, "/api/v1/tareas/"+uuidA, monitor, ""), http.StatusNoContent)
	if srv.eliminada != uuidA {
		t.Error("debería eliminarse la tarea indicada")
	}
}

func TestAdjuntosDeTareas(t *testing.T) {
	e, _ := entornoTareas(t)
	monitor := e.token(t, dominio.RolMonitor)

	esperarEstado(t, e.peticion(http.MethodPost, "/api/v1/tareas/"+uuidA+"/adjuntos", monitor, `{}`), http.StatusBadRequest)

	tipo, cuerpo := formulario(t, "", map[string]string{"nuevo.pdf": "%PDF"})
	esperarEstado(t, e.peticionCon(http.MethodPost, "/api/v1/tareas/"+uuidA+"/adjuntos", monitor, tipo, cuerpo), http.StatusCreated)

	w := e.peticion(http.MethodGet, "/api/v1/tareas/"+uuidA+"/adjuntos/"+uuidB, e.token(t, dominio.RolProfesor), "")
	esperarEstado(t, w, http.StatusOK)
	if w.Header().Get("Content-Type") != "application/pdf" || !strings.Contains(w.Header().Get("Content-Disposition"), "evidencia.pdf") {
		t.Errorf("cabeceras inesperadas: %v", w.Header())
	}

	esperarEstado(t, e.peticion(http.MethodDelete, "/api/v1/tareas/"+uuidA+"/adjuntos/"+uuidB, monitor, ""), http.StatusNoContent)
}

// --- usuarios, períodos, espacios y vinculaciones ------------------------------------

type servicioUsuariosFalso struct{ nuevo servicio.NuevoUsuario }

func (s *servicioUsuariosFalso) Crear(_ context.Context, _ auth.Actor, n servicio.NuevoUsuario) (dominio.Usuario, error) {
	s.nuevo = n
	return dominio.Usuario{ID: uuidA, Nombre: n.Nombre, Correo: n.Correo, Roles: n.Roles, PasswordHash: "secreto"}, nil
}
func (s *servicioUsuariosFalso) Listar(context.Context, auth.Actor, dominio.FiltroUsuarios) ([]dominio.Usuario, error) {
	return []dominio.Usuario{{ID: uuidA}}, nil
}
func (s *servicioUsuariosFalso) Obtener(context.Context, auth.Actor, string) (dominio.Usuario, error) {
	return dominio.Usuario{ID: uuidA}, nil
}
func (s *servicioUsuariosFalso) AsignarRoles(_ context.Context, _ auth.Actor, _ string, roles []dominio.Rol) (dominio.Usuario, error) {
	return dominio.Usuario{ID: uuidA, Roles: roles}, nil
}

func TestCrearUsuarioNoExponeElHash(t *testing.T) {
	srv := &servicioUsuariosFalso{}
	e := nuevoEntorno(t, func(d *Dependencias) { d.Usuarios = srv })
	w := e.peticion(http.MethodPost, "/api/v1/usuarios", e.token(t, dominio.RolAdministrador),
		`{"nombre":"Ana","correo":"ana@uniandes.edu.co","password":"Clave123*","roles":["monitor"]}`)
	esperarEstado(t, w, http.StatusCreated)
	if strings.Contains(w.Body.String(), "secreto") || strings.Contains(w.Body.String(), "password") {
		t.Errorf("la respuesta no debe incluir la contraseña ni su hash: %s", w.Body.String())
	}
	if srv.nuevo.Roles[0] != dominio.RolMonitor || srv.nuevo.Password != "Clave123*" {
		t.Errorf("datos recibidos inesperados: %+v", srv.nuevo)
	}
	esperarEstado(t, e.peticion(http.MethodPut, "/api/v1/usuarios/"+uuidA+"/roles", e.token(t, dominio.RolAdministrador), `{"roles":["profesor"]}`), http.StatusOK)
}

type servicioPeriodosFalso struct {
	nuevo   servicio.NuevoPeriodo
	cambios servicio.CambiosPeriodo
}

func (s *servicioPeriodosFalso) Crear(_ context.Context, _ auth.Actor, n servicio.NuevoPeriodo) (dominio.PeriodoAcademico, error) {
	s.nuevo = n
	return dominio.PeriodoAcademico{ID: uuidA, Codigo: n.Codigo, FechaInicio: n.FechaInicio, FechaCierre: n.FechaCierre}, nil
}
func (s *servicioPeriodosFalso) Listar(context.Context, auth.Actor, dominio.FiltroPeriodos) ([]dominio.PeriodoAcademico, error) {
	return nil, nil
}
func (s *servicioPeriodosFalso) Obtener(context.Context, auth.Actor, string) (dominio.PeriodoAcademico, error) {
	return dominio.PeriodoAcademico{}, nil
}
func (s *servicioPeriodosFalso) Actualizar(_ context.Context, _ auth.Actor, _ string, c servicio.CambiosPeriodo) (dominio.PeriodoAcademico, error) {
	s.cambios = c
	return dominio.PeriodoAcademico{}, nil
}

func TestPeriodos(t *testing.T) {
	srv := &servicioPeriodosFalso{}
	e := nuevoEntorno(t, func(d *Dependencias) { d.Periodos = srv })
	admin := e.token(t, dominio.RolAdministrador)

	w := e.peticion(http.MethodPost, "/api/v1/periodos", admin, `{"codigo":"2026-20","fecha_inicio":"2026-08-01","fecha_cierre":"2026-12-15"}`)
	esperarEstado(t, w, http.StatusCreated)
	if !srv.nuevo.CreacionHabilitada || !strings.Contains(w.Body.String(), `"fecha_inicio":"2026-08-01"`) {
		t.Errorf("la ventana de creación debería quedar habilitada por defecto: %+v %s", srv.nuevo, w.Body.String())
	}
	esperarEstado(t, e.peticion(http.MethodPost, "/api/v1/periodos", admin, `{"codigo":"2026-20","fecha_inicio":"1/8/2026","fecha_cierre":"2026-12-15"}`), http.StatusBadRequest)

	esperarEstado(t, e.peticion(http.MethodPatch, "/api/v1/periodos/"+uuidA, admin, `{"creacion_habilitada":false}`), http.StatusOK)
	if srv.cambios.CreacionHabilitada == nil || *srv.cambios.CreacionHabilitada || srv.cambios.Estado != nil {
		t.Errorf("cambios inesperados: %+v", srv.cambios)
	}
	esperarEstado(t, e.peticion(http.MethodGet, "/api/v1/periodos", e.token(t, dominio.RolMonitor), ""), http.StatusOK)
}

type servicioEspaciosFalso struct{ nuevo servicio.NuevoEspacio }

func (s *servicioEspaciosFalso) Crear(_ context.Context, _ auth.Actor, n servicio.NuevoEspacio) (dominio.Espacio, error) {
	s.nuevo = n
	return dominio.Espacio{ID: uuidA, Estado: dominio.EspacioActivo}, nil
}
func (s *servicioEspaciosFalso) Listar(context.Context, auth.Actor, dominio.FiltroEspacios) ([]dominio.Espacio, error) {
	return nil, nil
}
func (s *servicioEspaciosFalso) Obtener(context.Context, auth.Actor, string) (dominio.Espacio, error) {
	return dominio.Espacio{}, apperr.NoEncontrado("curso o proyecto no encontrado")
}
func (s *servicioEspaciosFalso) Cerrar(context.Context, auth.Actor, string) (dominio.Espacio, error) {
	return dominio.Espacio{ID: uuidA, Estado: dominio.EspacioCerrado}, nil
}

func TestEspacios(t *testing.T) {
	srv := &servicioEspaciosFalso{}
	e := nuevoEntorno(t, func(d *Dependencias) { d.Espacios = srv })
	profesor := e.token(t, dominio.RolProfesor)

	esperarEstado(t, e.peticion(http.MethodPost, "/api/v1/espacios", profesor,
		`{"tipo":"curso","nombre":"Nube","periodo_id":"`+uuidB+`","fecha_inicio":"2026-08-03","fecha_cierre":"2026-12-04"}`), http.StatusCreated)
	if srv.nuevo.PeriodoID != uuidB || srv.nuevo.Tipo != dominio.EspacioCurso {
		t.Errorf("datos recibidos inesperados: %+v", srv.nuevo)
	}
	w := e.peticion(http.MethodPost, "/api/v1/espacios/"+uuidA+"/cerrar", profesor, "")
	esperarEstado(t, w, http.StatusOK)
	if !strings.Contains(w.Body.String(), `"solo_lectura":true`) {
		t.Errorf("un espacio cerrado debe indicarse como solo lectura: %s", w.Body.String())
	}
	esperarEstado(t, e.peticion(http.MethodGet, "/api/v1/espacios/"+uuidA, e.token(t, dominio.RolMonitor), ""), http.StatusNotFound)
}

type servicioVinculacionesFalso struct {
	nueva         servicio.NuevaVinculacion
	rectificacion servicio.SolicitudRectificacion
}

func (s *servicioVinculacionesFalso) Crear(_ context.Context, _ auth.Actor, n servicio.NuevaVinculacion) (dominio.Vinculacion, error) {
	s.nueva = n
	if n.HorasSemanales > 22 {
		return dominio.Vinculacion{}, apperr.ReglaNegocio("supera las 22 horas")
	}
	return dominio.Vinculacion{ID: uuidA}, nil
}
func (s *servicioVinculacionesFalso) Listar(context.Context, auth.Actor, dominio.FiltroVinculaciones) ([]dominio.Vinculacion, error) {
	return nil, nil
}
func (s *servicioVinculacionesFalso) Obtener(context.Context, auth.Actor, string) (dominio.Vinculacion, error) {
	return dominio.Vinculacion{ID: uuidA}, nil
}
func (s *servicioVinculacionesFalso) Rectificar(_ context.Context, _ auth.Actor, _ string, r servicio.SolicitudRectificacion) (dominio.Vinculacion, error) {
	s.rectificacion = r
	return dominio.Vinculacion{ID: uuidA}, nil
}
func (s *servicioVinculacionesFalso) ListarRectificaciones(context.Context, auth.Actor, string) ([]dominio.Rectificacion, error) {
	return nil, nil
}

func TestVinculaciones(t *testing.T) {
	srv := &servicioVinculacionesFalso{}
	e := nuevoEntorno(t, func(d *Dependencias) { d.Vinculaciones = srv })
	profesor := e.token(t, dominio.RolProfesor)

	esperarEstado(t, e.peticion(http.MethodPost, "/api/v1/vinculaciones", profesor,
		`{"espacio_id":"`+uuidA+`","correo":"ana@uniandes.edu.co","rol":"monitor","horas_semanales":4}`), http.StatusCreated)
	if srv.nueva.Correo != "ana@uniandes.edu.co" || srv.nueva.Rol != dominio.RolMonitor || srv.nueva.HorasSemanales != 4 {
		t.Errorf("datos recibidos inesperados: %+v", srv.nueva)
	}
	// Las reglas de horas violadas responden 422.
	esperarEstado(t, e.peticion(http.MethodPost, "/api/v1/vinculaciones", profesor,
		`{"espacio_id":"`+uuidA+`","usuario_id":"`+uuidB+`","rol":"asistente_graduado","horas_semanales":23}`), http.StatusUnprocessableEntity)

	esperarEstado(t, e.peticion(http.MethodPut, "/api/v1/vinculaciones/"+uuidA, e.token(t, dominio.RolAdministrador),
		`{"horas_semanales":6,"motivo":"Error de digitación"}`), http.StatusOK)
	if srv.rectificacion.HorasSemanales == nil || *srv.rectificacion.HorasSemanales != 6 || srv.rectificacion.Rol != nil {
		t.Errorf("rectificación inesperada: %+v", srv.rectificacion)
	}
}
