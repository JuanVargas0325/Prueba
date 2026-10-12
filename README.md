# Paso 10 · Router por módulos y middleware de registro

> Rama `paso-a-paso` · paso 10 de 65 · anterior: **Paso 9 · Errores de aplicación** · siguiente: **Paso 11 · Dominio: usuarios, roles y validaciones**

---

## 1. Qué se hizo en este paso

Se preparó el router para recibir **módulos** (usuarios, tareas, reportes...) y se le agregaron cuatro comportamientos comunes a toda la API:

1. **Un log por petición** (método, ruta, código, duración e IP del cliente).
2. **Errores 404 y 405 en JSON**, con el mismo formato que tendrán todos los errores de la API.
3. **La IP real del cliente** cuando la petición viene a través de Nginx.
4. **CORS**: permiso para que un frontend servido desde otro origen (el de la Entrega 2) llame a la API desde el navegador.

| Archivo | Cambio |
|---|---|
| `backend/internal/handler/respuestas.go` | **Nuevo.** Formato estándar de error `RespuestaError` y la documentación del paquete `handler`. |
| `backend/internal/handler/middleware.go` | **Nuevo.** Middlewares `RegistrarPeticiones` y `CORS`. |
| `backend/internal/handler/router.go` | **Modificado.** Interfaz `modulo`, grupo `/api/v1`, `NoRoute`/`NoMethod`, proxies de confianza, CORS, y logger y orígenes en `Dependencias`. |
| `backend/internal/handler/router_test.go` | **Modificado.** Logger de prueba y pruebas de 404/405 y de CORS. |
| `backend/internal/config/config.go` | **Modificado.** `OrigenesCORS` (variable `CORS_ORIGENES`) y la función `lista`. |
| `backend/internal/config/config_test.go` | **Modificado.** Prueba de la lectura de orígenes. |
| `backend/cmd/api/main.go` | **Modificado.** Pasa el logger y los orígenes al router. |
| `.env.example` | **Modificado.** Variable `CORS_ORIGENES`. |

```bash
curl -i http://localhost:8080/no-existe
# HTTP/1.1 404 Not Found
# {"error":"no_encontrado","mensaje":"ruta no encontrada"}

curl -i -X DELETE http://localhost:8080/salud
# HTTP/1.1 405 Method Not Allowed
# {"error":"metodo_no_permitido","mensaje":"método no permitido"}
```

Y en `docker compose logs api` aparece una línea por petición:

```json
{"level":"INFO","msg":"petición","servicio":"api","metodo":"GET","ruta":"/no-existe","estado":404,"duracion_ms":0,"ip":"172.18.0.1"}
```

---

## 2. Por qué

- **Formato único de error.** Un cliente (Postman, el frontend de la Entrega 2) debe poder tratar todos los errores igual: siempre un JSON con `error` (código corto para programas) y `mensaje` (texto para personas). Antes, una ruta inexistente devolvía el texto plano `404 page not found` de Gin.
- **Observabilidad.** Con un log por petición se puede saber qué se pidió, qué se respondió y cuánto tardó, sin agregar código a cada endpoint. Es la base para diagnosticar problemas y para las pruebas de carga.
- **Organización por módulos.** Cada módulo (paso 17 en adelante) tendrá su archivo con su propio método `Registrar`. El router solo mantiene una lista. Agregar un módulo nuevo no exige modificar las rutas de los demás.
- **CORS desde el inicio.** La Entrega 2 exige un frontend, y su propio enunciado anticipa que, si se publica en un bucket, "el backend requerirá una configuración adecuada de CORS". La Entrega 4 pide verificarlo de nuevo con Cloud Run. Tenerlo desde ahora no cambia nada para Postman o curl, y el frontend podrá conectarse sin tocar el backend: solo con una variable de entorno.

---

## 3. Explicación detallada

### 3.1 ¿Qué es un middleware?

Un **middleware** es una función que se ejecuta **alrededor** de cada petición, antes y después del handler que la atiende. Gin los encadena en el orden en que se registran:

```
petición ─► Recovery ─► RegistrarPeticiones ─► CORS ─► (handler de la ruta) ─┐
respuesta ◄──────────────────────────────────────────────────────────────────┘
```

### 3.2 `middleware.go` · `RegistrarPeticiones`

```go
func RegistrarPeticiones(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		inicio := time.Now()
		c.Next()
		if c.Request.URL.Path == "/salud" {
			return
		}
		log.Info("petición",
			"metodo", c.Request.Method,
			"ruta", c.Request.URL.Path,
			"estado", c.Writer.Status(),
			"duracion_ms", time.Since(inicio).Milliseconds(),
			"ip", c.ClientIP(),
		)
	}
}
```

