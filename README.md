# Paso 25 · Dominio: cursos y proyectos

> Rama `paso-a-paso` · paso 25 de 65 · anterior: **Paso 24 · Endpoints de períodos** · siguiente: **Paso 26 · Repositorio de espacios**

---

## 1. Qué se hizo en este paso

Se modelaron los **cursos y proyectos**. En el código ambos se llaman **espacios** porque comparten casi todo (un profesor responsable, un período, fechas y estado) y solo se distinguen por su tipo.

| Archivo | Cambio |
|---|---|
| `backend/internal/dominio/espacio.go` | **Modificado.** `TipoEspacio`, `EstadoEspacio`, la entidad `Espacio`, sus reglas (`ValidarFechasEnPeriodo`, `PermiteRegistros`, `IncluyeSemana`), `FiltroEspacios` y la función auxiliar `seCruzan`. |
| `backend/internal/dominio/reglas_test.go` | **Modificado.** `TestEspacioFechasYEstado`. |

---

## 2. Por qué

- **RF-03.1 / RN-06:** el profesor crea cursos **y** proyectos. Como las reglas son las mismas, un único concepto con un campo `Tipo` evita duplicar todo el código.
- **RF-03.3:** información mínima: tipo, nombre, período, fechas de inicio y cierre, profesor responsable y observaciones.
- **RF-03.5 a RF-03.7 / RN-08 y RN-09:** un espacio se puede **cerrar**; cerrado, se consulta pero no admite registros nuevos.

---

## 3. Explicación detallada

### 3.1 Tipo y estado

```go
type TipoEspacio string

const (
	EspacioCurso    TipoEspacio = "curso"
	EspacioProyecto TipoEspacio = "proyecto"
)

type EstadoEspacio string

const (
	EspacioActivo  EstadoEspacio = "activo"
	EspacioCerrado EstadoEspacio = "cerrado"
)
```
- Cada uno con su `EsValido()`, como en los pasos anteriores. Los valores coinciden con los `CHECK` de la tabla `espacios` (paso 4).

### 3.2 La entidad

```go
type Espacio struct {
	ID             string
	Tipo           TipoEspacio
	Nombre         string
	PeriodoID      string
	PeriodoCodigo  string
	ProfesorID     string
	ProfesorNombre string
	FechaInicio    time.Time
	FechaCierre    time.Time
	Observaciones  string
	Estado         EstadoEspacio
	CreadoEn       time.Time
}
```
- Además de los IDs, trae `PeriodoCodigo` y `ProfesorNombre`. El repositorio los obtendrá con un `JOIN` (paso 26), para que la API pueda mostrar "2026-20" y "Carolina Rojas" sin consultas adicionales.

### 3.3 Reglas

```go
func (e Espacio) ValidarFechasEnPeriodo(p PeriodoAcademico) error {
	if e.FechaCierre.Before(e.FechaInicio) {
		return apperr.Validacion("la fecha de cierre no puede ser anterior a la de inicio")
	}
	if e.FechaInicio.Before(p.FechaInicio) || e.FechaCierre.After(p.FechaCierre) {
		return apperr.Validacion("las fechas deben estar dentro del período %s (%s a %s)",
			p.Codigo, p.FechaInicio.Format(FormatoFecha), p.FechaCierre.Format(FormatoFecha))
	}
	return nil
}
```
- Un curso no puede empezar antes ni terminar después de su período. El mensaje dice cuáles son las fechas válidas, para que el usuario sepa cómo corregir.
- Esta regla no se puede expresar con un `CHECK` de la tabla `espacios`, porque las fechas del período están en otra tabla. Por eso vive aquí.

```go
func (e Espacio) PermiteRegistros() error {
	if e.Estado == EspacioCerrado {
		return apperr.ReglaNegocio("el %s %q está cerrado y solo admite consultas", e.Tipo, e.Nombre)
	}
	return nil
}
```
- **RN-09 / RF-03.7:** un espacio cerrado no admite vinculaciones nuevas (paso 32) ni tareas nuevas o modificadas (pasos 36 y 39). Mensaje de ejemplo: `el curso "Nube" está cerrado y solo admite consultas`.

```go
func (e Espacio) IncluyeSemana(semanaInicio time.Time) bool {
	return seCruzan(e.FechaInicio, e.FechaCierre, InicioSemana(semanaInicio), FinSemana(semanaInicio))
}

func seCruzan(a1, a2, b1, b2 time.Time) bool {
	return !a1.After(b2) && !b1.After(a2)
}
```
- ¿La semana tiene al menos un día dentro de las fechas del espacio? Se usa para rechazar tareas en semanas en las que el curso no existía (paso 36).
- `seCruzan` es la prueba clásica de **intersección de intervalos**: dos intervalos `[a1, a2]` y `[b1, b2]` comparten algún día si **ninguno empieza después de que el otro termina**. Se reutilizará para la vigencia de las vinculaciones (paso 30).

```go
type FiltroEspacios struct {
	// VisiblePara limita el resultado a los espacios del profesor o en los que
	// el usuario participa. Vacío = sin restricción (administrador).
	VisiblePara string
	PeriodoID   string
	Estado      EstadoEspacio
	Tipo        TipoEspacio
	Paginacion
}
```
- `VisiblePara` es el mecanismo de **visibilidad** (RF-12) que usarán todos los listados: el servicio lo llena con el usuario del actor (o lo deja vacío para el administrador), y el repositorio filtra con él. Así un usuario nunca puede pedir ver lo de otros.

### 3.4 Prueba

`TestEspacioFechasYEstado` comprueba:
- fechas dentro del período → válidas;
- un cierre en enero de 2027, fuera del período → inválido;
- un cierre anterior al inicio → inválido;
- la semana del 5 de agosto está dentro del curso y la del 9 de diciembre no;
- un espacio cerrado no admite registros (regla de negocio).

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Un tipo para dos conceptos | `Espacio` con `Tipo` curso/proyecto en lugar de dos structs casi iguales. |
| Datos desnormalizados para lectura | `PeriodoCodigo`, `ProfesorNombre` vienen de otras tablas para no hacer consultas extra. |
| Intersección de intervalos | `!a1.After(b2) && !b1.After(a2)`. |
| `%q` en mensajes | Pone el nombre entre comillas: `"Nube"`. |

---

## 5. Cómo comprobarlo

```bash
cd backend
go test ./internal/dominio/ -v -run TestEspacio
```

---

## 6. Pasos para lograr esta versión desde el paso 24

1. En `dominio/espacio.go`, después de `FiltroPeriodos`, agregar `TipoEspacio`, `EstadoEspacio` (con constantes y `EsValido`), `Espacio`, `ValidarFechasEnPeriodo`, `PermiteRegistros`, `IncluyeSemana`, `FiltroEspacios` y `seCruzan`.
2. En `reglas_test.go`, después de `TestCodigoPeriodo`, agregar `TestEspacioFechasYEstado`.
3. `make pruebas lint`
4. Commit: `git add . && git commit -m "Paso 25: dominio de cursos y proyectos"`

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-03.1 / RN-06 Cursos y proyectos | `Espacio` con `TipoEspacio`. |
| RF-03.3 Información mínima | Campos de la entidad. |
| RF-03.5 / RN-08 Cerrar | `EstadoEspacio`. |
| RF-03.7 / RN-09 Sin registros en espacios cerrados | `PermiteRegistros`. |
| RF-12 Visibilidad | `FiltroEspacios.VisiblePara`. |
