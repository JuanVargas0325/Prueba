package servicio

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

// Dobles compartidos por las pruebas del paquete: un logger que descarta los
// mensajes, los actores con los que se prueban los permisos y un instante fijo
// para el reloj de prueba.
var (
	logPrueba = slog.New(slog.NewTextHandler(io.Discard, nil))
	profesor  = auth.Actor{UsuarioID: "prof-1", Roles: []dominio.Rol{dominio.RolProfesor}}
	otroProf  = auth.Actor{UsuarioID: "prof-2", Roles: []dominio.Rol{dominio.RolProfesor}}
	admin     = auth.Actor{UsuarioID: "admin-1", Roles: []dominio.Rol{dominio.RolAdministrador}}
	monitor   = auth.Actor{UsuarioID: "mon-1", Roles: []dominio.Rol{dominio.RolMonitor}}
	// miércoles 14 de octubre de 2026
	ahoraPrueba = time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC)
)

type usuariosFalsos struct {
	porCorreo map[string]dominio.Usuario
	creados   []dominio.Usuario
}

func (u *usuariosFalsos) BuscarPorCorreo(_ context.Context, correo string) (dominio.Usuario, error) {
	usuario, ok := u.porCorreo[strings.ToLower(correo)]
	if !ok {
		return usuario, apperr.NoEncontrado("usuario no encontrado")
	}
	return usuario, nil
}
func (u *usuariosFalsos) BuscarPorID(_ context.Context, id string) (dominio.Usuario, error) {
	for _, usuario := range u.porCorreo {
		if usuario.ID == id {
			return usuario, nil
		}
	}
	return dominio.Usuario{}, apperr.NoEncontrado("usuario no encontrado")
}
func (u *usuariosFalsos) Crear(_ context.Context, usuario dominio.Usuario) (dominio.Usuario, error) {
	usuario.ID = "nuevo"
	u.creados = append(u.creados, usuario)
	u.porCorreo[strings.ToLower(usuario.Correo)] = usuario
	return usuario, nil
}

func nuevaAutenticacionPrueba(t *testing.T) (*Autenticacion, *usuariosFalsos) {
	t.Helper()
	hash, err := auth.HashPassword("Clave123*")
	if err != nil {
		t.Fatal(err)
	}
	repo := &usuariosFalsos{porCorreo: map[string]dominio.Usuario{
		"profe@uniandes.edu.co":    {ID: "p1", Correo: "profe@uniandes.edu.co", PasswordHash: hash, Activo: true, Roles: []dominio.Rol{dominio.RolProfesor}},
		"inactivo@uniandes.edu.co": {ID: "i1", Correo: "inactivo@uniandes.edu.co", PasswordHash: hash, Activo: false},
	}}
	tokens := auth.NuevoGestorTokens("secreto-de-prueba-con-mas-de-32-caracteres", time.Hour)
	return NuevaAutenticacion(repo, tokens, logPrueba), repo
}

func TestIniciarSesion(t *testing.T) {
	s, _ := nuevaAutenticacionPrueba(t)

	sesion, err := s.IniciarSesion(context.Background(), " Profe@uniandes.edu.co ", "Clave123*")
	if err != nil {
		t.Fatal(err)
	}
	if sesion.Token == "" || sesion.Usuario.ID != "p1" {
		t.Errorf("sesión inesperada: %+v", sesion)
	}

	casos := []struct {
		correo, password string
		tipo             apperr.Tipo
	}{
		{"profe@uniandes.edu.co", "incorrecta", apperr.TipoNoAutenticado},
		{"noexiste@uniandes.edu.co", "Clave123*", apperr.TipoNoAutenticado},
		{"inactivo@uniandes.edu.co", "Clave123*", apperr.TipoNoAutenticado},
		{"", "Clave123*", apperr.TipoValidacion},
	}
	for _, c := range casos {
		if _, err := s.IniciarSesion(context.Background(), c.correo, c.password); !apperr.Es(err, c.tipo) {
			t.Errorf("%s/%s: se esperaba %s, se obtuvo %v", c.correo, c.password, c.tipo, err)
		}
	}
}

func TestAsegurarAdministrador(t *testing.T) {
	s, repo := nuevaAutenticacionPrueba(t)
	ctx := context.Background()

	if err := s.AsegurarAdministrador(ctx, "Admin", "admin@uniandes.edu.co", "Admin123*"); err != nil {
		t.Fatal(err)
	}
	if len(repo.creados) != 1 || !repo.creados[0].TieneRol(dominio.RolAdministrador) || !repo.creados[0].Activo {
		t.Fatalf("el administrador debería crearse: %+v", repo.creados)
	}
	if repo.creados[0].PasswordHash == "Admin123*" {
		t.Error("la contraseña no debe guardarse en texto plano")
	}

	// Segunda ejecución: ya existe, no se duplica.
	if err := s.AsegurarAdministrador(ctx, "Admin", "admin@uniandes.edu.co", "Admin123*"); err != nil {
		t.Fatal(err)
	}
	if len(repo.creados) != 1 {
		t.Error("el administrador no debería crearse dos veces")
	}

	// Sin configuración no se crea nada.
	if err := s.AsegurarAdministrador(ctx, "Admin", "", ""); err != nil || len(repo.creados) != 1 {
		t.Errorf("sin credenciales no debería crearse: err=%v", err)
	}
}
