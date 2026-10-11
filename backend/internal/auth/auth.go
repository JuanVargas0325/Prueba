// Package auth implementa la autenticación local: hash de contraseñas,
// emisión y validación de tokens JWT, y el actor autenticado (RF-01, RNF-03).
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"seguimiento/internal/dominio"
)

// Actor es el usuario autenticado que realiza una operación.
type Actor struct {
	UsuarioID string
	Roles     []dominio.Rol
}

// TieneRol indica si el actor tiene el rol dado.
func (a Actor) TieneRol(rol dominio.Rol) bool {
	for _, r := range a.Roles {
		if r == rol {
			return true
		}
	}
	return false
}

// TieneAlgunRol indica si el actor tiene al menos uno de los roles dados.
func (a Actor) TieneAlgunRol(roles ...dominio.Rol) bool {
	for _, r := range roles {
		if a.TieneRol(r) {
			return true
		}
	}
	return false
}

// EsAdministrador indica si el actor tiene visibilidad global (RN-05).
func (a Actor) EsAdministrador() bool { return a.TieneRol(dominio.RolAdministrador) }

// HashPassword genera el hash bcrypt de una contraseña.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("generando hash de contraseña: %w", err)
	}
	return string(hash), nil
}

// VerificarPassword compara una contraseña con su hash bcrypt.
func VerificarPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// ErrTokenInvalido indica que el token no es válido o expiró.
var ErrTokenInvalido = errors.New("token inválido o expirado")

const emisor = "seguimiento-api"

type claims struct {
	Roles []dominio.Rol `json:"roles"`
	jwt.RegisteredClaims
}

// GestorTokens emite y valida tokens JWT firmados con HS256.
type GestorTokens struct {
	secreto    []byte
	expiracion time.Duration
	ahora      func() time.Time
}

// NuevoGestorTokens crea un gestor de tokens.
func NuevoGestorTokens(secreto string, expiracion time.Duration) *GestorTokens {
	return &GestorTokens{secreto: []byte(secreto), expiracion: expiracion, ahora: time.Now}
}

// Generar emite un token para el usuario y devuelve su fecha de expiración.
func (g *GestorTokens) Generar(u dominio.Usuario) (string, time.Time, error) {
	ahora := g.ahora()
	expira := ahora.Add(g.expiracion)
	c := claims{
		Roles: u.Roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			Issuer:    emisor,
			IssuedAt:  jwt.NewNumericDate(ahora),
			ExpiresAt: jwt.NewNumericDate(expira),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(g.secreto)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("firmando token: %w", err)
	}
	return token, expira, nil
}

// Validar verifica la firma y la vigencia del token y devuelve el actor.
func (g *GestorTokens) Validar(token string) (Actor, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) {
		return g.secreto, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(emisor),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(g.ahora),
	)
	if err != nil || c.Subject == "" {
		return Actor{}, ErrTokenInvalido
	}
	return Actor{UsuarioID: c.Subject, Roles: c.Roles}, nil
}
