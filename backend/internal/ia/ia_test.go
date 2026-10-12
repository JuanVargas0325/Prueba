package ia

import (
	"context"
	"strings"
	"testing"
	"time"

	"seguimiento/internal/dominio"
)

func datosPrueba() dominio.DatosSemanaPersona {
	return dominio.DatosSemanaPersona{
		Persona:      "Ana Pérez",
		Correo:       "ana@uniandes.edu.co",
		SemanaInicio: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		Vinculaciones: []dominio.VinculacionSemana{
			{
				EspacioNombre:    "Desarrollo en la nube",
				EspacioTipo:      dominio.EspacioCurso,
				Rol:              dominio.RolMonitor,
				HorasContratadas: 6,
				Tareas: []dominio.TareaSemana{
					{Titulo: "Calificar taller 2", Descripcion: "Revisión de\n30 entregas", Estado: dominio.TareaFinalizada, Horas: 4},
					{Titulo: "Preparar laboratorio", Estado: dominio.TareaEnDesarrollo, Horas: 2, Observaciones: "Falta la guía", Tardia: true},
				},
			},
			{EspacioNombre: "Proyecto sin tareas", EspacioTipo: dominio.EspacioProyecto, Rol: dominio.RolAsistenteGraduado},
		},
	}
}

func TestMensajeUsuarioIncluyeSoloInsumosPermitidos(t *testing.T) {
	m := ConstruirMensajeUsuario(datosPrueba())

	for _, esperado := range []string{
		"2026-10-05", "2026-10-11", "Total de tareas: 2", "Total de horas reportadas: 6",
		"Calificar taller 2", "Revisión de 30 entregas", "en desarrollo", "Reporte tardío: sí",
		"Falta la guía", "<datos_semana>", "</datos_semana>",
	} {
		if !strings.Contains(m, esperado) {
			t.Errorf("el mensaje debería contener %q:\n%s", esperado, m)
		}
	}
	// Datos personales y horas contratadas no se envían al modelo.
	for _, prohibido := range []string{"Ana Pérez", "ana@uniandes.edu.co", "Proyecto sin tareas"} {
		if strings.Contains(m, prohibido) {
			t.Errorf("el mensaje no debería contener %q", prohibido)
		}
	}
}

func TestSimuladoEsDeterministico(t *testing.T) {
	s := NuevoSimulado()
	r1, err := s.Resumir(context.Background(), datosPrueba())
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := s.Resumir(context.Background(), datosPrueba())
	if r1 != r2 || !strings.Contains(r1, "2 tareas") || !strings.Contains(r1, "6 horas") {
		t.Fatalf("resumen simulado inesperado: %q", r1)
	}
}

func TestNuevoValidaConfiguracion(t *testing.T) {
	if r, err := Nuevo(Config{}); err != nil || r.Proveedor() != ProveedorSimulado {
		t.Errorf("sin proveedor debería usarse el simulado: %v", err)
	}
	if _, err := Nuevo(Config{Proveedor: "otro"}); err == nil {
		t.Error("un proveedor desconocido debería fallar")
	}
}
