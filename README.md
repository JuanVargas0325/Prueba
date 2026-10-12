# Paso 38 · Almacenamiento local de archivos

> Rama `paso-a-paso` · paso 38 de 65 · anterior: **Paso 37 · Endpoints de tareas** · siguiente: **Paso 39 · Adjuntos en el servicio de tareas**

---

## 1. Qué se hizo en este paso

Se creó el paquete que **guarda archivos** (adjuntos de tareas y, más adelante, los PDF de los reportes) en una carpeta que vive en un **volumen de Docker**.

| Archivo | Cambio |
|---|---|
| `backend/internal/almacenamiento/local.go` | **Nuevo.** `Local` con `Guardar`, `Abrir` y `Eliminar`, protegido contra rutas maliciosas. |
| `backend/internal/almacenamiento/almacenamiento.go` | **Nuevo.** `Config` (tipo y ruta), la interfaz `Archivos` y `Nuevo`, que crea el almacenamiento según la configuración. |
| `backend/internal/almacenamiento/local_test.go` | **Nuevo.** Pruebas de guardar, abrir y eliminar, de rechazo de rutas fuera de la carpeta raíz y de `Nuevo`. |
| `backend/internal/config/config.go` | **Modificado.** `Almacenamiento` (`ALMACENAMIENTO_TIPO`, por defecto `local`, y `ALMACENAMIENTO_RUTA`, por defecto `/data`), `MaxAdjuntoMB` (`ADJUNTO_MAX_MB`, por defecto 10) y la función `entero`. |
| `backend/internal/config/config_test.go` | **Modificado.** Valores por defecto y formato inválido de `ADJUNTO_MAX_MB`. |
| `backend/Dockerfile` | **Modificado.** Crea `/data`, se lo asigna al usuario `app` y lo declara como `VOLUME`. |
| `docker-compose.yml` | **Modificado.** Volumen con nombre `archivos` montado en `/data` de la API. |
| `.env.example` | **Modificado.** Nueva sección `Archivos`. |

---

## 2. Por qué

- **RF-06.4 / RN-23:** las tareas pueden llevar **archivos de evidencia**.
- **RF-14.4 y RF-14.5:** los reportes se generan en **PDF** y se **almacenan** para descargarlos después.
- **¿Por qué no en la base de datos?** Guardar archivos binarios en PostgreSQL agranda la base de datos, hace lentos los respaldos y ocupa conexiones durante las descargas. Lo habitual es guardar el archivo en un almacenamiento de archivos y en la base de datos **solo sus metadatos** (nombre, tipo, tamaño, ruta), como ya prevé la tabla `tarea_adjuntos` (paso 4).
- **¿Por qué un volumen?** Un contenedor pierde todo lo que escribe en su propio sistema de archivos cuando se recrea. Un volumen persiste. En el paso 55 el worker montará el **mismo** volumen: él escribirá los PDF y la API los servirá.

---

## 3. Explicación detallada

### 3.1 `local.go`

```go
// Package almacenamiento guarda los archivos binarios (PDF de reportes y
// adjuntos de tareas). En la Entrega 1 se usa un volumen de Docker montado en
// el contenedor; en entregas posteriores puede añadirse una implementación
// sobre Cloud Storage que cumpla la misma interfaz del servicio.
package almacenamiento

type Local struct {
	raiz string
}

func NuevoLocal(raiz string) (*Local, error) {
	abs, err := filepath.Abs(raiz)
	...
	if err := os.MkdirAll(abs, 0o750); err != nil { ... }
	return &Local{raiz: abs}, nil
}
```
- Convierte la raíz en ruta absoluta y la crea si no existe. `0o750` son los permisos en notación octal: el dueño lee, escribe y entra; el grupo lee y entra; el resto, nada.

#### Seguridad: que nadie escriba fuera de la raíz

```go
func (l *Local) rutaSegura(ruta string) (string, error) {
	limpia := filepath.Clean(filepath.FromSlash(strings.TrimSpace(ruta)))
	if limpia == "." || filepath.IsAbs(limpia) || limpia == ".." ||
		strings.HasPrefix(limpia, ".."+string(filepath.Separator)) {
		return "", apperr.Validacion("ruta de archivo inválida")
	}
	return filepath.Join(l.raiz, limpia), nil
}
```
- Las rutas internas usan `/` (`adjuntos/<tarea>/<archivo>.pdf`). `filepath.Clean` resuelve `a/../b` → `b`.
- Se rechazan: rutas vacías, absolutas (`/etc/passwd`) y las que **suben** de nivel (`../fuera.pdf`, `a/../../fuera.pdf`). Es la protección contra el ataque de **recorrido de directorios** (*path traversal*), que podría sobrescribir archivos del sistema.
- Las rutas las arma el servidor (paso 39), no el usuario; aun así se valida: defensa en profundidad.

#### Guardar de forma atómica

