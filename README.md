# Paso 39 · Adjuntos en el servicio de tareas

> Rama `paso-a-paso` · paso 39 de 65 · anterior: **Paso 38 · Almacenamiento local** · siguiente: **Paso 40 · Endpoints de adjuntos (multipart)**

---

## 1. Qué se hizo en este paso

El servicio de tareas ahora maneja **archivos de evidencia**: se pueden enviar al crear la tarea, agregar después, descargar y eliminar, con validación de tipo, tamaño y cantidad.

| Archivo | Cambio |
|---|---|
| `backend/internal/dominio/tarea.go` | **Modificado.** Límites `MaxAdjuntosPorTarea` (5) y `LongitudMaxNombreArchivo`, lista blanca de tipos permitidos y `ValidarArchivoAdjunto`. |
| `backend/internal/dominio/reglas_test.go` | **Modificado.** `TestValidarArchivoAdjunto`. |
| `backend/internal/servicio/tareas.go` | **Modificado.** `ArchivoAdjunto`; el servicio recibe un `Almacenamiento` y un tamaño máximo; `Crear` acepta archivos; nuevos `AgregarAdjuntos`, `AbrirAdjunto` y `EliminarAdjunto`; `Eliminar` borra también los archivos; interfaz `Almacenamiento`. |
| `backend/internal/servicio/modulos_test.go` | **Modificado.** Entorno de tareas con almacenamiento falso y límite de 10 bytes; dos pruebas nuevas. |
| `backend/internal/servicio/autenticacion_test.go` | **Modificado.** Doble `almacenamientoFalso` (archivos en memoria). |
| `backend/internal/handler/tareas.go` y `handler/modulos_test.go` | **Modificados (mínimo).** Se adaptan a la nueva firma de `Crear`; por ahora el handler pasa `nil` como archivos. |
| `backend/cmd/api/main.go` | **Modificado.** Crea el almacenamiento local y le pasa el tamaño máximo al servicio. |

---

## 2. Por qué

- **RF-06.4 / RN-23:** una tarea puede llevar archivos de evidencia.
- **Adjuntos al crear:** un reporte **tardío** no se puede modificar después (RN-33); si los adjuntos solo pudieran agregarse luego, una tarea tardía nunca podría tener evidencia. Por eso `Crear` también recibe archivos.
- **Seguridad:** sin límites, cualquiera podría subir ejecutables, páginas HTML con código o archivos gigantes que llenen el disco.

---

## 3. Explicación detallada

### 3.1 Dominio: qué archivos se aceptan

```go
const (
	...
	MaxAdjuntosPorTarea      = 5
	LongitudMaxNombreArchivo = 200
)

var tiposPermitidos = map[string]string{
	".pdf":  "application/pdf",
	".png":  "image/png",
	...
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	...
	".zip":  "application/zip",
}
```
- **Lista blanca**: solo PDF, imágenes, texto, Office y ZIP. Es más segura que una lista negra (siempre aparece una extensión peligrosa que nadie anotó). Quedan fuera `.exe` y `.html`.
- El tipo de contenido (*MIME type*) que se guarda **lo decide el servidor** según la extensión, no lo que diga el navegador.

