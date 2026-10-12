// Package pdf genera el documento PDF del reporte semanal (RF-14.4).
package pdf

import (
	"bytes"
	"fmt"
	"time"

	"codeberg.org/go-pdf/fpdf"

	"seguimiento/internal/dominio"
)

// Contenido es la información que se imprime en el reporte.
type Contenido struct {
	Datos       dominio.DatosSemanaPersona
	Resumen     string
	GeneradoEn  time.Time
	ProveedorIA string
	ModeloIA    string
}

// Generador produce reportes PDF en tamaño carta con fuentes estándar.
type Generador struct {
	zona *time.Location
}

// NuevoGenerador crea un generador que muestra las horas en la zona dada.
func NuevoGenerador(zona *time.Location) *Generador {
	if zona == nil {
		zona = time.UTC
	}
	return &Generador{zona: zona}
}

const (
	formatoFechaCorta = "02/01/2006"
	anchoUtil         = 176.0 // ancho de página carta (216 mm) menos márgenes
)

// Generar construye el PDF y lo devuelve en memoria.
func (g *Generador) Generar(c Contenido) ([]byte, error) {
	d := c.Datos
	doc := fpdf.New("P", "mm", "Letter", "")
	// Las fuentes estándar usan cp1252; el traductor convierte tildes y eñes.
	tr := doc.UnicodeTranslatorFromDescriptor("")

	doc.SetTitle(fmt.Sprintf("Reporte semanal %s - %s", d.Persona, d.SemanaInicio.Format(dominio.FormatoFecha)), true)
	doc.SetAuthor("Plataforma de seguimiento de monitores y asistentes", true)
	doc.SetCreator("seguimiento-worker", true)
	doc.SetMargins(20, 18, 20)
	doc.SetAutoPageBreak(true, 18)
	doc.AliasNbPages("")
	doc.SetFooterFunc(func() {
		doc.SetY(-14)
		doc.SetFont("Helvetica", "I", 8)
		doc.SetTextColor(120, 120, 120)
		doc.CellFormat(0, 6, tr(fmt.Sprintf("Página %d de {nb}", doc.PageNo())), "", 0, "C", false, 0, "")
	})
	doc.AddPage()

	// Encabezado
	doc.SetTextColor(30, 30, 30)
	doc.SetFont("Helvetica", "B", 17)
	doc.CellFormat(0, 9, tr("Reporte semanal de actividades"), "", 1, "L", false, 0, "")
	doc.SetFont("Helvetica", "", 11)
	doc.SetTextColor(80, 80, 80)
	doc.CellFormat(0, 6, tr(fmt.Sprintf("Semana del %s al %s",
		d.SemanaInicio.Format(formatoFechaCorta), d.SemanaFin().Format(formatoFechaCorta))), "", 1, "L", false, 0, "")
	doc.Ln(3)
	doc.SetDrawColor(200, 200, 200)
	doc.Line(20, doc.GetY(), 20+anchoUtil, doc.GetY())
	doc.Ln(4)

	// Datos generales
	doc.SetTextColor(30, 30, 30)
	filaDato(doc, tr, "Persona", d.Persona)
	filaDato(doc, tr, "Correo", d.Correo)
	filaDato(doc, tr, "Profesor responsable", d.Profesor)
	filaDato(doc, tr, "Tareas registradas", fmt.Sprintf("%d", d.TotalTareas()))
	filaDato(doc, tr, "Horas reportadas", fmt.Sprintf("%d", d.TotalHoras()))
	filaDato(doc, tr, "Generado", c.GeneradoEn.In(g.zona).Format("02/01/2006 15:04 MST"))
	doc.Ln(4)

	// Resumen generado con IA
	titulo(doc, tr, "Resumen de la semana")
	doc.SetFont("Helvetica", "", 10.5)
	doc.MultiCell(0, 5.5, tr(c.Resumen), "", "J", false)
	doc.Ln(1)
	doc.SetFont("Helvetica", "I", 8.5)
	doc.SetTextColor(110, 110, 110)
	nota := "Resumen generado automáticamente a partir de las tareas registradas"
	if c.ProveedorIA != "" {
		nota += fmt.Sprintf(" (IA: %s / %s)", c.ProveedorIA, c.ModeloIA)
	}
	nota += ". Sintetiza el trabajo reportado y no constituye una evaluación de desempeño."
	doc.MultiCell(0, 4.5, tr(nota), "", "L", false)
	doc.SetTextColor(30, 30, 30)
	doc.Ln(4)

	// Dedicación por vinculación
	titulo(doc, tr, "Dedicación por curso o proyecto")
	tablaDedicacion(doc, tr, d)
	doc.Ln(5)

	// Detalle de tareas
	titulo(doc, tr, "Detalle de tareas")
	if d.TotalTareas() == 0 {
		doc.SetFont("Helvetica", "", 10.5)
		doc.MultiCell(0, 5.5, tr("No se registraron tareas durante esta semana."), "", "L", false)
	}
	for _, v := range d.Vinculaciones {
		if len(v.Tareas) == 0 {
			continue
		}
		doc.SetFont("Helvetica", "B", 11)
		doc.MultiCell(0, 6, tr(fmt.Sprintf("%s: %s (%s)", tipoEspacio(v.EspacioTipo), v.EspacioNombre, rolTexto(v.Rol))), "", "L", false)
		doc.Ln(1)
		for _, t := range v.Tareas {
			detalleTarea(doc, tr, t)
		}
		doc.Ln(2)
	}

	if err := doc.Error(); err != nil {
		return nil, fmt.Errorf("generando PDF: %w", err)
	}
	var buf bytes.Buffer
	if err := doc.Output(&buf); err != nil {
		return nil, fmt.Errorf("escribiendo PDF: %w", err)
	}
	return buf.Bytes(), nil
}

