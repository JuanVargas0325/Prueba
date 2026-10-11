package handler

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

// ServicioReportes es lo que el handler necesita de la capa de servicio.
type ServicioReportes interface {
	SolicitarLote(ctx context.Context, actor auth.Actor, fechaSemana string) (dominio.LoteReportes, error)
	ListarLotes(ctx context.Context, actor auth.Actor, f dominio.FiltroLotes) ([]dominio.LoteReportes, error)
	ObtenerLote(ctx context.Context, actor auth.Actor, id string) (dominio.LoteReportes, []dominio.ReporteSemanal, error)
	ListarReportes(ctx context.Context, actor auth.Actor, f dominio.FiltroReportes) ([]dominio.ReporteSemanal, error)
	ObtenerReporte(ctx context.Context, actor auth.Actor, id string) (dominio.ReporteSemanal, error)
	AbrirPDF(ctx context.Context, actor auth.Actor, id string) (io.ReadCloser, dominio.ReporteSemanal, error)
}

// HandlerReportes expone la generación y consulta de reportes semanales (RF-14).
type HandlerReportes struct {
	servicio ServicioReportes
	log      *slog.Logger
}

// Registrar declara las rutas del módulo: consultas para profesores y
// administradores; la generación es exclusiva del profesor.
func (h *HandlerReportes) Registrar(g *gin.RouterGroup) {
	r := g.Group("/reportes", RequiereRol(dominio.RolProfesor, dominio.RolAdministrador))
	r.POST("/lotes", RequiereRol(dominio.RolProfesor), h.SolicitarLote)
	r.GET("/lotes", h.ListarLotes)
	r.GET("/lotes/:id", h.ObtenerLote)
	r.GET("", h.ListarReportes)
	r.GET("/:id", h.ObtenerReporte)
	r.GET("/:id/pdf", h.DescargarPDF)
}

// --- formatos de respuesta ---------------------------------------------------

type loteJSON struct {
	ID                  string             `json:"id"`
	ProfesorID          string             `json:"profesor_id"`
	SemanaInicio        string             `json:"semana_inicio"`
	SemanaFin           string             `json:"semana_fin"`
	Estado              dominio.EstadoLote `json:"estado"`
	TotalReportes       int                `json:"total_reportes"`
	ReportesCompletados int                `json:"reportes_completados"`
	ReportesFallidos    int                `json:"reportes_fallidos"`
	Error               string             `json:"error,omitempty"`
	CreadoEn            time.Time          `json:"creado_en"`
	FinalizadoEn        *time.Time         `json:"finalizado_en"`
	Reportes            []reporteJSON      `json:"reportes,omitempty"`
}

type reporteJSON struct {
	ID           string                `json:"id"`
	LoteID       string                `json:"lote_id"`
	ProfesorID   string                `json:"profesor_id"`
	UsuarioID    string                `json:"usuario_id"`
	Persona      string                `json:"persona"`
	SemanaInicio string                `json:"semana_inicio"`
	SemanaFin    string                `json:"semana_fin"`
	Estado       dominio.EstadoReporte `json:"estado"`
	Resumen      string                `json:"resumen,omitempty"`
	Intentos     int                   `json:"intentos"`
	Error        string                `json:"error,omitempty"`
	ProveedorIA  string                `json:"proveedor_ia,omitempty"`
	ModeloIA     string                `json:"modelo_ia,omitempty"`
	TamanoBytes  int64                 `json:"tamano_bytes,omitempty"`
	GeneradoEn   *time.Time            `json:"generado_en"`
	URLDescarga  string                `json:"url_descarga,omitempty"`
}

func aLoteJSON(l dominio.LoteReportes) loteJSON {
	return loteJSON{
		ID: l.ID, ProfesorID: l.ProfesorID,
		SemanaInicio: fecha(l.SemanaInicio),
		SemanaFin:    fecha(dominio.FinSemana(l.SemanaInicio)),
		Estado:       l.Estado, TotalReportes: l.TotalReportes,
		ReportesCompletados: l.ReportesCompletados, ReportesFallidos: l.ReportesFallidos,
		Error: l.Error, CreadoEn: l.CreadoEn, FinalizadoEn: l.FinalizadoEn,
	}
}

