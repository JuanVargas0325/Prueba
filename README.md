# Paso 17 · Endpoints de inicio de sesión

> Rama `paso-a-paso` · paso 17 de 65 · anterior: **Paso 16 · Middleware de autenticación** · siguiente: **Paso 18 · Servicio de gestión de usuarios**

---

## 1. Qué se hizo en este paso

La API tiene sus **primeros endpoints de negocio**:

| Método | Ruta | Acceso | Descripción |
|---|---|---|---|
| POST | `/api/v1/auth/login` | público | Recibe correo y contraseña; devuelve un token JWT y los datos del usuario. |
| GET | `/api/v1/auth/yo` | con token | Devuelve el usuario autenticado. |

| Archivo | Cambio |
|---|---|
| `backend/internal/handler/autenticacion.go` | **Nuevo.** El primer **módulo** del handler: interfaz del servicio, formatos JSON y los endpoints `Login` y `Yo`. |
| `backend/internal/handler/formatos.go` | **Nuevo.** Formatos JSON compartidos entre módulos; por ahora `usuarioJSON`. |
| `backend/internal/handler/respuestas.go` | **Modificado.** Traducción de `apperr` a códigos HTTP (`responderError`) y lectura estricta de JSON (`leerJSON`). |
| `backend/internal/handler/router.go` | **Modificado.** Rutas públicas y grupo de rutas protegidas con `Autenticar`. |
| `backend/internal/handler/router_test.go` | **Modificado.** Gestor de tokens de prueba, `peticion` acepta un token y nueva prueba de rutas protegidas. |
| `backend/cmd/api/main.go` | **Modificado.** Entrega el servicio de autenticación y el gestor de tokens al router. |

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"correo":"admin@uniandes.edu.co","password":"cambie-esta-clave"}'
# {"token":"eyJhbGciOi...","expira_en":"2026-10-12T08:59:00Z","usuario":{"id":"f7c5...","nombre":"Administrador",
#  "correo":"admin@uniandes.edu.co","activo":true,"roles":["administrador"],"creado_en":"..."}}

curl http://localhost:8080/api/v1/auth/yo -H "Authorization: Bearer eyJhbGciOi..."
```

---

## 2. Por qué

- **RF-01.1 Inicio de sesión local.** Con este paso un usuario ya puede autenticarse y usar el token en las demás rutas.
- **La estructura de un módulo queda fijada.** Todos los módulos siguientes (usuarios, períodos, espacios, vinculaciones, tareas, seguimiento, reportes) tendrán exactamente la misma forma (sección 3.1).
- **Errores consistentes:** desde ahora cualquier error de un servicio se responde con el código HTTP correcto y el formato `{"error", "mensaje"}`.

---

## 3. Explicación detallada

### 3.1 Anatomía de un módulo (`autenticacion.go`)

**1) La interfaz del servicio que necesita** (declarada aquí, como en el paso 14):

```go
type ServicioAutenticacion interface {
	IniciarSesion(ctx context.Context, correo, password string) (servicio.Sesion, error)
	UsuarioActual(ctx context.Context, actor auth.Actor) (dominio.Usuario, error)
}
```

**2) El struct del handler**, con su servicio y su logger:

```go
type HandlerAutenticacion struct {
	servicio ServicioAutenticacion
	log      *slog.Logger
}
```

**3) Los formatos de entrada y salida** (DTO, *Data Transfer Objects*):

```go
type peticionLogin struct {
	Correo   string `json:"correo"`
	Password string `json:"password"`
}

type respuestaLogin struct {
	Token    string      `json:"token"`
	ExpiraEn time.Time   `json:"expira_en"`
	Usuario  usuarioJSON `json:"usuario"`
}
```
- `time.Time` se convierte automáticamente al formato estándar RFC 3339 (`2026-10-12T08:59:00Z`).

**4) El registro de rutas:**

```go
func (h *HandlerAutenticacion) RegistrarPublicas(g *gin.RouterGroup) {
	g.POST("/auth/login", h.Login)
}