- Recibe el logger y **devuelve** el middleware (otra vez el patrón *closure* del paso 2).
- `c.Next()` ejecuta el resto de la cadena (el handler). Lo que está **antes** corre al llegar la petición y lo que está **después**, cuando ya se generó la respuesta. Por eso se puede medir la duración y leer el código de estado.
- **`/salud` no se registra:** Docker la consulta cada 10 segundos; registrarla llenaría el log de líneas sin valor.
- `time.Since(inicio).Milliseconds()`: tiempo transcurrido en milisegundos.
- `c.ClientIP()`: la IP del cliente (ver 3.4).

### 3.3 `middleware.go` · `CORS`

**El problema que resuelve.** Por seguridad, un navegador **no deja** que el JavaScript de una página lea respuestas de un servidor con otro **origen**. El origen es la combinación de esquema, dominio y puerto: `http://localhost:5173` y `http://localhost:8080` son orígenes distintos. Sin esta regla, cualquier página que el usuario visite podría usar sus sesiones en otros sitios.

**CORS** (*Cross-Origin Resource Sharing*) es la forma en que el servidor le dice al navegador "este otro origen sí puede llamarme", mediante cabeceras `Access-Control-*`. Solo aplica a navegadores: Postman, curl o Newman no lo revisan.

**La consulta previa (*preflight*).** Antes de una petición "no simple" (por ejemplo, un `POST` con JSON o cualquier petición con `Authorization`), el navegador envía automáticamente un `OPTIONS` preguntando si puede hacerla. Solo si la respuesta lo permite, envía la petición real.

```go
func CORS(origenes []string) gin.HandlerFunc {
	permitidos := map[string]bool{}
	for _, o := range origenes {
		permitidos[o] = true
	}
	return func(c *gin.Context) {
		origen := c.GetHeader("Origin")
		if origen == "" || (!permitidos["*"] && !permitidos[origen]) {
			c.Next()
			return
		}
```
- La lista de orígenes se convierte una sola vez en un **mapa** (`map[string]bool`), para preguntar "¿está permitido?" sin recorrer la lista en cada petición.
- El navegador envía la cabecera `Origin` en las peticiones entre orígenes. Si no viene (Postman, curl) o el origen no está permitido, la petición sigue **sin cambios**. En ese caso el navegador bloqueará la respuesta por su cuenta.
- `"*"` en la configuración acepta cualquier origen; sirve para pruebas, no para producción.

```go
		cabeceras := c.Writer.Header()
		cabeceras.Set("Access-Control-Allow-Origin", origen)
		cabeceras.Add("Vary", "Origin")
		// Consulta previa (preflight): el navegador pregunta si puede enviar la petición real.
		if c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != "" {
			cabeceras.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
			cabeceras.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			cabeceras.Set("Access-Control-Max-Age", "600")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		// Permite que el frontend lea el nombre de los archivos descargados.
		cabeceras.Set("Access-Control-Expose-Headers", "Content-Disposition, Location")
		c.Next()
```
- `Access-Control-Allow-Origin` responde con el mismo origen que preguntó.
- `Vary: Origin` avisa a cualquier caché intermedia que la respuesta depende del origen, para que no le entregue a un sitio la respuesta preparada para otro.
- **Consulta previa:** un `OPTIONS` con `Access-Control-Request-Method` se responde aquí mismo con **204** (sin cuerpo), indicando los métodos y cabeceras permitidos. `Max-Age: 600` deja que el navegador recuerde el permiso 10 minutos y no repita la consulta en cada llamada. `AbortWithStatus` corta la cadena: la petición no llega a ningún handler.
- `Expose-Headers`: por defecto el JavaScript solo puede leer unas pocas cabeceras. Se exponen `Content-Disposition` (el nombre del PDF al descargarlo) y `Location` (la dirección del recurso creado).
- **No se usan cookies** (la autenticación será con un token en `Authorization`, paso 16), así que no hace falta `Access-Control-Allow-Credentials`.

**Configuración** (`config.go` y `.env.example`):

```go
		OrigenesCORS: lista("CORS_ORIGENES"),
...
// lista lee una variable con valores separados por comas, sin espacios ni vacíos.
func lista(clave string) []string {
	var valores []string
	for _, v := range strings.Split(os.Getenv(clave), ",") {
		if v = strings.TrimSpace(v); v != "" {
			valores = append(valores, v)
		}
	}
	return valores
}
```
```dotenv
CORS_ORIGENES=
# ejemplo con el servidor de desarrollo de un frontend y un bucket:
# CORS_ORIGENES=http://localhost:5173,https://storage.googleapis.com
```
- Por defecto queda **vacía**: ningún sitio externo puede llamar a la API desde un navegador. Es lo seguro; cada despliegue declara su frontend.
- `if v = strings.TrimSpace(v); v != ""`: en Go, un `if` puede tener una instrucción inicial antes del `;`. Aquí primero limpia el valor y luego lo evalúa.

