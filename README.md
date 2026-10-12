# Paso 24 · Endpoints de períodos

> Rama `paso-a-paso` · paso 24 de 65 · anterior: **Paso 23 · Servicio de períodos** · siguiente: **Paso 25 · Dominio: cursos y proyectos**

---

## 1. Qué se hizo en este paso

Los períodos académicos se gestionan por HTTP:

| Método | Ruta | Rol | Descripción |
|---|---|---|---|
| POST | `/api/v1/periodos` | administrador | Crea un período. La ventana de creación queda **habilitada** salvo que se envíe `"creacion_habilitada": false`. |
| GET | `/api/v1/periodos` | todos | Lista los períodos (filtro `?estado=activo|cerrado`). |
| GET | `/api/v1/periodos/{id}` | todos | Detalle de un período. |
| PATCH | `/api/v1/periodos/{id}` | administrador | Cambia `estado` y/o `creacion_habilitada`. |

| Archivo | Cambio |
|---|---|
| `backend/internal/handler/periodos.go` | **Nuevo.** Módulo de períodos. |
| `backend/internal/handler/respuestas.go` | **Modificado.** `parsearFecha` (entrada `AAAA-MM-DD`) y `fecha` (salida `AAAA-MM-DD`). |
| `backend/internal/handler/router.go` | **Modificado.** Servicio `Periodos` y su módulo. |
| `backend/internal/handler/router_test.go` | **Modificado.** Un profesor no puede crear períodos (403). |
| `backend/internal/handler/modulos_test.go` | **Modificado.** Servicio falso de períodos y `TestPeriodos`. |
| `backend/cmd/api/main.go` | **Modificado.** Repositorio y servicio de períodos. |

```bash
curl -X POST localhost:8080/api/v1/periodos -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"codigo":"2026-20","fecha_inicio":"2026-08-01","fecha_cierre":"2026-12-15"}'
# {"id":"7665...","codigo":"2026-20","fecha_inicio":"2026-08-01","fecha_cierre":"2026-12-15",
#  "estado":"activo","creacion_habilitada":true,"creado_en":"..."}
```

---

## 2. Por qué

- **RF-03.2** queda disponible: el administrador define los períodos y abre o cierra la ventana de creación (RN-07); los profesores los consultan.
- **`PATCH` y no `PUT`:** `PATCH` es el verbo HTTP para **actualizaciones parciales** (se envía solo lo que cambia). Encaja con los punteros opcionales del servicio (paso 23).
- **Rutas con roles distintos dentro del mismo grupo:** consultar es para todos, modificar solo para el administrador.

---

## 3. Explicación detallada

### 3.1 Registro de rutas con permisos por ruta

```go
func (h *HandlerPeriodos) Registrar(g *gin.RouterGroup) {
	r := g.Group("/periodos")
	r.GET("", h.Listar)
	r.GET("/:id", h.Obtener)
	r.POST("", RequiereRol(dominio.RolAdministrador), h.Crear)
	r.PATCH("/:id", RequiereRol(dominio.RolAdministrador), h.Actualizar)
}
```
- A diferencia de usuarios (todo el grupo protegido por rol), aquí `RequiereRol` se pasa **por ruta**: Gin ejecuta los handlers en orden, primero el middleware y después el endpoint.

### 3.2 Formatos

```go
type periodoJSON struct {
	ID                 string                `json:"id"`
	Codigo             string                `json:"codigo"`
	FechaInicio        string                `json:"fecha_inicio"`
	FechaCierre        string                `json:"fecha_cierre"`
	Estado             dominio.EstadoPeriodo `json:"estado"`
	CreacionHabilitada bool                  `json:"creacion_habilitada"`
	CreadoEn           time.Time             `json:"creado_en"`
}
```
- Las fechas de calendario salen como **texto `AAAA-MM-DD`** (`"2026-08-01"`), no como fecha y hora (`2026-08-01T00:00:00Z`). Así el cliente no tiene que adivinar la zona horaria de un día.
- `creado_en` sí es un instante, por eso conserva la fecha y hora completas.

```go
type peticionPeriodo struct {
	Codigo             string `json:"codigo"`
	FechaInicio        string `json:"fecha_inicio"`
	FechaCierre        string `json:"fecha_cierre"`
	CreacionHabilitada *bool  `json:"creacion_habilitada"`
}
```
- Las fechas llegan como texto y se validan con `parsearFecha`.
- `CreacionHabilitada *bool`: un puntero para saber si el campo **vino** en el JSON. En `Crear`:
  ```go
  habilitada := p.CreacionHabilitada == nil || *p.CreacionHabilitada
  ```
  Si no se envía, la ventana queda **abierta** (lo habitual al crear un período al inicio del semestre).