func aReporteJSON(r dominio.ReporteSemanal) reporteJSON {
	j := reporteJSON{
		ID: r.ID, LoteID: r.LoteID, ProfesorID: r.ProfesorID, UsuarioID: r.UsuarioID, Persona: r.Persona,
		SemanaInicio: fecha(r.SemanaInicio),
		SemanaFin:    fecha(dominio.FinSemana(r.SemanaInicio)),
		Estado:       r.Estado, Resumen: r.Resumen, Intentos: r.Intentos, Error: r.Error,
		ProveedorIA: r.ProveedorIA, ModeloIA: r.ModeloIA, TamanoBytes: r.TamanoBytes, GeneradoEn: r.GeneradoEn,
	}
	if r.Estado == dominio.ReporteCompletado {
		j.URLDescarga = "/api/v1/reportes/" + r.ID + "/pdf"
	}
	return j
}

// --- endpoints ---------------------------------------------------------------

type peticionLote struct {
	// Cualquier fecha (AAAA-MM-DD) de la semana que se quiere reportar.
	Semana string `json:"semana"`
}

// SolicitarLote inicia la generación por lotes de los reportes de una semana.
// Responde 202: los reportes se generan en segundo plano en el worker.
func (h *HandlerReportes) SolicitarLote(c *gin.Context) {
	var p peticionLote
	if err := leerJSON(c, &p); err != nil {
		responderError(c, h.log, err)
		return
	}
	if strings.TrimSpace(p.Semana) == "" {
		responderError(c, h.log, apperr.Validacion("semana es obligatoria (AAAA-MM-DD)"))
		return
	}
	lote, err := h.servicio.SolicitarLote(c.Request.Context(), ActorDe(c), p.Semana)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.Header("Location", "/api/v1/reportes/lotes/"+lote.ID)
	c.JSON(http.StatusAccepted, aLoteJSON(lote))
}

// ListarLotes devuelve los lotes visibles para el usuario.
func (h *HandlerReportes) ListarLotes(c *gin.Context) {
	semana, err := consultaSemana(c, "semana")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	profesorID, err := consultaUUID(c, "profesor_id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	lotes, err := h.servicio.ListarLotes(c.Request.Context(), ActorDe(c), dominio.FiltroLotes{
		ProfesorID: profesorID, SemanaInicio: semana, Paginacion: consultaPaginacion(c),
	})
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, convertir(lotes, aLoteJSON))
}

// ObtenerLote devuelve el avance de un lote y el estado de cada reporte.
func (h *HandlerReportes) ObtenerLote(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	lote, reportes, err := h.servicio.ObtenerLote(c.Request.Context(), ActorDe(c), id)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	j := aLoteJSON(lote)
	j.Reportes = convertir(reportes, aReporteJSON)
	c.JSON(http.StatusOK, j)
}

// ListarReportes devuelve los reportes visibles, con filtros opcionales
// semana, usuario_id, lote_id y profesor_id (este último solo para administradores).
func (h *HandlerReportes) ListarReportes(c *gin.Context) {
	semana, err := consultaSemana(c, "semana")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	f := dominio.FiltroReportes{SemanaInicio: semana, Paginacion: consultaPaginacion(c)}
	for nombre, destino := range map[string]*string{"usuario_id": &f.UsuarioID, "lote_id": &f.LoteID, "profesor_id": &f.ProfesorID} {
		if *destino, err = consultaUUID(c, nombre); err != nil {
			responderError(c, h.log, err)
			return
		}
	}
	reportes, err := h.servicio.ListarReportes(c.Request.Context(), ActorDe(c), f)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, convertir(reportes, aReporteJSON))
}

// ObtenerReporte devuelve el detalle de un reporte, incluido el resumen.
func (h *HandlerReportes) ObtenerReporte(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	rep, err := h.servicio.ObtenerReporte(c.Request.Context(), ActorDe(c), id)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, aReporteJSON(rep))
}

// DescargarPDF entrega el archivo PDF del reporte (RF-14.6).
func (h *HandlerReportes) DescargarPDF(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	archivo, rep, err := h.servicio.AbrirPDF(c.Request.Context(), ActorDe(c), id)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	defer func() { _ = archivo.Close() }()

	nombre := fmt.Sprintf("reporte-%s-%s.pdf", nombreArchivo(rep.Persona), fecha(rep.SemanaInicio))
	servirArchivo(c, archivo, rep.TamanoBytes, "application/pdf", nombre)
}