### 3.4 `respuestas.go` · el formato de error

```go
// Package handler es la capa de presentación: expone la API REST, valida la
// entrada, aplica autenticación y autorización por rol, y traduce los
// resultados y errores del servicio a respuestas HTTP.
// ...
package handler

// RespuestaError es el formato de todos los errores de la API.
type RespuestaError struct {
	Error   string `json:"error"`
	Mensaje string `json:"mensaje"`
}
```

- Este archivo reunirá todo lo relacionado con **leer peticiones y escribir respuestas** (en el paso 17 se agrega la traducción de `apperr` a códigos HTTP).
- El comentario del paquete se movió aquí desde `router.go`: en Go basta con que **un** archivo del paquete lo tenga, y este describe mejor la responsabilidad de toda la capa.
- `Error` coincidirá con los tipos de `apperr` (`validacion`, `no_encontrado`...). Para la ruta inexistente se usa `no_encontrado` y para el método no permitido, `metodo_no_permitido`.

### 3.5 `router.go`

```go
type Dependencias struct {
	Salud map[string]ComprobadorSalud
	// OrigenesCORS son los orígenes de los frontends que pueden usar la API.
	OrigenesCORS []string
	Log          *slog.Logger
}
```
- Se agregan el logger y los orígenes de CORS. El router no crea su logger ni lee variables de entorno: lo recibe todo de `main`, que es quien decide el formato del log (JSON), los campos fijos (`servicio=api`) y la configuración.

```go
type modulo interface {
	Registrar(g *gin.RouterGroup)
}
```
- Una **interfaz**: "cualquier cosa que tenga el método `Registrar(g *gin.RouterGroup)`". Cada handler de módulo lo implementará **automáticamente** al tener ese método (en Go no se declara `implements`). Está en minúscula porque es un detalle interno del paquete.

```go
	r.Use(gin.Recovery(), RegistrarPeticiones(d.Log), CORS(d.OrigenesCORS))
```
- Primero `Recovery`, para que un pánico en cualquier punto (incluido el log) se convierta en un 500.
- `CORS` va registrado en el **router completo** (`r.Use`), no en un grupo. Así también responde las consultas previas `OPTIONS`, que no coinciden con ninguna ruta declarada: Gin aplica estos middlewares incluso a las respuestas 404 y 405.

```go
	// Detrás de Nginx la IP del cliente llega en X-Forwarded-For.
	_ = r.SetTrustedProxies([]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.1"})
```
- Con Nginx delante (paso 7), todas las peticiones vienen desde la IP del contenedor de Nginx. Nginx informa la IP original en la cabecera `X-Forwarded-For`.
- **Pero cualquiera puede escribir esa cabecera.** Por eso Gin solo la cree si la petición viene de un **proxy de confianza**. Se confía en las redes privadas (las que usan Docker y las redes internas de la nube) y en `127.0.0.1`. Una petición desde Internet que traiga una cabecera falsa no puede suplantar su IP.
- `_ =` descarta el error que devuelve la función: con estas direcciones fijas nunca falla.

```go
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, RespuestaError{Error: "no_encontrado", Mensaje: "ruta no encontrada"})
	})
	r.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, RespuestaError{Error: "metodo_no_permitido", Mensaje: "método no permitido"})
	})
	r.HandleMethodNotAllowed = true
```
- `NoRoute`: qué responder cuando ninguna ruta coincide (404).
- `NoMethod` + `HandleMethodNotAllowed = true`: si la ruta existe pero con otro método (por ejemplo, `DELETE /salud`), se responde **405** en lugar de 404. Es más preciso y ayuda a quien consume la API a entender su error.
- Las funciones se escriben **en línea** (funciones anónimas) porque son cortas y solo se usan aquí.

```go
	api := r.Group("/api/v1")
	// Los módulos se agregan a partir del paso 17 (inicio de sesión).
	modulos := []modulo{}
	for _, m := range modulos {
		m.Registrar(api)
	}
```
- Todas las rutas de negocio vivirán bajo `/api/v1`. La **versión en la ruta** permite publicar algún día una `/api/v2` sin romper a los clientes de la `v1`.
- `/salud` queda fuera del prefijo: es de infraestructura, no de negocio.
- Hoy la lista está vacía. En el paso 17 recibe su primer módulo.

### 3.6 Pruebas

