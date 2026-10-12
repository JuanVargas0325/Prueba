package dominio

import (
	"net/mail"
	"strings"
	"unicode/utf8"

	"seguimiento/internal/apperr"
)

// Límites de longitud de los textos que ingresan los usuarios.
const (
	LongitudMaxNombre        = 150
	LongitudMaxObservaciones = 4000
	LongitudMaxMotivo        = 1000
	LongitudMinPassword      = 8
	LongitudMaxPassword      = 72 // límite de bcrypt
)

// Paginación por defecto y máxima de los listados.
const (
	LimitePorDefecto = 50
	LimiteMaximo     = 200
)

// Paginacion limita los resultados de un listado.
type Paginacion struct {
	Limite         int
	Desplazamiento int
}

// SinPaginar pide a un repositorio todos los resultados. Solo lo usan los
// servicios internamente (por ejemplo, para calcular el seguimiento); las
// peticiones HTTP siempre se paginan.
var SinPaginar = Paginacion{Limite: -1}

// Normalizar aplica los valores por defecto y los máximos permitidos.
func (p Paginacion) Normalizar() Paginacion {
	if p.Limite <= 0 {
		p.Limite = LimitePorDefecto
	}
	if p.Limite > LimiteMaximo {
		p.Limite = LimiteMaximo
	}
	if p.Desplazamiento < 0 {
		p.Desplazamiento = 0
	}
	return p
}

// Texto limpia un texto ingresado por el usuario y valida su longitud.
func Texto(campo, valor string, obligatorio bool, maximo int) (string, error) {
	valor = strings.TrimSpace(valor)
	if obligatorio && valor == "" {
		return "", apperr.Validacion("%s es obligatorio", campo)
	}
	if utf8.RuneCountInString(valor) > maximo {
		return "", apperr.Validacion("%s no puede superar %d caracteres", campo, maximo)
	}
	return valor, nil
}

// NormalizarCorreo valida un correo electrónico y lo devuelve en minúsculas.
func NormalizarCorreo(correo string) (string, error) {
	correo = strings.ToLower(strings.TrimSpace(correo))
	direccion, err := mail.ParseAddress(correo)
	if correo == "" || err != nil || direccion.Address != correo {
		return "", apperr.Validacion("correo inválido")
	}
	return correo, nil
}

// ValidarPassword verifica la política mínima de contraseñas.
func ValidarPassword(password string) error {
	n := len(password)
	if n < LongitudMinPassword || n > LongitudMaxPassword {
		return apperr.Validacion("la contraseña debe tener entre %d y %d caracteres", LongitudMinPassword, LongitudMaxPassword)
	}
	return nil
}
