# Paso 49 · Endpoints de reportes

> Rama `paso-a-paso` · paso 49 de 65 · anterior: **Paso 48 · Servicio de reportes** · siguiente: **Paso 50 · Generador de PDF**

---

## 1. Qué se hizo en este paso

La API expone el módulo de reportes y se conecta a Redis:

| Método | Ruta | Rol | Descripción |
|---|---|---|---|
| POST | `/api/v1/reportes/lotes` | profesor | Solicita la generación de los reportes de una semana (`{"semana":"AAAA-MM-DD"}`). Responde **202 Accepted** y queda encolado. |
| GET | `/api/v1/reportes/lotes` | profesor o administrador | Lista lotes (`?semana=`, `?profesor_id=` para el administrador). |
| GET | `/api/v1/reportes/lotes/{id}` | profesor o administrador | Avance del lote y estado de cada reporte. |
| GET | `/api/v1/reportes` | profesor o administrador | Lista reportes (`?semana=`, `?usuario_id=`, `?lote_id=`, `?profesor_id=`). |
| GET | `/api/v1/reportes/{id}` | profesor o administrador | Detalle con el resumen. |
| GET | `/api/v1/reportes/{id}/pdf` | profesor o administrador | Descarga el PDF. |

| Archivo | Cambio |
|---|---|
| `backend/internal/handler/reportes.go` | **Nuevo.** Módulo de reportes. |
| `backend/internal/handler/reportes_test.go` | **Nuevo.** Seis pruebas del módulo. |
| `backend/internal/handler/archivos.go` | **Modificado.** `nombreArchivo`: nombre de persona → nombre de archivo seguro. |
| `backend/internal/handler/router.go` | **Modificado.** Servicio `Reportes` y su módulo (el router queda completo). |
| `backend/internal/handler/router_test.go` | **Modificado.** Rutas de reportes protegidas, permisos y prueba de nombres de archivo. |
| `backend/cmd/api/main.go` | **Modificado.** Repositorio de reportes, **cliente de la cola** y Redis en `/salud`. |
| `docker-compose.yml` | **Modificado.** La API recibe `REDIS_DIRECCION=redis:6379` y espera a que Redis esté sano. |

```bash
curl -i -X POST localhost:8080/api/v1/reportes/lotes -H "Authorization: Bearer $TOKEN_PROFESOR" \
  -H 'Content-Type: application/json' -d '{"semana":"2026-10-08"}'
# HTTP/1.1 202 Accepted
# Location: /api/v1/reportes/lotes/d9d3...
# {"id":"d9d3...","semana_inicio":"2026-10-05","semana_fin":"2026-10-11","estado":"pendiente","total_reportes":0,...}
```

Como todavía no hay worker (paso 55), el lote queda **pendiente** en la cola de Redis.

---

## 2. Por qué

- **RF-14.1:** el profesor dispara la generación **manualmente**.
- **202 Accepted** es el código HTTP para "recibido, se procesará después". Con la cabecera `Location` el cliente sabe dónde consultar el avance (`GET /reportes/lotes/{id}`), sin esperar con la conexión abierta.
- **RF-14.6:** los reportes terminados se descargan como PDF.

---

## 3. Explicación detallada

### 3.1 Rutas

```go
func (h *HandlerReportes) Registrar(g *gin.RouterGroup) {
	r := g.Group("/reportes", RequiereRol(dominio.RolProfesor, dominio.RolAdministrador))
	r.POST("/lotes", RequiereRol(dominio.RolProfesor), h.SolicitarLote)
	r.GET("/lotes", h.ListarLotes)
	r.GET("/lotes/:id", h.ObtenerLote)
	r.GET("", h.ListarReportes)
	r.GET("/:id", h.ObtenerReporte)
	r.GET("/:id/pdf", h.DescargarPDF)
}
```
- Todo el grupo es para profesores y administradores; solicitar un lote, solo para profesores. Los middleware se **acumulan**: el `POST` pasa por los dos `RequiereRol`.