func (h *HandlerAutenticacion) Registrar(g *gin.RouterGroup) {
	g.GET("/auth/yo", h.Yo)
}
```
- Este módulo es especial: tiene una ruta **pública** (no se puede pedir un token para obtener el token). `Registrar` es el método de la interfaz `modulo` (paso 10) y recibe el grupo protegido.
- `h.Login` es un **método usado como valor** (como `db.Ping` en el paso 3).

**5) Los endpoints:**

```go
func (h *HandlerAutenticacion) Login(c *gin.Context) {
	var p peticionLogin
	if err := leerJSON(c, &p); err != nil {
		responderError(c, h.log, err)
		return
	}
	sesion, err := h.servicio.IniciarSesion(c.Request.Context(), p.Correo, p.Password)
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	c.JSON(http.StatusOK, respuestaLogin{Token: sesion.Token, ExpiraEn: sesion.ExpiraEn, Usuario: aUsuarioJSON(sesion.Usuario)})
}
```
- El patrón de **todo** endpoint: (1) leer y validar la entrada; (2) llamar al servicio con el contexto de la petición y el actor; (3) traducir el resultado o el error a HTTP. **El handler no tiene lógica de negocio.**
- `c.Request.Context()`: si el cliente cierra la conexión, ese contexto se cancela y las consultas a la base de datos se abortan, sin trabajar para nadie.

```go
func (h *HandlerAutenticacion) Yo(c *gin.Context) {
	u, err := h.servicio.UsuarioActual(c.Request.Context(), ActorDe(c))
	...
	c.JSON(http.StatusOK, aUsuarioJSON(u))
}
```
- `ActorDe(c)` toma el actor que dejó `Autenticar` (paso 16).

### 3.2 `formatos.go`

```go
// Formatos JSON compartidos por varios módulos. Las entidades del dominio no
// llevan etiquetas JSON: la forma de la API se define solo en esta capa.

type usuarioJSON struct {
	ID       string    `json:"id"`
	Nombre   string    `json:"nombre"`
	Correo   string    `json:"correo"`
	Activo   bool      `json:"activo"`
	Roles    []string  `json:"roles"`
	CreadoEn time.Time `json:"creado_en"`
}

func aUsuarioJSON(u dominio.Usuario) usuarioJSON { ... }
```
- `usuarioJSON` **no tiene `PasswordHash`**. Como el dominio no se serializa directamente, es imposible que el hash termine en una respuesta por descuido.
- Este archivo reúne los formatos que **varios** módulos reutilizan (por ejemplo, el usuario aparece en el login y en la gestión de usuarios del paso 19).

### 3.3 `respuestas.go`: de `apperr` a HTTP

```go
var estadosPorTipo = map[apperr.Tipo]int{
	apperr.TipoValidacion:    http.StatusBadRequest,          // 400
	apperr.TipoNoAutenticado: http.StatusUnauthorized,        // 401
	apperr.TipoProhibido:     http.StatusForbidden,           // 403
	apperr.TipoNoEncontrado:  http.StatusNotFound,            // 404
	apperr.TipoConflicto:     http.StatusConflict,            // 409
	apperr.TipoReglaNegocio:  http.StatusUnprocessableEntity, // 422
}

func responderError(c *gin.Context, log *slog.Logger, err error) {
	var e *apperr.Error
	if errors.As(err, &e) {
		if estado, ok := estadosPorTipo[e.Tipo]; ok {
			c.AbortWithStatusJSON(estado, RespuestaError{Error: string(e.Tipo), Mensaje: e.Mensaje})
			return
		}
	}
	var demasiadoGrande *http.MaxBytesError
	if errors.As(err, &demasiadoGrande) {
		c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, RespuestaError{
			Error: "demasiado_grande", Mensaje: "la petición supera el tamaño máximo permitido"})
		return
	}
	log.Error("error interno", "ruta", c.FullPath(), "metodo", c.Request.Method, "error", err)
	mensaje := "ocurrió un error inesperado"
	if e != nil && e.Tipo == apperr.TipoInterno {
		mensaje = e.Mensaje
	}
	c.AbortWithStatusJSON(http.StatusInternalServerError, RespuestaError{Error: string(apperr.TipoInterno), Mensaje: mensaje})
}
```
- **El único lugar** donde se decide el código HTTP de un error.
- `estado, ok := mapa[clave]`: el `ok` dice si la clave existe.
- **413 (petición demasiado grande):** si una lectura del cuerpo superó el límite (`http.MaxBytesError`). Se usará con los archivos adjuntos (paso 40).
- **500:** cualquier otro error. Se **registra completo en el log** (con ruta y método), pero al cliente solo se le dice "ocurrió un error inesperado". Los detalles internos (consultas SQL, nombres de tablas) nunca se exponen. Si es un `apperr.Interno`, se usa su mensaje genérico.

### 3.4 `respuestas.go`: leer JSON de forma estricta

```go
const maxCuerpoJSON = 1 << 20 // 1 MB

