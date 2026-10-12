# Paso 37 · Endpoints de tareas

> Rama `paso-a-paso` · paso 37 de 65 · anterior: **Paso 36 · Servicio de tareas** · siguiente: **Paso 38 · Almacenamiento local de archivos**

---

## 1. Qué se hizo en este paso

El registro semanal de tareas queda disponible por HTTP (con JSON; los archivos adjuntos llegan en el paso 40):

| Método | Ruta | Rol | Descripción |
|---|---|---|---|
| POST | `/api/v1/tareas` | monitor o asistente | Registra una tarea. `semana` (cualquier fecha de la semana) es opcional: sin ella se usa la semana en curso. |
| GET | `/api/v1/tareas` | todos | Lista las tareas visibles. Filtros: `?semana=`, `?vinculacion_id=`, `?espacio_id=`, `?usuario_id=`. |
| GET | `/api/v1/tareas/{id}` | quien pueda verla | Detalle. |
| PUT | `/api/v1/tareas/{id}` | monitor o asistente (dueño) | Reemplaza los datos editables mientras la semana está activa. |
| DELETE | `/api/v1/tareas/{id}` | monitor o asistente (dueño) | Elimina la tarea (204). |

| Archivo | Cambio |
|---|---|
| `backend/internal/handler/tareas.go` | **Nuevo.** Módulo de tareas. |
| `backend/internal/handler/respuestas.go` | **Modificado.** `consultaFecha` y `consultaSemana`. |
| `backend/internal/handler/router.go` | **Modificado.** Servicio `Tareas` y su módulo. |
| `backend/internal/handler/router_test.go` | **Modificado.** `/api/v1/tareas` exige token; un profesor no registra tareas (403). |
| `backend/internal/handler/modulos_test.go` | **Modificado.** Servicio falso de tareas y cuatro pruebas. |
| `backend/cmd/api/main.go` | **Modificado.** Repositorio y servicio de tareas. |

```bash
curl -s -X POST localhost:8080/api/v1/tareas -H "Authorization: Bearer $TOKEN_MONITOR" -H 'Content-Type: application/json' \
  -d '{"vinculacion_id":"<id>","titulo":"Calificar taller 2","estado":"en_desarrollo","horas_invertidas":3}'
# {"id":"...","vinculacion_id":"...","usuario":{...},"espacio":{...},"rol":"monitor",
#  "semana_inicio":"2026-10-05","semana_fin":"2026-10-11","titulo":"Calificar taller 2",
#  "estado":"en_desarrollo","horas_invertidas":3,"observaciones":"","tardia":false,"modificable":true,...}
```

---

## 2. Por qué

- **RF-06 a RF-08** quedan disponibles: registrar, consultar, editar y eliminar tareas con las reglas de semana activa, tardanza e inmutabilidad.
- **Solo monitores y asistentes registran tareas** (`dominio.RolesOperativos`, paso 11). El profesor las **consulta** (RF-10.5).
- **`semana_inicio`, `semana_fin`, `tardia` y `modificable` en cada respuesta:** el cliente sabe a qué semana pertenece la tarea, si fue un reporte tardío y si todavía puede editarla, sin calcular nada.

---

## 3. Explicación detallada

### 3.1 Rutas

```go
func (h *HandlerTareas) Registrar(g *gin.RouterGroup) {
	operativos := RequiereRol(dominio.RolesOperativos...)
	r := g.Group("/tareas")
	r.GET("", h.Listar)
	r.GET("/:id", h.Obtener)
	r.POST("", operativos, h.Crear)
	r.PUT("/:id", operativos, h.Actualizar)
	r.DELETE("/:id", operativos, h.Eliminar)
}
```
- `dominio.RolesOperativos...` **desempaca** la lista `[monitor, asistente_graduado]` como argumentos de `RequiereRol`.
- El middleware se guarda en una variable (`operativos`) para reutilizarlo en varias rutas.

