package handler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
	"seguimiento/internal/servicio"
)

// ServicioTareas es lo que el handler de tareas necesita.
type ServicioTareas interface {
	Crear(ctx context.Context, actor auth.Actor, n servicio.NuevaTarea, archivos []servicio.ArchivoAdjunto) (dominio.Tarea, error)
	Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroTareas) ([]dominio.Tarea, error)
	Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.Tarea, error)
	Actualizar(ctx context.Context, actor auth.Actor, id string, d dominio.DatosTarea) (dominio.Tarea, error)
	Eliminar(ctx context.Context, actor auth.Actor, id string) error
	AgregarAdjuntos(ctx context.Context, actor auth.Actor, id string, archivos []servicio.ArchivoAdjunto) (dominio.Tarea, error)
	AbrirAdjunto(ctx context.Context, actor auth.Actor, tareaID, adjuntoID string) (io.ReadCloser, dominio.Adjunto, error)
	EliminarAdjunto(ctx context.Context, actor auth.Actor, tareaID, adjuntoID string) error
	Modificable(actor auth.Actor, t dominio.Tarea) bool
}

// HandlerTareas expone el registro semanal de tareas (RF-06 a RF-08).
type HandlerTareas struct {
	servicio ServicioTareas
	// maxCuerpo limita las peticiones con archivos (todos los adjuntos juntos).
	maxCuerpo int64
	log       *slog.Logger
}

// Registrar declara las rutas del módulo. Registrar, editar y eliminar es
// exclusivo de monitores y asistentes graduados; consultar, de todos los roles
// según la visibilidad que aplica el servicio.
func (h *HandlerTareas) Registrar(g *gin.RouterGroup) {
	operativos := RequiereRol(dominio.RolesOperativos...)
	r := g.Group("/tareas")
	r.GET("", h.Listar)
	r.GET("/:id", h.Obtener)
	r.POST("", operativos, h.Crear)
	r.PUT("/:id", operativos, h.Actualizar)
	r.DELETE("/:id", operativos, h.Eliminar)
	r.POST("/:id/adjuntos", operativos, h.AgregarAdjuntos)
	r.GET("/:id/adjuntos/:adjunto_id", h.DescargarAdjunto)
	r.DELETE("/:id/adjuntos/:adjunto_id", operativos, h.EliminarAdjunto)
}

// --- formatos -------------------------------------------------------------------

type tareaJSON struct {
	ID               string              `json:"id"`
	VinculacionID    string              `json:"vinculacion_id"`
	Usuario          personaRef          `json:"usuario"`
	Espacio          espacioRef          `json:"espacio"`
	Rol              dominio.Rol         `json:"rol"`
	SemanaInicio     string              `json:"semana_inicio"`
	SemanaFin        string              `json:"semana_fin"`
	Titulo           string              `json:"titulo"`
	Descripcion      string              `json:"descripcion"`
	Estado           dominio.EstadoTarea `json:"estado"`
	HorasInvertidas  int                 `json:"horas_invertidas"`
	Observaciones    string              `json:"observaciones"`
	Tardia           bool                `json:"tardia"`
	Modificable      bool                `json:"modificable"`
	CantidadAdjuntos int                 `json:"cantidad_adjuntos"`
	Adjuntos         []adjuntoJSON       `json:"adjuntos,omitempty"`
	CreadoEn         time.Time           `json:"creado_en"`
	ActualizadoEn    time.Time           `json:"actualizado_en"`
}

type adjuntoJSON struct {
	ID            string    `json:"id"`
	Nombre        string    `json:"nombre"`
	TipoContenido string    `json:"tipo_contenido"`
	TamanoBytes   int64     `json:"tamano_bytes"`
	CreadoEn      time.Time `json:"creado_en"`
	URLDescarga   string    `json:"url_descarga"`
}

