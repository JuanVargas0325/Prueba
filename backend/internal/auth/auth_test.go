package auth

import (
	"testing"
	"time"

	"seguimiento/internal/dominio"
)

const secretoPrueba = "secreto-de-prueba-con-mas-de-32-caracteres"

func TestHashYVerificacionDePassword(t *testing.T) {
	hash, err := HashPassword("Clave123*")
	if err != nil {
		t.Fatal(err)
	}
	if !VerificarPassword(hash, "Clave123*") {
		t.Error("la contraseña correcta debería verificarse")
	}
	if VerificarPassword(hash, "otra") {
		t.Error("una contraseña incorrecta no debería verificarse")
	}
}

func TestTokenIdaYVuelta(t *testing.T) {
	g := NuevoGestorTokens(secretoPrueba, time.Hour)
	u := dominio.Usuario{ID: "u-1", Roles: []dominio.Rol{dominio.RolProfesor, dominio.RolMonitor}}

	token, expira, err := g.Generar(u)
	if err != nil {
		t.Fatal(err)
	}
	if !expira.After(time.Now()) {
		t.Error("la expiración debería estar en el futuro")
	}

	actor, err := g.Validar(token)
	if err != nil {
		t.Fatal(err)
	}
	if actor.UsuarioID != "u-1" || !actor.TieneRol(dominio.RolProfesor) || !actor.TieneRol(dominio.RolMonitor) {
		t.Errorf("actor inesperado: %+v", actor)
	}
	if actor.EsAdministrador() {
		t.Error("el actor no debería ser administrador")
	}
}

func TestTokenExpiradoEsRechazado(t *testing.T) {
	g := NuevoGestorTokens(secretoPrueba, time.Minute)
	g.ahora = func() time.Time { return time.Now().Add(-time.Hour) }
	token, _, err := g.Generar(dominio.Usuario{ID: "u-1"})
	if err != nil {
		t.Fatal(err)
	}
	g.ahora = time.Now
	if _, err := g.Validar(token); err == nil {
		t.Fatal("un token expirado debería rechazarse")
	}
}

func TestTokenConOtraFirmaEsRechazado(t *testing.T) {
	token, _, err := NuevoGestorTokens(secretoPrueba, time.Hour).Generar(dominio.Usuario{ID: "u-1"})
	if err != nil {
		t.Fatal(err)
	}
	otro := NuevoGestorTokens("otro-secreto-distinto-con-mas-de-32-caracteres", time.Hour)
	if _, err := otro.Validar(token); err == nil {
		t.Fatal("un token firmado con otro secreto debería rechazarse")
	}
}

func TestTieneAlgunRol(t *testing.T) {
	a := Actor{Roles: []dominio.Rol{dominio.RolMonitor}}
	if !a.TieneAlgunRol(dominio.RolAsistenteGraduado, dominio.RolMonitor) {
		t.Error("debería tener alguno de los roles")
	}
	if a.TieneAlgunRol(dominio.RolProfesor) {
		t.Error("no debería tener el rol profesor")
	}
}
