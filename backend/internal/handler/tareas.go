package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
	"seguimiento/internal/servicio"
)

// ServicioTareas es lo que el handler de tareas necesita.
type ServicioTareas interface {
	Crear(ctx context.Context, actor auth.Actor, n servicio.NuevaTarea) (dominio.Tarea, error)
	Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroTareas) ([]dominio.Tarea, error)
	Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.Tarea, error)
	Actualizar(ctx context.Context, actor auth.Actor, id string, d dominio.DatosTarea) (dominio.Tarea, error)
	Eliminar(ctx context.Context, actor auth.Actor, id string) error
	Modificable(actor auth.Actor, t dominio.Tarea) bool
}

// HandlerTareas expone el registro semanal de tareas (RF-06 a RF-08).
type HandlerTareas struct {
	servicio ServicioTareas
	log      *slog.Logger
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
}

// --- formatos -------------------------------------------------------------------

type tareaJSON struct {
	ID              string              `json:"id"`
	VinculacionID   string              `json:"vinculacion_id"`
	Usuario         personaRef          `json:"usuario"`
	Espacio         espacioRef          `json:"espacio"`
	Rol             dominio.Rol         `json:"rol"`
	SemanaInicio    string              `json:"semana_inicio"`
	SemanaFin       string              `json:"semana_fin"`
	Titulo          string              `json:"titulo"`
	Descripcion     string              `json:"descripcion"`
	Estado          dominio.EstadoTarea `json:"estado"`
	HorasInvertidas int                 `json:"horas_invertidas"`
	Observaciones   string              `json:"observaciones"`
	Tardia          bool                `json:"tardia"`
	Modificable     bool                `json:"modificable"`
	CreadoEn        time.Time           `json:"creado_en"`
	ActualizadoEn   time.Time           `json:"actualizado_en"`
}

func (h *HandlerTareas) aTareaJSON(actor auth.Actor, t dominio.Tarea) tareaJSON {
	return tareaJSON{
		ID: t.ID, VinculacionID: t.VinculacionID,
		Usuario:      personaRef{ID: t.UsuarioID, Nombre: t.UsuarioNombre},
		Espacio:      espacioRef{ID: t.EspacioID, Nombre: t.EspacioNombre, Tipo: t.EspacioTipo, Estado: t.EspacioEstado},
		Rol:          t.Rol,
		SemanaInicio: fecha(t.SemanaInicio), SemanaFin: fecha(dominio.FinSemana(t.SemanaInicio)),
		Titulo: t.Titulo, Descripcion: t.Descripcion, Estado: t.Estado, HorasInvertidas: t.HorasInvertidas,
		Observaciones: t.Observaciones, Tardia: t.Tardia, Modificable: h.servicio.Modificable(actor, t),
		CreadoEn: t.CreadoEn, ActualizadoEn: t.ActualizadoEn,
	}
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

// --- endpoints ------------------------------------------------------------------

// Crear registra una tarea (RF-06.1 a RF-06.6).
func (h *HandlerTareas) Crear(c *gin.Context) {
	var p peticionTarea
	if err := leerJSON(c, &p); err != nil {
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
	})
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

// Obtener devuelve una tarea.
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
