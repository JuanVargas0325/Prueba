# Paso 36 · Servicio de tareas

> Rama `paso-a-paso` · paso 36 de 65 · anterior: **Paso 35 · Repositorio de tareas** · siguiente: **Paso 37 · Endpoints de tareas**

---

## 1. Qué se hizo en este paso

Se creó el caso de uso del **registro semanal de tareas**: los monitores y asistentes registran tareas en sus vinculaciones, las editan o eliminan mientras la semana está activa, y cada usuario consulta lo que le corresponde. Los adjuntos se agregarán en el paso 39.

| Archivo | Cambio |
|---|---|
| `backend/internal/servicio/tareas.go` | **Nuevo.** Servicio `Tareas` con `Crear`, `Listar`, `Obtener`, `Actualizar`, `Eliminar` y `Modificable`, más `obtenerModificable`, `verificarModificable` y `espacioDe`. |
| `backend/internal/servicio/modulos_test.go` | **Modificado.** Repositorio falso de tareas, lector falso de vinculaciones y cinco pruebas. |

---

## 2. Por qué

| Requisito | Cómo lo resuelve el servicio |
|---|---|
| RF-06.1 / RN-21 Registrar tareas en **una vinculación propia** | Verifica que la vinculación sea del actor. |
| RF-06.6 Varias tareas por semana | No hay límite por semana. |
| RF-08.1 Semana de lunes a domingo | Cualquier fecha se normaliza al lunes. |
| RF-08.3 Semanas pasadas | Se aceptan. |
| RF-08.4 / RN-32 Tardanza | Si la semana ya cerró, la tarea se marca `Tardia`. |
| RF-06.7, RF-06.8 / RN-30, RN-33, RN-34 | Editar o eliminar solo si es propia, la semana sigue activa, no es tardía y el espacio no está cerrado. |
| RN-09 Espacio cerrado | No admite tareas nuevas ni cambios. |
| RF-10.5, RF-11.2, RF-12 Visibilidad | Cada uno ve las suyas; el profesor, las de sus espacios; el administrador, todas. |

---

## 3. Explicación detallada

### 3.1 Dependencias

```go
type RepositorioTareas interface {
	Crear(ctx context.Context, t dominio.Tarea, adjuntos []dominio.Adjunto) (dominio.Tarea, error)
	Obtener(ctx context.Context, id string) (dominio.Tarea, error)
	Listar(ctx context.Context, f dominio.FiltroTareas) ([]dominio.Tarea, error)
	Actualizar(ctx context.Context, t dominio.Tarea) (dominio.Tarea, error)
	Eliminar(ctx context.Context, id string) ([]string, error)
}

type LectorVinculaciones interface {
	Obtener(ctx context.Context, id string) (dominio.Vinculacion, error)
}

type NuevaTarea struct {
	VinculacionID string
	Semana        time.Time // cualquier fecha de la semana (se normaliza al lunes); cero = semana en curso
	Datos         dominio.DatosTarea
}
```
- Del repositorio de tareas (paso 35) se usan por ahora cinco métodos; el paso 39 agregará los de adjuntos.
- `Semana` en su **valor cero** (`time.Time{}`) significa "la semana en curso": el caso más común es registrar lo de esta semana sin indicarla.

### 3.2 `Crear`

```go
func (s *Tareas) Crear(ctx context.Context, actor auth.Actor, n NuevaTarea) (dominio.Tarea, error) {
	v, err := s.vinculaciones.Obtener(ctx, n.VinculacionID)
	if apperr.Es(err, apperr.TipoNoEncontrado) || (err == nil && v.UsuarioID != actor.UsuarioID) {
		// Solo se registran tareas en vinculaciones propias (RN-04, RN-21).
		return dominio.Tarea{}, apperr.Validacion("la vinculación indicada no existe o no le pertenece")
	}
	...
	espacio := espacioDe(v)
	if err := espacio.PermiteRegistros(); err != nil { ... }

	semana := s.reloj.SemanaActual()
	if !n.Semana.IsZero() {
		semana = dominio.InicioSemana(n.Semana)
	}
	if err := dominio.ValidarSemanaDeRegistro(semana, s.reloj.Ahora(), s.reloj.Zona()); err != nil { ... }
	if !espacio.IncluyeSemana(semana) {
		return dominio.Tarea{}, apperr.Validacion("la semana del %s está fuera de las fechas del %s (%s a %s)", ...)
	}
	datos, err := n.Datos.Normalizar()
	...
	t := dominio.Tarea{
		ID:            uuid.NewString(),
		VinculacionID: v.ID,
		SemanaInicio:  semana,
		Tardia:        dominio.SemanaCerrada(semana, s.reloj.Ahora(), s.reloj.Zona()),
	}
	t.Aplicar(datos)

	creada, err := s.repo.Crear(ctx, t, nil)
	...
}
```
1. **La vinculación debe ser del actor.** Una vinculación ajena se responde igual que una inexistente ("no existe o no le pertenece"), sin revelar si existe.
2. **El espacio no puede estar cerrado** (RN-09, 422). `espacioDe(v)` arma un `dominio.Espacio` con los datos que trae la vinculación, para reutilizar sus reglas sin otra consulta.
3. **La semana:** la actual por defecto o el lunes de la fecha indicada. No futura y dentro de las fechas del espacio.
4. **Los datos** se validan con el dominio (paso 34).
5. **El ID se genera aquí** (`uuid.NewString()`, paso 35).
6. **`Tardia`** se calcula con la hora del reloj: `true` si la semana ya cerró (RN-32). Es la única forma en que una tarea queda tardía; el usuario no puede marcarla.
7. Se guarda sin adjuntos (`nil`).

