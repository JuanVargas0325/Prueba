package dominio

import (
	"testing"
	"time"
)

func fecha(t *testing.T, texto string) time.Time {
	t.Helper()
	f, err := ParsearFecha(texto)
	if err != nil {
		t.Fatalf("fecha inválida %q: %v", texto, err)
	}
	return f
}

func TestInicioSemanaDevuelveLunes(t *testing.T) {
	casos := map[string]string{
		"2026-10-05": "2026-10-05", // lunes
		"2026-10-08": "2026-10-05", // jueves
		"2026-10-11": "2026-10-05", // domingo
		"2026-10-12": "2026-10-12", // lunes siguiente
		"2026-01-01": "2025-12-29", // cruce de año
	}
	for entrada, esperado := range casos {
		got := InicioSemana(fecha(t, entrada)).Format(FormatoFecha)
		if got != esperado {
			t.Errorf("InicioSemana(%s) = %s, se esperaba %s", entrada, got, esperado)
		}
	}
}

func TestFinSemanaDevuelveDomingo(t *testing.T) {
	got := FinSemana(fecha(t, "2026-10-07")).Format(FormatoFecha)
	if got != "2026-10-11" {
		t.Fatalf("FinSemana = %s, se esperaba 2026-10-11", got)
	}
}

func TestFechaUsaLaZonaHoraria(t *testing.T) {
	bogota, err := time.LoadLocation("America/Bogota")
	if err != nil {
		t.Skip("zona horaria no disponible:", err)
	}
	// Lunes 2026-10-12 a las 02:00 UTC sigue siendo domingo 11 en Bogotá (UTC-5).
	instante := time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)
	if got := Fecha(instante, bogota).Format(FormatoFecha); got != "2026-10-11" {
		t.Fatalf("Fecha = %s, se esperaba 2026-10-11", got)
	}
	if got := SemanaActual(instante, bogota).Format(FormatoFecha); got != "2026-10-05" {
		t.Fatalf("SemanaActual = %s, se esperaba 2026-10-05", got)
	}
}

func TestSemanaCerrada(t *testing.T) {
	ahora := time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC) // miércoles
	if SemanaCerrada(fecha(t, "2026-10-12"), ahora, time.UTC) {
		t.Error("la semana en curso no debería estar cerrada")
	}
	if !SemanaCerrada(fecha(t, "2026-10-05"), ahora, time.UTC) {
		t.Error("la semana anterior debería estar cerrada")
	}
}

func TestParsearFechaRechazaFormatoInvalido(t *testing.T) {
	if _, err := ParsearFecha("14/10/2026"); err == nil {
		t.Fatal("se esperaba error con un formato inválido")
	}
}

func TestTotalesDatosSemana(t *testing.T) {
	d := DatosSemanaPersona{
		SemanaInicio: fecha(t, "2026-10-05"),
		Vinculaciones: []VinculacionSemana{
			{Tareas: []TareaSemana{{Horas: 2}, {Horas: 3}}},
			{Tareas: []TareaSemana{{Horas: 4}}},
		},
	}
	if d.TotalTareas() != 3 || d.TotalHoras() != 9 {
		t.Fatalf("totales incorrectos: tareas=%d horas=%d", d.TotalTareas(), d.TotalHoras())
	}
	if d.SemanaFin().Format(FormatoFecha) != "2026-10-11" {
		t.Fatalf("SemanaFin incorrecta: %s", d.SemanaFin().Format(FormatoFecha))
	}
}