type traductor func(string) string

func titulo(doc *fpdf.Fpdf, tr traductor, texto string) {
	doc.SetFont("Helvetica", "B", 12.5)
	doc.SetTextColor(20, 60, 110)
	doc.CellFormat(0, 7, tr(texto), "", 1, "L", false, 0, "")
	doc.SetTextColor(30, 30, 30)
	doc.Ln(1)
}

func filaDato(doc *fpdf.Fpdf, tr traductor, etiqueta, valor string) {
	doc.SetFont("Helvetica", "B", 10)
	doc.CellFormat(45, 5.5, tr(etiqueta+":"), "", 0, "L", false, 0, "")
	doc.SetFont("Helvetica", "", 10)
	doc.MultiCell(0, 5.5, tr(valor), "", "L", false)
}

func tablaDedicacion(doc *fpdf.Fpdf, tr traductor, d dominio.DatosSemanaPersona) {
	anchos := []float64{86, 36, 27, 27}
	encabezados := []string{"Curso o proyecto", "Rol", "Contratadas", "Reportadas"}

	doc.SetFont("Helvetica", "B", 9.5)
	doc.SetFillColor(230, 236, 245)
	for i, e := range encabezados {
		alin := "L"
		if i >= 2 {
			alin = "C"
		}
		doc.CellFormat(anchos[i], 7, tr(e), "1", 0, alin, true, 0, "")
	}
	doc.Ln(-1)

	doc.SetFont("Helvetica", "", 9.5)
	contratadas, reportadas := 0, 0
	for _, v := range d.Vinculaciones {
		nombre := fmt.Sprintf("%s (%s)", v.EspacioNombre, tipoEspacio(v.EspacioTipo))
		doc.CellFormat(anchos[0], 6.5, recortar(doc, tr(nombre), anchos[0]-2), "1", 0, "L", false, 0, "")
		doc.CellFormat(anchos[1], 6.5, tr(rolTexto(v.Rol)), "1", 0, "L", false, 0, "")
		doc.CellFormat(anchos[2], 6.5, fmt.Sprintf("%d h", v.HorasContratadas), "1", 0, "C", false, 0, "")
		doc.CellFormat(anchos[3], 6.5, fmt.Sprintf("%d h", v.HorasReportadas()), "1", 0, "C", false, 0, "")
		doc.Ln(-1)
		contratadas += v.HorasContratadas
		reportadas += v.HorasReportadas()
	}

	doc.SetFont("Helvetica", "B", 9.5)
	doc.CellFormat(anchos[0]+anchos[1], 6.5, "Total", "1", 0, "R", true, 0, "")
	doc.CellFormat(anchos[2], 6.5, fmt.Sprintf("%d h", contratadas), "1", 0, "C", true, 0, "")
	doc.CellFormat(anchos[3], 6.5, fmt.Sprintf("%d h", reportadas), "1", 0, "C", true, 0, "")
	doc.Ln(-1)
}

func detalleTarea(doc *fpdf.Fpdf, tr traductor, t dominio.TareaSemana) {
	doc.SetFont("Helvetica", "B", 10)
	doc.MultiCell(0, 5.5, tr("- "+t.Titulo), "", "L", false)

	doc.SetFont("Helvetica", "", 9.5)
	doc.SetTextColor(80, 80, 80)
	linea := fmt.Sprintf("Estado: %s   |   Horas: %d", t.Estado.Texto(), t.Horas)
	if t.Tardia {
		linea += "   |   Reporte tardío"
	}
	doc.SetX(24)
	doc.MultiCell(0, 5, tr(linea), "", "L", false)
	doc.SetTextColor(30, 30, 30)

	if t.Descripcion != "" {
		doc.SetX(24)
		doc.MultiCell(0, 5, tr("Descripción: "+t.Descripcion), "", "L", false)
	}
	if t.Observaciones != "" {
		doc.SetX(24)
		doc.MultiCell(0, 5, tr("Observaciones: "+t.Observaciones), "", "L", false)
	}
	doc.Ln(2)
}

// recortar acorta el texto con "..." para que quepa en el ancho dado.
func recortar(doc *fpdf.Fpdf, texto string, ancho float64) string {
	if doc.GetStringWidth(texto) <= ancho {
		return texto
	}
	r := []rune(texto)
	for len(r) > 0 && doc.GetStringWidth(string(r)+"...") > ancho {
		r = r[:len(r)-1]
	}
	return string(r) + "..."
}

func tipoEspacio(t dominio.TipoEspacio) string {
	if t == dominio.EspacioProyecto {
		return "Proyecto"
	}
	return "Curso"
}

func rolTexto(r dominio.Rol) string {
	switch r {
	case dominio.RolAsistenteGraduado:
		return "Asistente graduado"
	case dominio.RolMonitor:
		return "Monitor"
	default:
		return string(r)
	}
}
