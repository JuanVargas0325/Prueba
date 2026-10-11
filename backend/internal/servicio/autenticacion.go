// Package servicio contiene la capa de lógica de negocio: casos de uso,
// reglas del dominio y control de acceso a la información. No conoce HTTP ni
// SQL; depende de interfaces que implementan las capas de datos e infraestructura.
package servicio

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

// RepositorioUsuarios es lo que la autenticación necesita de la capa de datos.
type RepositorioUsuarios interface {
	BuscarPorCorreo(ctx context.Context, correo string) (dominio.Usuario, error)
	BuscarPorID(ctx context.Context, id string) (dominio.Usuario, error)
	Crear(ctx context.Context, u dominio.Usuario) (dominio.Usuario, error)
}

// EmisorTokens emite tokens de acceso.
type EmisorTokens interface {
	Generar(u dominio.Usuario) (string, time.Time, error)
}

// Autenticacion implementa el inicio de sesión local (RF-01.1).
type Autenticacion struct {
	usuarios RepositorioUsuarios
	tokens   EmisorTokens
	log      *slog.Logger
	// hashFicticio se compara cuando el correo no existe, para que el tiempo de
	// respuesta no revele qué correos están registrados.
	hashFicticio string
}

// Sesion es el resultado de un inicio de sesión exitoso.
type Sesion struct {
	Token    string
	ExpiraEn time.Time
	Usuario  dominio.Usuario
}

// NuevaAutenticacion crea el servicio de autenticación.
func NuevaAutenticacion(usuarios RepositorioUsuarios, tokens EmisorTokens, log *slog.Logger) *Autenticacion {
	hash, _ := auth.HashPassword("contraseña-ficticia-para-comparar")
	return &Autenticacion{usuarios: usuarios, tokens: tokens, log: log, hashFicticio: hash}
}

// IniciarSesion valida las credenciales y emite un token.
func (s *Autenticacion) IniciarSesion(ctx context.Context, correo, password string) (Sesion, error) {
	correo = strings.TrimSpace(correo)
	if correo == "" || password == "" {
		return Sesion{}, apperr.Validacion("correo y contraseña son obligatorios")
	}
	u, err := s.usuarios.BuscarPorCorreo(ctx, correo)
	if err != nil && !apperr.Es(err, apperr.TipoNoEncontrado) {
		return Sesion{}, err
	}
	hash := s.hashFicticio
	if err == nil {
		hash = u.PasswordHash
	}
	if !auth.VerificarPassword(hash, password) || err != nil || !u.Activo {
		return Sesion{}, apperr.NoAutenticado("credenciales inválidas")
	}
	token, expira, err := s.tokens.Generar(u)
	if err != nil {
		return Sesion{}, apperr.Interno(err, "no fue posible iniciar sesión")
	}
	return Sesion{Token: token, ExpiraEn: expira, Usuario: u}, nil
}

// UsuarioActual devuelve el usuario autenticado.
func (s *Autenticacion) UsuarioActual(ctx context.Context, actor auth.Actor) (dominio.Usuario, error) {
	return s.usuarios.BuscarPorID(ctx, actor.UsuarioID)
}

// AsegurarAdministrador crea el administrador inicial si se configuró y aún no existe.
// Así el sistema siempre tiene una cuenta con la que crear los demás usuarios.
func (s *Autenticacion) AsegurarAdministrador(ctx context.Context, nombre, correo, password string) error {
	if correo == "" || password == "" {
		s.log.Warn("ADMIN_CORREO o ADMIN_PASSWORD no están definidos: no se crea el administrador inicial")
		return nil
	}
	_, err := s.usuarios.BuscarPorCorreo(ctx, correo)
	if err == nil {
		return nil
	}
	if !apperr.Es(err, apperr.TipoNoEncontrado) {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.usuarios.Crear(ctx, dominio.Usuario{
		Nombre:       nombre,
		Correo:       correo,
		PasswordHash: hash,
		Activo:       true,
		Roles:        []dominio.Rol{dominio.RolAdministrador},
	})
	if apperr.Es(err, apperr.TipoConflicto) {
		return nil // otra instancia lo creó al mismo tiempo
	}
	if err != nil {
		return err
	}
	s.log.Info("administrador inicial creado", "correo", correo)
	return nil
}
