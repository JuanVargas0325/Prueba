# Paso 47 · Cola de tareas con Asynq y Redis

> Rama `paso-a-paso` · paso 47 de 65 · anterior: **Paso 46 · Repositorio de reportes** · siguiente: **Paso 48 · Servicio de reportes**

---

## 1. Qué se hizo en este paso

Se agregó la **cola de tareas asíncronas**: la API podrá dejar trabajos pendientes en Redis y un worker los ejecutará en segundo plano, con reintentos.

| Archivo | Cambio |
|---|---|
| `backend/go.mod`, `go.sum` | Nueva dependencia `github.com/hibiken/asynq v0.26.0` (trae el cliente de Redis). |
| `backend/internal/cola/cola.go` | **Nuevo.** Cliente que **encola** trabajos (`EncolarLote`, `EncolarReporte`) y enrutador que los **ejecuta** (`NuevoMux`) con la política de reintentos. |
| `backend/internal/cola/cola_test.go` | **Nuevo.** Pruebas del enrutamiento y de la política de fallos. |
| `backend/internal/config/config.go` | **Modificado.** Secciones `Redis` (dirección, contraseña, base) y `Worker` (concurrencia, reintentos, tiempo máximo por reporte). |
| `backend/internal/config/config_test.go` | **Modificado.** Valores por defecto y formatos de las nuevas variables. |
| `docker-compose.yml` | **Modificado.** Servicio `redis` (Debian 12) con persistencia y healthcheck. |
| `.env.example` | **Modificado.** Secciones `Redis` y `Generación de reportes (worker)`. |

---

## 2. Por qué

- **Stack del curso:** Asynq sobre Redis (o RabbitMQ). Se eligió Redis por su simplicidad y porque Asynq es la librería de colas más usada en Go.
- **RF-14.1 / RN-41:** los reportes se generan **por lotes**, y cada uno puede tardar varios segundos (llamada a la IA + PDF). Si la API los generara dentro de la petición HTTP, el profesor esperaría minutos, la conexión podría cortarse y un pico de solicitudes bloquearía la API. Con una cola:
  1. la API **registra** el trabajo en Redis y responde de inmediato (202 Accepted);
  2. un **worker** (otro proceso) toma los trabajos y los ejecuta;
  3. si algo falla por un motivo transitorio (la IA saturada), **se reintenta** automáticamente con espera creciente.

```
API ──encola──► Redis (cola "reportes") ──entrega──► Worker ──► IA, PDF, almacenamiento
```

---

## 3. Explicación detallada de `cola.go`

### 3.1 Tipos de trabajo y su carga

```go
const (
	TipoProcesarLote   = "reportes:procesar_lote"
	TipoGenerarReporte = "reportes:generar_reporte"
	ColaReportes       = "reportes"
)

type cargaLote struct {
	LoteID string `json:"lote_id"`
}

type cargaReporte struct {
	ReporteID string `json:"reporte_id"`
}
```
- Dos clases de trabajo: **procesar un lote** (repartirlo en un trabajo por persona) y **generar el reporte** de una persona.
- La **carga** de cada trabajo es un JSON mínimo con el **identificador**, no los datos. El worker lee todo lo demás de la base de datos en el momento de ejecutar, así trabaja siempre con datos actuales y la cola no guarda información personal.

### 3.2 El cliente (lo usa la API)

```go
type Opciones struct {
	MaxReintentos  int
	TimeoutReporte time.Duration
}

type Cliente struct {
	asynq *asynq.Client
	op    Opciones
}

func NuevoCliente(redis asynq.RedisClientOpt, op Opciones) *Cliente { ... }
func (c *Cliente) Cerrar() error { return c.asynq.Close() }
func (c *Cliente) Ping() error { return c.asynq.Ping() }
```
- `Ping` se usará en `/salud` (paso 49): si Redis cae, la API se reporta degradada.

```go
func (c *Cliente) encolar(ctx context.Context, tipo string, carga any, id string, timeout time.Duration) error {
	datos, err := json.Marshal(carga)
	...
	_, err = c.asynq.EnqueueContext(ctx, asynq.NewTask(tipo, datos),
		asynq.Queue(ColaReportes),
		asynq.MaxRetry(c.op.MaxReintentos),
		asynq.Timeout(timeout),
		// El identificador evita encolar dos veces el mismo trabajo si una tarea se reintenta.
		asynq.TaskID(id),
		asynq.Retention(24*time.Hour),
	)
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil // ya estaba en la cola
	}
	...
}
```

