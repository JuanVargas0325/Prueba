# Paso 28 · Servicio de espacios

> Rama `paso-a-paso` · paso 28 de 65 · anterior: **Paso 27 · Reloj inyectable** · siguiente: **Paso 29 · Endpoints de espacios**

---

## 1. Qué se hizo en este paso

Se creó el caso de uso de **cursos y proyectos**: el profesor los crea dentro de un período habilitado, todos consultan los que les corresponden y el profesor responsable (o el administrador) los cierra.

| Archivo | Cambio |
|---|---|
| `backend/internal/servicio/espacios.go` | **Nuevo.** Servicio `Espacios` con `Crear`, `Listar`, `Obtener` y `Cerrar`. |
| `backend/internal/servicio/acceso.go` | **Modificado.** Reglas de visibilidad por profesor: `esProfesorResponsable`, `puedeVerDeProfesor` y `visiblePara`. |
| `backend/internal/servicio/modulos_test.go` | **Modificado.** Repositorio falso de espacios y dos pruebas. |
| `backend/internal/servicio/autenticacion_test.go` | **Modificado.** Agrega el actor `otroProf` y el instante fijo `ahoraPrueba` (miércoles 14 de octubre de 2026). |

---

## 2. Por qué

- **RF-03.1 y RF-03.2:** solo un profesor crea espacios, y solo en un período habilitado (reglas del dominio, paso 21).
- **RF-03.4, RF-03.6 y RF-11.1:** cada usuario consulta sus espacios; un monitor ve aquellos en los que está vinculado.
- **RF-03.5:** solo el profesor **responsable** cierra su espacio (o el administrador).
- **RF-12 / RN-03 a RN-05:** la visibilidad se decide en el servicio con reglas reutilizables.

---

## 3. Explicación detallada

### 3.1 Reglas de visibilidad (`acceso.go`)

```go
func esProfesorResponsable(actor auth.Actor, profesorID string) bool {
	return actor.TieneRol(dominio.RolProfesor) && actor.UsuarioID == profesorID
}

func puedeVerDeProfesor(actor auth.Actor, profesorID string) bool {
	return actor.EsAdministrador() || esProfesorResponsable(actor, profesorID)
}

func visiblePara(actor auth.Actor) string {
	if actor.EsAdministrador() {
		return ""
	}
	return actor.UsuarioID
}
```
- **RN-03:** un profesor ve lo de sus espacios. **RN-05:** el administrador ve todo.
- `puedeVerDeProfesor` responde "¿este actor puede ver un recurso que pertenece a tal profesor?". Se reutilizará en vinculaciones, tareas y reportes.
- `visiblePara` produce el valor del campo `VisiblePara` de los filtros (paso 25): vacío para el administrador (sin restricción) y el propio ID para los demás. El repositorio hace el resto en SQL (paso 26).

### 3.2 Dependencias del servicio

```go
type RepositorioEspacios interface {
	Crear(ctx context.Context, e dominio.Espacio) (dominio.Espacio, error)
	Obtener(ctx context.Context, id string) (dominio.Espacio, error)
	Listar(ctx context.Context, f dominio.FiltroEspacios) ([]dominio.Espacio, error)
	Cerrar(ctx context.Context, id string) (dominio.Espacio, error)
	EsParticipante(ctx context.Context, espacioID, usuarioID string) (bool, error)
}

type LectorPeriodos interface {
	Obtener(ctx context.Context, id string) (dominio.PeriodoAcademico, error)
}

type Espacios struct {
	repo     RepositorioEspacios
	periodos LectorPeriodos
	reloj    Reloj
	log      *slog.Logger
}
```
- `LectorPeriodos`: de los períodos solo necesita **leer uno**; se declara una interfaz de un solo método. El repositorio de períodos (paso 22) la cumple.
- Recibe el `Reloj` (paso 27) para saber qué día es hoy.

### 3.3 `Crear`

```go
func (s *Espacios) Crear(ctx context.Context, actor auth.Actor, n NuevoEspacio) (dominio.Espacio, error) {
	if err := exigirRol(actor, dominio.RolProfesor); err != nil { ... }
	if !n.Tipo.EsValido() {
		return dominio.Espacio{}, apperr.Validacion("tipo inválido: use curso o proyecto")
	}
	nombre, err := dominio.Texto("el nombre", n.Nombre, true, dominio.LongitudMaxNombre)
	...
	observaciones, err := dominio.Texto("las observaciones", n.Observaciones, false, dominio.LongitudMaxObservaciones)
	...
	periodo, err := s.periodos.Obtener(ctx, n.PeriodoID)
	if apperr.Es(err, apperr.TipoNoEncontrado) {
		return dominio.Espacio{}, apperr.Validacion("el período académico indicado no existe")
	}
	...
	if err := periodo.PermiteCrearEspacios(s.reloj.Hoy()); err != nil { ... }
	e := dominio.Espacio{
		Tipo: n.Tipo, Nombre: nombre, PeriodoID: periodo.ID, ProfesorID: actor.UsuarioID,
		FechaInicio: n.FechaInicio, FechaCierre: n.FechaCierre, Observaciones: observaciones,
		Estado: dominio.EspacioActivo,
	}
	if err := e.ValidarFechasEnPeriodo(periodo); err != nil { ... }
	creado, err := s.repo.Crear(ctx, e)
	...
}
```
- El **profesor responsable es siempre quien lo crea** (`ProfesorID: actor.UsuarioID`); no se puede crear un curso a nombre de otro.
- Un período inexistente se informa como **dato inválido** (400) y no como 404: el recurso que se está creando es el espacio; el período es un dato de la petición.
- `PermiteCrearEspacios(s.reloj.Hoy())`: período activo, ventana habilitada y no terminado (RF-03.2, RN-07) → si no, **422**.
- Las fechas deben caber dentro del período.

