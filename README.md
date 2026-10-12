# Paso 21 · Dominio: períodos académicos

> Rama `paso-a-paso` · paso 21 de 65 · anterior: **Paso 20 · Fechas y semanas** · siguiente: **Paso 22 · Repositorio de períodos**

---

## 1. Qué se hizo en este paso

Se modeló el **período académico** (por ejemplo `2026-20`) y sus reglas: formato del código, fechas coherentes y cuándo se pueden crear cursos y proyectos en él.

| Archivo | Cambio |
|---|---|
| `backend/internal/dominio/espacio.go` | **Nuevo.** `EstadoPeriodo`, `PeriodoAcademico`, `ValidarCodigoPeriodo`, `ValidarFechas`, `PermiteCrearEspacios` y `FiltroPeriodos`. |
| `backend/internal/dominio/reglas_test.go` | **Modificado.** Pruebas de las reglas de períodos. |

El archivo se llama `espacio.go` porque en el paso 25 recibirá también los cursos y proyectos ("espacios"), que siempre pertenecen a un período.

---

## 2. Por qué

**RF-03.2** dice: "El sistema debe permitir la creación de cursos y proyectos únicamente dentro de un período académico previamente definido y habilitado para creación... registrando como mínimo la identificación del período (por ejemplo 2026-10 o 2026-20), la fecha de inicio, la fecha de cierre y su estado, como activo o cerrado... No deberá permitir la creación en períodos cerrados, inactivos o no habilitados."

Y **RN-07**: "Los cursos y proyectos solo pueden ser creados al inicio de un período académico... El sistema debe bloquear la creación fuera de la ventana autorizada."

Estas reglas son puras (no necesitan base de datos) y por eso viven en el dominio.

---

## 3. Explicación detallada

### 3.1 Estado y entidad

```go
type EstadoPeriodo string

const (
	PeriodoActivo  EstadoPeriodo = "activo"
	PeriodoCerrado EstadoPeriodo = "cerrado"
)

func (e EstadoPeriodo) EsValido() bool { return e == PeriodoActivo || e == PeriodoCerrado }
```
- Mismo patrón que los roles (paso 11): un tipo basado en `string`, sus valores posibles y un método para validar lo que llega del exterior.

```go
type PeriodoAcademico struct {
	ID          string
	Codigo      string // por ejemplo 2026-20
	FechaInicio time.Time
	FechaCierre time.Time
	Estado      EstadoPeriodo
	// CreacionHabilitada abre o cierra la ventana en la que los profesores
	// pueden crear cursos y proyectos (RN-07).
	CreacionHabilitada bool
	CreadoEn           time.Time
}
```
- `CreacionHabilitada` es la **ventana de creación** de RN-07. El administrador la abre al inicio del semestre y la cierra cuando ya no se deben crear más cursos o proyectos.
- **Decisión de diseño:** se eligió un interruptor que controla el administrador en lugar de una fecha límite automática, porque el enunciado no define cuántos días dura "el inicio del período". Una alternativa válida (propuesta en el equipo) es una columna `fecha_limite_creacion`; se puede agregar en el futuro con una migración nueva.

### 3.2 Reglas

```go
var formatoCodigoPeriodo = regexp.MustCompile(`^\d{4}-\d{2}$`)

func ValidarCodigoPeriodo(codigo string) error {
	if !formatoCodigoPeriodo.MatchString(codigo) {
		return apperr.Validacion("el código del período debe tener formato AAAA-NN, por ejemplo 2026-20")
	}
	return nil
}
```
- Una **expresión regular**: `^` inicio, `\d{4}` cuatro dígitos, `-` un guion, `\d{2}` dos dígitos, `$` fin. Acepta `2026-20` y rechaza `2026-2` o `20262`.
- `regexp.MustCompile` compila la expresión **una sola vez** al cargar el programa (variable de paquete) y entra en pánico si estuviera mal escrita: un error de programación, no de datos.
- El texto entre comillas invertidas evita tener que escribir `\\d`.

```go
func (p PeriodoAcademico) ValidarFechas() error {
	if !p.FechaCierre.After(p.FechaInicio) {
		return apperr.Validacion("la fecha de cierre del período debe ser posterior a la de inicio")
	}
	return nil
}
```
- Coincide con el `CHECK (fecha_cierre > fecha_inicio)` de la tabla (paso 4), pero aquí produce un mensaje claro (400) en lugar de un error de la base de datos.

```go
func (p PeriodoAcademico) PermiteCrearEspacios(hoy time.Time) error {
	switch {
	case p.Estado != PeriodoActivo:
		return apperr.ReglaNegocio("el período %s está cerrado", p.Codigo)
	case !p.CreacionHabilitada:
		return apperr.ReglaNegocio("el período %s no está habilitado para crear cursos o proyectos", p.Codigo)
	case hoy.After(p.FechaCierre):
		return apperr.ReglaNegocio("el período %s ya terminó", p.Codigo)
	}
	return nil
}
```
- Tres condiciones para poder crear cursos o proyectos (RF-03.2, RN-07): el período está **activo**, su ventana de creación está **habilitada** y **no ha terminado**.
- Son violaciones de **reglas de negocio** (422), no errores de formato.
- Recibe `hoy` como parámetro (no llama a `time.Now`), para poder probarla con cualquier fecha.

```go
type FiltroPeriodos struct {
	Estado EstadoPeriodo
	Paginacion
}
```

### 3.3 Pruebas

| Prueba | Casos |
|---|---|
| `TestPeriodoPermiteCrearEspacios` | Un período activo, habilitado y vigente permite crear. Cerrado, inhabilitado o ya terminado → regla de negocio. Fechas iguales (cierre = inicio) → inválido. |
| `TestCodigoPeriodo` | `2026-20` válido; `2026-2` y `20262` inválidos. |

La prueba crea copias modificadas del período (`cerrado := p; cerrado.Estado = PeriodoCerrado`): asignar un struct **copia** todos sus campos, así que modificar la copia no afecta al original.

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| `regexp` | Expresiones regulares; `MustCompile` para patrones fijos. |
| Variables de paquete | `var formatoCodigoPeriodo = ...` se inicializa una vez al arrancar. |
| Copia de structs | `b := a` copia todos los campos. |
| `switch` con condiciones | Devuelve en el primer caso que se cumpla. |

---

## 5. Cómo comprobarlo

```bash
cd backend
go test ./internal/dominio/ -v -run 'Periodo'
```

---

## 6. Pasos para lograr esta versión desde el paso 20

1. Crear `backend/internal/dominio/espacio.go` con `EstadoPeriodo` (y sus constantes y `EsValido`), `PeriodoAcademico`, `formatoCodigoPeriodo`, `ValidarCodigoPeriodo`, `ValidarFechas`, `PermiteCrearEspacios` y `FiltroPeriodos`.
2. Agregar a `reglas_test.go` (antes de las pruebas de correo) `TestPeriodoPermiteCrearEspacios` y `TestCodigoPeriodo`, con el import de `apperr`.
3. `make pruebas lint`
4. Commit: `git add . && git commit -m "Paso 21: dominio de períodos académicos"`

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-03.2 Períodos explícitos | Código, fechas, estado y ventana de creación. |
| RN-07 Creación solo en la ventana autorizada | `PermiteCrearEspacios`. |
