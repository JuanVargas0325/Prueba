# Paso 55 · Worker

> Rama `paso-a-paso` · paso 55 de 65 · anterior: **Paso 54 · Generación asíncrona** · siguiente: **Paso 56 · Panel de la cola (Asynqmon)**

---

## 1. Qué se hizo en este paso

Se creó el **segundo programa** del sistema: el **worker**, que consume la cola de Redis y genera los reportes. Con él, el flujo de reportes funciona **de punta a punta**.

| Archivo | Cambio |
|---|---|
| `backend/cmd/worker/main.go` | **Nuevo.** Programa del worker: conecta todo y arranca el servidor de Asynq. |
| `backend/Dockerfile` | **Modificado.** Compila los dos programas y agrega la etapa `worker`. |
| `docker-compose.yml` | **Modificado.** Servicio `worker`, que comparte el volumen de archivos con la API. |
| `Makefile` | **Modificado.** `make logs` muestra la API y el worker. |

```
Profesor ── POST /reportes/lotes ──► API ── encola ──► Redis ──► Worker
                                                                  │ IA (simulado / OpenAI / Claude)
                                                                  │ PDF
                                                                  ▼
Profesor ◄── GET /reportes/{id}/pdf ── API ◄── volumen "archivos" ◄┘
```

---

## 2. Por qué

- **Separar procesos:** la API atiende peticiones HTTP rápidas; el worker hace el trabajo lento (IA y PDF). Si la IA tarda, la API no se entera. Si llegan muchos lotes, se agregan workers sin tocar la API.
- **Arquitectura §6 (flujo asíncrono)** del documento de arquitectura, y el stack Asynq + Redis del curso.

---

## 3. Explicación detallada

### 3.1 `cmd/worker/main.go`

```go
func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("servicio", "worker")
	if err := ejecutar(log); err != nil { ... os.Exit(1) }
}
```
- El mismo patrón `main → ejecutar` de la API (paso 0), con `servicio=worker` en todos sus logs.

```go
	db, err := basedatos.Conectar(ctx, cfg.BaseDatos, log)
	...
	almacen, err := almacenamiento.Nuevo(cfg.Almacenamiento)
	...
	resumidor, err := ia.Nuevo(cfg.LLM)
	if err != nil {
		return err
	}
	log.Info("proveedor de IA configurado", "proveedor", resumidor.Proveedor(), "modelo", resumidor.Modelo())
```
- La **misma configuración** que la API (`config.Cargar`). El worker no aplica migraciones (las aplica la API) y no crea el administrador.
- Crea el almacenamiento con la misma función `almacenamiento.Nuevo` que la API (paso 38): ambos usan siempre el mismo tipo de almacenamiento, y cuando se agregue Cloud Storage el worker no cambiará.
- **Si el proveedor de IA está mal configurado, el worker no arranca** y el log dice por qué (por ejemplo, "LLM_MODELO es obligatorio con el proveedor openai"). Es mejor fallar al inicio que en cada reporte.
- El log `proveedor de IA configurado` confirma qué modelo quedó activo.

```go
	redis := asynq.RedisClientOpt{Addr: cfg.Redis.Direccion, Password: cfg.Redis.Password, DB: cfg.Redis.DB}
	clienteCola := cola.NuevoCliente(redis, cola.Opciones{...})
	...
	generador := servicio.NuevoGeneradorReportes(
		repositorio.NuevoReportes(db),
		clienteCola,
		almacen,
		resumidor,
		pdf.NuevoGenerador(cfg.ZonaHoraria),
		servicio.NuevoReloj(cfg.ZonaHoraria),
		log,
	)
```
- El worker **también encola**: al procesar un lote, encola un trabajo por persona (paso 54).
- Aquí se conectan las implementaciones concretas con las interfaces del generador (como la API en el paso 15).

```go
	servidor := asynq.NewServer(redis, asynq.Config{
		Concurrency: cfg.Worker.Concurrencia,
		Queues:      map[string]int{cola.ColaReportes: 1},
		Logger:      nuevoLoggerAsynq(log),
	})
	if err := servidor.Start(cola.NuevoMux(generador, servicio.EsErrorPermanente, log)); err != nil {
		return err
	}
	log.Info("worker iniciado", "concurrencia", cfg.Worker.Concurrencia, "max_reintentos", cfg.Worker.MaxReintentos)

	<-ctx.Done()
	log.Info("deteniendo el worker")
	servidor.Shutdown()
	return nil
```
- `asynq.NewServer` crea el consumidor: `Concurrency` = cuántos trabajos a la vez (`WORKER_CONCURRENCIA`, 2 por defecto); `Queues` = qué colas atiende.
- `Start` arranca el procesamiento en segundo plano con el **mux** del paso 47, al que se le entrega el generador y la función que distingue errores permanentes (paso 54).
- `<-ctx.Done()` **espera** a Ctrl+C o `docker stop`. Entonces `Shutdown` deja de tomar trabajos nuevos y espera a que terminen los que están en curso. Un trabajo cortado a la mitad vuelve a la cola y se reintenta (por eso todo es idempotente).

