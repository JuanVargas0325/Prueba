# Paso 56 · Panel de la cola (Asynqmon)

> Rama `paso-a-paso` · paso 56 de 65 · anterior: **Paso 55 · Worker** · siguiente: **Paso 57 · Modelo local con Ollama**

---

## 1. Qué se hizo en este paso

Se agregó **Asynqmon**, el panel web oficial de Asynq, como servicio **opcional** de Docker Compose.

| Archivo | Cambio |
|---|---|
| `docker-compose.yml` | **Modificado.** Servicio `asynqmon` en el perfil `monitoreo` y nota en el comentario inicial. |

```bash
docker compose --profile monitoreo up -d
# Abrir http://localhost:8081
```

---

## 2. Por qué

La cola es invisible desde la API: si un reporte tarda o falla, ¿está esperando, procesándose, reintentándose o ya falló? Asynqmon muestra en el navegador:
- las colas y cuántos trabajos hay **pendientes**, **activos**, **programados para reintento**, **fallidos** (archivados) y **completados**;
- el detalle de cada trabajo: su carga (el `lote_id` o `reporte_id`), el número de intentos y el **último error**;
- permite reintentar o borrar trabajos a mano.

Es útil para depurar, para la **sustentación** (mostrar el procesamiento asíncrono en vivo) y durante las pruebas de carga.

---

## 3. Explicación detallada

```yaml
  asynqmon:
    profiles: ["monitoreo"]
    image: hibiken/asynqmon:latest
    command: ["--redis-addr=redis:6379"]
    ports:
      - "8081:8080"
    depends_on:
      - redis
```
- **`profiles: ["monitoreo"]`**: un servicio con perfil **no se levanta** con `docker compose up` normal; solo cuando se pide `--profile monitoreo`. Así el panel no consume recursos ni abre un puerto si no se necesita.
- Se conecta al **mismo Redis** que la API y el worker (`--redis-addr=redis:6379`). Asynqmon no guarda nada: lee lo que Asynq ya tiene en Redis.
- `"8081:8080"`: el panel escucha en el 8080 de su contenedor y se publica en el **8081** de su equipo (el 8080 ya lo usa Nginx).
- `depends_on: - redis` (forma corta): solo espera a que el contenedor de Redis **arranque**, sin exigir el healthcheck.
- La retención de 24 h configurada al encolar (paso 47) es lo que permite ver aquí los trabajos **completados**.

```
# Perfiles opcionales:
#   docker compose --profile monitoreo up -d   -> Asynqmon (panel de la cola) en http://localhost:8081
```
- El comentario inicial del archivo documenta cómo activarlo.

**Seguridad:** el panel **no tiene autenticación**. Está pensado para uso local; no debe publicarse en un servidor accesible desde internet.

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Perfiles de Compose | Servicios que solo se levantan si se activa su perfil (`--profile`). |
| Observabilidad | Poder ver el estado interno del sistema (aquí, la cola). |
| `depends_on` corto | Esperar a que arranque otro servicio, sin condición de salud. |

---

## 5. Cómo comprobarlo

```bash
docker compose --profile monitoreo up --build -d --wait
docker compose ps                                     # aparece asynqmon con 0.0.0.0:8081->8080
```

1. Abra <http://localhost:8081>.
2. Solicite un lote de reportes (paso 55). En la cola `reportes` verá pasar los trabajos `reportes:procesar_lote` y `reportes:generar_reporte` a **Completed**.
3. Para ver reintentos: configure un proveedor de IA con una URL inalcanzable (por ejemplo `LLM_PROVEEDOR=openai`, `LLM_MODELO=x`, `LLM_URL_BASE=http://no-existe:1234/v1`), reinicie el worker y solicite otro lote. Los trabajos aparecerán en **Retry** con el error de red, y después de los reintentos, en **Archived**.

Para detenerlo: `docker compose --profile monitoreo down` (o `stop asynqmon`).

---

## 6. Pasos para lograr esta versión desde el paso 55

1. En `docker-compose.yml`, antes de `volumes:`, agregar el servicio `asynqmon` (sección 3).
2. En el comentario inicial, agregar la sección "Perfiles opcionales" con la línea de `monitoreo`.
3. `docker compose --profile monitoreo up -d` y abrir <http://localhost:8081>.
4. Commit: `git add docker-compose.yml README.md && git commit -m "Paso 56: panel de la cola"`

---

## 7. Preparación para las próximas entregas

- En las **pruebas de carga** con generación de reportes, el panel muestra la profundidad de la cola y la tasa de procesamiento: dos medidas clave para dimensionar workers.
- En la nube, si se mantiene Asynq, el panel puede correr en una máquina interna o detrás de un proxy con autenticación.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Stack: Asynq | Herramienta oficial de observación de la cola. |
| Sustentación (lineamientos §3) | Permite mostrar el procesamiento asíncrono en vivo. |