- `logPrueba` escribe en `io.Discard` (un "agujero negro"): el código que registra se ejecuta, pero no ensucia la salida de las pruebas.
- `TestRutaInexistenteYMetodoNoPermitido` verifica el 404 y el 405.
- `TestCORS` tiene tres **subpruebas** (`t.Run`): un origen permitido recibe la cabecera, la consulta previa responde 204 y un origen no configurado no recibe permiso. `conOrigen` es una función auxiliar definida dentro de la prueba que arma la petición con su cabecera `Origin`.
- `TestListaSeparaPorComas` comprueba que `lista` ignora espacios y valores vacíos. Prueba `lista` directamente y no `Cargar`, porque `Cargar` exigirá más variables en los pasos siguientes.

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Interfaz implícita | Un tipo implementa una interfaz con solo tener sus métodos. |
| Función anónima | `func(c *gin.Context) { ... }` escrita donde se usa. |
| `_ =` | Descarta un valor de forma explícita. |
| Literal de slice vacío | `[]modulo{}`: lista vacía de módulos. |
| `io.Discard` | Destino de escritura que descarta todo. |
| `map[string]bool` | Mapa (diccionario) de texto a booleano; un valor ausente devuelve `false`. |
| `if inicio; condición` | Un `if` puede ejecutar una instrucción antes de evaluar la condición. |
| `t.Run` | Subprueba con nombre propio dentro de una prueba. |

---

## 5. Cómo comprobarlo

```bash
make pruebas                     # o: cd backend && go test ./...
docker compose up --build -d --wait
curl -i http://localhost:8080/no-existe
curl -i -X DELETE http://localhost:8080/salud
docker compose logs api | grep petición

# CORS: con CORS_ORIGENES=http://localhost:5173 en el .env (y docker compose up -d api)
curl -i -X OPTIONS http://localhost:8080/salud -H "Origin: http://localhost:5173" -H "Access-Control-Request-Method: GET"
# HTTP/1.1 204 No Content
# Access-Control-Allow-Origin: http://localhost:5173
# Access-Control-Allow-Methods: GET, POST, PUT, PATCH, DELETE
```

---

## 6. Pasos para lograr esta versión desde el paso 9

1. Crear `backend/internal/handler/respuestas.go` con el comentario del paquete y `RespuestaError`.
2. En `backend/internal/handler/router.go`, quitar el comentario del paquete (ahora está en `respuestas.go`).
3. Crear `backend/internal/handler/middleware.go` con `RegistrarPeticiones` y `CORS` (secciones 3.2 y 3.3).
4. En `config.go`: el campo `OrigenesCORS`, su carga con `lista("CORS_ORIGENES")` y la función `lista`. En `config_test.go`, `TestListaSeparaPorComas`.
5. En `.env.example`, después de `NGINX_PUERTO`, la variable `CORS_ORIGENES` con su comentario.
6. Modificar `router.go`:
   - campos `OrigenesCORS []string` y `Log *slog.Logger` en `Dependencias`;
   - interfaz `modulo`;
   - en `NuevoRouter`: `Use(Recovery, RegistrarPeticiones, CORS)`, `SetTrustedProxies`, `NoRoute`, `NoMethod`, `HandleMethodNotAllowed`, grupo `/api/v1` y la lista de módulos.
7. En `router_test.go`: variable `logPrueba`, `Log: logPrueba` en `nuevoEntorno` y las pruebas `TestRutaInexistenteYMetodoNoPermitido` y `TestCORS`.
8. En `backend/cmd/api/main.go`: `OrigenesCORS: cfg.OrigenesCORS` y `Log: log` dentro de `handler.Dependencias{...}`.
9. `make lint pruebas` y commit: `git add . && git commit -m "Paso 10: router por módulos y middleware de registro"`

---

## 7. Preparación para las próximas entregas

- **Proxies de confianza en redes privadas:** en GCP, el balanceador de carga y las VM están en redes privadas (`10.x.x.x`), así que la API ya registrará la IP real de los usuarios sin cambios.
- **Duración por petición en los logs:** durante las pruebas de carga se puede cruzar con lo que mide la herramienta de carga para saber si el tiempo se va en la API o en la red.
- **Versión en la ruta (`/api/v1`):** el frontend de la Entrega 2 dependerá de esta API; si hiciera falta un cambio incompatible, puede convivir una `v2`.
- **CORS listo para el frontend:** en la Entrega 2 basta con poner en `CORS_ORIGENES` la dirección desde la que se sirva el frontend (el servidor de desarrollo, como `http://localhost:5173`, o el bucket de Cloud Storage). En la Entrega 4, el frontend llamará a la URL de Cloud Run y se configura igual.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RNF-05 Mantenibilidad | Módulos con registro de rutas propio; comportamiento común en middleware. |
| RNF-01 Backend sólido | Errores uniformes en JSON; 405 cuando corresponde. |
| Arquitectura §4 (Nginx delante de la API) | IP real del cliente a través del proxy, sin aceptar cabeceras falsas. |
| Entregas 2 y 4: frontend conectado al backend | CORS configurable por variable de entorno. |
