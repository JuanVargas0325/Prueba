# Paso 2 · Servidor HTTP con Gin y endpoint `/salud`

> Rama `paso-a-paso` · paso 2 de 65 · anterior: **Paso 1 · Configuración** · siguiente: **Paso 3 · Conexión a PostgreSQL**

---

## 1. Qué se hizo en este paso

La API ya es un **servidor web**: escucha en el puerto configurado (`APP_PUERTO`, 8080 por defecto) y responde a `GET /salud`.

| Archivo | Cambio |
|---|---|
| `backend/go.mod`, `backend/go.sum` | **Modificados / nuevo.** Se agrega la librería **Gin** (`github.com/gin-gonic/gin v1.12.0`). |
| `backend/internal/handler/router.go` | **Nuevo.** Crea el router de Gin y el endpoint `/salud`. |
| `backend/internal/handler/router_test.go` | **Nuevo.** Utilidades de prueba HTTP y dos pruebas de `/salud`. |
| `backend/cmd/api/main.go` | **Modificado.** Levanta el servidor HTTP y lo apaga de forma ordenada al recibir Ctrl+C o `SIGTERM`. |

Ejemplo:

```bash
curl http://localhost:8080/salud
# {"estado":"ok","componentes":{}}
```

---

## 2. Por qué

- **Gin** es uno de los dos frameworks web que exige el stack del curso (Gin o Echo). Sobre la librería estándar `net/http` agrega: rutas con parámetros (`/tareas/:id`), grupos de rutas, middleware (código que se ejecuta antes o después de cada petición) y utilidades para leer y escribir JSON.
- **`/salud` (health check)** es el primer endpoint porque lo necesita la infraestructura:
  - Docker lo consultará (paso 5) para saber si el contenedor está sano;
  - Docker Compose y Nginx esperarán a que la API esté sana antes de enviarle tráfico (pasos 6 y 7);
  - en la Entrega 3, el balanceador de carga de la nube lo usará para sacar de servicio una instancia que falle.
- **Apagado ordenado (graceful shutdown):** cuando Docker detiene un contenedor envía la señal `SIGTERM`. Si el programa simplemente muriera, las peticiones en curso se cortarían a la mitad. Con el apagado ordenado, el servidor deja de aceptar conexiones nuevas, termina las que están en curso (hasta 15 segundos) y sale.

---

## 3. Explicación detallada

### 3.1 Agregar la dependencia

```bash
cd backend
go get github.com/gin-gonic/gin@v1.12.0
go mod tidy
```

- `go get` descarga la librería en la versión indicada y la anota en `go.mod` (`require github.com/gin-gonic/gin v1.12.0`).
- `go mod tidy` ordena `go.mod`: agrega las dependencias indirectas (las librerías que Gin necesita, marcadas `// indirect`) y quita las que ya no se usan.
- Se crea **`go.sum`**, que guarda un hash criptográfico de cada dependencia. Garantiza que todos (usted, sus compañeros, Docker y GitHub Actions) descarguen exactamente el mismo código. Se versiona, pero nunca se edita a mano.
- Las librerías se descargan del proxy oficial `proxy.golang.org` y quedan en la caché local (`%USERPROFILE%\go\pkg\mod` en Windows).

### 3.2 `internal/handler/router.go`

El paquete `handler` será la **capa de presentación**: la única que sabe de HTTP. En los pasos siguientes recibirá un archivo por módulo (usuarios, tareas, reportes...).

```go
type ComprobadorSalud func(ctx context.Context) error
```

- Define un **tipo función**: cualquier función que reciba un `context.Context` y devuelva un `error` es un `ComprobadorSalud`. En el paso 3 se pasará la función que hace *ping* a PostgreSQL; en el paso 49, la que hace *ping* a Redis. El router no necesita saber qué hay detrás.

```go
type Dependencias struct {
	Salud map[string]ComprobadorSalud
}
```

- `Dependencias` agrupa todo lo que el router necesita desde fuera. Hoy solo los comprobadores de salud; después, los servicios de cada módulo. Así `NuevoRouter` no crea nada por su cuenta: **recibe** lo que usa (inyección de dependencias), y las pruebas pueden pasarle versiones falsas.
- `map[string]ComprobadorSalud` es un diccionario: nombre de la dependencia → función que la comprueba.

```go
func NuevoRouter(d Dependencias) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/salud", salud(d.Salud))
	return r
}
```

- `gin.New()` crea un router vacío (sin el middleware que trae `gin.Default()`, para decidir explícitamente cuál usar).
- `r.Use(gin.Recovery())`: si algún handler entra en **pánico** (un error grave como acceder a un puntero `nil`), Recovery lo captura y responde 500 en lugar de tumbar todo el servidor.
- `r.GET("/salud", ...)` asocia la ruta y el método HTTP con una función que atiende la petición.

```go
type estadoSalud struct {
	Estado      string            `json:"estado"`
	Componentes map[string]string `json:"componentes"`
}
```

