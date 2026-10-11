package config

import (
	"strings"
	"testing"
	"time"
)

const secretoValido = "un-secreto-de-prueba-con-mas-de-32-caracteres"

func TestCargarConstruyeLaURLDesdeVariablesDePostgres(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("POSTGRES_USER", "app")
	t.Setenv("POSTGRES_PASSWORD", "cl@ve:con/símbolos")
	t.Setenv("POSTGRES_DB", "seguimiento")
	t.Setenv("POSTGRES_HOST", "postgres")
	t.Setenv("JWT_SECRETO", secretoValido)

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
	if c.LLM.Proveedor != "simulado" || c.Worker.MaxReintentos != 3 || c.JWT.Expiracion != 8*time.Hour {
		t.Errorf("valores por defecto inesperados: %+v", c)
	}
	if c.ZonaHoraria.String() != "America/Bogota" {
		t.Errorf("zona horaria por defecto = %s", c.ZonaHoraria)
	}
}

func TestCargarExigeSecretoYBaseDeDatos(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("POSTGRES_USER", "")
	t.Setenv("POSTGRES_DB", "")
	t.Setenv("JWT_SECRETO", "corto")

	_, err := Cargar()
	if err == nil {
		t.Fatal("se esperaba error de configuración")
	}
	for _, esperado := range []string{"JWT_SECRETO", "DATABASE_URL"} {
		if !strings.Contains(err.Error(), esperado) {
			t.Errorf("el error debería mencionar %s: %v", esperado, err)
		}
	}
}

func TestCargarValidaFormatos(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x@localhost/db")
	t.Setenv("JWT_SECRETO", secretoValido)
	t.Setenv("WORKER_CONCURRENCIA", "dos")
	t.Setenv("LLM_TIMEOUT", "mucho")
	t.Setenv("LLM_TEMPERATURA", "0.2")

	c, err := Cargar()
	if err == nil || !strings.Contains(err.Error(), "WORKER_CONCURRENCIA") || !strings.Contains(err.Error(), "LLM_TIMEOUT") {
		t.Fatalf("se esperaban errores de formato: %v", err)
	}
	if c.LLM.Temperatura == nil || *c.LLM.Temperatura != 0.2 {
		t.Error("la temperatura debería leerse como decimal")
	}
}