### 3.4 `Listar`, `Obtener` y `Cerrar`

```go
func (s *Espacios) Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroEspacios) ([]dominio.Espacio, error) {
	... validar estado y tipo ...
	f.VisiblePara = visiblePara(actor)
	return s.repo.Listar(ctx, f)
}
```
- **El servicio sobrescribe `VisiblePara`.** Aunque el cliente intentara mandar otro valor, no puede: el filtro de visibilidad lo decide el servicio a partir del actor.

```go
func (s *Espacios) Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.Espacio, error) {
	e, err := s.repo.Obtener(ctx, id)
	...
	if puedeVerDeProfesor(actor, e.ProfesorID) {
		return e, nil
	}
	participa, err := s.repo.EsParticipante(ctx, id, actor.UsuarioID)
	...
	if !participa {
		return dominio.Espacio{}, noEncontrado("curso o proyecto")
	}
	return e, nil
}
```
- Pueden verlo su profesor, el administrador o quien participe en él. A los demás se les responde **404** (no se revela que existe).
- **RF-03.6:** un espacio cerrado se sigue consultando; el handler (paso 29) lo marcará como "solo lectura".

```go
func (s *Espacios) Cerrar(ctx context.Context, actor auth.Actor, id string) (dominio.Espacio, error) {
	e, err := s.Obtener(ctx, actor, id)
	...
	if !puedeVerDeProfesor(actor, e.ProfesorID) {
		return dominio.Espacio{}, apperr.Prohibido("solo el profesor responsable puede cerrar el %s", e.Tipo)
	}
	cerrado, err := s.repo.Cerrar(ctx, id)
	...
}
```
- Primero se verifica que pueda **verlo** (si no, 404); después que sea el **responsable** (si no, 403). Un monitor vinculado ve el curso, pero no puede cerrarlo.

---

## 4. Las pruebas

`repoEspaciosFalso` guarda espacios en un mapa y **registra el último filtro recibido** (`r.filtro`), para comprobar qué visibilidad aplicó el servicio. `entornoEspacios` arma el servicio con un período `2026-20` habilitado y un reloj fijo (`RelojFijo(ahoraPrueba, time.UTC)`).

| Prueba | Qué verifica |
|---|---|
| `TestCrearEspacio` | El profesor crea un espacio activo a su nombre. Un monitor no puede (403). Fechas fuera del período (400). Período inexistente (400). Período con la ventana cerrada (422, RF-03.2). |
| `TestVisibilidadYCierreDeEspacios` | Un monitor lista con `VisiblePara = "mon-1"`; el administrador sin restricción. Un monitor que no participa recibe 404; si participa, lo ve. Un monitor no puede cerrar (403); otro profesor tampoco, aunque participe (403); el responsable sí. |

`otroProf` y `ahoraPrueba` se agregaron al bloque de dobles compartidos de `autenticacion_test.go` (se trasladará a `reportes_test.go` en el paso 48).

---

## 5. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Interfaz de un método | `LectorPeriodos`: se pide lo mínimo necesario. |
| Visibilidad decidida por el servidor | El servicio fija `VisiblePara`; el cliente no puede cambiarlo. |
| 404 antes que 403 | Primero "¿puede verlo?" (404), luego "¿puede modificarlo?" (403). |
| Dobles que registran | El repositorio falso guarda el filtro recibido para verificarlo. |

---

## 6. Cómo comprobarlo

```bash
cd backend
go test ./internal/servicio/ -v -run 'Espacio'
```

---

## 7. Pasos para lograr esta versión desde el paso 27

1. En `servicio/acceso.go`: `esProfesorResponsable`, `puedeVerDeProfesor` y `visiblePara`.
2. Crear `backend/internal/servicio/espacios.go` con `RepositorioEspacios`, `LectorPeriodos`, `NuevoEspacio`, `Espacios`, `NuevoEspacios`, `Crear`, `Listar`, `Obtener` y `Cerrar`.
3. En `autenticacion_test.go`, agregar al bloque `var (...)` los dobles `otroProf` y `ahoraPrueba`.
4. En `modulos_test.go`, después de períodos, la sección `// --- espacios ---` con `repoEspaciosFalso`, `entornoEspacios`, `TestCrearEspacio` y `TestVisibilidadYCierreDeEspacios`.
5. `make pruebas lint`
6. Commit: `git add . && git commit -m "Paso 28: servicio de espacios"`

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-03.1 Crear cursos y proyectos | `Crear` (solo profesores, a su nombre). |
| RF-03.2 / RN-07 Período habilitado | `PermiteCrearEspacios` con la fecha del reloj. |
| RF-03.4 / RF-11.1 Consultar sus espacios | `Listar` con `visiblePara`. |
| RF-03.5 Cerrar | `Cerrar` solo por el responsable o el administrador. |
| RF-03.6 Consultar cerrados | `Obtener` también devuelve espacios cerrados. |
| RF-12 / RN-03 a RN-05 Visibilidad | `acceso.go`. |