### 3.2 Formato de salida

```go
type tareaJSON struct {
	ID              string              `json:"id"`
	VinculacionID   string              `json:"vinculacion_id"`
	Usuario         personaRef          `json:"usuario"`
	Espacio         espacioRef          `json:"espacio"`
	Rol             dominio.Rol         `json:"rol"`
	SemanaInicio    string              `json:"semana_inicio"`
	SemanaFin       string              `json:"semana_fin"`
	...
	Tardia          bool                `json:"tardia"`
	Modificable     bool                `json:"modificable"`
	...
}

func (h *HandlerTareas) aTareaJSON(actor auth.Actor, t dominio.Tarea) tareaJSON {
	return tareaJSON{
		...
		SemanaInicio: fecha(t.SemanaInicio), SemanaFin: fecha(dominio.FinSemana(t.SemanaInicio)),
		...
		Modificable: h.servicio.Modificable(actor, t),
		...
	}
}
```
- `aTareaJSON` es un **método** del handler (no una función suelta como en otros módulos) porque necesita el servicio para calcular `modificable` **según quién pregunta**: la misma tarea es modificable para su dueña y no para el profesor.

### 3.3 Peticiones

```go
// peticionTarea crea una tarea. semana es cualquier fecha (AAAA-MM-DD) de la
// semana reportada; si se omite, se usa la semana en curso.
type peticionTarea struct {
	VinculacionID   string              `json:"vinculacion_id"`
	Semana          string              `json:"semana"`
	Titulo          string              `json:"titulo"`
	Descripcion     string              `json:"descripcion"`
	Estado          dominio.EstadoTarea `json:"estado"`
	HorasInvertidas int                 `json:"horas_invertidas"`
	Observaciones   string              `json:"observaciones"`
}

type peticionActualizarTarea struct { ... los cinco campos editables ... }

func (p peticionActualizarTarea) datos() dominio.DatosTarea { ... }
```
- `HorasInvertidas int`: `"horas_invertidas": 2.5` → **400** "el campo \"horas_invertidas\" tiene un tipo inválido (se esperaba int)" (RN-24).
- La actualización **no** tiene `semana` ni `vinculacion_id`: no se pueden cambiar (RN-26).
- `datos()` convierte la petición en `dominio.DatosTarea`. `Crear` también la reutiliza para los campos comunes.

```go
	var semana time.Time // vacía = semana en curso, la decide el servicio
	if p.Semana != "" {
		if semana, err = parsearFecha("semana", p.Semana); err != nil { ... }
	}
```
- Si no se envía la semana, se pasa el **valor cero** al servicio (paso 36).

### 3.4 Listado con semana normalizada

```go
func consultaFecha(c *gin.Context, nombre string) (*time.Time, error) { ... }

func consultaSemana(c *gin.Context, nombre string) (*time.Time, error) {
	f, err := consultaFecha(c, nombre)
	if f == nil || err != nil {
		return nil, err
	}
	semana := dominio.InicioSemana(*f)
	return &semana, nil
}
```
- `?semana=2026-10-08` (un jueves) se convierte en el lunes `2026-10-05`. El cliente puede mandar cualquier día.
- Devuelven **punteros**: `nil` = filtro no indicado.

```go
	f := dominio.FiltroTareas{SemanaInicio: semana, Paginacion: consultaPaginacion(c)}
	for nombre, destino := range map[string]*string{"vinculacion_id": &f.VinculacionID, "espacio_id": &f.EspacioID, "usuario_id": &f.UsuarioID} {
		if *destino, err = consultaUUID(c, nombre); err != nil { ... }
	}
```
- Un **mapa de punteros** para leer tres filtros iguales sin repetir el mismo bloque tres veces: para cada nombre de parámetro, se escribe su valor validado en el campo correspondiente del filtro.

### 3.5 Eliminar