```go
func ValidarArchivoAdjunto(nombre string) (nombreLimpio, extension, tipo string, err error) {
	nombreLimpio = strings.TrimSpace(filepath.Base(strings.ReplaceAll(nombre, "\\", "/")))
	...
	if r := []rune(nombreLimpio); len(r) > LongitudMaxNombreArchivo {
		nombreLimpio = string(r[:LongitudMaxNombreArchivo])
	}
	extension = strings.ToLower(filepath.Ext(nombreLimpio))
	tipo, ok := tiposPermitidos[extension]
	if !ok {
		return "", "", "", apperr.Validacion("tipo de archivo no permitido (%s): use PDF, imágenes, texto, Office o ZIP", extension)
	}
	return nombreLimpio, extension, tipo, nil
}
```
- **Resultados con nombre:** `(nombreLimpio, extension, tipo string, err error)` declara las variables de retorno en la firma.
- Algunos navegadores (Internet Explorer, ciertas versiones de Edge) envían la ruta completa (`C:\fakepath\..\Informe Final.PDF`). Se reemplazan `\` por `/` y `filepath.Base` deja solo el nombre: `Informe Final.PDF`.
- La extensión se compara en minúsculas (`.PDF` = `.pdf`).

### 3.2 La interfaz `Almacenamiento`

```go
// Almacenamiento guarda y recupera archivos binarios.
type Almacenamiento interface {
	Guardar(ctx context.Context, ruta string, contenido io.Reader) (int64, error)
	Abrir(ctx context.Context, ruta string) (io.ReadCloser, error)
	Eliminar(ctx context.Context, ruta string) error
}
```
- El servicio depende de **esta interfaz**, no de `almacenamiento.Local` (paso 38), que la cumple. Por ahora se declara al final de `tareas.go`; en el paso 48 se trasladará a `reportes.go`, junto a las demás interfaces de infraestructura que también usa el generador de reportes.

### 3.3 `Crear` con archivos

```go
type ArchivoAdjunto struct {
	Nombre    string
	Contenido io.Reader
}

