# Paso 40 · Endpoints de adjuntos (multipart)

> Rama `paso-a-paso` · paso 40 de 65 · anterior: **Paso 39 · Adjuntos en el servicio** · siguiente: **Paso 41 · Dominio: seguimiento semanal y "sin reporte"**

---

## 1. Qué se hizo en este paso

Los archivos de evidencia ya se pueden enviar y descargar por HTTP:

| Método | Ruta | Rol | Descripción |
|---|---|---|---|
| POST | `/api/v1/tareas` | monitor o asistente | Además de JSON, acepta un formulario **multipart** con el campo `datos` (el JSON de la tarea) y los archivos en `adjuntos`: tarea y evidencia en una sola petición. |
| POST | `/api/v1/tareas/{id}/adjuntos` | monitor o asistente (dueño) | Agrega archivos a una tarea modificable. |
| GET | `/api/v1/tareas/{id}/adjuntos/{adjunto_id}` | quien pueda ver la tarea | Descarga el archivo. |
| DELETE | `/api/v1/tareas/{id}/adjuntos/{adjunto_id}` | monitor o asistente (dueño) | Elimina un adjunto. |

| Archivo | Cambio |
|---|---|
| `backend/internal/handler/archivos.go` | **Nuevo.** Lectura de formularios multipart, límite del cuerpo y entrega segura de archivos. |
| `backend/internal/handler/tareas.go` | **Modificado.** `Crear` acepta multipart; rutas y endpoints de adjuntos; cada tarea muestra sus adjuntos con la URL de descarga. |
| `backend/internal/handler/router.go` | **Modificado.** `MaxCuerpoArchivosMB` y memoria máxima para formularios. |
| `backend/internal/handler/router_test.go` y `modulos_test.go` | **Modificados.** Pruebas de multipart, 413 y adjuntos. |
| `backend/cmd/api/main.go` | **Modificado.** Límite del cuerpo = 5 adjuntos × `ADJUNTO_MAX_MB` + 1 MB. |
| `nginx/nginx.conf` | **Modificado.** `client_max_body_size 51m`. |
| `.env.example` | **Modificado.** Nota sobre el límite de Nginx. |
| `backend/go.mod` | `golang.org/x/text` pasa a ser dependencia directa (quitar tildes de los nombres de archivo). |

```bash
curl -s -X POST localhost:8080/api/v1/tareas -H "Authorization: Bearer $TOKEN_MONITOR" \
  -F 'datos={"vinculacion_id":"<id>","titulo":"Con evidencia","horas_invertidas":1}' \
  -F 'adjuntos=@evidencia.txt'
# {... "cantidad_adjuntos":1,"adjuntos":[{"id":"450e...","nombre":"evidencia.txt","tipo_contenido":"text/plain; charset=utf-8",
#   "tamano_bytes":23,"url_descarga":"/api/v1/tareas/ec19.../adjuntos/450e..."}] ...}
```

---

## 2. Por qué

- **RF-06.4** queda completo por la API.
- **`multipart/form-data`** es el formato estándar de los navegadores para enviar archivos. JSON no sirve para binarios (habría que codificarlos en Base64, que ocupa un 33 % más).
- **Dos límites coordinados:** Nginx corta peticiones enormes antes de que lleguen a la API, y la API también limita (por si algún día se expone sin Nginx). Además, cada archivo tiene su propio máximo (paso 39).
- **Descargas seguras:** un archivo subido por un usuario nunca debe ejecutarse en el navegador de otro (por ejemplo, un SVG o HTML con código). Siempre se entrega como descarga.

---

## 3. Explicación detallada

### 3.1 `archivos.go`: lectura del formulario

```go
const campoAdjuntos = "adjuntos"

func limitarCuerpo(c *gin.Context, maxBytes int64) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
}

func esMultipart(c *gin.Context) bool {
	return strings.HasPrefix(c.ContentType(), "multipart/form-data")
}
```
- `http.MaxBytesReader` envuelve el cuerpo: si se leen más bytes del máximo, la lectura falla con `*http.MaxBytesError`, que `responderError` (paso 17) traduce en **413 Payload Too Large**.