### 3.3 Consultar

```go
func (s *Tareas) Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroTareas) ([]dominio.Tarea, error) {
	f.VisiblePara = visiblePara(actor)
	return s.repo.Listar(ctx, f)
}

func (s *Tareas) Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.Tarea, error) {
	t, err := s.repo.Obtener(ctx, id)
	...
	if t.UsuarioID != actor.UsuarioID && !puedeVerDeProfesor(actor, t.ProfesorID) {
		return dominio.Tarea{}, noEncontrado("tarea")
	}
	return t, nil
}
```
- Ven una tarea: la persona que la registró, el profesor del espacio y el administrador. Los demás reciben 404.

### 3.4 Modificar

```go
func (s *Tareas) verificarModificable(actor auth.Actor, t dominio.Tarea) error {
	if t.UsuarioID != actor.UsuarioID {
		return apperr.Prohibido("solo la persona que registró la tarea puede modificarla")
	}
	if t.EspacioEstado == dominio.EspacioCerrado {
		return apperr.ReglaNegocio("el %s %q está cerrado y solo admite consultas", t.EspacioTipo, t.EspacioNombre)
	}
	return t.PuedeModificarse(s.reloj.Ahora(), s.reloj.Zona())
}

func (s *Tareas) obtenerModificable(ctx context.Context, actor auth.Actor, id string) (dominio.Tarea, error) {
	t, err := s.Obtener(ctx, actor, id)
	if err != nil {
		return t, err
	}
	return t, s.verificarModificable(actor, t)
}
```
- Cuatro condiciones: puede **verla** (si no, 404); es **suya** (el profesor la ve pero no la edita: 403); el espacio **no está cerrado** (422); y el dominio lo permite (**no tardía**, **semana activa**: 422).
- `Actualizar` y `Eliminar` empiezan con `obtenerModificable`. `Actualizar` normaliza los datos, los aplica (`t.Aplicar`) y guarda; la semana y la vinculación no cambian (RN-26).

```go
func (s *Tareas) Modificable(actor auth.Actor, t dominio.Tarea) bool {
	return s.verificarModificable(actor, t) == nil
}
```
- Responde sí o no. El handler (paso 37) lo incluirá en cada tarea (`"modificable": true/false`) para que una interfaz sepa si mostrar los botones de editar y eliminar.

---

## 4. Las pruebas

El reloj de prueba marca el **miércoles 14 de octubre de 2026** (semana del 12 al 18). `lectorVinculacionesFalso` tiene tres vinculaciones: `v1` (propia, espacio activo de agosto a diciembre), `cerrada` (propia, espacio cerrado) y `ajena` (de otra persona).

| Prueba | Qué verifica |
|---|---|
| `TestCrearTareaEnLaSemanaActual` | Con la fecha 15/10 la semana queda en el lunes 12/10; no es tardía; se genera ID; las horas llegan al repositorio. |
| `TestCrearTareaSinSemanaUsaLaSemanaActual` | Sin semana → semana actual (12/10). |
| `TestCrearTareaEnSemanaPasadaEsTardia` | Una tarea del 07/10 (semana cerrada) queda **tardía** (RN-32). |
| `TestCrearTareaValidaciones` | Vinculación ajena o inexistente (400), espacio cerrado (422), semana futura (400), semana fuera de las fechas del espacio (400) y título vacío (400). |
| `TestModificarTareas` | La tarea de la semana activa se edita; `Modificable` es verdadero solo para su dueña. Pasada, tardía o de espacio cerrado → 422 al editar y al eliminar. El profesor la ve pero no la edita (403); otro profesor ni la ve (404); la dueña la elimina. |

---

## 5. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| `t.IsZero()` | ¿Es el valor cero de `time.Time`? (fecha "no indicada"). |
| `uuid.NewString()` | Genera un UUID aleatorio (versión 4). |
| Reutilizar reglas con un valor armado | `espacioDe(v)` construye un `Espacio` para usar `PermiteRegistros` e `IncluyeSemana`. |
| 404 → 403 → 422 | No verla, no ser dueño, regla de negocio: en ese orden. |

---

## 6. Cómo comprobarlo

```bash
cd backend
go test ./internal/servicio/ -v -run 'Tarea'
```

---

## 7. Pasos para lograr esta versión desde el paso 35

1. Crear `backend/internal/servicio/tareas.go` con `RepositorioTareas` (cinco métodos), `LectorVinculaciones`, `NuevaTarea`, `Tareas`, `NuevoTareas(repo, vinculaciones, reloj, log)` y los métodos de la sección 3.
2. En `modulos_test.go`, después de vinculaciones, la sección `// --- tareas ---` con `repoTareasFalso`, `lectorVinculacionesFalso`, `entornoTareas`, `nuevaTarea` y las cinco pruebas.
3. `make pruebas lint`
4. Commit: `git add . && git commit -m "Paso 36: servicio de tareas"`

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-06.1, RF-06.2, RF-06.6 | `Crear` en una vinculación propia; varias por semana. |
| RF-06.7, RF-06.8 | `Actualizar`, `Eliminar`. |
| RF-08.1 a RF-08.5 | Semana normalizada, pasadas permitidas, futuras no, tardanza e inmutabilidad. |
| RN-09, RN-21, RN-26, RN-30, RN-32 a RN-34 | Ver tabla de la sección 2. |
| RF-10.5, RF-11.2, RF-12 | `Listar` y `Obtener` con visibilidad. |