func leerJSON(c *gin.Context, destino any) error {
	return decodificarJSON(io.LimitReader(c.Request.Body, maxCuerpoJSON), destino)
}

func decodificarJSON(r io.Reader, destino any) error {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	err := dec.Decode(destino)
	if err == nil {
		return nil
	}
	var tipo *json.UnmarshalTypeError
	switch {
	case errors.Is(err, io.EOF):
		return apperr.Validacion("el cuerpo de la petición está vacío")
	case errors.As(err, &tipo):
		return apperr.Validacion("el campo %q tiene un tipo inválido (se esperaba %s)", tipo.Field, tipo.Type.String())
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		return apperr.Validacion("campo desconocido %s", strings.TrimPrefix(err.Error(), "json: unknown field "))
	default:
		return apperr.Validacion("el cuerpo no es un JSON válido")
	}
}
```
- `1 << 20` es "1 desplazado 20 bits" = 1 048 576 bytes (1 MB). `io.LimitReader` corta la lectura ahí: nadie puede enviar un JSON gigante para agotar la memoria.
- `DisallowUnknownFields()`: un campo que la API no conoce es un **error**. Si un cliente escribe `"clave"` en lugar de `"password"`, recibe "campo desconocido \"clave\"" en lugar de un login fallido sin explicación.
- Los errores del decodificador se traducen a mensajes claros y siempre a **400**:
  - cuerpo vacío;
  - tipo equivocado: por ejemplo, `"horas_invertidas": 2.5` donde se espera un entero (`%q` pone el nombre entre comillas);
  - campo desconocido;
  - JSON mal formado.
- `switch { case ...: }` sin expresión es un `if / else if` encadenado.
- `decodificarJSON` está separada de `leerJSON` porque en el paso 40 se reutiliza para leer JSON que llega dentro de un formulario con archivos.

```go
func rolesTexto(roles []dominio.Rol) []string { ... }
```
- Convierte `[]dominio.Rol` a `[]string` para el JSON.

### 3.5 `router.go`

```go
type Dependencias struct {
	Autenticacion ServicioAutenticacion

	Tokens ValidadorTokens
	Salud  map[string]ComprobadorSalud
	// OrigenesCORS son los orígenes de los frontends que pueden usar la API.
	OrigenesCORS []string
	Log          *slog.Logger
}
```

```go
	api := r.Group("/api/v1")
	autenticacion := &HandlerAutenticacion{servicio: d.Autenticacion, log: d.Log}
	autenticacion.RegistrarPublicas(api)

	protegidas := api.Group("", Autenticar(d.Tokens))
	modulos := []modulo{
		autenticacion,
	}
	for _, m := range modulos {
		m.Registrar(protegidas)
	}
