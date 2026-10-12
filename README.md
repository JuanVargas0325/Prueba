# Paso 33 · Endpoints de vinculaciones

> Rama `paso-a-paso` · paso 33 de 65 · anterior: **Paso 32 · Servicio de vinculaciones** · siguiente: **Paso 34 · Dominio: tareas semanales**

---

## 1. Qué se hizo en este paso

Las vinculaciones se gestionan por HTTP:

| Método | Ruta | Rol | Descripción |
|---|---|---|---|
| POST | `/api/v1/vinculaciones` | profesor | Vincula a una persona (por `usuario_id` o `correo`) a un curso o proyecto propio, con rol y horas. Aplica las reglas de horas. |
| GET | `/api/v1/vinculaciones` | todos | Lista las vinculaciones visibles. Filtros: `?espacio_id=`, `?usuario_id=`, `?solo_activas=true`. Para un monitor es el **listado unificado** de sus cursos y proyectos (RF-11.1). |
| GET | `/api/v1/vinculaciones/{id}` | quien pueda verla | Detalle. |
| PUT | `/api/v1/vinculaciones/{id}` | administrador | Rectifica rol y/o horas con un motivo obligatorio. |
| GET | `/api/v1/vinculaciones/{id}/rectificaciones` | profesor o administrador | Historial de correcciones. |

| Archivo | Cambio |
|---|---|
| `backend/internal/handler/vinculaciones.go` | **Nuevo.** Módulo de vinculaciones. |
| `backend/internal/handler/formatos.go` | **Modificado.** `espacioRef` y `vinculacionJSON` (compartidos con tareas y seguimiento). |
| `backend/internal/handler/router.go` | **Modificado.** Servicio `Vinculaciones` y su módulo. |
| `backend/internal/handler/router_test.go` | **Modificado.** Un monitor no vincula; un profesor no rectifica. |
| `backend/internal/handler/modulos_test.go` | **Modificado.** Servicio falso y `TestVinculaciones`. |
| `backend/cmd/api/main.go` | **Modificado.** Repositorio y servicio de vinculaciones. |

Ejemplo del enunciado por HTTP (Laura tiene 12 h de asistencia y una monitoría de 3 h):

```bash
curl -s -X POST localhost:8080/api/v1/vinculaciones -H "Authorization: Bearer $TOKEN_PROFESOR" -H 'Content-Type: application/json' \
  -d '{"espacio_id":"<curso>","usuario_id":"<laura>","rol":"monitor","horas_semanales":3}'
# 422 {"error":"regla_negocio","mensaje":"con 12 horas como asistente graduado, las horas de monitoría
#      no pueden superar 5 (40 % redondeado hacia arriba); quedaría con 6"}
```

---

## 2. Por qué

- **RF-04 y RF-05** quedan disponibles por la API, con las reglas de horas aplicadas de forma atómica.
- **El profesor no tiene endpoint para cambiar horas** (RF-04.4): solo existe `PUT` para el administrador (RF-04.5).
- **422 frente a 409:** violar una regla de horas es una **regla de negocio** (422); vincular dos veces lo mismo es un **conflicto** (409). El cliente puede distinguirlos.

---

## 3. Explicación detallada

### 3.1 Rutas

```go
func (h *HandlerVinculaciones) Registrar(g *gin.RouterGroup) {
	r := g.Group("/vinculaciones")
	r.GET("", h.Listar)
	r.GET("/:id", h.Obtener)
	r.POST("", RequiereRol(dominio.RolProfesor), h.Crear)
	r.PUT("/:id", RequiereRol(dominio.RolAdministrador), h.Rectificar)
	r.GET("/:id/rectificaciones", RequiereRol(dominio.RolProfesor, dominio.RolAdministrador), h.ListarRectificaciones)
}
```

### 3.2 Peticiones

```go
type peticionVinculacion struct {
	EspacioID      string      `json:"espacio_id"`
	UsuarioID      string      `json:"usuario_id"`
	Correo         string      `json:"correo"`
	Rol            dominio.Rol `json:"rol"`
	HorasSemanales int         `json:"horas_semanales"`
}

type peticionRectificacion struct {
	Rol            *dominio.Rol `json:"rol"`
	HorasSemanales *int         `json:"horas_semanales"`
	Motivo         string       `json:"motivo"`
}
```
- `HorasSemanales int`: un valor decimal (`4.5`) se rechaza automáticamente con 400 (paso 17): las horas son enteras.
- En la rectificación, rol y horas son **punteros** (opcionales), igual que en el servicio (paso 32).
- `Crear` valida con `validarUUIDOpcional` que `espacio_id` y `usuario_id` sean UUID antes de llamar al servicio.

### 3.3 Formatos compartidos (`formatos.go`)

