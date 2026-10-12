# Paso 19 · Endpoints de usuarios

> Rama `paso-a-paso` · paso 19 de 65 · anterior: **Paso 18 · Servicio de gestión de usuarios** · siguiente: **Paso 20 · Dominio: fechas y semanas**

---

## 1. Qué se hizo en este paso

El administrador ya puede gestionar usuarios por HTTP:

| Método | Ruta | Rol | Descripción |
|---|---|---|---|
| POST | `/api/v1/usuarios` | administrador | Crea un usuario con sus roles (201 + cabecera `Location`). |
| GET | `/api/v1/usuarios` | administrador | Lista usuarios. Filtros: `?rol=`, `?busqueda=`, `?limite=`, `?desplazamiento=`. |
| GET | `/api/v1/usuarios/{id}` | administrador | Detalle de un usuario. |
| PUT | `/api/v1/usuarios/{id}/roles` | administrador | Reemplaza sus roles. |

| Archivo | Cambio |
|---|---|
| `backend/go.mod`, `go.sum` | Nueva dependencia `github.com/google/uuid v1.6.0` (validar identificadores). |
| `backend/internal/handler/usuarios.go` | **Nuevo.** Módulo de usuarios. |
| `backend/internal/handler/respuestas.go` | **Modificado.** `parametroUUID`, `consultaPaginacion` y la función genérica `convertir`. |
| `backend/internal/handler/router.go` | **Modificado.** Servicio `Usuarios` en `Dependencias` y el módulo en la lista. |
| `backend/internal/handler/router_test.go` | **Modificado.** El entorno guarda el gestor de tokens y emite tokens por rol (`e.token`); prueba de control de acceso por rol. |
| `backend/internal/handler/modulos_test.go` | **Nuevo.** Pruebas de los módulos; la primera, del módulo de usuarios. |
| `backend/cmd/api/main.go` | **Modificado.** Crea el servicio de usuarios. |

---

## 2. Por qué

- **RF-02.1 a RF-02.3 y RF-15.2** quedan accesibles para el administrador.
- **Validar los identificadores en la entrada:** si alguien pide `/usuarios/123`, la base de datos fallaría al convertir `123` a UUID. Es mejor responder **400** con un mensaje claro antes de llegar a la base de datos.
- **REST:** se usan los verbos y códigos estándar. `POST` crea y responde **201 Created** con la cabecera `Location` (dónde quedó el recurso); `PUT` reemplaza (los roles se envían completos).

---

## 3. Explicación detallada

### 3.1 `usuarios.go` (handler)

```go
type ServicioUsuarios interface {
	Crear(ctx context.Context, actor auth.Actor, n servicio.NuevoUsuario) (dominio.Usuario, error)
	Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroUsuarios) ([]dominio.Usuario, error)
	Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.Usuario, error)
	AsignarRoles(ctx context.Context, actor auth.Actor, id string, roles []dominio.Rol) (dominio.Usuario, error)
}

type HandlerUsuarios struct {
	servicio ServicioUsuarios
	log      *slog.Logger
}

func (h *HandlerUsuarios) Registrar(g *gin.RouterGroup) {
	r := g.Group("/usuarios", RequiereRol(dominio.RolAdministrador))
	r.POST("", h.Crear)
	r.GET("", h.Listar)
	r.GET("/:id", h.Obtener)
	r.PUT("/:id/roles", h.AsignarRoles)
}
```
- La misma anatomía del paso 17. Aquí **todo el grupo** exige el rol de administrador.
- `/:id` es un **parámetro de ruta**: en `/usuarios/6f1c...`, `id` vale `6f1c...`.

```go
type peticionUsuario struct {
	Nombre   string        `json:"nombre"`
	Correo   string        `json:"correo"`
	Password string        `json:"password"`
	Roles    []dominio.Rol `json:"roles"`
}
```
- `[]dominio.Rol` se lee directamente del JSON (`["monitor","asistente_graduado"]`), porque `Rol` está basado en `string`. Que cada rol sea válido lo verifica el servicio.