```go
	c.Status(http.StatusNoContent)
```
- **204 No Content**: se eliminó y no hay nada que devolver.

### 3.6 Pruebas

- `TestRutasProtegidasExigenToken` incluye `/api/v1/tareas`; `TestControlDeAccesoPorRol`, que un profesor no registra tareas.
- En `modulos_test.go`:

| Prueba | Qué verifica |
|---|---|
| `TestCrearTareaConJSON` | Los datos llegan al servicio (vinculación, semana del 07/10, horas, estado) y la respuesta trae `semana_inicio` 05/10, `semana_fin` 11/10 y `modificable`. |
| `TestCrearTareaSinSemanaUsaLaSemanaEnCurso` | Sin semana, el servicio recibe el valor cero. |
| `TestCrearTareaRechazaHorasNoEnterasYCamposDesconocidos` | Horas `2.5`, campo desconocido, vinculación que no es UUID y fecha `07/10/2026` → 400. |
| `TestListarActualizarYEliminarTareas` | `?semana=2026-10-08` llega como el lunes 05/10, con la vinculación y el límite; la actualización llega al servicio; eliminar responde 204. |

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| `lista...` en una llamada | Pasa los elementos de una lista como argumentos separados. |
| Método de conversión | `h.aTareaJSON(actor, t)` usa el servicio para un campo que depende del actor. |
| Mapa de punteros | Leer varios filtros con el mismo código. |
| 204 No Content | Éxito sin cuerpo de respuesta. |

---

## 5. Cómo comprobarlo

```bash
make pruebas
docker compose up --build -d --wait
```

Con un profesor, Laura vinculada como monitora (paso 33) y su token:
1. Crear una tarea sin `semana` → `semana_inicio` = lunes de esta semana, `tardia: false`, `modificable: true`.
2. Crear otra con `"semana":"<una fecha de la semana pasada>"` → `tardia: true`, `modificable: false`.
3. `PUT` sobre la tardía → **422** "los reportes tardíos no pueden modificarse ni eliminarse".
4. Con el token del profesor: `POST /api/v1/tareas` → **403**; `GET /api/v1/tareas?usuario_id=<laura>` → las 2 tareas.
5. `"horas_invertidas": 2.5` → **400**; una semana futura → **400**.
6. `DELETE /api/v1/tareas/<la de esta semana>` con el token de Laura → **204**.

---

## 6. Pasos para lograr esta versión desde el paso 36

1. En `respuestas.go`: `consultaFecha` y `consultaSemana`.
2. Crear `backend/internal/handler/tareas.go` con `ServicioTareas` (sin adjuntos), `HandlerTareas`, `Registrar`, `tareaJSON`, `aTareaJSON`, las peticiones y los cinco endpoints.
3. En `router.go`: campo `Tareas` y `&HandlerTareas{servicio: d.Tareas, log: d.Log}`.
4. En `router_test.go`: `/api/v1/tareas` en la lista de rutas protegidas (y el token inválido sobre esa ruta) y el caso del profesor.
5. En `handler/modulos_test.go`: `servicioTareasFalso`, `tareaFalsa`, `entornoTareas` y las cuatro pruebas.
6. En `main.go`: `tareas = repositorio.NuevoTareas(db)` y `Tareas: servicio.NuevoTareas(tareas, vinculaciones, reloj, log)`.
7. `make pruebas lint` y commit: `git add . && git commit -m "Paso 37: endpoints de tareas"`

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-06.1 a RF-06.3, RF-06.5, RF-06.6 | `POST /api/v1/tareas` con datos validados y horas enteras. |
| RF-06.7 / RF-06.8 | `PUT` y `DELETE` sobre tareas propias y modificables. |
| RF-08.1 a RF-08.5 | Semana normalizada, pasadas permitidas (tardías), futuras rechazadas, tardías inmutables. |
| RF-10.5 / RF-11.2 / RF-12 | `GET /api/v1/tareas` según quién consulta. |