```go
func formularioMultipart(c *gin.Context) (*multipart.Form, error) {
	form, err := c.MultipartForm()
	if err != nil {
		var demasiadoGrande *http.MaxBytesError
		if errors.As(err, &demasiadoGrande) {
			return nil, err
		}
		return nil, apperr.Validacion("formulario multipart inválido")
	}
	return form, nil
}
```
- Interpreta el formulario. Gin guarda en memoria hasta `MaxMultipartMemory` (8 MB, ver 3.3) y el resto **en archivos temporales del sistema**, así un archivo grande no se queda en la RAM.

```go
func leerArchivos(form *multipart.Form) ([]servicio.ArchivoAdjunto, func(), error) {
	var abiertos []multipart.File
	cerrar := func() {
		for _, f := range abiertos {
			_ = f.Close()
		}
	}
	var archivos []servicio.ArchivoAdjunto
	for _, cabecera := range form.File[campoAdjuntos] {
		f, err := cabecera.Open()
		if err != nil {
			cerrar() // cierra los que ya se habían abierto
			return nil, func() {}, apperr.Validacion(...)
		}
		abiertos = append(abiertos, f)
		archivos = append(archivos, servicio.ArchivoAdjunto{Nombre: cabecera.Filename, Contenido: f})
	}
	return archivos, cerrar, nil
}
```
- Abre cada archivo del campo `adjuntos` y lo entrega al servicio como `servicio.ArchivoAdjunto` (nombre + lector).
- Devuelve **una función para cerrarlos** (`cerrar`); el handler la llama con `defer cerrar()`. Es un patrón común en Go para recursos que hay que liberar.
- Si un archivo no se puede abrir, cierra los que ya abrió y devuelve una función **vacía** (`func() {}`): así quien la reciba puede llamarla siempre sin preguntar si hubo error.

### 3.2 `archivos.go`: entrega segura

```go
func servirArchivo(c *gin.Context, contenido io.Reader, tamano int64, tipo, nombre string) {
	c.DataFromReader(http.StatusOK, tamano, tipo, contenido, map[string]string{
		"Content-Disposition":    disposicionAdjunto(nombre),
		"X-Content-Type-Options": "nosniff",
		"Cache-Control":          "private, no-store",
	})
}
```
- `DataFromReader` envía el archivo **en flujo** (*streaming*), sin cargarlo entero en memoria.
- `Content-Disposition: attachment` → el navegador lo **descarga**, nunca lo abre ni ejecuta.
- `X-Content-Type-Options: nosniff` → el navegador no "adivina" el tipo a partir del contenido; usa el que dice el servidor.
- `Cache-Control: private, no-store` → ningún proxy ni caché intermedio guarda copias de evidencias personales.

```go
func disposicionAdjunto(nombre string) string {
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, nombreASCII(nombre), url.PathEscape(nombre))
}
```
- **RFC 6266:** `filename` lleva una versión **solo ASCII** (para navegadores antiguos) y `filename*` el nombre original en UTF-8 codificado. Un archivo `Informe Final ñ.pdf` llega como `Informe_Final_n.pdf` o con su nombre exacto, según el navegador.

```go
func quitarTildes(texto string) string {
	sinTildes, _, _ := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), texto)
	return sinTildes
}

func nombreASCII(nombre string) string {
	var b strings.Builder
	for _, r := range quitarTildes(nombre) {
		...
	}
	...
}
```
- `quitarTildes` usa la técnica estándar de Unicode: `NFD` separa cada letra de su tilde (`é` → `e` + `´`), `runes.Remove(runes.In(unicode.Mn))` borra las marcas diacríticas y `NFC` vuelve a componer. Usa el paquete `golang.org/x/text`. Está en su propia función porque el paso 49 la reutiliza para el nombre de los PDF.
- `nombreASCII` recorre el texto sin tildes y solo deja letras, números, `.`, `-` y `_` (espacios → `_`).

### 3.3 `router.go`

```go
	// MaxCuerpoArchivosMB limita las peticiones que traen archivos adjuntos.
	MaxCuerpoArchivosMB int
	...
	r.MaxMultipartMemory = 8 << 20 // el resto de cada archivo se guarda en disco temporal
	...
	&HandlerTareas{servicio: d.Tareas, maxCuerpo: int64(d.MaxCuerpoArchivosMB) << 20, log: d.Log},
```