### 3.2 Formatos

```go
type loteJSON struct {
	ID                  string             `json:"id"`
	...
	Estado              dominio.EstadoLote `json:"estado"`
	TotalReportes       int                `json:"total_reportes"`
	ReportesCompletados int                `json:"reportes_completados"`
	ReportesFallidos    int                `json:"reportes_fallidos"`
	Error               string             `json:"error,omitempty"`
	CreadoEn            time.Time          `json:"creado_en"`
	FinalizadoEn        *time.Time         `json:"finalizado_en"`
	Reportes            []reporteJSON      `json:"reportes,omitempty"`
}
```
- `Reportes` solo se llena en el detalle de un lote (`omitempty` lo omite en el listado).

```go
func aReporteJSON(r dominio.ReporteSemanal) reporteJSON {
	j := reporteJSON{...}
	if r.Estado == dominio.ReporteCompletado {
		j.URLDescarga = "/api/v1/reportes/" + r.ID + "/pdf"
	}
	return j
}
```
- La URL de descarga solo aparece cuando el PDF existe.

### 3.3 `SolicitarLote`

```go
type peticionLote struct {
	// Cualquier fecha (AAAA-MM-DD) de la semana que se quiere reportar.
	Semana string `json:"semana"`
}
...
	if strings.TrimSpace(p.Semana) == "" {
		responderError(c, h.log, apperr.Validacion("semana es obligatoria (AAAA-MM-DD)"))
		return
	}
	lote, err := h.servicio.SolicitarLote(c.Request.Context(), ActorDe(c), p.Semana)
	...
	c.Header("Location", "/api/v1/reportes/lotes/"+lote.ID)
	c.JSON(http.StatusAccepted, aLoteJSON(lote))
```

### 3.4 Descarga del PDF con nombre legible

```go
	nombre := fmt.Sprintf("reporte-%s-%s.pdf", nombreArchivo(rep.Persona), fecha(rep.SemanaInicio))
	servirArchivo(c, archivo, rep.TamanoBytes, "application/pdf", nombre)
```
- El archivo se descarga como `reporte-jose-pena-2026-10-05.pdf`.
- `nombreArchivo` (en `archivos.go`) quita las tildes reutilizando `quitarTildes` (paso 40), pasa a minúsculas, cambia espacios por guiones y descarta cualquier otro carácter. Un nombre vacío o raro termina en `persona`; uno como `../../etc` queda `etc` (nada de rutas).
- Se usa `servirArchivo` (paso 40): descarga como adjunto, `nosniff`, sin caché.

### 3.5 `main.go`: conexión con Redis

```go
	clienteCola := cola.NuevoCliente(
		asynq.RedisClientOpt{Addr: cfg.Redis.Direccion, Password: cfg.Redis.Password, DB: cfg.Redis.DB},
		cola.Opciones{MaxReintentos: cfg.Worker.MaxReintentos, TimeoutReporte: cfg.Worker.TimeoutReporte},
	)
	defer func() {
		if err := clienteCola.Cerrar(); err != nil {
			log.Warn("cerrando el cliente de la cola", "error", err)
		}
	}()
	...
		Reportes:      servicio.NuevoReportes(reportes, clienteCola, almacen, reloj, log),
	...
		Salud: map[string]handler.ComprobadorSalud{
			"postgres": db.Ping,
			"redis":    func(context.Context) error { return clienteCola.Ping() },
		},
```
- El cliente de la cola es el `Encolador` del servicio (paso 48).
- El `defer` con una función anónima permite **registrar** el error al cerrar (en lugar de ignorarlo, lo que el linter `errcheck` señalaría).
- `/salud` ahora comprueba **también Redis**. `clienteCola.Ping` no recibe contexto, así que se **adapta** con una función anónima a la forma `func(context.Context) error` de un `ComprobadorSalud`.