```go
type peticionCambiosPeriodo struct {
	Estado             *dominio.EstadoPeriodo `json:"estado"`
	CreacionHabilitada *bool                  `json:"creacion_habilitada"`
}
```
- Para el `PATCH`, ambos son opcionales y se pasan tal cual al servicio (`nil` = sin cambio).

### 3.3 `respuestas.go`: fechas

```go
func parsearFecha(campo, texto string) (time.Time, error) {
	f, err := dominio.ParsearFecha(texto)
	if err != nil {
		return f, apperr.Validacion("%s debe ser una fecha con formato AAAA-MM-DD", campo)
	}
	return f, nil
}

func fecha(t time.Time) string { return t.Format(dominio.FormatoFecha) }
```
- `parsearFecha` usa el dominio (paso 20) y produce un error que **nombra el campo**: "fecha_inicio debe ser una fecha con formato AAAA-MM-DD".
- `fecha` hace lo inverso para las respuestas.

### 3.4 `main.go`

```go
	var (
		usuarios = repositorio.NuevoUsuarios(db)
		periodos = repositorio.NuevoPeriodos(db)
	)
```
- Un bloque `var (...)` agrupa los repositorios; crecerá con cada módulo.

### 3.5 Pruebas

- `TestControlDeAccesoPorRol` agrega: un profesor **no** puede crear períodos (403).
- `TestPeriodos` (handler), con un servicio falso que guarda lo recibido:
  - crear sin `creacion_habilitada` → el servicio recibe `true` y la respuesta trae `"fecha_inicio":"2026-08-01"`;
  - una fecha `1/8/2026` → 400;
  - `PATCH` con `{"creacion_habilitada": false}` → el servicio recibe ese cambio y `Estado` en `nil`;
  - un monitor puede **listar** períodos (200).

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| `PATCH` | Actualización parcial: solo los campos enviados. |
| Middleware por ruta | `r.POST("", RequiereRol(...), h.Crear)`. |
| Fechas como texto | Los días de calendario viajan como `AAAA-MM-DD`. |
| `nil \|\| *p` | Valor por defecto cuando un campo opcional no llegó. |

---

## 5. Cómo comprobarlo

```bash
make pruebas
docker compose up --build -d --wait
# TOKEN = token del administrador

curl -s -X POST localhost:8080/api/v1/periodos -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"codigo":"2026-20","fecha_inicio":"2026-08-01","fecha_cierre":"2026-12-15"}'      # 201
# repetir → 409 "ya existe el período 2026-20"
# "fecha_inicio":"1/8/2026" → 400 "fecha_inicio debe ser una fecha con formato AAAA-MM-DD"

curl -s -X PATCH localhost:8080/api/v1/periodos/<id> -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"estado":"cerrado"}'
# "estado":"cerrado","creacion_habilitada":false   ← cerrar también cierra la ventana

curl -s "localhost:8080/api/v1/periodos?estado=activo" -H "Authorization: Bearer $TOKEN"     # []
```

---

## 6. Pasos para lograr esta versión desde el paso 23

1. En `respuestas.go`: `parsearFecha` y `fecha`.
2. Crear `backend/internal/handler/periodos.go` (interfaz, handler, `Registrar`, `periodoJSON`, `aPeriodoJSON`, peticiones y los cuatro endpoints).
3. En `router.go`: campo `Periodos ServicioPeriodos` y `&HandlerPeriodos{...}` en la lista de módulos.
4. En `router_test.go`: variable `profesor` y el caso `POST /api/v1/periodos` en `TestControlDeAccesoPorRol`.
5. En `handler/modulos_test.go`: `servicioPeriodosFalso` y `TestPeriodos`.
6. En `main.go`: bloque `var (...)` con `periodos = repositorio.NuevoPeriodos(db)` y `Periodos: servicio.NuevoPeriodos(periodos, log)`.
7. `make pruebas lint` y commit: `git add . && git commit -m "Paso 24: endpoints de períodos"`

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-03.2 Períodos académicos | CRUD de períodos con código, fechas y estado. |
| RN-07 Ventana de creación | `creacion_habilitada` modificable por el administrador. |
| RF-01.2 Control de acceso | Lectura para todos; escritura solo administrador (probado). |