| Opción | Para qué |
|---|---|
| `Queue("reportes")` | Cola con nombre: separa estos trabajos de otros futuros. |
| `MaxRetry(n)` | Cuántas veces reintentar ante un error (`REPORTE_MAX_REINTENTOS`, 3 por defecto). |
| `Timeout(t)` | Tiempo máximo de ejecución: 1 minuto para repartir un lote y `REPORTE_TIMEOUT` (5 min) para generar un reporte. Si se supera, se cancela su `context` y cuenta como fallo. |
| `TaskID("reporte:<id>")` | Identificador único: si se intenta encolar **otra vez** el mismo reporte (por ejemplo, porque el trabajo del lote se reintentó), Asynq lo rechaza con `ErrTaskIDConflict` y se ignora. **Evita duplicados en la cola.** |
| `Retention(24h)` | Los trabajos terminados se conservan un día para consultarlos en el panel (paso 56). |

### 3.3 El enrutador (lo usa el worker)

```go
type Procesador interface {
	ProcesarLote(ctx context.Context, loteID string) error
	GenerarReporte(ctx context.Context, reporteID string) error
	RegistrarFalloLote(ctx context.Context, loteID string, causa error) error
	RegistrarFalloReporte(ctx context.Context, reporteID string, causa error) error
}
```
- Lo que el worker sabe hacer. Lo implementará el generador de reportes del servicio (paso 54). El paquete `cola` no conoce los reportes: solo conecta tipos de trabajo con métodos.

```go
func NuevoMux(p Procesador, esPermanente func(error) bool, log *slog.Logger) *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TipoGenerarReporte, func(ctx context.Context, t *asynq.Task) error {
		var carga cargaReporte
		if err := json.Unmarshal(t.Payload(), &carga); err != nil || carga.ReporteID == "" {
			return fmt.Errorf("carga inválida: %w", asynq.SkipRetry)
		}
		return ejecutar(ctx, log, "reporte_id", carga.ReporteID, esPermanente,
			func(ctx context.Context) error { return p.GenerarReporte(ctx, carga.ReporteID) },
			func(ctx context.Context, err error) error { return p.RegistrarFalloReporte(ctx, carga.ReporteID, err) })
	})
	... (igual para TipoProcesarLote) ...
	return mux
}
```
- Un **mux** (como el router de Gin, pero para trabajos): según el tipo, llama al método correspondiente.
- Una carga ilegible nunca va a funcionar por más que se reintente: se devuelve envuelta en `asynq.SkipRetry`, que le dice a Asynq **no reintentar**.
- `esPermanente` es una **función** que decide si un error vale la pena reintentarlo; la definirá el servicio (paso 54) porque solo él sabe qué errores son permanentes.

### 3.4 La política de reintentos

```go
func ejecutar(ctx context.Context, log *slog.Logger, campo, id string, esPermanente func(error) bool,
	trabajo func(context.Context) error, registrarFallo func(context.Context, error) error) error {

	err := trabajo(ctx)
	if err == nil {
		return nil
	}

	reintento, _ := asynq.GetRetryCount(ctx)
	maximo, _ := asynq.GetMaxRetry(ctx)
	if !esPermanente(err) && reintento < maximo {
		log.Warn("trabajo fallido, se reintentará", campo, id, "intento", reintento+1, "error", err)
		return err
	}

	log.Error("trabajo fallido definitivamente", campo, id, "intento", reintento+1, "error", err)
	// Se usa un contexto nuevo: el del trabajo puede haber expirado.
	ctxFallo, cancelar := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancelar()
	if errFallo := registrarFallo(ctxFallo, err); errFallo != nil {
		log.Error("no fue posible registrar el fallo", campo, id, "error", errFallo)
	}
	return fmt.Errorf("%w: %w", err, asynq.SkipRetry)
}
```
- **Error transitorio y quedan intentos** → se devuelve el error; Asynq **reintenta** con espera exponencial (cada vez más tiempo entre intentos).
- **Error permanente** (por ejemplo, la clave de la IA es inválida) **o último intento** → se **registra el fallo** en la base de datos (el reporte queda `fallido` con el motivo) y se le dice a Asynq que no reintente.
- `asynq.GetRetryCount` / `GetMaxRetry` leen del `context` cuántos intentos van y cuántos hay.
- **`context.WithoutCancel(ctx)` + 10 s:** si el trabajo falló por **tiempo agotado**, su contexto ya está cancelado y cualquier consulta con él fallaría. Para poder guardar el fallo se crea un contexto nuevo (que conserva los valores pero no la cancelación) con su propio límite.
- `fmt.Errorf("%w: %w", err, asynq.SkipRetry)`: desde Go 1.20 un error puede **envolver dos** errores a la vez; así se conserva la causa original **y** la marca de no reintentar.
- Las funciones `trabajo` y `registrarFallo` llegan como parámetros (*closures* creadas en `NuevoMux`); `ejecutar` es la misma lógica para los dos tipos de trabajo.

### 3.5 Pruebas (`cola_test.go`)

Con un procesador falso, se entrega cada trabajo directamente al mux con `mux.ProcessTask` (sin Redis):