```go
func (l *Local) Guardar(_ context.Context, ruta string, contenido io.Reader) (int64, error) {
	destino, err := l.rutaSegura(ruta)
	...
	if err := os.MkdirAll(filepath.Dir(destino), 0o750); err != nil { ... }
	tmp, err := os.CreateTemp(filepath.Dir(destino), ".tmp-*")
	...
	defer func() { _ = os.Remove(tmp.Name()) }()

	n, err := io.Copy(tmp, contenido)
	...
	if err := tmp.Close(); err != nil { ... }
	if err := os.Rename(tmp.Name(), destino); err != nil { ... }
	return n, nil
}
```
- Recibe un `io.Reader`: el contenido llega **por partes**, sin cargar el archivo completo en memoria (importante con archivos de varios MB).
- Escribe primero en un **archivo temporal** y al final lo **renombra** al destino. El renombrado es atómico: nadie puede ver un archivo a medio escribir, y si algo falla a la mitad, el destino no queda corrupto.
- `defer func() { _ = os.Remove(tmp.Name()) }()`: si algo falla antes del renombrado, el temporal se borra. Después del renombrado el temporal ya no existe y el `Remove` simplemente falla en silencio (`_ =` ignora el error a propósito).
- Devuelve los **bytes escritos**, con los que el servicio controla el tamaño máximo (paso 39).

```go
func (l *Local) Abrir(_ context.Context, ruta string) (io.ReadCloser, error) {
	...
	f, err := os.Open(origen)
	if errors.Is(err, os.ErrNotExist) {
		return nil, apperr.NoEncontrado("el archivo no existe")
	}
	...
	return f, nil
}

func (l *Local) Eliminar(_ context.Context, ruta string) error {
	...
	if err := os.Remove(objetivo); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("eliminando archivo: %w", err)
	}
	return nil
}
```
- `Abrir` devuelve un `io.ReadCloser` (algo que se lee y **se debe cerrar**); quien lo pide es responsable de cerrarlo.
- `Eliminar` no falla si el archivo ya no existe: el resultado deseado ("que no esté") ya se cumple. Esto se llama **idempotencia**.
- Los métodos reciben `context.Context` aunque el disco local no lo use (`_`): así su forma coincidirá con la interfaz `Almacenamiento` del servicio (paso 39), que una implementación en la nube sí necesitará.

### 3.2 `almacenamiento.go` · elegir el almacenamiento en un solo lugar

```go
const TipoLocal = "local"

type Config struct {
	Tipo string // ALMACENAMIENTO_TIPO
	Ruta string // ALMACENAMIENTO_RUTA: carpeta raíz del almacenamiento local
}

type Archivos interface {
	Guardar(ctx context.Context, ruta string, contenido io.Reader) (int64, error)
	Abrir(ctx context.Context, ruta string) (io.ReadCloser, error)
	Eliminar(ctx context.Context, ruta string) error
}

func Nuevo(c Config) (Archivos, error) {
	switch c.Tipo {
	case TipoLocal:
		local, err := NuevoLocal(c.Ruta)
		if err != nil {
			return nil, err
		}
		return local, nil
	default:
		return nil, fmt.Errorf("ALMACENAMIENTO_TIPO %q no es válido (use %q)", c.Tipo, TipoLocal)
	}
}
```
- **Por qué existe.** La API (paso 39) y el worker (paso 55) necesitan un almacenamiento. Si cada uno llamara directamente a `NuevoLocal`, al agregar Cloud Storage en la Entrega 2 habría que cambiar ambos. Con `Nuevo`, ambos piden "el almacenamiento configurado" y la decisión vive en **un solo `switch`**: agregar Cloud Storage será un archivo nuevo (`gcs.go`) más un `case` aquí.
- **`Archivos` es una interfaz**: los tres métodos que tiene cualquier almacenamiento. `Nuevo` debe devolver una interfaz porque, según la configuración, entregará tipos distintos (`*Local` hoy, el de Cloud Storage después). `*Local` la cumple sin declararlo, porque tiene esos tres métodos (interfaz implícita, paso 10).
- **¿Por qué `local, err := ...; return local, nil` y no `return NuevoLocal(c.Ruta)`?** Es una trampa conocida de Go: si `NuevoLocal` falla devuelve un `*Local` nulo, y al convertirlo en interfaz **la interfaz no es `nil`** (lleva dentro el tipo `*Local`). Devolver `nil` explícitamente en el error evita esa confusión.
- Un tipo desconocido es un error de arranque con un mensaje que dice qué valor usar.

### 3.3 Configuración

```go
	c.Almacenamiento = almacenamiento.Config{
		Tipo: texto("ALMACENAMIENTO_TIPO", almacenamiento.TipoLocal),
		Ruta: texto("ALMACENAMIENTO_RUTA", "/data"),
	}
	c.MaxAdjuntoMB = entero("ADJUNTO_MAX_MB", 10, &errs)
```
- Igual que `ia.Config` más adelante (paso 51), cada paquete define **su** configuración y `config` solo la llena desde las variables de entorno.
- `entero` es como `duracion` (paso 15) pero con `strconv.Atoi`. `ADJUNTO_MAX_MB=diez` produce un error de configuración.

