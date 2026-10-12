# Paso 29 · Endpoints de espacios

> Rama `paso-a-paso` · paso 29 de 65 · anterior: **Paso 28 · Servicio de espacios** · siguiente: **Paso 30 · Dominio: vinculaciones y reglas de horas**

---

## 1. Qué se hizo en este paso

Cursos y proyectos se gestionan por HTTP:

| Método | Ruta | Rol | Descripción |
|---|---|---|---|
| POST | `/api/v1/espacios` | profesor | Crea un curso o proyecto en un período habilitado. |
| GET | `/api/v1/espacios` | todos | Lista los espacios visibles. Filtros: `?periodo_id=`, `?estado=`, `?tipo=`. |
| GET | `/api/v1/espacios/{id}` | todos (si lo puede ver) | Detalle; los cerrados traen `"solo_lectura": true`. |
| POST | `/api/v1/espacios/{id}/cerrar` | profesor responsable o administrador | Cierra el espacio. |

| Archivo | Cambio |
|---|---|
| `backend/internal/handler/espacios.go` | **Nuevo.** Módulo de espacios. |
| `backend/internal/handler/formatos.go` | **Modificado.** `personaRef`: referencia corta a una persona (id, nombre y correo opcional). |
| `backend/internal/handler/respuestas.go` | **Modificado.** `consultaUUID` y `validarUUIDOpcional`. |
| `backend/internal/handler/router.go` | **Modificado.** Servicio `Espacios` y su módulo. |
| `backend/internal/handler/router_test.go` | **Modificado.** Constante `uuidB` y caso "un monitor no crea espacios". |
| `backend/internal/handler/modulos_test.go` | **Modificado.** Servicio falso de espacios y `TestEspacios`. |
| `backend/cmd/api/main.go` | **Modificado.** Repositorio de espacios, el **reloj** de la aplicación y el servicio de espacios. |

```bash
curl -s -X POST localhost:8080/api/v1/espacios -H "Authorization: Bearer $TOKEN_PROFESOR" -H 'Content-Type: application/json' \
  -d '{"tipo":"curso","nombre":"Desarrollo en la nube","periodo_id":"<id del período>","fecha_inicio":"2026-08-03","fecha_cierre":"2026-12-04"}'
# {"id":"372c...","tipo":"curso","nombre":"Desarrollo en la nube",
#  "periodo":{"id":"8e99...","codigo":"2026-20"},"profesor":{"id":"4ed2...","nombre":"Carolina Rojas"},
#  "fecha_inicio":"2026-08-03","fecha_cierre":"2026-12-04","observaciones":"","estado":"activo","solo_lectura":false,...}
```

---

## 2. Por qué

- **RF-03.1 a RF-03.7** quedan disponibles por la API.
- **`POST /espacios/{id}/cerrar` en lugar de `PATCH` con `estado`:** cerrar es una **acción** con reglas propias (solo el responsable, no se puede reabrir), no la edición de un campo cualquiera. Un endpoint de acción lo deja explícito.
- **`solo_lectura`:** RF-03.6 pide que los espacios cerrados se puedan consultar "en modo de solo lectura". El campo le dice al cliente (el frontend de la Entrega 2) que no debe ofrecer acciones de edición.

---

## 3. Explicación detallada

### 3.1 Registro de rutas

```go
func (h *HandlerEspacios) Registrar(g *gin.RouterGroup) {
	r := g.Group("/espacios")
	r.GET("", h.Listar)
	r.GET("/:id", h.Obtener)
	r.POST("", RequiereRol(dominio.RolProfesor), h.Crear)
	r.POST("/:id/cerrar", RequiereRol(dominio.RolProfesor, dominio.RolAdministrador), h.Cerrar)
}
```
- La lectura no exige rol en el router: la visibilidad la decide el servicio (paso 28).

### 3.2 Formato de salida

```go
type espacioJSON struct {
	ID            string                `json:"id"`
	Tipo          dominio.TipoEspacio   `json:"tipo"`
	Nombre        string                `json:"nombre"`
	Periodo       periodoRef            `json:"periodo"`
	Profesor      personaRef            `json:"profesor"`
	FechaInicio   string                `json:"fecha_inicio"`
	FechaCierre   string                `json:"fecha_cierre"`
	Observaciones string                `json:"observaciones"`
	Estado        dominio.EstadoEspacio `json:"estado"`
	SoloLectura   bool                  `json:"solo_lectura"`
	CreadoEn      time.Time             `json:"creado_en"`
}

type periodoRef struct {
	ID     string `json:"id"`
	Codigo string `json:"codigo"`
}
```
- **Objetos anidados** para las referencias: `"periodo": {"id", "codigo"}` y `"profesor": {"id", "nombre"}`. El cliente obtiene lo que necesita para mostrar ("2026-20", "Carolina Rojas") y el ID por si quiere más detalle.
- `personaRef` va en `formatos.go` porque la reutilizarán vinculaciones, tareas y seguimiento:
  ```go
  type personaRef struct {
  	ID     string `json:"id"`
  	Nombre string `json:"nombre"`
  	Correo string `json:"correo,omitempty"`
  }
  ```
  `omitempty`: si el correo está vacío, el campo **no aparece** en el JSON.
