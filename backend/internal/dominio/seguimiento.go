package dominio

import (
	"sort"
	"strings"
	"time"
)

// EstadoSemana resume si una persona reportó trabajo en una semana.
// "sin_reporte" es un estado derivado de la ausencia de tareas (RN-35, RN-36).
type EstadoSemana string

const (
	SemanaEnCurso    EstadoSemana = "en_curso"
	SemanaConReporte EstadoSemana = "con_reporte"
	SemanaSinReporte EstadoSemana = "sin_reporte"
)

// MaxSemanasConsulta limita el rango de las consultas semanales.
const MaxSemanasConsulta = 53

// PersonaACargo es una persona vinculada a los espacios de un profesor (RF-10.1, RF-10.2).
type PersonaACargo struct {
	UsuarioID        string
	Nombre           string
	Correo           string
	HorasContratadas int
	Vinculaciones    []Vinculacion
}

// AgruparPorPersona agrupa vinculaciones por persona, ordenadas por nombre.
func AgruparPorPersona(vinculaciones []Vinculacion) []PersonaACargo {
	indice := map[string]int{}
	var personas []PersonaACargo
	for _, v := range vinculaciones {
		i, ok := indice[v.UsuarioID]
		if !ok {
			i = len(personas)
			indice[v.UsuarioID] = i
			personas = append(personas, PersonaACargo{UsuarioID: v.UsuarioID, Nombre: v.UsuarioNombre, Correo: v.UsuarioCorreo})
		}
		personas[i].Vinculaciones = append(personas[i].Vinculaciones, v)
		if v.EspacioEstado == EspacioActivo {
			personas[i].HorasContratadas += v.HorasSemanales
		}
	}
	sort.SliceStable(personas, func(a, b int) bool {
		return strings.ToLower(personas[a].Nombre) < strings.ToLower(personas[b].Nombre)
	})
	return personas
}

// DetalleVinculacionSemana es lo reportado en una vinculación durante una semana.
type DetalleVinculacionSemana struct {
	Vinculacion     Vinculacion
	Estado          EstadoSemana
	HorasReportadas int
	TareasTardias   int
	Tareas          []Tarea
}

// SemanaPersona es el detalle semanal del trabajo de una persona (RF-10.3, RF-10.6).
type SemanaPersona struct {
	SemanaInicio     time.Time
	Estado           EstadoSemana
	HorasContratadas int
	HorasReportadas  int
	Vinculaciones    []DetalleVinculacionSemana
}

// SemanaFin devuelve el domingo de la semana.
func (s SemanaPersona) SemanaFin() time.Time { return FinSemana(s.SemanaInicio) }

// ConstruirSemanas arma, de la semana más reciente a la más antigua, el detalle
// de cada semana entre desde y hasta en que la persona tenía alguna vinculación
// vigente. Una semana cerrada sin tareas queda como "sin_reporte".
func ConstruirSemanas(vinculaciones []Vinculacion, tareas []Tarea, desde, hasta, ahora time.Time, zona *time.Location) []SemanaPersona {
	actual := SemanaActual(ahora, zona)
	porSemana := agruparTareas(tareas)

	var semanas []SemanaPersona
	for semana := InicioSemana(hasta); !semana.Before(InicioSemana(desde)); semana = semana.AddDate(0, 0, -7) {
		if s, ok := construirSemana(vinculaciones, porSemana, semana, actual, zona); ok {
			semanas = append(semanas, s)
		}
	}
	return semanas
}

// construirSemana arma el detalle de una semana; ok es falso si ninguna
// vinculación estaba vigente ni tiene tareas en esa semana.
func construirSemana(vinculaciones []Vinculacion, porSemana map[claveTareas][]Tarea, semana, actual time.Time, zona *time.Location) (SemanaPersona, bool) {
	s := SemanaPersona{SemanaInicio: semana}
	totalTareas := 0
	for _, v := range vinculaciones {
		delSemana := porSemana[claveTareas{v.ID, semana}]
		if !v.VigenteEnSemana(semana, zona) && len(delSemana) == 0 {
			continue
		}
		d := detalleVinculacion(v, delSemana)
		d.Estado = estadoSemana(semana, actual, len(delSemana))
		s.Vinculaciones = append(s.Vinculaciones, d)
		s.HorasContratadas += v.HorasSemanales
		s.HorasReportadas += d.HorasReportadas
		totalTareas += len(delSemana)
	}
	s.Estado = estadoSemana(semana, actual, totalTareas)
	return s, len(s.Vinculaciones) > 0
}

