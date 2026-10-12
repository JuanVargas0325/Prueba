package servicio

import (
	"time"

	"seguimiento/internal/dominio"
)

// Reloj entrega la fecha y hora actuales en la zona horaria de la aplicación.
// Se inyecta en los servicios para que las reglas que dependen del tiempo
// (semanas, tardanza, períodos) se puedan probar con una fecha fija.
type Reloj struct {
	zona  *time.Location
	ahora func() time.Time
}

// NuevoReloj crea un reloj con la hora del sistema.
func NuevoReloj(zona *time.Location) Reloj {
	if zona == nil {
		zona = time.UTC
	}
	return Reloj{zona: zona, ahora: time.Now}
}

// RelojFijo crea un reloj que siempre devuelve el mismo instante (para pruebas).
func RelojFijo(instante time.Time, zona *time.Location) Reloj {
	r := NuevoReloj(zona)
	r.ahora = func() time.Time { return instante }
	return r
}

// Ahora devuelve el instante actual.
func (r Reloj) Ahora() time.Time { return r.ahora() }

// Zona devuelve la zona horaria de la aplicación.
func (r Reloj) Zona() *time.Location { return r.zona }

// Hoy devuelve la fecha actual (sin hora) en la zona de la aplicación.
func (r Reloj) Hoy() time.Time { return dominio.Fecha(r.ahora(), r.zona) }

// SemanaActual devuelve el lunes de la semana en curso.
func (r Reloj) SemanaActual() time.Time { return dominio.SemanaActual(r.ahora(), r.zona) }