### 3.4 `tareas.go`: `Crear` con JSON o multipart

```go
func (h *HandlerTareas) leerPeticionTarea(c *gin.Context) (p peticionTarea, archivos []servicio.ArchivoAdjunto, cerrar func(), err error) {
	cerrar = func() {}
	if !esMultipart(c) {
		return p, nil, cerrar, leerJSON(c, &p)
	}
	limitarCuerpo(c, h.maxCuerpo)
	form, err := formularioMultipart(c)
	if err != nil {
		return p, nil, cerrar, err
	}
	datos := form.Value["datos"]
	if len(datos) != 1 {
		return p, nil, cerrar, apperr.Validacion("el formulario debe incluir el campo \"datos\" con el JSON de la tarea")
	}
	if err := decodificarJSON(strings.NewReader(datos[0]), &p); err != nil {
		return p, nil, cerrar, err
	}
	archivos, cerrar, err = leerArchivos(form)
	return p, archivos, cerrar, err
}

func (h *HandlerTareas) Crear(c *gin.Context) {
	p, archivos, cerrar, err := h.leerPeticionTarea(c)
	defer cerrar()
	if err != nil {
		responderError(c, h.log, err)
		return
	}
	... (igual que en el paso 37) ...
	t, err := h.servicio.Crear(c.Request.Context(), actor, servicio.NuevaTarea{...}, archivos)
```
- **Un endpoint, dos formatos:** con JSON (como en el paso 37) o con multipart. El campo `datos` trae el **mismo JSON** y se valida con la misma función estricta `decodificarJSON` (paso 17): campos desconocidos, horas decimales, etc.
- **Por qué una función aparte.** Leer la petición tiene dos caminos y varios errores posibles. Si todo eso estuviera dentro de `Crear`, el método quedaría con `if` anidados y sería difícil ver qué hace el endpoint. Así, `Crear` se lee de arriba abajo: leer la petición, validar, llamar al servicio, responder.
- **Resultados con nombre:** `(p peticionTarea, archivos ..., cerrar func(), err error)` declara las variables de retorno en la firma. Ya existen al empezar la función (con su valor cero), por eso se puede asignar `cerrar = func() {}` al inicio.
- `cerrar` **siempre** es una función válida (la vacía si no se abrió nada), así `Crear` puede hacer `defer cerrar()` antes de revisar el error.

```go
type adjuntoJSON struct {
	ID            string    `json:"id"`
	Nombre        string    `json:"nombre"`
	TipoContenido string    `json:"tipo_contenido"`
	TamanoBytes   int64     `json:"tamano_bytes"`
	CreadoEn      time.Time `json:"creado_en"`
	URLDescarga   string    `json:"url_descarga"`
}
```
- La tarea ahora incluye `cantidad_adjuntos` y la lista `adjuntos` (en el detalle), cada uno con su **URL de descarga** lista para usar. La ruta interna del archivo en el disco **nunca** se expone.

### 3.5 Límites coordinados

```go
		// Todos los adjuntos de una tarea más un margen para los demás campos.
		MaxCuerpoArchivosMB: cfg.MaxAdjuntoMB*dominio.MaxAdjuntosPorTarea + 1,
```
```nginx
    # Tamaño máximo de las peticiones: hasta 5 adjuntos de ADJUNTO_MAX_MB (10 MB)
    # por tarea, más los demás campos. La API aplica el mismo límite.
    client_max_body_size 51m;
```
- 5 archivos × 10 MB + 1 MB de margen = **51 MB**, igual en Nginx y en la API. Por defecto Nginx solo acepta **1 MB**, así que sin este ajuste ningún adjunto real pasaría.
- Si cambia `ADJUNTO_MAX_MB`, hay que ajustar Nginx a mano (está anotado en `.env.example`).

---

## 4. Las pruebas

