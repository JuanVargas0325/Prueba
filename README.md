# Paso 44 · Endpoints de seguimiento

> Rama `paso-a-paso` · paso 44 de 65 · anterior: **Paso 43 · Servicio de seguimiento** · siguiente: **Paso 45 · Dominio: lotes y reportes**

---

## 1. Qué se hizo en este paso

Las vistas de seguimiento quedan disponibles por HTTP:

| Método | Ruta | Rol | Descripción |
|---|---|---|---|
| GET | `/api/v1/seguimiento/personas` | profesor o administrador | Vista principal: personas a cargo, sus vinculaciones y la semana actual y la anterior. Filtros: `?espacio_id=` y, para el administrador, `?profesor_id=`. |
| GET | `/api/v1/seguimiento/personas/{id}/semanas` | profesor de la persona, administrador o la propia persona | Detalle semana a semana con tareas y estado. Rango opcional `?desde=&hasta=`. |
| GET | `/api/v1/seguimiento/personas/{id}/consolidado` | ídem | Resumen acumulado por vinculación. |
| GET | `/api/v1/seguimiento/mi-historial` | cualquier usuario | El historial semanal del usuario autenticado. |

| Archivo | Cambio |
|---|---|
| `backend/internal/handler/seguimiento.go` | **Nuevo.** Módulo de seguimiento con sus formatos JSON. |
| `backend/internal/handler/formatos.go` | **Modificado.** `tareaResumenJSON`: forma corta de una tarea dentro de otras respuestas. |
| `backend/internal/handler/router.go` | **Modificado.** Servicio `Seguimiento` y su módulo. |
| `backend/internal/handler/router_test.go` | **Modificado.** La ruta exige token y un monitor no ve "personas a cargo". |
| `backend/internal/handler/modulos_test.go` | **Modificado.** Servicio falso y `TestSeguimiento`. |
| `backend/cmd/api/main.go` | **Modificado.** Servicio de seguimiento. |

Ejemplo de la vista del profesor:

```json
[{"usuario":{"id":"75b5...","nombre":"Laura Martínez","correo":"laura@uniandes.edu.co"},
  "horas_contratadas":4,
  "vinculaciones":[{"id":"...","espacio":{"id":"...","nombre":"Desarrollo en la nube","tipo":"curso","estado":"activo"},"rol":"monitor","horas_semanales":4}],
  "semana_actual":{"semana_inicio":"2026-10-05","semana_fin":"2026-10-11","estado":"en_curso","horas_contratadas":4,"horas_reportadas":3,"tareas":1},
  "semana_anterior":{"semana_inicio":"2026-09-28","semana_fin":"2026-10-04","estado":"sin_reporte","horas_contratadas":4,"horas_reportadas":0,"tareas":0}}]
```

---

## 2. Por qué

- **RF-10:** la pantalla principal del profesor debe mostrar, por persona, sus vinculaciones, lo que reportó y si dejó semanas sin reporte. Con esta respuesta un frontend puede pintar una tabla con una fila por persona y un indicador rojo para `sin_reporte`.
- **RF-11.3:** el monitor o asistente ve su propio historial con `mi-historial`, sin conocer su ID.
- **Visibilidad en el servicio:** las rutas de detalle no exigen rol en el router porque **la misma ruta** sirve a tres actores distintos (la persona, su profesor y el administrador). La decisión la toma el servicio (paso 43).

---

## 3. Explicación detallada

### 3.1 Rutas

```go
func (h *HandlerSeguimiento) Registrar(g *gin.RouterGroup) {
	r := g.Group("/seguimiento")
	r.GET("/personas", RequiereRol(dominio.RolProfesor, dominio.RolAdministrador), h.PersonasACargo)
	// La visibilidad por persona la decide el servicio: la propia persona, su
	// profesor (solo sus espacios) o el administrador.
	r.GET("/personas/:id/semanas", h.SemanasPersona)
	r.GET("/personas/:id/consolidado", h.Consolidado)
	r.GET("/mi-historial", h.MiHistorial)
}
```

### 3.2 Formatos

```go
type vinculacionResumenJSON struct {
	ID             string      `json:"id"`
	Espacio        espacioRef  `json:"espacio"`
	Rol            dominio.Rol `json:"rol"`
	HorasSemanales int         `json:"horas_semanales"`
}

type resumenSemanaJSON struct {
	SemanaInicio     string               `json:"semana_inicio"`
	SemanaFin        string               `json:"semana_fin"`
	Estado           dominio.EstadoSemana `json:"estado"`
	HorasContratadas int                  `json:"horas_contratadas"`
	HorasReportadas  int                  `json:"horas_reportadas"`
	Tareas           int                  `json:"tareas"`
}

func aResumenSemanaJSON(s *dominio.SemanaPersona) *resumenSemanaJSON {
	if s == nil {
		return nil
	}
	...
}
```
- Dentro de la vista de seguimiento la vinculación se muestra **resumida** (sin repetir la persona en cada una).
- Si la persona no estaba vinculada esa semana, el servicio devuelve `nil` y el JSON muestra `"semana_actual": null`.

```go
type semanaJSON struct {
	resumenSemanaJSON
	Vinculaciones []detalleVinculacionSemanaJSON `json:"vinculaciones"`
}
```
- **Embeber un struct en JSON:** `semanaJSON` incluye `resumenSemanaJSON` sin nombre de campo; al serializar, sus campos aparecen **al mismo nivel** (`semana_inicio`, `estado`, …, `vinculaciones`). Así se reutiliza el resumen y se le agrega el detalle.

