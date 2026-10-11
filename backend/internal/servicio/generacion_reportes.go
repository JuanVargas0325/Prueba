package servicio

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"

	"seguimiento/internal/apperr"
	"seguimiento/internal/dominio"
	"seguimiento/internal/ia"
	"seguimiento/internal/pdf"
)

// GeneradorPDF convierte el contenido del reporte en un documento PDF.
type GeneradorPDF interface {
	Generar(c pdf.Contenido) ([]byte, error)
}

// ResumenSinTareas es el texto del reporte cuando la persona no registró tareas.
// En ese caso no se consulta al modelo, para que no invente actividades.
const ResumenSinTareas = "No se registraron tareas durante esta semana, por lo que no hay actividades para resumir."

// GeneradorReportes ejecuta, en el worker, la generación asíncrona de reportes:
//
//  1. ProcesarLote: identifica a las personas vinculadas al profesor, crea un
//     reporte pendiente por cada una y encola su generación.
//  2. GenerarReporte: reúne las tareas de la semana, pide el resumen al modelo
//     de IA, construye el PDF, lo guarda en el almacenamiento y registra el resultado.
type GeneradorReportes struct {
	repo           RepositorioReportes
	encolador      Encolador
	almacenamiento Almacenamiento
	resumidor      ia.Resumidor
	pdf            GeneradorPDF
	reloj          Reloj
	log            *slog.Logger
}

// NuevoGeneradorReportes crea el generador de reportes.
func NuevoGeneradorReportes(repo RepositorioReportes, encolador Encolador, almacenamiento Almacenamiento,
	resumidor ia.Resumidor, generadorPDF GeneradorPDF, reloj Reloj, log *slog.Logger) *GeneradorReportes {
	return &GeneradorReportes{
		repo: repo, encolador: encolador, almacenamiento: almacenamiento,
		resumidor: resumidor, pdf: generadorPDF, reloj: reloj, log: log,
	}
}

// ProcesarLote reparte el lote en un trabajo por persona. Es idempotente:
// si la tarea se reintenta, no duplica reportes ni trabajos en la cola.
func (g *GeneradorReportes) ProcesarLote(ctx context.Context, loteID string) error {
	lote, err := g.repo.ObtenerLote(ctx, loteID)
	if err != nil {
		return err
	}
	if !lote.Estado.EstaActivo() {
		g.log.Info("lote ya procesado, se omite", "lote_id", loteID, "estado", lote.Estado)
		return nil
	}

	personas, err := g.repo.ListarPersonasParaReporte(ctx, lote.ProfesorID, lote.SemanaInicio, dominio.FinSemana(lote.SemanaInicio))
	if err != nil {
		return err
	}
	reportes, err := g.repo.CrearReportesPendientes(ctx, lote.ID, lote.ProfesorID, lote.SemanaInicio, personas)
	if err != nil {
		return err
	}
	if err := g.repo.IniciarLote(ctx, lote.ID, len(reportes)); err != nil {
		return err
	}
	g.log.Info("lote iniciado", "lote_id", loteID, "reportes", len(reportes))

	for _, reporteID := range reportes {
		if err := g.encolador.EncolarReporte(ctx, reporteID); err != nil {
			g.log.Error("no fue posible encolar el reporte", "reporte_id", reporteID, "error", err)
			if errFallo := g.repo.FallarReporte(ctx, reporteID, "no fue posible encolar el reporte: "+err.Error()); errFallo != nil {
				return errFallo
			}
		}
	}
	return nil
}

// GenerarReporte produce el PDF de una persona. Si falla con un error
// transitorio, la cola lo reintenta; si el reporte ya terminó, no hace nada.
func (g *GeneradorReportes) GenerarReporte(ctx context.Context, reporteID string) error {
	rep, err := g.repo.ObtenerReporte(ctx, reporteID)
	if err != nil {
		return err
	}
	if rep.Estado.EsFinal() {
		g.log.Info("reporte ya finalizado, se omite", "reporte_id", reporteID, "estado", rep.Estado)
		return nil
	}
	if err := g.repo.MarcarReporteEnProceso(ctx, reporteID); err != nil {
		return err
	}

	datos, err := g.repo.ObtenerDatosSemana(ctx, rep.ProfesorID, rep.UsuarioID, rep.SemanaInicio)
	if err != nil {
		return err
	}

	resumen := ResumenSinTareas
	proveedor, modelo := "", ""
	if datos.TotalTareas() > 0 {
		inicio := g.reloj.Ahora()
		resumen, err = g.resumidor.Resumir(ctx, datos)
		if err != nil {
			return fmt.Errorf("generando resumen con IA: %w", err)
		}
		proveedor, modelo = g.resumidor.Proveedor(), g.resumidor.Modelo()
		g.log.Info("resumen generado", "reporte_id", reporteID, "proveedor", proveedor,
			"modelo", modelo, "duracion_ms", g.reloj.Ahora().Sub(inicio).Milliseconds())
	}

	contenido, err := g.pdf.Generar(pdf.Contenido{
		Datos:       datos,
		Resumen:     resumen,
		GeneradoEn:  g.reloj.Ahora(),
		ProveedorIA: proveedor,
		ModeloIA:    modelo,
	})
	if err != nil {
		return err
	}

	ruta := RutaPDFReporte(rep)
	tamano, err := g.almacenamiento.Guardar(ctx, ruta, bytes.NewReader(contenido))
	if err != nil {
		return fmt.Errorf("guardando PDF: %w", err)
	}

	if err := g.repo.CompletarReporte(ctx, reporteID, dominio.ResultadoReporte{
		Resumen: resumen, RutaPDF: ruta, TamanoBytes: tamano, ProveedorIA: proveedor, ModeloIA: modelo,
	}); err != nil {
		return err
	}
	g.log.Info("reporte generado", "reporte_id", reporteID, "ruta", ruta, "bytes", tamano)
	return nil
}

// RegistrarFalloReporte marca el reporte como fallido cuando se agotaron los
// reintentos o el error no es recuperable.
func (g *GeneradorReportes) RegistrarFalloReporte(ctx context.Context, reporteID string, causa error) error {
	return g.repo.FallarReporte(ctx, reporteID, MensajeFallo(causa))
}

// RegistrarFalloLote marca el lote como fallido.
func (g *GeneradorReportes) RegistrarFalloLote(ctx context.Context, loteID string, causa error) error {
	return g.repo.MarcarLoteFallido(ctx, loteID, MensajeFallo(causa))
}

// RutaPDFReporte construye la ruta del PDF dentro del almacenamiento.
func RutaPDFReporte(rep dominio.ReporteSemanal) string {
	return fmt.Sprintf("reportes/%s/%s.pdf", rep.SemanaInicio.Format(dominio.FormatoFecha), rep.ID)
}

// EsErrorPermanente indica si no vale la pena reintentar la operación: datos
// inexistentes o inválidos, o un error del proveedor de IA que no es transitorio
// (credenciales incorrectas, modelo inexistente, petición rechazada).
func EsErrorPermanente(err error) bool {
	switch apperr.TipoDe(err) {
	case apperr.TipoNoEncontrado, apperr.TipoValidacion, apperr.TipoProhibido:
		return true
	}
	return ia.EsErrorDeIA(err) && !ia.EsReintentable(err)
}

// MensajeFallo produce el texto que se guarda como causa del fallo.
func MensajeFallo(err error) string {
	if err == nil {
		return "error desconocido"
	}
	return err.Error()
}