| Prueba | Qué verifica |
|---|---|
| `TestCrearTareaConJSON` (actualizada) | La respuesta incluye la URL de descarga del adjunto. |
| `TestCrearTareaConAdjuntosMultipart` | Datos y dos archivos llegan al servicio con su contenido. |
| `TestCrearTareaMultipartSinDatosEsInvalida` | Un formulario sin `datos` → 400. |
| `TestPeticionDemasiadoGrandeResponde413` | Con un límite de 1 MB en el entorno de prueba, un archivo de 2 MB → **413**. |
| `TestAdjuntosDeTareas` | Agregar adjuntos sin multipart → 400; con multipart → 201; la descarga trae `Content-Type` y `Content-Disposition` correctos; eliminar → 204. |

La función de prueba `formulario` arma un cuerpo multipart con `mime/multipart.NewWriter`, igual que un navegador.

---

## 5. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Resultados con nombre | `func f() (p T, err error)`: las variables de retorno se declaran en la firma. |
| `multipart/form-data` | Formato de los formularios con archivos. |
| `http.MaxBytesReader` | Limita el tamaño del cuerpo; al superarlo, 413. |
| Función de limpieza devuelta | `archivos, cerrar, err := ...; defer cerrar()`. |
| Streaming | `DataFromReader`, `io.Copy`: datos por partes, sin cargar todo en memoria. |
| Cabeceras de seguridad | `attachment`, `nosniff`, `no-store`. |
| Normalización Unicode | NFD + quitar marcas + NFC para eliminar tildes. |

---

## 6. Cómo comprobarlo

```bash
make pruebas
docker compose up --build -d --wait
```

Con Laura vinculada como monitora y su token:
1. `curl -X POST .../api/v1/tareas -H "Authorization: Bearer $TOKEN" -F 'datos={...}' -F 'adjuntos=@evidencia.txt'` → la tarea con `cantidad_adjuntos: 1` y su `url_descarga`.
2. Con el token del profesor: `curl -D - localhost:8080<url_descarga>` → 200, `Content-Disposition: attachment; filename="evidencia.txt"...` y `X-Content-Type-Options: nosniff`.
3. Un PDF de 11 MB → **400** "supera el tamaño máximo de 10 MB" (y no queda guardado: `docker compose exec api find /data -type f`).
4. Un `.exe` → **400** "tipo de archivo no permitido".

En PowerShell, use `curl.exe` (no el alias `curl`) para que `-F` funcione.

---

## 7. Pasos para lograr esta versión desde el paso 39

1. `go get golang.org/x/text@v0.43.0 && go mod tidy` (desde `backend/`).
2. Crear `backend/internal/handler/archivos.go` con `campoAdjuntos`, `limitarCuerpo`, `esMultipart`, `leerArchivos`, `formularioMultipart`, `servirArchivo`, `disposicionAdjunto`, `quitarTildes` y `nombreASCII`.
3. En `handler/tareas.go`: interfaz completa (`AgregarAdjuntos`, `AbrirAdjunto`, `EliminarAdjunto`), campo `maxCuerpo`, las tres rutas de adjuntos, `adjuntoJSON` y los adjuntos en `aTareaJSON`, `leerPeticionTarea` y `Crear` con multipart, y los endpoints `AgregarAdjuntos`, `DescargarAdjunto` y `EliminarAdjunto`.
4. En `router.go`: `MaxCuerpoArchivosMB`, `r.MaxMultipartMemory = 8 << 20` y `maxCuerpo` en `HandlerTareas`.
5. En `router_test.go`: `MaxCuerpoArchivosMB: 1` en `nuevoEntorno`. En `modulos_test.go`: servicio falso completo (guarda el contenido de los archivos), `formulario` y las pruebas de la sección 4.
6. En `main.go`: `MaxCuerpoArchivosMB: cfg.MaxAdjuntoMB*dominio.MaxAdjuntosPorTarea + 1`.
7. En `nginx/nginx.conf`: `client_max_body_size 51m;` con su comentario. En `.env.example`: la nota sobre Nginx.
8. `make pruebas lint`, probar con `curl` y commit: `git add . && git commit -m "Paso 40: endpoints de adjuntos"`

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-06.4 / RN-23 Evidencias | Subir (al crear o después), descargar y eliminar adjuntos. |
| RF-08.5 / RN-33 | Una tarea tardía recibe su evidencia en la misma petición con que se crea. |
| RNF-03 Seguridad básica | Límite de tamaño en Nginx y en la API (413), descargas como adjunto con `nosniff`. |