- `SoloLectura: e.Estado == dominio.EspacioCerrado` se calcula al convertir.

### 3.3 Validar identificadores que llegan en el cuerpo o la consulta

```go
func consultaUUID(c *gin.Context, nombre string) (string, error) {
	return validarUUIDOpcional(nombre, c.Query(nombre))
}

func validarUUIDOpcional(nombre, valor string) (string, error) {
	if valor == "" {
		return "", nil
	}
	if _, err := uuid.Parse(valor); err != nil {
		return "", apperr.Validacion("%s debe ser un identificador válido", nombre)
	}
	return valor, nil
}
```
- `parametroUUID` (paso 19) valida parámetros de **ruta**, que son obligatorios. Estos validan IDs **opcionales**: en la consulta (`?periodo_id=`) o dentro del JSON (`"periodo_id"` en `Crear`). Vacío está bien; un texto que no es UUID → 400.

### 3.4 `main.go`

```go
	reloj := servicio.NuevoReloj(cfg.ZonaHoraria)
	...
		Espacios: servicio.NuevoEspacios(espacios, periodos, reloj, log),
```
- Se crea **el** reloj de la aplicación con `APP_ZONA_HORARIA` (paso 1). Todos los servicios que dependen de la fecha lo compartirán.
- El servicio de espacios recibe el repositorio de períodos como su `LectorPeriodos`.

### 3.5 Pruebas

- `TestControlDeAccesoPorRol`: un monitor no puede crear espacios (403).
- `TestEspacios` (handler):
  - crear un curso pasa al servicio el `periodo_id` y el tipo;
  - cerrar devuelve `"solo_lectura":true`;
  - un `apperr.NoEncontrado` del servicio se traduce en **404**.
- `uuidB` es un segundo UUID fijo para las pruebas.

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Endpoint de acción | `POST /recurso/{id}/accion` para operaciones con reglas propias. |
| Objetos anidados en JSON | Structs dentro de structs (`periodoRef`, `personaRef`). |
| `omitempty` | El campo se omite del JSON si tiene su valor cero. |
| Campos calculados | `solo_lectura` se deriva del estado al responder. |

---

## 5. Cómo comprobarlo

```bash
make pruebas
docker compose up --build -d --wait
```

1. Con el token del administrador: crear un usuario con rol `profesor` (paso 19) y un período (paso 24).
2. Iniciar sesión como el profesor y crear un curso (ejemplo de la sección 1).
3. `POST /api/v1/espacios/<id>/cerrar` → `"estado":"cerrado","solo_lectura":true`; repetir → **409**.
4. Con el administrador, `PATCH /api/v1/periodos/<id>` con `{"creacion_habilitada":false}`; crear otro curso → **422** "el período 2026-20 no está habilitado para crear cursos o proyectos".

---

## 6. Pasos para lograr esta versión desde el paso 28

1. En `formatos.go`: `personaRef`.
2. En `respuestas.go`: `consultaUUID` y `validarUUIDOpcional`.
3. Crear `backend/internal/handler/espacios.go` (interfaz, handler, `Registrar`, `espacioJSON`, `periodoRef`, `aEspacioJSON`, `peticionEspacio` y los cuatro endpoints).
4. En `router.go`: campo `Espacios` y `&HandlerEspacios{...}`.
5. En `router_test.go`: `uuidB` y el caso `POST /api/v1/espacios` con token de monitor.
6. En `handler/modulos_test.go`: `servicioEspaciosFalso` y `TestEspacios`.
7. En `main.go`: `espacios = repositorio.NuevoEspacios(db)`, `reloj := servicio.NuevoReloj(cfg.ZonaHoraria)` y `Espacios: servicio.NuevoEspacios(espacios, periodos, reloj, log)`.
8. `make pruebas lint` y commit: `git add . && git commit -m "Paso 29: endpoints de espacios"`

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-03.1 a RF-03.3 Crear con información mínima | `POST /api/v1/espacios`. |
| RF-03.4 / RF-11.1 Consultar sus espacios | `GET /api/v1/espacios` con visibilidad. |
| RF-03.5 Cerrar | `POST /api/v1/espacios/{id}/cerrar`. |
| RF-03.6 Consultar cerrados en solo lectura | Campo `solo_lectura`. |