```go
type tareaResumenJSON struct {
	ID              string              `json:"id"`
	Titulo          string              `json:"titulo"`
	Descripcion     string              `json:"descripcion"`
	Estado          dominio.EstadoTarea `json:"estado"`
	HorasInvertidas int                 `json:"horas_invertidas"`
	Observaciones   string              `json:"observaciones"`
	Tardia          bool                `json:"tardia"`
}
```
- En `formatos.go`: dentro de una semana, cada tarea se muestra en su forma corta (ya se sabe de qué vinculación y semana es).

```go
type consolidadoVinculacionJSON struct {
	...
	TareasPorEstado      map[dominio.EstadoTarea]int `json:"tareas_por_estado"`
	PromedioHorasSemana  float64                     `json:"promedio_horas_semana"`
}
```
- Un **mapa** se serializa como objeto JSON: `"tareas_por_estado": {"abierto": 2, "en_desarrollo": 0, "finalizado": 1}`.

### 3.3 `mi-historial` reutiliza el detalle

```go
func (h *HandlerSeguimiento) SemanasPersona(c *gin.Context) {
	id, err := parametroUUID(c, "id")
	...
	h.responderSemanas(c, id)
}

func (h *HandlerSeguimiento) MiHistorial(c *gin.Context) {
	h.responderSemanas(c, ActorDe(c).UsuarioID)
}
```
- Las dos rutas comparten la misma función; la única diferencia es **de quién**: el ID de la ruta o el del usuario autenticado.

### 3.4 Pruebas

- `TestRutasProtegidasExigenToken` incluye `/api/v1/seguimiento/personas`; `TestControlDeAccesoPorRol`, que un monitor no ve personas a cargo (403).
- `TestSeguimiento` (handler):
  - la vista de personas incluye `"estado":"sin_reporte"` en la semana anterior y `"semana_actual":null` cuando no hay semana actual;
  - `?desde=2026-09-01` llega al servicio y `hasta` queda en `nil`;
  - `mi-historial` consulta al usuario autenticado (`usuario-1`);
  - el consolidado responde 200.

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Struct embebido en JSON | Sus campos aparecen al mismo nivel que los del struct que lo contiene. |
| `null` en JSON | Un puntero `nil` se serializa como `null`. |
| Mapa → objeto JSON | `map[clave]valor` produce `{"clave": valor}`. |
| Una ruta, varios actores | Permisos decididos por el servicio según la relación con el dato. |

---

## 5. Cómo comprobarlo

```bash
make pruebas
docker compose up --build -d --wait
```

Con la profesora, Laura vinculada a uno de sus cursos y algunas tareas (pasos 33 y 37):

```bash
curl -s localhost:8080/api/v1/seguimiento/personas -H "Authorization: Bearer $TOKEN_PROFESORA"
curl -s "localhost:8080/api/v1/seguimiento/mi-historial?desde=2026-09-01" -H "Authorization: Bearer $TOKEN_LAURA"
curl -s localhost:8080/api/v1/seguimiento/personas/<id de Laura>/consolidado -H "Authorization: Bearer $TOKEN_PROFESORA"
curl -s -o /dev/null -w "%{http_code}" localhost:8080/api/v1/seguimiento/personas -H "Authorization: Bearer $TOKEN_LAURA"   # 403
```

Para ver semanas **sin reporte** en una prueba local, la vinculación debe existir desde semanas atrás (por la API se crea con la fecha de hoy). Se puede simular:

```bash
docker compose exec postgres psql -U seguimiento -d seguimiento -c "UPDATE vinculaciones SET creado_en = '2026-09-01'"
```

Los datos de demostración del paso 59 ya traen vinculaciones y tareas en varias semanas.

---

## 6. Pasos para lograr esta versión desde el paso 43

1. En `formatos.go`: `tareaResumenJSON` y `aTareaResumenJSON`.
2. Crear `backend/internal/handler/seguimiento.go` (interfaz, handler, `Registrar`, formatos y los cuatro endpoints con `responderSemanas`).
3. En `router.go`: campo `Seguimiento` y `&HandlerSeguimiento{...}`.
4. En `router_test.go`: `/api/v1/seguimiento/personas` en la lista de rutas protegidas y el caso del monitor.
5. En `handler/modulos_test.go`: `servicioSeguimientoFalso` y `TestSeguimiento`.
6. En `main.go`: `Seguimiento: servicio.NuevoSeguimiento(vinculaciones, tareas, usuarios, reloj)`.
7. `make pruebas lint` y commit: `git add . && git commit -m "Paso 44: endpoints de seguimiento"`

---

## 7. Preparación para las próximas entregas

- **El frontend de la Entrega 2** tiene aquí su pantalla principal: una llamada a `/seguimiento/personas` trae todo lo necesario para la tabla del profesor.
- **Escenario de carga E2:** esta es la consulta que se mide en las pruebas de carga (paso 64), la más costosa de la API después del inicio de sesión.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-10.1 / RF-10.2 Personas a cargo y sus vinculaciones | `GET /seguimiento/personas`. |
| RF-10.3 / RF-10.6 Detalle semanal con horas | `GET /seguimiento/personas/{id}/semanas`. |
| RF-10.4 Consolidado | `GET /seguimiento/personas/{id}/consolidado`. |
| RF-11.3 Historial propio | `GET /seguimiento/mi-historial`. |
| RF-09 Sin reporte | Estado `sin_reporte` en las semanas cerradas sin tareas. |
| RF-12 Visibilidad | Según la relación del actor con la persona. |
