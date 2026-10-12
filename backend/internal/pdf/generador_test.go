package pdf

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"seguimiento/internal/dominio"
)

func contenidoPrueba(conTareas bool) Contenido {
	d := dominio.DatosSemanaPersona{
		Persona:      "José Muñoz",
		Correo:       "jose@uniandes.edu.co",
		Profesor:     "Profesora Peña",
		SemanaInicio: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		Vinculaciones: []dominio.VinculacionSemana{{
			EspacioNombre:    "Un curso con un nombre muy largo que no cabe en la columna de la tabla de dedicación",
			EspacioTipo:      dominio.EspacioCurso,
			Rol:              dominio.RolMonitor,
			HorasContratadas: 6,
		}},
	}
	if conTareas {
		d.Vinculaciones[0].Tareas = []dominio.TareaSemana{
			{Titulo: "Revisión de exámenes", Descripcion: strings.Repeat("Texto largo. ", 80), Estado: dominio.TareaFinalizada, Horas: 4},
			{Titulo: "Atención a estudiantes", Estado: dominio.TareaAbierta, Horas: 1, Observaciones: "Pendiente", Tardia: true},
		}
	}
	return Contenido{Datos: d, Resumen: "Durante la semana se revisaron exámenes.", GeneradoEn: time.Now(), ProveedorIA: "simulado", ModeloIA: "ninguno"}
}

func TestGenerarProducePDFValido(t *testing.T) {
	for _, conTareas := range []bool{true, false} {
		contenido, err := NuevoGenerador(time.UTC).Generar(contenidoPrueba(conTareas))
		if err != nil {
			t.Fatalf("conTareas=%v: %v", conTareas, err)
		}
		if !bytes.HasPrefix(contenido, []byte("%PDF-")) {
			t.Fatalf("conTareas=%v: el resultado no es un PDF", conTareas)
		}
		if !bytes.Contains(contenido, []byte("%%EOF")) {
			t.Fatalf("conTareas=%v: el PDF está incompleto", conTareas)
		}
	}
}