```
- `api.Group("", Autenticar(...))` crea un subgrupo con el **mismo prefijo** (`/api/v1`) pero con el middleware de autenticación: **toda** ruta que un módulo registre ahí exige token. Para hacer pública una ruta hay que registrarla a propósito en `api` (como `login`).
- `*HandlerAutenticacion` cumple la interfaz `modulo` porque tiene el método `Registrar`.

### 3.6 Pruebas

- `nuevoEntorno` ahora crea un gestor de tokens real con un secreto de prueba.
- `peticion(metodo, ruta, token, cuerpo)`: si `token` no está vacío, agrega `Authorization: Bearer ...`.
- `TestRutasProtegidasExigenToken`: `/api/v1/auth/yo` sin token y con un token inválido responde 401. En los pasos siguientes la lista de rutas crecerá con cada módulo.

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| DTO con etiquetas JSON | Structs solo para la forma de la API, separados del dominio. |
| `mapa[clave]` con `ok` | Saber si la clave existe. |
| `switch` sin expresión | Cadena de condiciones. |
| `1 << 20` | Desplazamiento de bits: 2²⁰ = 1 MB. |
| `io.LimitReader` | Lector que no entrega más de N bytes. |
| `%q` | Formato que pone el texto entre comillas y escapa caracteres especiales. |

---

## 5. Cómo comprobarlo

```bash
make pruebas
docker compose up --build -d --wait

# Login correcto (el correo admite mayúsculas)
curl -s -X POST localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"correo":"ADMIN@uniandes.edu.co","password":"cambie-esta-clave"}'

# Guardar el token (Git Bash / Linux)
TOKEN=$(curl -s -X POST localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"correo":"admin@uniandes.edu.co","password":"cambie-esta-clave"}' | python -c "import sys,json;print(json.load(sys.stdin)['token'])")
curl -s localhost:8080/api/v1/auth/yo -H "Authorization: Bearer $TOKEN"

# Errores
curl -s -X POST localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' -d '{"correo":"admin@uniandes.edu.co","password":"otra"}'
# {"error":"no_autenticado","mensaje":"credenciales inválidas"}                      → 401
curl -s -X POST localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' -d '{"correo":"a","clave":"x"}'
# {"error":"validacion","mensaje":"campo desconocido \"clave\""}                     → 400
curl -s localhost:8080/api/v1/auth/yo
# {"error":"no_autenticado","mensaje":"se requiere un token de acceso"}              → 401
```

También se puede hacer con Postman: método POST, pestaña *Body* → *raw* → *JSON*. La colección completa llega en el paso 60.

---

## 6. Pasos para lograr esta versión desde el paso 16

1. Crear `backend/internal/handler/formatos.go` con el comentario explicativo, `usuarioJSON` y `aUsuarioJSON`.
2. En `respuestas.go` agregar `estadosPorTipo`, `responderError`, `maxCuerpoJSON`, `leerJSON`, `decodificarJSON` y `rolesTexto` (más los imports que necesitan).
3. Crear `backend/internal/handler/autenticacion.go` con `ServicioAutenticacion`, `HandlerAutenticacion`, `peticionLogin`, `respuestaLogin`, `RegistrarPublicas`, `Registrar`, `Login` y `Yo`.
4. En `router.go`: campos `Autenticacion` y `Tokens` en `Dependencias`; registrar el login en `api`; crear el grupo `protegidas` con `Autenticar` y la lista de módulos con `autenticacion`.
5. En `router_test.go`: constante `secretoPrueba`, `Tokens` en `nuevoEntorno`, parámetro `token` en `peticion`/`peticionCon` (actualizar las llamadas existentes) y `TestRutasProtegidasExigenToken`.
6. En `main.go`: `Autenticacion: servicioAuth` y `Tokens: tokens` en `handler.Dependencias`.
7. `make pruebas lint`, levantar con Docker Compose y probar con `curl` (sección 5).
8. Commit: `git add . && git commit -m "Paso 17: endpoints de inicio de sesión"`

---

## 7. Preparación para las próximas entregas

- **El frontend de la Entrega 2** usará `POST /auth/login` para obtener el token y `GET /auth/yo` para saber quién inició sesión y con qué roles (para mostrar u ocultar opciones).
- **Errores estandarizados:** el frontend podrá mostrar `mensaje` directamente al usuario y decidir con `error` (por ejemplo, redirigir al login ante `no_autenticado`).

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-01.1 Inicio de sesión local | `POST /api/v1/auth/login`. |
| RF-01.2 Control de acceso | Grupo protegido con `Autenticar`; 401 sin token. |
| RNF-03 Seguridad básica | El hash nunca se serializa; errores internos sin detalles; cuerpo limitado a 1 MB. |