func (h *HandlerTareas) aTareaJSON(actor auth.Actor, t dominio.Tarea) tareaJSON {
	j := tareaJSON{
		ID: t.ID, VinculacionID: t.VinculacionID,
		Usuario:      personaRef{ID: t.UsuarioID, Nombre: t.UsuarioNombre},
		Espacio:      espacioRef{ID: t.EspacioID, Nombre: t.EspacioNombre, Tipo: t.EspacioTipo, Estado: t.EspacioEstado},
		Rol:          t.Rol,
		SemanaInicio: fecha(t.SemanaInicio), SemanaFin: fecha(dominio.FinSemana(t.SemanaInicio)),
		Titulo: t.Titulo, Descripcion: t.Descripcion, Estado: t.Estado, HorasInvertidas: t.HorasInvertidas,
		Observaciones: t.Observaciones, Tardia: t.Tardia, Modificable: h.servicio.Modificable(actor, t),
		CantidadAdjuntos: t.CantidadAdjuntos, CreadoEn: t.CreadoEn, ActualizadoEn: t.ActualizadoEn,
	}
	for _, a := range t.Adjuntos {
		j.Adjuntos = append(j.Adjuntos, adjuntoJSON{
			ID: a.ID, Nombre: a.NombreOriginal, TipoContenido: a.TipoContenido, TamanoBytes: a.TamanoBytes, CreadoEn: a.CreadoEn,
			URLDescarga: "/api/v1/tareas/" + t.ID + "/adjuntos/" + a.ID,
		})
	}
	return j
}

// peticionTarea crea una tarea. semana es cualquier fecha (AAAA-MM-DD) de la
// semana reportada; si se omite, se usa la semana en curso.
type peticionTarea struct {
	VinculacionID   string              `json:"vinculacion_id"`
	Semana          string              `json:"semana"`
	Titulo          string              `json:"titulo"`
	Descripcion     string              `json:"descripcion"`
	Estado          dominio.EstadoTarea `json:"estado"`
	HorasInvertidas int                 `json:"horas_invertidas"`
	Observaciones   string              `json:"observaciones"`
}

type peticionActualizarTarea struct {
	Titulo          string              `json:"titulo"`
	Descripcion     string              `json:"descripcion"`
	Estado          dominio.EstadoTarea `json:"estado"`
	HorasInvertidas int                 `json:"horas_invertidas"`
	Observaciones   string              `json:"observaciones"`
}

func (p peticionActualizarTarea) datos() dominio.DatosTarea {
	return dominio.DatosTarea{Titulo: p.Titulo, Descripcion: p.Descripcion, Estado: p.Estado,
		HorasInvertidas: p.HorasInvertidas, Observaciones: p.Observaciones}
}

// leerPeticionTarea lee la tarea de un JSON o, si la petición es multipart, del
// campo "datos" junto con los archivos del campo "adjuntos". cerrar libera los
// archivos abiertos y siempre debe invocarse, incluso si hay error.
func (h *HandlerTareas) leerPeticionTarea(c *gin.Context) (p peticionTarea, archivos []servicio.ArchivoAdjunto, cerrar func(), err error) {
	cerrar = func() {}
	if !esMultipart(c) {
		return p, nil, cerrar, leerJSON(c, &p)
	}
	limitarCuerpo(c, h.maxCuerpo)
	form, err := formularioMultipart(c)
	if err != nil {
		return p, nil, cerrar, err
	}
	datos := form.Value["datos"]
	if len(datos) != 1 {
		return p, nil, cerrar, apperr.Validacion("el formulario debe incluir el campo \"datos\" con el JSON de la tarea")
	}
	if err := decodificarJSON(strings.NewReader(datos[0]), &p); err != nil {
		return p, nil, cerrar, err
	}
	archivos, cerrar, err = leerArchivos(form)
	return p, archivos, cerrar, err
}

// --- endpoints ------------------------------------------------------------------

// Crear registra una tarea (RF-06.1 a RF-06.6). Acepta JSON, o un formulario
// multipart con el campo "datos" (el mismo JSON) y los archivos en "adjuntos",
// para registrar la tarea y su evidencia en una sola operación.
func (h *HandlerTareas) Crear(c *gin.Context) {
	p, archivos, cerrar, err := h.leerPeticionTarea(c)
	defer cerrar()
	if err != nil {
		responderError(c, h.log, err)
		return
	}

	vinculacionID, err := validarUUIDOpcional("vinculacion_id", p.VinculacionID)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	var semana time.Time // vacía = semana en curso, la decide el servicio
	if p.Semana != "" {
		if semana, err = parsearFecha("semana", p.Semana); err != nil {
			responderError(c, h.log, err)
			return
		}
	}
	actor := ActorDe(c)
	t, err := h.servicio.Crear(c.Request.Context(), actor, servicio.NuevaTarea{
		VinculacionID: vinculacionID,
		Semana:        semana,
		Datos: peticionActualizarTarea{Titulo: p.Titulo, Descripcion: p.Descripcion, Estado: p.Estado,
			HorasInvertidas: p.HorasInvertidas, Observaciones: p.Observaciones}.datos(),
	}, archivos)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.Header("Location", "/api/v1/tareas/"+t.ID)
	c.JSON(http.StatusCreated, h.aTareaJSON(actor, t))
}