// detalleVinculacion suma las horas y los reportes tardíos de las tareas de una semana.
func detalleVinculacion(v Vinculacion, tareas []Tarea) DetalleVinculacionSemana {
	d := DetalleVinculacionSemana{Vinculacion: v, Tareas: tareas}
	for _, t := range tareas {
		d.HorasReportadas += t.HorasInvertidas
		if t.Tardia {
			d.TareasTardias++
		}
	}
	return d
}

// ConsolidadoVinculacion resume el trabajo reportado en una vinculación (RF-10.4).
type ConsolidadoVinculacion struct {
	Vinculacion          Vinculacion
	SemanasTranscurridas int // semanas cerradas en las que la vinculación estuvo vigente
	SemanasConReporte    int
	SemanasSinReporte    int
	TotalTareas          int
	TotalHoras           int
	TareasTardias        int
	TareasPorEstado      map[EstadoTarea]int
	PromedioHorasSemana  float64 // horas reportadas en semanas cerradas / semanas transcurridas
}

// ConstruirConsolidado calcula el consolidado de cada vinculación.
func ConstruirConsolidado(vinculaciones []Vinculacion, tareas []Tarea, ahora time.Time, zona *time.Location) []ConsolidadoVinculacion {
	actual := SemanaActual(ahora, zona)
	porSemana := agruparTareas(tareas)

	resultado := make([]ConsolidadoVinculacion, 0, len(vinculaciones))
	for _, v := range vinculaciones {
		c, horasCerradas := totalizarTareas(v, tareas, actual)
		contarSemanas(&c, porSemana, actual, zona)
		if c.SemanasTranscurridas > 0 {
			c.PromedioHorasSemana = float64(horasCerradas) / float64(c.SemanasTranscurridas)
		}
		resultado = append(resultado, c)
	}
	return resultado
}

// totalizarTareas suma las tareas de la vinculación y devuelve también las
// horas reportadas en semanas ya cerradas (base del promedio semanal).
func totalizarTareas(v Vinculacion, tareas []Tarea, actual time.Time) (ConsolidadoVinculacion, int) {
	c := ConsolidadoVinculacion{Vinculacion: v, TareasPorEstado: map[EstadoTarea]int{}}
	for _, e := range EstadosTarea {
		c.TareasPorEstado[e] = 0
	}
	horasCerradas := 0
	for _, t := range tareas {
		if t.VinculacionID != v.ID {
			continue
		}
		c.TotalTareas++
		c.TotalHoras += t.HorasInvertidas
		c.TareasPorEstado[t.Estado]++
		if t.Tardia {
			c.TareasTardias++
		}
		if InicioSemana(t.SemanaInicio).Before(actual) {
			horasCerradas += t.HorasInvertidas
		}
	}
	return c, horasCerradas
}

// contarSemanas recorre las semanas cerradas en que la vinculación estuvo
// vigente y cuenta cuáles tienen reporte y cuáles no.
func contarSemanas(c *ConsolidadoVinculacion, porSemana map[claveTareas][]Tarea, actual time.Time, zona *time.Location) {
	v := c.Vinculacion
	fin := InicioSemana(v.EspacioFechaCierre)
	if !fin.Before(actual) {
		fin = actual.AddDate(0, 0, -7) // solo semanas cerradas
	}
	for semana := InicioSemana(v.InicioVigencia(zona)); !semana.After(fin); semana = semana.AddDate(0, 0, 7) {
		c.SemanasTranscurridas++
		if len(porSemana[claveTareas{v.ID, semana}]) > 0 {
			c.SemanasConReporte++
		} else {
			c.SemanasSinReporte++
		}
	}
}

type claveTareas struct {
	vinculacionID string
	semana        time.Time
}

func agruparTareas(tareas []Tarea) map[claveTareas][]Tarea {
	m := map[claveTareas][]Tarea{}
	for _, t := range tareas {
		k := claveTareas{t.VinculacionID, InicioSemana(t.SemanaInicio)}
		m[k] = append(m[k], t)
	}
	return m
}

func estadoSemana(semana, actual time.Time, tareas int) EstadoSemana {
	switch {
	case !semana.Before(actual):
		return SemanaEnCurso
	case tareas > 0:
		return SemanaConReporte
	default:
		return SemanaSinReporte
	}
}
