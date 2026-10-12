package config

import (
	"strings"
	"testing"
)

func TestCargarUsaValoresPorDefecto(t *testing.T) {
	t.Setenv("APP_ENTORNO", "")
	t.Setenv("APP_PUERTO", "")
	t.Setenv("APP_ZONA_HORARIA", "")

	c, err := Cargar()
	if err != nil {
		t.Fatal(err)
	}
	if c.Entorno != "desarrollo" || !c.EsDesarrollo() || c.PuertoHTTP != "8080" {
		t.Errorf("valores por defecto inesperados: %+v", c)
	}
	if c.ZonaHoraria.String() != "America/Bogota" {
		t.Errorf("zona horaria por defecto = %s", c.ZonaHoraria)
	}
}

func TestCargarRechazaZonaHorariaInvalida(t *testing.T) {
	t.Setenv("APP_ZONA_HORARIA", "Marte/Monte_Olimpo")

	_, err := Cargar()
	if err == nil || !strings.Contains(err.Error(), "APP_ZONA_HORARIA") {
		t.Fatalf("se esperaba un error que mencionara APP_ZONA_HORARIA: %v", err)
	}
}