### 3.4 Docker

```dockerfile
 && mkdir -p /data \
 && chown app:app /data
# /data es el punto de montaje del volumen de archivos (PDF y adjuntos).
VOLUME ["/data"]
```
- El proceso corre como `app` (no `root`, paso 5), así que la carpeta debe pertenecerle para poder escribir. Cuando Docker monta un volumen nuevo y vacío en `/data`, copia el dueño y los permisos de esa carpeta de la imagen.

```yaml
  api:
    environment:
      POSTGRES_HOST: postgres
      ALMACENAMIENTO_RUTA: /data
    volumes:
      - archivos:/data
...
volumes:
  datos-postgres:
  archivos:
```

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| `io.Reader` / `io.ReadCloser` | Interfaces para leer datos por partes (y cerrarlos). |
| `io.Copy` | Copia de un lector a un escritor sin cargar todo en memoria. |
| Escritura atómica | Temporal + `os.Rename`. |
| Path traversal | Ataque con `../` para salir de una carpeta; se evita con `filepath.Clean` y validaciones. |
| Permisos octales | `0o750`. |
| Idempotencia | Repetir la operación produce el mismo resultado (eliminar algo que ya no existe no falla). |
| `VOLUME` | Declara en la imagen una carpeta pensada para un volumen. |
| Función fábrica | Función (`Nuevo`) que decide qué implementación crear según la configuración. |
| Interfaz con valor `nil` | Una interfaz que contiene un puntero nulo no es `nil`; por eso se devuelve `nil` explícito. |

---

## 5. Cómo comprobarlo

```bash
cd backend && go test ./internal/almacenamiento/ -v
# TestGuardarAbrirYEliminar, TestRutasFueraDeLaRaizSonRechazadas, TestNuevoEligeElTipo

docker compose up --build -d --wait
docker compose exec api sh -c 'ls -ld /data && touch /data/prueba && echo ok'
# drwxr-xr-x 2 app app ... /data   ok
docker volume ls | grep archivos
```

`local_test.go` usa `t.TempDir()`, una carpeta temporal que Go borra al terminar la prueba.

---

## 6. Pasos para lograr esta versión desde el paso 37

1. Crear `backend/internal/almacenamiento/local.go` (`Local`, `NuevoLocal`, `rutaSegura`, `Guardar`, `Abrir`, `Eliminar`) y `local_test.go`.
2. Crear `backend/internal/almacenamiento/almacenamiento.go` (`TipoLocal`, `Config`, `Archivos`, `Nuevo`) y su prueba `TestNuevoEligeElTipo` en `local_test.go`.
3. En `config.go`: campos `Almacenamiento` y `MaxAdjuntoMB`, su carga, la función `entero` y los imports `strconv` y `seguimiento/internal/almacenamiento`. En `config_test.go`, los nuevos valores por defecto y `ADJUNTO_MAX_MB=diez`.
4. En `backend/Dockerfile`, etapa `base`: crear `/data`, `chown app:app /data` y `VOLUME ["/data"]`.
5. En `docker-compose.yml`: en `api`, `ALMACENAMIENTO_RUTA: /data` y el volumen `archivos:/data`; declarar `archivos` en `volumes`.
6. En `.env.example` (y su `.env`): la sección `Archivos` con `ALMACENAMIENTO_TIPO`, `ALMACENAMIENTO_RUTA` y `ADJUNTO_MAX_MB`.
7. `make pruebas lint`, `docker compose up --build -d --wait` y commit: `git add . && git commit -m "Paso 38: almacenamiento local de archivos"`

---

## 7. Preparación para las próximas entregas

- **Este es el punto de extensión para la nube.** La Entrega 2 exige guardar los PDF y los adjuntos en un bucket de **Cloud Storage**, y en la Entrega 3, con varias instancias de la API, un volumen local ya no serviría (cada máquina tendría sus propios archivos). Para agregarlo basta con:
  1. crear `almacenamiento/gcs.go` con los mismos tres métodos (`Guardar`, `Abrir`, `Eliminar`);
  2. agregar un `case` en `Nuevo` (por ejemplo `ALMACENAMIENTO_TIPO=gcs`) y una variable con el nombre del bucket.

  Ni los servicios ni los `main` cambian. Está registrado en la deuda técnica (DT-02).
- Los métodos ya reciben `context.Context`, que las llamadas de red a Cloud Storage necesitarán para sus tiempos máximos.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-06.4 / RN-23 Evidencias | Base para guardar los adjuntos (paso 39). |
| RF-14.5 Almacenar los PDF | El mismo almacenamiento guardará los reportes (paso 54). |
| Stack: volúmenes de archivos | Volumen `archivos` en Docker Compose. |
| RNF-03 Seguridad básica | Protección contra recorrido de directorios; escritura atómica. |
