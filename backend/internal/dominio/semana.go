package dominio

import (
	"strings"
	"time"
)

// FormatoFecha es el formato usado en la API y en la base de datos para fechas sin hora.
const FormatoFecha = "2006-01-02"

// Las fechas sin hora (DATE en PostgreSQL) se representan como time.Time a
// medianoche UTC. Así una fecha no cambia de día al pasar entre zonas horarias.

// Fecha convierte un instante en la fecha calendario que le corresponde en la
// zona horaria dada (por ejemplo America/Bogota).
func Fecha(t time.Time, zona *time.Location) time.Time {
	if zona != nil {
		t = t.In(zona)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// ParsearFecha interpreta una fecha con formato AAAA-MM-DD.
func ParsearFecha(texto string) (time.Time, error) {
	return time.ParseInLocation(FormatoFecha, strings.TrimSpace(texto), time.UTC)
}

// InicioSemana devuelve el lunes de la semana a la que pertenece la fecha.
// La semana operativa va de lunes a domingo (RF-08.1, RN-29).
func InicioSemana(fecha time.Time) time.Time {
	d := time.Date(fecha.Year(), fecha.Month(), fecha.Day(), 0, 0, 0, 0, time.UTC)
	desplazamiento := (int(d.Weekday()) + 6) % 7 // lunes=0 ... domingo=6
	return d.AddDate(0, 0, -desplazamiento)
}

// FinSemana devuelve el domingo de la semana a la que pertenece la fecha.
func FinSemana(fecha time.Time) time.Time {
	return InicioSemana(fecha).AddDate(0, 0, 6)
}

// SemanaActual devuelve el lunes de la semana en curso según la zona horaria.
func SemanaActual(ahora time.Time, zona *time.Location) time.Time {
	return InicioSemana(Fecha(ahora, zona))
}

// SemanaCerrada indica si la semana que inicia en semanaInicio ya terminó.
// Un registro creado para una semana cerrada es un reporte tardío (RN-32).
func SemanaCerrada(semanaInicio, ahora time.Time, zona *time.Location) bool {
	return InicioSemana(semanaInicio).Before(SemanaActual(ahora, zona))
}
