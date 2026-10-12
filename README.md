# Paso 41 · Dominio: seguimiento semanal y "sin reporte"

> Rama `paso-a-paso` · paso 41 de 65 · anterior: **Paso 40 · Endpoints de adjuntos** · siguiente: **Paso 42 · Consulta de tareas por vinculaciones**

---

## 1. Qué se hizo en este paso

Se agregaron al dominio los cálculos del **seguimiento**: cómo le fue a cada persona en cada semana (con reporte, sin reporte o en curso), cuántas horas reportó frente a las contratadas y un consolidado acumulado por vinculación.

| Archivo | Cambio |
|---|---|
| `backend/internal/dominio/seguimiento.go` | **Nuevo.** `EstadoSemana`, `PersonaACargo` y `AgruparPorPersona`, `SemanaPersona` y `ConstruirSemanas`, `ConsolidadoVinculacion` y `ConstruirConsolidado`, más funciones auxiliares. |
| `backend/internal/dominio/reglas_test.go` | **Modificado.** Tres pruebas sobre un escenario de dos vinculaciones y cuatro tareas. |

---

## 2. Por qué

- **RF-09 / RN-35 a RN-37:** "cuando una persona no consigne actividad alguna durante una semana, el sistema deberá marcar dicho periodo con el estado *sin reporte*". El enunciado lo excluye de la Entrega 1, pero se implementa **por adelantado** porque la vista del profesor (RF-10) pierde mucho valor sin él, y el cálculo es puro.
- **RN-36: "sin reporte" es un estado derivado.** No se guarda en ninguna tabla: se **calcula** a partir de las vinculaciones vigentes y las tareas. Así nunca queda desactualizado (si alguien registra una tarea tardía, la semana pasa sola a "con reporte").
- **RF-10.3, RF-10.4 y RF-10.6:** detalle semanal, vista consolidada y horas reportadas frente a contratadas.

---

## 3. Explicación detallada

### 3.1 Estados de una semana

```go
type EstadoSemana string

const (
	SemanaEnCurso    EstadoSemana = "en_curso"
	SemanaConReporte EstadoSemana = "con_reporte"
	SemanaSinReporte EstadoSemana = "sin_reporte"
)

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
```
- La semana **actual** siempre está "en curso" (todavía hay tiempo de reportar, RN-37).
- Una semana **cerrada** con al menos una tarea está "con reporte"; sin ninguna, "sin reporte".

```go
const MaxSemanasConsulta = 53
```
- Un año como máximo por consulta, para que nadie pida diez años de historial de una vez.

### 3.2 Personas a cargo

```go
type PersonaACargo struct {
	UsuarioID        string
	Nombre           string
	Correo           string
	HorasContratadas int
	Vinculaciones    []Vinculacion
}

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
```
- **RF-10.1 / RF-10.2:** el profesor ve a sus monitores y asistentes con todas sus vinculaciones.
- Un mapa `UsuarioID → posición` agrupa en una sola pasada (eficiente aunque haya muchas vinculaciones).
- Solo suman horas contratadas las vinculaciones de espacios **activos**.
- `sort.SliceStable` ordena por nombre sin distinguir mayúsculas, con una función que compara dos elementos.

### 3.3 El detalle semana a semana

```go
type DetalleVinculacionSemana struct {
	Vinculacion     Vinculacion
	Estado          EstadoSemana
	HorasReportadas int
	TareasTardias   int
	Tareas          []Tarea
}

type SemanaPersona struct {
	SemanaInicio     time.Time
	Estado           EstadoSemana
	HorasContratadas int
	HorasReportadas  int
	Vinculaciones    []DetalleVinculacionSemana
}
```
- Para cada semana: el estado general de la persona, horas contratadas frente a reportadas (RF-10.6) y el detalle por vinculación, con sus tareas y cuántas fueron tardías.

```go
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
```
- Recorre las semanas **de la más reciente a la más antigua**, restando 7 días.
- `agruparTareas` arma un mapa `(vinculación, semana) → tareas` **una sola vez**, para no recorrer todas las tareas en cada semana:

```go
type claveTareas struct {
	vinculacionID string
	semana        time.Time
}
```
  En Go se puede usar un **struct como clave de un mapa**, siempre que sus campos sean comparables.

```go
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
```
- Una vinculación entra en la semana si estaba **vigente** (paso 30: desde la fecha más tardía entre el inicio del espacio y la creación de la vinculación) **o** si tiene tareas en ella.
- Las semanas sin ninguna vinculación vigente **no aparecen** (por ejemplo, antes de que la persona fuera vinculada): no tiene sentido marcarlas "sin reporte".
- `ok` en el retorno (segundo valor `bool`) es una convención de Go para "¿hay resultado?".