```yaml
  api:
    environment:
      POSTGRES_HOST: postgres
      REDIS_DIRECCION: redis:6379
    ...
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
```

### 3.6 Pruebas

En `router_test.go` (ya en su versión final):
- `/api/v1/reportes` exige token;
- un monitor no consulta reportes y un administrador no solicita lotes (403);
- `TestNombresDeArchivo`: `"José Peña Núñez"` → `jose-pena-nunez`, `"  Ana  María "` → `ana-maria`, `"../../etc"` → `etc`, `""` → `persona`, y la cabecera `Content-Disposition` de un nombre con eñe.

En `reportes_test.go`:

| Prueba | Qué verifica |
|---|---|
| `TestSolicitarLoteResponde202` | 202, semana normalizada en la respuesta, `Location` correcto y la fecha original llega al servicio. |
| `TestSolicitarLoteValidaElCuerpo` | Cuerpo vacío, `{}`, campo desconocido y semana numérica → 400. |
| `TestErroresDelServicioSeTraducen` | Conflicto → 409 con su mensaje; regla de negocio → 422; error inesperado → 500 **sin** el detalle interno. |
| `TestIdentificadoresInvalidosResponden400` | IDs que no son UUID y una semana `ayer` → 400. |
| `TestObtenerLoteIncluyeURLDeDescarga` | El reporte completado trae su `url_descarga`. |
| `TestDescargarPDF` | `application/pdf`, `nosniff`, nombre `reporte-jose-pena-2026-10-05.pdf` y el contenido. |

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| 202 Accepted + `Location` | Respuesta para trabajos asíncronos: dónde consultar el avance. |
| Middleware acumulados | Los del grupo y los de la ruta se ejecutan en orden. |
| Adaptar una función | `func(context.Context) error { return x.Ping() }`. |
| `defer` con función anónima | Para registrar el error de un cierre. |

---

## 5. Cómo comprobarlo

```bash
make pruebas
docker compose up --build -d --wait
curl -s localhost:8080/salud      # {"estado":"ok","componentes":{"postgres":"ok","redis":"ok"}}
```

Con el token de un profesor:
1. `POST /api/v1/reportes/lotes` con `{"semana":"2026-10-08"}` → **202**, estado `pendiente`.
2. Repetir → **409** "ya hay un lote en proceso para la semana del 2026-10-05".
3. Con el token de un monitor → **403**.
4. `docker compose exec redis redis-cli LLEN 'asynq:{reportes}:pending'` → `1` (el trabajo espera al worker del paso 55).

---

## 6. Pasos para lograr esta versión desde el paso 48

1. En `archivos.go`: `nombreArchivo`.
2. Crear `backend/internal/handler/reportes.go` y `reportes_test.go`.
3. En `router.go`: campo `Reportes` y `&HandlerReportes{...}`.
4. En `router_test.go`: `/api/v1/reportes` en las rutas protegidas, los dos casos de permisos y `TestNombresDeArchivo`.
5. En `main.go`: imports `asynq` y `cola`; `reportes = repositorio.NuevoReportes(db)`; `clienteCola` con su `defer`; `Reportes: servicio.NuevoReportes(...)`; `"redis"` en `Salud`.
6. En `docker-compose.yml`: `REDIS_DIRECCION: redis:6379` y la dependencia de `redis` en la API.
7. `make pruebas lint`, `docker compose up --build -d --wait` y commit: `git add . && git commit -m "Paso 49: endpoints de reportes"`

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-14.1 Ejecución manual por lotes | `POST /api/v1/reportes/lotes` → 202 y trabajo encolado. |
| RF-14.6 Descarga posterior | `GET /api/v1/reportes/{id}/pdf`. |
| RF-14.7 Semana concreta | `semana` normalizada al lunes. |
| Stack: Asynq + Redis | La API encola en Redis y lo reporta en `/salud`. |
| RN-03 / RN-05 Visibilidad | Profesor y administrador según el servicio. |