- Las **etiquetas** entre comillas invertidas (`` `json:"estado"` ``) le dicen al paquete de JSON qué nombre usar para cada campo. Sin ellas, el JSON tendría `"Estado"` con mayúscula.

```go
func salud(comprobadores map[string]ComprobadorSalud) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancelar := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancelar()
		respuesta := estadoSalud{Estado: "ok", Componentes: map[string]string{}}
		codigo := http.StatusOK
		for nombre, comprobar := range comprobadores {
			if err := comprobar(ctx); err != nil {
				respuesta.Componentes[nombre] = "error"
				respuesta.Estado = "degradado"
				codigo = http.StatusServiceUnavailable
				continue
			}
			respuesta.Componentes[nombre] = "ok"
		}
		c.JSON(codigo, respuesta)
	}
}
```

- `salud` es una función que **devuelve otra función** (el handler). La función interna "recuerda" el mapa `comprobadores` aunque `salud` ya haya terminado: eso se llama **closure**.
- `gin.Context` (`c`) representa la petición y la respuesta en curso.
- `context.WithTimeout(..., 3*time.Second)`: cada comprobación tiene como máximo 3 segundos. Si la base de datos no responde, `/salud` no se queda colgado.
- `defer cancelar()`: `defer` programa una instrucción para que se ejecute **al salir de la función**, pase lo que pase. Aquí libera los recursos del temporizador.
- `for nombre, comprobar := range comprobadores` recorre el mapa (clave y valor).
- Si alguna dependencia falla, la respuesta es **503 Service Unavailable** con estado `degradado`; si todas responden, **200 OK**. Ese código es lo que mirarán Docker y el balanceador.
- `continue` salta a la siguiente vuelta del ciclo.
- `c.JSON(codigo, respuesta)` convierte el struct a JSON y lo envía con el código indicado.
- En este paso el mapa está vacío, así que la respuesta es `{"estado":"ok","componentes":{}}`.

### 3.3 `internal/handler/router_test.go`

Las pruebas HTTP no levantan un servidor real: usan `httptest`, que permite entregar una petición falsa al router y capturar la respuesta en memoria. Son rápidas y no ocupan puertos.

- `nuevoEntorno(t, configurar)` arma un router con dependencias por defecto (un "postgres" que siempre responde bien). El parámetro `configurar` permite que cada prueba cambie lo que necesite.
- `peticion(metodo, ruta, cuerpo)` crea la petición con `httptest.NewRequest`, la pasa por el router (`ServeHTTP`) y devuelve el `ResponseRecorder` con el código y el cuerpo.
- `esperarEstado` falla la prueba si el código no es el esperado y muestra el cuerpo para entender por qué.
- `t.Helper()` le indica a Go que, si la prueba falla dentro de esta función auxiliar, reporte la línea de **quien la llamó**, que es la que interesa.
- `gin.SetMode(gin.TestMode)` silencia los mensajes de depuración de Gin durante las pruebas.

Las dos pruebas verifican el contrato de `/salud`: 200 cuando todo está bien y 503 cuando una dependencia falla.

### 3.4 `cmd/api/main.go`

```go
	if !cfg.EsDesarrollo() {
		gin.SetMode(gin.ReleaseMode)
	}
```
- En producción, Gin no imprime sus avisos de depuración.

```go
	ctx, detener := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer detener()
```
- Crea un **contexto** que se cancela cuando el proceso recibe Ctrl+C (`os.Interrupt`) o `SIGTERM` (lo que envía `docker stop`). Un `context.Context` es la forma estándar de Go de avisar "hay que parar" a todo lo que esté trabajando.

```go
	router := handler.NuevoRouter(handler.Dependencias{})

	servidor := &http.Server{
		Addr:              ":" + cfg.PuertoHTTP,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
```
- Se usa el servidor de la librería estándar (`http.Server`) con el router de Gin como `Handler`. Así se controlan los **tiempos máximos**:
  - `ReadHeaderTimeout` (10 s): un cliente que envía las cabeceras muy despacio no puede retener una conexión indefinidamente (protección contra el ataque *Slowloris*);
  - `ReadTimeout` y `WriteTimeout` (5 min): margen para subir y descargar archivos (adjuntos y PDF, pasos 40 y 49);
  - `IdleTimeout` (2 min): cuánto se mantiene abierta una conexión sin uso.
- `":" + cfg.PuertoHTTP` produce `":8080"`: escuchar en el puerto 8080 de **todas** las interfaces de red (necesario dentro de un contenedor).
- `&http.Server{...}` crea el struct y devuelve un **puntero** a él (`&`).