func (s *Tareas) Crear(ctx context.Context, actor auth.Actor, n NuevaTarea, archivos []ArchivoAdjunto) (dominio.Tarea, error) {
	... (validaciones del paso 36) ...
	if len(archivos) > dominio.MaxAdjuntosPorTarea {
		return dominio.Tarea{}, apperr.Validacion("una tarea admite máximo %d adjuntos", dominio.MaxAdjuntosPorTarea)
	}

	t := dominio.Tarea{ID: uuid.NewString(), ...}
	t.Aplicar(datos)

	adjuntos, err := s.guardarArchivos(ctx, t.ID, archivos)
	if err != nil {
		return dominio.Tarea{}, err
	}
	creada, err := s.repo.Crear(ctx, t, adjuntos)
	if err != nil {
		s.eliminarArchivos(ctx, adjuntos)
		return dominio.Tarea{}, err
	}
	...
}
```
- El servicio recibe los archivos como **lectores** (`io.Reader`), no como bytes: el contenido pasa del formulario HTTP al disco sin cargarse entero en memoria.
- Orden: (1) guardar los archivos; (2) guardar la tarea y sus metadatos en una transacción (paso 35). **Si falla la base de datos, se borran los archivos** ya escritos: no quedan archivos huérfanos.
- Aquí se ve por qué el ID se genera en el servicio (paso 36): los archivos van a `adjuntos/<id de la tarea>/...` **antes** de que exista la fila.

### 3.4 `guardarArchivos`: tamaño máximo sin confiar en el cliente

```go
func (s *Tareas) guardarArchivos(ctx context.Context, tareaID string, archivos []ArchivoAdjunto) ([]dominio.Adjunto, error) {
	adjuntos := make([]dominio.Adjunto, 0, len(archivos))
	for _, archivo := range archivos {
		nombre, extension, tipo, err := dominio.ValidarArchivoAdjunto(archivo.Nombre)
		if err != nil {
			s.eliminarArchivos(ctx, adjuntos)
			return nil, err
		}
		id := uuid.NewString()
		ruta := fmt.Sprintf("adjuntos/%s/%s%s", tareaID, id, extension)
		// Se lee un byte más del máximo para detectar archivos demasiado grandes.
		tamano, err := s.almacenamiento.Guardar(ctx, ruta, io.LimitReader(archivo.Contenido, s.maxBytes+1))
		if err == nil && tamano > s.maxBytes {
			s.eliminarArchivo(ctx, ruta)
			err = apperr.Validacion("el archivo %q supera el tamaño máximo de %d MB", nombre, s.maxBytes>>20)
		}
		if err == nil && tamano == 0 {
			s.eliminarArchivo(ctx, ruta)
			err = apperr.Validacion("el archivo %q está vacío", nombre)
		}
		if err != nil {
			s.eliminarArchivos(ctx, adjuntos)
			return nil, err
		}
		adjuntos = append(adjuntos, dominio.Adjunto{...})
	}
	return adjuntos, nil
}
```
- **El nombre en disco no es el del usuario:** es `<UUID><extensión>`. El nombre original se guarda solo en la base de datos (para mostrarlo y para la descarga). Así no hay choques de nombres ni nombres maliciosos en el disco.
- **El truco del byte extra:** se leen como máximo `maxBytes + 1` bytes. Si se escribieron más de `maxBytes`, el archivo era demasiado grande: se borra y se responde 400. No se confía en el tamaño que declare el cliente, y nunca se escriben gigabytes.
- `s.maxBytes>>20` convierte bytes a MB (desplazar 20 bits = dividir por 2²⁰).
- **Todo o nada:** si cualquier archivo falla, se borran los que ya se habían guardado.

### 3.5 Agregar, abrir y eliminar adjuntos

```go
func (s *Tareas) AgregarAdjuntos(ctx context.Context, actor auth.Actor, id string, archivos []ArchivoAdjunto) (dominio.Tarea, error) {
	if len(archivos) == 0 { ... 400 ... }
	if _, err := s.obtenerModificable(ctx, actor, id); err != nil { ... }
	adjuntos, err := s.guardarArchivos(ctx, id, archivos)
	...
	if err := s.repo.AgregarAdjuntos(ctx, id, adjuntos, dominio.MaxAdjuntosPorTarea); err != nil {
		s.eliminarArchivos(ctx, adjuntos)
		return dominio.Tarea{}, err
	}
	return s.repo.Obtener(ctx, id)
}
```
- Solo sobre tareas **modificables** (propias, semana activa, no tardías). El máximo de 5 lo controla el repositorio con bloqueo (paso 35).

```go
func (s *Tareas) AbrirAdjunto(ctx context.Context, actor auth.Actor, tareaID, adjuntoID string) (io.ReadCloser, dominio.Adjunto, error) {
	if _, err := s.Obtener(ctx, actor, tareaID); err != nil { ... }
	a, err := s.repo.ObtenerAdjunto(ctx, tareaID, adjuntoID)
	...
	contenido, err := s.almacenamiento.Abrir(ctx, a.RutaAlmacenamiento)
	return contenido, a, err
}
```
- Descargar exige poder **ver** la tarea (su dueña, su profesor o el administrador).

### 3.6 Borrar archivos sin romper la operación

```go
func (s *Tareas) eliminarArchivo(ctx context.Context, ruta string) {
	if err := s.almacenamiento.Eliminar(context.WithoutCancel(ctx), ruta); err != nil {
		s.log.Warn("no fue posible eliminar el archivo", "ruta", ruta, "error", err)
	}
}
```
- Si falla el borrado de un archivo, **solo se registra**: la base de datos ya quedó consistente y un archivo huérfano no afecta al usuario.
- `context.WithoutCancel(ctx)`: la limpieza se hace aunque el cliente haya cerrado la conexión.
- `Eliminar` (de la tarea) ahora usa las rutas que devuelve el repositorio para borrar los archivos.

### 3.7 Cambios mínimos fuera del servicio

- El handler pasa `nil` como archivos (`}, nil)`): en el paso 40 se leerán del formulario multipart.
- `main.go` crea el almacenamiento configurado (con la función `Nuevo` del paso 38, que hoy entrega el local) y calcula el tamaño máximo:
  ```go
  	// Infraestructura
  	almacen, err := almacenamiento.Nuevo(cfg.Almacenamiento)
  	...
  	maxBytesAdjunto := int64(cfg.MaxAdjuntoMB) << 20
  	...
  	Tareas: servicio.NuevoTareas(tareas, vinculaciones, almacen, reloj, maxBytesAdjunto, log),
  ```
  `<< 20` multiplica por 2²⁰ (MB → bytes).

---

## 4. Las pruebas

El entorno de tareas usa un **almacenamiento falso en memoria** y un tamaño máximo de **10 bytes**, para probar el límite sin crear archivos grandes.

| Prueba | Qué verifica |
|---|---|
| `TestValidarArchivoAdjunto` (dominio) | `C:\fakepath\..\Informe Final.PDF` → `Informe Final.PDF`, `.pdf`, `application/pdf`. Se rechazan `virus.exe`, `pagina.html`, un nombre sin extensión y uno vacío. |
| `TestCrearTareaConAdjuntos` | Un PDF se guarda en `adjuntos/<id de la tarea>/...` con su tipo; un archivo de 50 bytes supera el límite → 400 y **no queda nada guardado** (tampoco el archivo válido que iba antes); `virus.exe` → 400; si falla la base de datos, el archivo se borra. |
| `TestAgregarAdjuntosATareaModificable` | Sin archivos → 400; un PNG se agrega y se guarda. |
| `TestModificarTareas` (actualizada) | Eliminar una tarea ahora pasa por el almacenamiento. |

---

## 5. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Resultados con nombre | `func f() (a, b string, err error)`. |
| Lista blanca | Solo se acepta lo explícitamente permitido. |
| `io.LimitReader(r, n+1)` | Detectar archivos que superan `n` bytes sin leerlos completos. |
| Compensación | Si un paso posterior falla, se deshace lo anterior (borrar archivos). |
| `<<` / `>>` | Desplazamiento de bits: multiplicar o dividir por potencias de 2. |

---

## 6. Cómo comprobarlo

```bash
cd backend
go test ./internal/dominio/ ./internal/servicio/ -v -run 'Adjunto|Tarea'
```

Por HTTP los adjuntos estarán disponibles en el paso 40.

---

## 7. Pasos para lograr esta versión desde el paso 38

1. En `dominio/tarea.go`: `MaxAdjuntosPorTarea` y `LongitudMaxNombreArchivo` en las constantes, `tiposPermitidos` y `ValidarArchivoAdjunto`. En `reglas_test.go`: `TestValidarArchivoAdjunto`.
2. En `servicio/tareas.go`:
   - en `RepositorioTareas`, los métodos `AgregarAdjuntos`, `ObtenerAdjunto` y `EliminarAdjunto`;
   - el tipo `ArchivoAdjunto`; los campos `almacenamiento` y `maxBytes` y los parámetros de `NuevoTareas`;
   - `Crear` con `archivos`; `Eliminar` que borra los archivos; `AgregarAdjuntos`, `AbrirAdjunto`, `EliminarAdjunto`, `guardarArchivos`, `eliminarArchivos` y `eliminarArchivo`;
   - al final, la interfaz `Almacenamiento`.
3. En `autenticacion_test.go`: `almacenamientoFalso`. En `modulos_test.go`: `entornoTareas` con almacenamiento y límite de 10 bytes, `Crear(..., nil)` en las pruebas existentes y las dos pruebas nuevas.
4. En `handler/tareas.go`: la nueva firma de `Crear` en `ServicioTareas` y `nil` en la llamada; en `handler/modulos_test.go`, la firma del servicio falso.
5. En `main.go`: `almacen` con `almacenamiento.Nuevo(cfg.Almacenamiento)`, `maxBytesAdjunto` y los nuevos argumentos de `NuevoTareas`.
6. `make pruebas lint` y commit: `git add . && git commit -m "Paso 39: adjuntos en el servicio de tareas"`

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-06.4 / RN-23 Evidencias | Adjuntos al crear o después; descarga y eliminación. |
| RF-08.5 / RN-33 | Los adjuntos de una tarea tardía se envían al crearla (después no se modifica). |
| RNF-03 Seguridad básica | Lista blanca de tipos, tamaño máximo verificado en el servidor, nombres en disco generados. |
| RNF-06 Consistencia | Archivos y metadatos se guardan o se descartan juntos. |