| Prueba | Qué verifica |
|---|---|
| `TestMuxEnrutaCadaTipoDeTarea` | Cada tipo de trabajo llega al método correcto con su ID. |
| `TestCargaInvalidaNoSeReintenta` | Una carga sin ID termina con `SkipRetry`. |
| `TestErrorPermanenteRegistraElFalloSinReintentar` | Un error permanente registra el fallo y no se reintenta. |
| `TestErrorTransitorioEnElUltimoIntentoRegistraElFallo` | Fuera del servidor de Asynq no hay datos de reintento, así que cuenta como último intento: también registra el fallo. |

---

## 4. Configuración y Docker

```go
type Redis struct {
	Direccion string
	Password  string
	DB        int
}

type Worker struct {
	Concurrencia   int
	MaxReintentos  int
	TimeoutReporte time.Duration
}
```

| Variable | Por defecto | Uso |
|---|---|---|
| `REDIS_DIRECCION` | `localhost:6379` (Compose usará `redis:6379`) | Dónde está Redis. |
| `REDIS_PASSWORD`, `REDIS_DB` | vacío, 0 | Credenciales y número de base de datos de Redis. |
| `WORKER_CONCURRENCIA` | 2 | Reportes en paralelo por worker (paso 55). |
| `REPORTE_MAX_REINTENTOS` | 3 | Reintentos ante errores transitorios. |
| `REPORTE_TIMEOUT` | 5m | Tiempo máximo por reporte. |

```yaml
  redis:
    image: redis:7-bookworm
    restart: unless-stopped
    command: ["redis-server", "--appendonly", "yes"]
    volumes:
      - datos-redis:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
```
- Redis 7 sobre **Debian 12**.
- `--appendonly yes`: Redis escribe cada operación en un archivo en disco (en el volumen `datos-redis`). Si el contenedor se reinicia, **los trabajos pendientes no se pierden**.
- No publica puertos: solo la API y el worker lo alcanzan por la red interna.

---

## 5. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Cola de mensajes | Desacopla quien pide un trabajo de quien lo ejecuta. |
| Productor / consumidor | La API encola (productor); el worker procesa (consumidor). |
| Reintentos con espera exponencial | Ante fallos transitorios, reintentar cada vez más espaciado. |
| Error permanente vs. transitorio | Reintentar solo lo que puede funcionar después. |
| Entrega "al menos una vez" | Un trabajo puede ejecutarse dos veces; por eso todo es idempotente (paso 46). |
| `%w` doble | Un error que envuelve dos causas. |
| `context.WithoutCancel` | Contexto que conserva valores pero ignora la cancelación del padre. |

---

## 6. Cómo comprobarlo

```bash
make pruebas                       # incluye ok seguimiento/internal/cola
docker compose up --build -d --wait
docker compose ps                  # redis "(healthy)"
docker compose exec redis redis-cli ping    # PONG
```

En el paso 49 la API empezará a encolar lotes y en el paso 55 el worker los procesará.

---

## 7. Pasos para lograr esta versión desde el paso 46

1. `go get github.com/hibiken/asynq@v0.26.0 && go mod tidy` (desde `backend/`).
2. Crear `backend/internal/cola/cola.go` y `cola_test.go` (sección 3).
3. En `config.go`: tipos `Redis` y `Worker`, campos en `Config` y su carga en `Cargar()`. En `config_test.go`: el valor por defecto de `REPORTE_MAX_REINTENTOS` y los formatos inválidos de `WORKER_CONCURRENCIA` y `REPORTE_TIMEOUT`.
4. En `docker-compose.yml`: el servicio `redis` y el volumen `datos-redis`.
5. En `.env.example` (y su `.env`): las secciones `Redis` (después de PostgreSQL) y `Generación de reportes (worker)` (al final).
6. `make pruebas lint`, `docker compose up --build -d --wait` y commit: `git add . && git commit -m "Paso 47: cola de tareas con Asynq y Redis"`

---

## 8. Preparación para las próximas entregas

- **Redis administrado:** en GCP puede usarse Memorystore for Redis cambiando solo `REDIS_DIRECCION` (y `REDIS_PASSWORD`).
- **Escalar el procesamiento** (Entrega 3) es agregar workers o subir `WORKER_CONCURRENCIA`: la cola reparte el trabajo y la idempotencia del paso 46 evita inconsistencias.
- **Entrega 4 (serverless):** el `Procesador` no depende de Asynq; los mismos métodos podrían invocarse desde Pub/Sub o Cloud Tasks reemplazando solo el paquete `cola`.

---

## 9. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Stack: Asynq + Redis | Cola `reportes` en Redis 7 (Debian 12). |
| RF-14.1 / RN-41 Generación por lotes, asíncrona | Trabajos de lote y de reporte. |
| RNF-01 Backend sólido | Reintentos, tiempos máximos y registro de fallos definitivos. |