### 3.4 El consolidado

```go
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
```
- **RF-10.4:** una vista acumulada por vinculación.
- `totalizarTareas` suma tareas, horas, tardías y cuenta por estado (inicializando los tres estados en 0 para que siempre aparezcan).
- `contarSemanas` recorre solo las semanas **cerradas** de la vigencia y cuenta con y sin reporte.
- **El promedio usa solo semanas cerradas:** las horas de la semana en curso no se suman al promedio, porque esa semana todavía no terminó y bajaría artificialmente el valor. Es `float64` (decimal) porque un promedio no es entero.
- Se separó en funciones pequeñas para que cada una sea fácil de leer (SonarQube reportaría una función demasiado compleja, paso 62).

---

## 4. Las pruebas

Escenario (`datosSeguimiento`): Laura tiene una **monitoría** (`v1`, 4 h, espacio desde el 07/09) y una **asistencia** (`v2`, 12 h, espacio desde el 21/09). Tareas: 3 h en v1 la semana del 07/09, 10 h en v2 la del 21/09, 2 h **tardías** en v1 la del 21/09 y 1 h en v1 la del 05/10. "Ahora" = 7 de octubre (semana actual: 05/10).

| Prueba | Qué verifica |
|---|---|
| `TestConstruirSemanasDetectaSinReporte` | Del 31/08 al 05/10 hay 5 semanas (la del 31/08 no aparece: ningún espacio había empezado). Estados: 05/10 en curso; 28/09 **sin reporte**; 21/09 con reporte; 14/09 **sin reporte**; 07/09 con reporte. Orden descendente. La semana del 21/09 suma 12 h reportadas, 16 h contratadas, 2 vinculaciones y 1 tarea tardía. El 14/09 solo estaba vigente la monitoría. |
| `TestConstruirConsolidado` | Monitoría: 4 semanas cerradas, 2 con reporte, 2 sin reporte, 3 tareas, 6 h, 1 tardía, 2 abiertas; promedio **1,25** (5 h de semanas cerradas / 4; la hora de la semana en curso no cuenta). |
| `TestAgruparPorPersona` | Ordena por nombre sin distinguir mayúsculas ("andrés" antes que "Zoe") y no suma horas de espacios cerrados. |

---

## 5. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Struct como clave de mapa | `map[claveTareas][]Tarea`. |
| `sort.SliceStable` | Ordena con una función de comparación, manteniendo el orden de los iguales. |
| `(resultado, ok)` | Convención para "puede no haber resultado". |
| `float64` | Número decimal (promedios). |
| Ciclos con fechas | `for semana := ...; cond; semana = semana.AddDate(0, 0, -7)`. |

---

## 6. Cómo comprobarlo

```bash
cd backend
go test ./internal/dominio/ -v -run 'Construir|Agrupar'
```

---

## 7. Pasos para lograr esta versión desde el paso 40

1. Crear `backend/internal/dominio/seguimiento.go` con los tipos y funciones de la sección 3.
2. Al final de `reglas_test.go`, la sección `// --- seguimiento ---` con `datosSeguimiento` y las tres pruebas.
3. `make pruebas lint`
4. Commit: `git add . && git commit -m "Paso 41: dominio del seguimiento semanal"`

---

## 8. Preparación para las próximas entregas

- **El cálculo se hace en memoria en cada consulta.** Es simple y siempre consistente, pero su costo crece con el volumen de tareas. Por eso, en los pasos 42 y 43 se cuida cómo se usa: la consulta trae solo las columnas necesarias y cada persona recibe solo sus propias tareas. Las pruebas de carga del paso 64 miden el resultado (deuda técnica DT-08).
- Como el estado "sin reporte" ya existe, la futura notificación de los viernes (fuera del alcance de la Entrega 1) podrá reutilizar `ConstruirSemanas` para saber a quién recordar.

---

## 9. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-09 / RN-35 a RN-37 Sin reporte (adelantado) | `estadoSemana` derivado, nunca almacenado. |
| RF-10.1 / RF-10.2 Personas a cargo | `AgruparPorPersona`. |
| RF-10.3 Detalle semanal | `ConstruirSemanas`. |
| RF-10.4 Consolidado | `ConstruirConsolidado`. |
| RF-10.6 Horas reportadas frente a contratadas | `SemanaPersona.HorasContratadas` y `HorasReportadas`. |