// Listar devuelve las tareas visibles con filtros opcionales semana,
// vinculacion_id, espacio_id y usuario_id (RF-10.5, RF-11.2, RF-11.3).
func (h *HandlerTareas) Listar(c *gin.Context) {
	semana, err := consultaSemana(c, "semana")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	f := dominio.FiltroTareas{SemanaInicio: semana, Paginacion: consultaPaginacion(c)}
	for nombre, destino := range map[string]*string{"vinculacion_id": &f.VinculacionID, "espacio_id": &f.EspacioID, "usuario_id": &f.UsuarioID} {
		if *destino, err = consultaUUID(c, nombre); err != nil {
			responderError(c, h.log, err)
			return
		}
	}
	actor := ActorDe(c)
	tareas, err := h.servicio.Listar(c.Request.Context(), actor, f)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, convertir(tareas, func(t dominio.Tarea) tareaJSON { return h.aTareaJSON(actor, t) }))
}

// Obtener devuelve una tarea con sus adjuntos.
func (h *HandlerTareas) Obtener(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	actor := ActorDe(c)
	t, err := h.servicio.Obtener(c.Request.Context(), actor, id)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, h.aTareaJSON(actor, t))
}

// Actualizar reemplaza los datos editables de la tarea (RF-06.7, RF-07.2).
func (h *HandlerTareas) Actualizar(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	var p peticionActualizarTarea
	if err := leerJSON(c, &p); err != nil {
		responderError(c, h.log, err)
		return
	}
	actor := ActorDe(c)
	t, err := h.servicio.Actualizar(c.Request.Context(), actor, id, p.datos())
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, h.aTareaJSON(actor, t))
}

// Eliminar borra la tarea (RF-06.8).
func (h *HandlerTareas) Eliminar(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	if err := h.servicio.Eliminar(c.Request.Context(), ActorDe(c), id); err != nil {
		responderError(c, h.log, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// AgregarAdjuntos añade archivos (campo multipart "adjuntos") a la tarea (RF-06.4).
func (h *HandlerTareas) AgregarAdjuntos(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	if !esMultipart(c) {
		responderError(c, h.log, apperr.Validacion("envíe los archivos como multipart/form-data en el campo \"adjuntos\""))
		return
	}
	limitarCuerpo(c, h.maxCuerpo)
	form, err := formularioMultipart(c)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	archivos, cerrar, err := leerArchivos(form)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	defer cerrar()
	actor := ActorDe(c)
	t, err := h.servicio.AgregarAdjuntos(c.Request.Context(), actor, id, archivos)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusCreated, h.aTareaJSON(actor, t))
}

// DescargarAdjunto entrega el archivo adjunto.
func (h *HandlerTareas) DescargarAdjunto(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	adjuntoID, err := parametroUUID(c, "adjunto_id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	contenido, a, err := h.servicio.AbrirAdjunto(c.Request.Context(), ActorDe(c), id, adjuntoID)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	defer func() { _ = contenido.Close() }()
	servirArchivo(c, contenido, a.TamanoBytes, a.TipoContenido, a.NombreOriginal)
}

// EliminarAdjunto quita un archivo de la tarea.
func (h *HandlerTareas) EliminarAdjunto(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	adjuntoID, err := parametroUUID(c, "adjunto_id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	if err := h.servicio.EliminarAdjunto(c.Request.Context(), ActorDe(c), id, adjuntoID); err != nil {
		responderError(c, h.log, err)
		return
	}
	c.Status(http.StatusNoContent)
}