```go
type espacioRef struct {
	ID          string                `json:"id"`
	Nombre      string                `json:"nombre"`
	Tipo        dominio.TipoEspacio   `json:"tipo"`
	Estado      dominio.EstadoEspacio `json:"estado,omitempty"`
	FechaInicio string                `json:"fecha_inicio,omitempty"`
	FechaCierre string                `json:"fecha_cierre,omitempty"`
}

type vinculacionJSON struct {
	ID             string      `json:"id"`
	Usuario        personaRef  `json:"usuario"`
	Espacio        espacioRef  `json:"espacio"`
	Profesor       personaRef  `json:"profesor"`
	Rol            dominio.Rol `json:"rol"`
	HorasSemanales int         `json:"horas_semanales"`
	CreadoEn       time.Time   `json:"creado_en"`
	ActualizadoEn  time.Time   `json:"actualizado_en"`
}
```
- Una vinculación se muestra con **a quién**, **dónde** y **con qué profesor**, en objetos anidados. Es justo lo que necesita la vista del monitor (RF-11.1: sus cursos y proyectos con rol y horas) y la del profesor (RF-10.2).
- `espacioRef` es una referencia **resumida** a un espacio; con `omitempty` sirve tanto para respuestas cortas (solo id, nombre y tipo) como completas.

### 3.4 Filtros del listado

```go
	vinculaciones, err := h.servicio.Listar(c.Request.Context(), ActorDe(c), dominio.FiltroVinculaciones{
		EspacioID: espacioID, UsuarioID: usuarioID,
		SoloEspaciosActivos: c.Query("solo_activas") == "true",
		Paginacion:          consultaPaginacion(c),
	})
```
- `?solo_activas=true` excluye las vinculaciones de espacios cerrados (útil para mostrar solo lo vigente).
- `VisiblePara` no viene del cliente: lo fija el servicio (paso 32).

### 3.5 Pruebas

- `TestControlDeAccesoPorRol` agrega: un monitor no puede vincular; un profesor no puede rectificar (`PUT /vinculaciones/{id}`).
- `TestVinculaciones` (handler):
  - vincular por correo pasa correo, rol y horas al servicio (201);
  - cuando el servicio responde una **regla de negocio**, la API responde **422**;
  - la rectificación lleva solo las horas (el rol queda en `nil`).

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Referencias anidadas reutilizables | `personaRef`, `espacioRef` en varias respuestas. |
| Parámetros booleanos en la consulta | `c.Query("solo_activas") == "true"`. |
| 409 vs 422 | Conflicto de existencia frente a regla de negocio. |

---

## 5. Cómo comprobarlo

```bash
make pruebas
docker compose up --build -d --wait
```

Con un profesor, una persona con roles `monitor` y `asistente_graduado` (Laura), un período y tres espacios del profesor:

1. Vincular a Laura como **asistente** con 12 h en el espacio 2 (por correo) → 201.
2. Vincularla como **monitor** con 3 h en el espacio 1 → 201.
3. Vincularla como **monitor** con 3 h en el espacio 3 → **422** (12 h de asistencia permiten 5 h de monitoría; quedaría con 6).
4. Repetir la vinculación del paso 2 → **409** "la persona ya está vinculada como monitor en este curso o proyecto".
5. Con el profesor, `PUT /api/v1/vinculaciones/<id>` → **403**.
6. Con el administrador, `PUT /api/v1/vinculaciones/<id>` con `{"horas_semanales":10,"motivo":"Error de digitación"}` → 200; `GET .../rectificaciones` muestra 12 → 10 con el motivo.
7. Con el token de Laura, `GET /api/v1/vinculaciones` → sus 2 vinculaciones.

---

## 6. Pasos para lograr esta versión desde el paso 32

1. En `formatos.go`: `espacioRef`, `vinculacionJSON` y `aVinculacionJSON`.
2. Crear `backend/internal/handler/vinculaciones.go` (interfaz, handler, `Registrar`, peticiones, `rectificacionJSON` y los cinco endpoints).
3. En `router.go`: campo `Vinculaciones` y `&HandlerVinculaciones{...}`.
4. En `router_test.go`: casos `POST /api/v1/vinculaciones` (monitor) y `PUT /api/v1/vinculaciones/{uuidA}` (profesor).
5. En `handler/modulos_test.go`: `servicioVinculacionesFalso` y `TestVinculaciones`.
6. En `main.go`: `vinculaciones = repositorio.NuevoVinculaciones(db)` y `Vinculaciones: servicio.NuevoVinculaciones(vinculaciones, usuarios, espacios, log)`.
7. `make pruebas lint` y commit: `git add . && git commit -m "Paso 33: endpoints de vinculaciones"`

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-04.1 a RF-04.3 Vincular con rol y horas | `POST /api/v1/vinculaciones`. |
| RF-04.4 Horas no modificables por el profesor | Sin endpoint de edición para el profesor (403 en `PUT`). |
| RF-04.5 Rectificación | `PUT /api/v1/vinculaciones/{id}` y su historial. |
| RF-05.1 a RF-05.6 Reglas de horas | Respuesta 422 con el detalle de la regla violada. |
| RF-10.2 / RF-11.1 Listados | `GET /api/v1/vinculaciones` según quién consulta. |
