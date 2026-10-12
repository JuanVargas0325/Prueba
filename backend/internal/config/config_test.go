package config

import (
	"strings"
	"testing"
)

func TestCargarConstruyeLaURLDesdeVariablesDePostgres(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("POSTGRES_USER", "app")
	t.Setenv("POSTGRES_PASSWORD", "cl@ve:con/símbolos")
	t.Setenv("POSTGRES_DB", "seguimiento")
	t.Setenv("POSTGRES_HOST", "postgres")

	c, err := Cargar()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.BaseDatos, "postgres://app:") || !strings.Contains(c.BaseDatos, "@postgres:5432/seguimiento?sslmode=disable") {
		t.Errorf("URL inesperada: %s", c.BaseDatos)
	}
	if strings.Contains(c.BaseDatos, "cl@ve:con/") {
		t.Error("la contraseña debería ir codificada en la URL")
	}
	if c.ZonaHoraria.String() != "America/Bogota" {
		t.Errorf("zona horaria por defecto = %s", c.ZonaHoraria)
	}
}

func TestCargarExigeBaseDeDatos(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("POSTGRES_USER", "")
	t.Setenv("POSTGRES_DB", "")

	_, err := Cargar()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("se esperaba un error que mencionara DATABASE_URL: %v", err)
	}
}

func TestCargarRechazaZonaHorariaInvalida(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x@localhost/db")
	t.Setenv("APP_ZONA_HORARIA", "Marte/Monte_Olimpo")

	_, err := Cargar()
	if err == nil || !strings.Contains(err.Error(), "APP_ZONA_HORARIA") {
		t.Fatalf("se esperaba un error que mencionara APP_ZONA_HORARIA: %v", err)
	}
}