```go
// loggerAsynq adapta slog a la interfaz de logs de Asynq.
type loggerAsynq struct{ log *slog.Logger }

func (l loggerAsynq) Debug(args ...any) { l.log.Debug(fmt.Sprint(args...)) }
...
func (l loggerAsynq) Fatal(args ...any) {
	l.log.Error(fmt.Sprint(args...))
	os.Exit(1)
}
```
- **Patrón adaptador:** Asynq tiene su propia interfaz de logs (`Debug`, `Info`, `Warn`, `Error`, `Fatal`). Este tipo la implementa reenviando todo a `slog`, para que los mensajes internos de Asynq salgan en el mismo formato JSON, con `componente=asynq`.

### 3.2 `Dockerfile`

```dockerfile
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /salida/api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /salida/worker ./cmd/worker
...
# --- Worker ------------------------------------------------------------------
FROM base AS worker
COPY --from=compilacion /salida/worker /usr/local/bin/worker
ENTRYPOINT ["/usr/local/bin/worker"]
```
- Se compilan **los dos programas en la misma etapa**: comparten la descarga de dependencias y la caché de compilación.
- La etapa `worker` reutiliza `base` (Debian 12 mínima, usuario `app`, `/data`). Es la razón por la que en el paso 5 se separó `base`.
- El worker **no expone puertos** ni tiene `HEALTHCHECK` HTTP: no atiende peticiones.

### 3.3 `docker-compose.yml`

```yaml
  worker:
    build:
      context: ./backend
      target: worker
    image: seguimiento-worker:local
    restart: unless-stopped
    env_file: .env
    environment:
      POSTGRES_HOST: postgres
      REDIS_DIRECCION: redis:6379
      ALMACENAMIENTO_RUTA: /data
    # Comparte el volumen con la API: el worker escribe los PDF y la API los sirve.
    volumes:
      - archivos:/data
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      # La API aplica las migraciones al arrancar.
      api:
        condition: service_healthy
```
- **El mismo volumen `archivos`** que la API: el worker escribe `reportes/<semana>/<id>.pdf` y la API lo lee al descargar.
- Espera a que la **API esté sana**, porque es la API quien crea las tablas.
- También se quitó el comentario que tenía `redis` desde el paso 47 (ya no hace falta: ahora la API y el worker lo usan).

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Proceso consumidor | Programa que toma trabajos de una cola y los ejecuta. |
| Concurrencia del worker | Cuántos trabajos procesa a la vez. |
| `<-ctx.Done()` | Bloquear hasta que llegue la señal de apagado. |
| Patrón adaptador | Un tipo que traduce una interfaz a otra (logs de Asynq → slog). |
| Volumen compartido | Dos contenedores leen y escriben los mismos archivos. |

---

## 5. Cómo comprobarlo

```bash
docker compose up --build -d --wait
docker compose ps                    # api, nginx, postgres, redis y worker
docker compose logs worker           # "proveedor de IA configurado" ... "worker iniciado"
```

De punta a punta (profesora, Laura vinculada y al menos una tarea en la semana, pasos 33 y 37):
1. `POST /api/v1/reportes/lotes` con `{"semana":"<fecha de esta semana>"}` → 202.
2. `GET /api/v1/reportes/lotes/<id>` → en uno o dos segundos, `"estado":"completado"`, con el reporte de Laura, su `resumen` y su `url_descarga`.
3. `GET <url_descarga>` → se descarga `reporte-laura-martinez-<semana>.pdf`.
4. `docker compose logs worker` → `lote iniciado`, `resumen generado`, `reporte generado`.

Para probar un fallo: ponga `LLM_PROVEEDOR=openai` **sin** `LLM_MODELO` y ejecute `docker compose up -d worker`. El worker no arranca y `docker compose logs worker` explica el motivo.

---

## 6. Pasos para lograr esta versión desde el paso 54

1. Crear `backend/cmd/worker/main.go` (sección 3.1).
2. En `backend/Dockerfile`: compilar también `./cmd/worker` y agregar la etapa `worker`; actualizar el comentario inicial.
3. En `docker-compose.yml`: el servicio `worker` (sección 3.3), quitar el comentario del servicio `redis` y actualizar el comentario inicial.
4. En el `Makefile`: `logs` muestra `api worker`.
5. `docker compose up --build -d --wait` y probar el flujo completo (sección 5).
6. Commit: `git add . && git commit -m "Paso 55: worker"`

---

## 7. Preparación para las próximas entregas

- **Escalado independiente (Entrega 3):** la API y el worker se escalan por separado. Más profesores consultando → más instancias de la API; más reportes → más workers o más concurrencia.
- **Volumen compartido → Cloud Storage:** con API y workers en máquinas distintas, el volumen local ya no sirve; se reemplaza el almacenamiento por Cloud Storage (paso 38, deuda técnica DT-02) sin tocar el generador.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-14.1 a RF-14.7 | El flujo de reportes funciona completo: lote, uno por persona, IA, PDF, almacenamiento y descarga. |
| Stack: Asynq + Redis, Docker Compose | Worker como servicio de Compose que consume la cola. |
| Arquitectura §6 Flujo asíncrono | API y worker desacoplados por la cola. |