```go
	errServidor := make(chan error, 1)
	go func() {
		log.Info("API escuchando", "puerto", cfg.PuertoHTTP, "entorno", cfg.Entorno)
		errServidor <- servidor.ListenAndServe()
	}()
```
- `ListenAndServe` **bloquea**: se queda atendiendo peticiones hasta que el servidor se detiene. Por eso se ejecuta en una **goroutine** (`go func() {...}()`), un hilo de ejecución liviano de Go, para que `ejecutar` pueda seguir y esperar la señal de apagado.
- `make(chan error, 1)` crea un **canal**: una "tubería" por la que la goroutine envía (`<-`) el error con el que terminó el servidor. El `1` le da espacio para un mensaje sin que nadie lo esté leyendo todavía.

```go
	select {
	case err := <-errServidor:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("deteniendo la API")
	}
```
- `select` espera a que **uno** de los casos ocurra:
  - el servidor terminó por su cuenta (por ejemplo, el puerto ya estaba ocupado): se devuelve ese error;
  - llegó Ctrl+C o `SIGTERM` (`ctx.Done()` se cierra): se procede a apagar.
- `errors.Is(err, http.ErrServerClosed)` distingue el "error" normal de un servidor que se cerró a propósito.

```go
	ctxCierre, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	return servidor.Shutdown(ctxCierre)
```
- `Shutdown` deja de aceptar conexiones y espera a que terminen las peticiones en curso, como máximo 15 segundos. Docker espera 10 segundos por defecto antes de matar el proceso; para la mayoría de peticiones es suficiente.

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Dependencias externas | `go get` + `go mod tidy`; quedan en `go.mod` y `go.sum`. |
| Tipo función | `type ComprobadorSalud func(ctx context.Context) error`. |
| `map[K]V` | Diccionario. Se recorre con `for clave, valor := range m`. |
| Closure | Función interna que usa variables de la función que la creó (`salud`). |
| Etiquetas de struct | `` `json:"estado"` `` define el nombre del campo en JSON. |
| `defer` | Ejecuta algo al salir de la función, aunque haya error. |
| `context.Context` | Lleva cancelación y tiempos máximos a través de las llamadas. |
| Goroutine | `go f()` ejecuta `f` en paralelo. |
| Canal | `make(chan T, n)`; se envía con `c <- v` y se recibe con `v := <-c`. |
| `select` | Espera al primero de varios canales que tenga algo. |
| `&T{...}` | Crea un valor y devuelve un puntero a él. |
| `httptest` | Paquete estándar para probar handlers HTTP sin red. |

---

## 5. Cómo comprobarlo

```bash
cd backend
go test ./...
go run ./cmd/api
# en otra terminal:
curl -i http://localhost:8080/salud      # 200 {"estado":"ok","componentes":{}}
curl -i http://localhost:8080/no-existe  # 404 (texto plano por ahora; en el paso 10 será JSON)
```

Con Docker hay que publicar el puerto (`-p`):

```bash
docker run --rm -p 8080:8080 -v "${PWD}/backend:/src" -w /src golang:1.26 go run ./cmd/api
```

Al presionar Ctrl+C se ve `deteniendo la API` y el programa termina sin error.

---

## 6. Pasos para lograr esta versión desde el paso 1

1. Agregar Gin (desde `backend/`):
   ```bash
   go get github.com/gin-gonic/gin@v1.12.0
   go mod tidy
   ```
2. Crear `backend/internal/handler/router.go` con `ComprobadorSalud`, `Dependencias`, `NuevoRouter`, `estadoSalud` y `salud` (sección 3.2).
3. Crear `backend/internal/handler/router_test.go` con las utilidades (`entorno`, `nuevoEntorno`, `peticion`, `peticionCon`, `esperarEstado`) y las dos pruebas de `/salud`.
4. Modificar `backend/cmd/api/main.go`:
   - imports: `context`, `errors`, `net/http`, `os/signal`, `syscall`, `time`, `github.com/gin-gonic/gin` y `seguimiento/internal/handler`;
   - modo release de Gin fuera de desarrollo;
   - contexto que se cancela con Ctrl+C o `SIGTERM`;
   - `http.Server` con los tiempos máximos;
   - `ListenAndServe` en una goroutine, `select` y `Shutdown` con 15 s.
5. Probar: `go test ./...` y `go run ./cmd/api` + `curl http://localhost:8080/salud`.
6. Commit: `git add . && git commit -m "Paso 2: servidor HTTP con Gin y endpoint de salud"`

---

## 7. Preparación para las próximas entregas

- **`/salud` devuelve 503 si una dependencia falla.** En la Entrega 3, el balanceador de carga de GCP usará exactamente ese código para dejar de enviar tráfico a una instancia enferma.
- **El apagado ordenado** permite reiniciar o escalar instancias (Entrega 3) y desplegar versiones nuevas en Cloud Run (Entrega 4) sin cortar peticiones en curso.
- **El puerto sale de `APP_PUERTO`.** Cloud Run asigna el puerto con una variable de entorno; bastará con mapearla.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Stack: Go + Gin | Servidor HTTP con Gin. |
| RNF-01: backend sólido | Tiempos máximos, recuperación de pánicos y apagado ordenado. |
| Despliegue reproducible (pasos 5 a 7) | `/salud` es la base de los healthchecks de Docker. |