```go
func (h *HandlerUsuarios) Crear(c *gin.Context) {
	var p peticionUsuario
	if err := leerJSON(c, &p); err != nil { ... }
	u, err := h.servicio.Crear(c.Request.Context(), ActorDe(c), servicio.NuevoUsuario{...})
	if err != nil { ... }
	c.Header("Location", "/api/v1/usuarios/"+u.ID)
	c.JSON(http.StatusCreated, aUsuarioJSON(u))
}
```

```go
func (h *HandlerUsuarios) Listar(c *gin.Context) {
	usuarios, err := h.servicio.Listar(c.Request.Context(), ActorDe(c), dominio.FiltroUsuarios{
		Rol:        dominio.Rol(c.Query("rol")),
		Busqueda:   c.Query("busqueda"),
		Paginacion: consultaPaginacion(c),
	})
	...
	c.JSON(http.StatusOK, convertir(usuarios, aUsuarioJSON))
}
```
- `c.Query("rol")` lee un parámetro de la consulta (`?rol=monitor`); devuelve `""` si no viene.

### 3.2 `respuestas.go`: nuevas utilidades

```go
func parametroUUID(c *gin.Context, nombre string) (string, error) {
	valor := c.Param(nombre)
	if _, err := uuid.Parse(valor); err != nil {
		return "", apperr.Validacion("%s debe ser un identificador válido", nombre)
	}
	return valor, nil
}
```
- Lee un parámetro de ruta y verifica que sea un UUID.

```go
func consultaPaginacion(c *gin.Context) dominio.Paginacion {
	limite, _ := strconv.Atoi(c.Query("limite"))
	desplazamiento, _ := strconv.Atoi(c.Query("desplazamiento"))
	return dominio.Paginacion{Limite: max(limite, 0), Desplazamiento: max(desplazamiento, 0)}
}
```
- `strconv.Atoi` convierte texto a número. Si el texto no es un número, devuelve 0 (y se ignora el error con `_`).
- `max(x, 0)` (función incorporada desde Go 1.21) evita negativos. Así nadie puede enviar `limite=-1` y obtener el `SinPaginar` interno (paso 11). El máximo de 200 lo aplica el repositorio.

```go
func convertir[E, S any](entrada []E, f func(E) S) []S {
	salida := make([]S, 0, len(entrada))
	for _, e := range entrada {
		salida = append(salida, f(e))
	}
	return salida
}
```
- **Genérica con dos tipos**: convierte una lista de `E` (por ejemplo `dominio.Usuario`) en una lista de `S` (`usuarioJSON`) aplicando `f` a cada elemento. Se usará en todos los listados.
- `make([]S, 0, len(entrada))` crea una lista vacía con **capacidad** reservada: `append` no tendrá que redimensionarla.
- Siempre devuelve una lista (nunca `nil`), así el JSON es `[]` y no `null`.

### 3.3 `router.go`

```go
type Dependencias struct {
	Autenticacion ServicioAutenticacion
	Usuarios      ServicioUsuarios
	...
}
...
	modulos := []modulo{
		autenticacion,
		&HandlerUsuarios{servicio: d.Usuarios, log: d.Log},
	}
```
- Agregar un módulo = un campo en `Dependencias` + una línea en la lista. Ese será el patrón en cada paso de endpoints.

### 3.4 Pruebas

En `router_test.go`:
- `entorno` guarda el gestor de tokens y `e.token(t, dominio.RolMonitor)` emite un token para el usuario ficticio `usuario-1` con los roles pedidos.
- `uuidA` es un UUID fijo para las rutas de prueba.
- `TestRutasProtegidasExigenToken` incluye `/api/v1/usuarios`.
- **`TestControlDeAccesoPorRol`** (nueva): una tabla de casos *(método, ruta, token)* que deben responder **403**. Hoy: un monitor no puede listar usuarios. Cada módulo agregará sus casos.

