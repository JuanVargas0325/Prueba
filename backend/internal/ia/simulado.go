package ia

import (
	"context"
	"fmt"
	"strings"

	"seguimiento/internal/dominio"
)

// Simulado genera un resumen determinístico sin llamar a ningún modelo.
// Permite probar el flujo completo (cola, PDF, almacenamiento, descarga)
// mientras el equipo define qué proveedor de IA usará.
type Simulado struct{}

// NuevoSimulado crea el resumidor simulado.
func NuevoSimulado() *Simulado { return &Simulado{} }

func (s *Simulado) Proveedor() string { return ProveedorSimulado }
func (s *Simulado) Modelo() string    { return "ninguno" }

// Resumir construye un resumen a partir de los totales y títulos de las tareas.
func (s *Simulado) Resumir(_ context.Context, d dominio.DatosSemanaPersona) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "Resumen generado en modo simulado, sin modelo de IA. Durante la semana del %s al %s se registraron %d tareas con un total de %d horas reportadas.",
		d.SemanaInicio.Format(dominio.FormatoFecha), d.SemanaFin().Format(dominio.FormatoFecha), d.TotalTareas(), d.TotalHoras())

	for _, v := range d.Vinculaciones {
		if len(v.Tareas) == 0 {
			continue
		}
		titulos := make([]string, 0, len(v.Tareas))
		for _, t := range v.Tareas {
			titulos = append(titulos, fmt.Sprintf("%s (%s)", unaLinea(t.Titulo), t.Estado.Texto()))
		}
		fmt.Fprintf(&b, " En %s \"%s\" se reportaron %d horas en: %s.",
			strings.ToLower(tipoEspacio(v.EspacioTipo)), unaLinea(v.EspacioNombre), v.HorasReportadas(), strings.Join(titulos, "; "))
	}
	return b.String(), nil
}