En `modulos_test.go` (nuevo):
- `servicioUsuariosFalso`: servicio falso que guarda lo que recibió y devuelve un usuario **con un hash ficticio**.
- `TestCrearUsuarioNoExponeElHash`: la respuesta de crear usuario **no** contiene ni `password` ni el hash; el servicio recibió los roles y la contraseña; y asignar roles responde 200.

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Parámetros de ruta | `/:id` en Gin; se leen con `c.Param("id")`. |
| Parámetros de consulta | `?rol=x`; se leen con `c.Query("rol")`. |
| Genéricos con varios tipos | `func convertir[E, S any](...)`. |
| `make` con capacidad | `make([]T, 0, n)` reserva espacio para `n` elementos. |
| `max` / `min` | Funciones incorporadas (Go 1.21+). |
| `strconv.Atoi` | Texto → entero. |

---

## 5. Cómo comprobarlo

```bash
make pruebas
docker compose up --build -d --wait
TOKEN=...   # token del administrador (paso 17)

curl -i -X POST localhost:8080/api/v1/usuarios -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"nombre":"Laura Martínez","correo":"Laura@uniandes.edu.co","password":"Clave123*","roles":["monitor","monitor","asistente_graduado"]}'
# 201 Created · Location: /api/v1/usuarios/<id> · correo en minúsculas · roles sin repetir

curl -s "localhost:8080/api/v1/usuarios?rol=monitor&busqueda=lau" -H "Authorization: Bearer $TOKEN"
curl -s -X PUT localhost:8080/api/v1/usuarios/<id>/roles -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"roles":["monitor"]}'

# Mismo correo otra vez → 409 conflicto
# Con el token de Laura: GET /api/v1/usuarios → 403 prohibido
# GET /api/v1/usuarios/no-uuid → 400 "id debe ser un identificador válido"
```

---

## 6. Pasos para lograr esta versión desde el paso 18

1. `go get github.com/google/uuid@v1.6.0 && go mod tidy` (desde `backend/`).
2. En `respuestas.go` agregar `parametroUUID`, `consultaPaginacion` y `convertir`.
3. Crear `backend/internal/handler/usuarios.go` (interfaz, handler, `Registrar`, `peticionUsuario`, `peticionRoles` y los cuatro endpoints).
4. En `router.go`: campo `Usuarios ServicioUsuarios` y `&HandlerUsuarios{...}` en la lista de módulos.
5. En `router_test.go`: campo `tokens` en `entorno`, método `token`, constante `uuidA`, `/api/v1/usuarios` en la prueba de rutas protegidas y la nueva `TestControlDeAccesoPorRol`.
6. Crear `backend/internal/handler/modulos_test.go` con `servicioUsuariosFalso` y `TestCrearUsuarioNoExponeElHash`.
7. En `main.go`: `Usuarios: servicio.NuevoUsuarios(usuarios, log)`.
8. `make pruebas lint`, probar con `curl` y commit: `git add . && git commit -m "Paso 19: endpoints de usuarios"`

---

## 7. Preparación para las próximas entregas

- **`Location` en los 201** y listados paginados son convenciones que el frontend (Entrega 2) puede aprovechar directamente.
- **La tabla de `TestControlDeAccesoPorRol`** documenta, en forma ejecutable, quién puede hacer qué. Cualquier cambio de permisos que la rompa se detecta en CI.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-02.1 Crear usuarios | `POST /api/v1/usuarios`. |
| RF-02.2 Asignar roles | `PUT /api/v1/usuarios/{id}/roles`. |
| RF-02.3 Consulta global | `GET /api/v1/usuarios` con filtros y paginación. |
| RF-01.2 Control de acceso por rol | Grupo exclusivo del administrador, probado en `TestControlDeAccesoPorRol`. |
| RF-15.2 Administración | Todo el módulo es del administrador. |
