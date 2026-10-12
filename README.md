# Paso 0 · Estructura inicial del repositorio

> Rama `paso-a-paso` · paso 0 de 65 · siguiente: **Paso 1 · Configuración por variables de entorno**
>
> Esta rama reconstruye, commit a commit, el backend de la *Plataforma de seguimiento de monitores y asistentes graduados* (MISW4204). Cada commit agrega una sola cosa y este README explica **solo lo que se hizo en ese paso**. Para ver el paso siguiente, avance al siguiente commit de la rama.

---

## 1. Qué se hizo en este paso

Se creó el esqueleto mínimo del proyecto:

| Archivo | Para qué sirve |
|---|---|
| `.gitignore` | Le dice a Git qué archivos **no** debe guardar (secretos, binarios, resultados de pruebas). |
| `.gitattributes` | Le dice a Git cómo tratar los saltos de línea y los archivos binarios. |
| `.env.example` | Plantilla de la configuración. Cada persona la copia como `.env` y pone sus valores. |
| `backend/go.mod` | Declara el **módulo de Go** (el proyecto) y la versión mínima de Go. |
| `backend/cmd/api/main.go` | El programa de la API. Por ahora solo escribe un mensaje de inicio. |
| `README.md` | Este documento. |

Al final de este paso el proyecto **compila y se ejecuta**, aunque todavía no hace nada útil: imprime `API iniciada` y termina.

---

## 2. Por qué se empieza así

- **Primero la estructura, después las funcionalidades.** Si desde el inicio está claro dónde va cada cosa (código Go en `backend/`, configuración en `.env`, secretos fuera de Git), todo lo que se agregue después tiene un lugar natural.
- **Los lineamientos del curso** piden un único repositorio organizado (sección 1.2) y que no haya credenciales en el repositorio (sección 10). El `.gitignore` y el `.env.example` resuelven lo segundo desde el día uno.
- **El backend va en su propia carpeta** (`backend/`) porque el repositorio también tendrá archivos que no son Go: `docker-compose.yml`, la configuración de Nginx, la colección de Postman, scripts de pruebas de carga y, en la Entrega 2, el frontend.

---

## 3. Explicación detallada de cada archivo

### 3.1 `.gitignore`

```gitignore
# Secretos y configuración local
.env
*.env.local
```
- `.env` es el archivo donde cada persona pone sus contraseñas reales. **Nunca** se sube.
- `*.env.local` cubre variantes locales (por ejemplo `pruebas.env.local`). El `*` significa "cualquier texto".

```gitignore
# Resultados de compilación y pruebas
backend/coverage.out
backend/bin/
# Binarios que genera go build en la carpeta actual
backend/api
backend/worker
*.test
```
- `coverage.out` es el reporte de cobertura que generan las pruebas (paso 8). Se regenera cada vez, no se versiona.
- `backend/bin/` es la carpeta recomendada para compilar a mano: `go build -o bin/api ./cmd/api`.
- `backend/api` y `backend/worker`: si alguien ejecuta `go build ./...` dentro de `backend/`, Go deja el ejecutable en la carpeta actual con el nombre del programa. Con estas reglas ese binario nunca termina en un commit por accidente. Un binario pesa varios MB, no se puede revisar en un diff y se regenera compilando.
- `*.test` son ejecutables de pruebas que Go puede generar con `go test -c`.

```gitignore
# Archivos generados localmente
data/
*.pdf
```
- `data/` es una carpeta de datos locales (por ejemplo, si se ejecuta la API fuera de Docker y guarda archivos ahí).
- `*.pdf`: los reportes PDF se generan en tiempo de ejecución (paso 50); no son código.

```gitignore
# Editores y sistema operativo
.vscode/
.idea/
.DS_Store
Thumbs.db
```
- Configuración personal de VS Code y de los editores de JetBrains, y archivos basura que crean macOS (`.DS_Store`) y Windows (`Thumbs.db`).

**Regla de lectura:** cada línea es un patrón. Una línea que termina en `/` indica una carpeta. Una línea con `/` en medio (`backend/api`) se interpreta desde la raíz del repositorio. Las líneas que empiezan con `#` son comentarios.

### 3.2 `.gitattributes`

```gitattributes
# Normaliza los saltos de línea a LF en el repositorio (Go, Docker y scripts lo requieren).
* text=auto eol=lf
*.png binary
*.pdf binary
```

**El problema.** Windows termina cada línea con dos caracteres invisibles (`CRLF`, `\r\n`) y Linux con uno (`LF`, `\n`). Git para Windows suele convertir los archivos a `CRLF` al descargarlos. Este proyecto se ejecuta en Linux (contenedores Debian y GitHub Actions), donde un `\r` sobrante rompe cosas:
- un script `.sh` falla con `/bin/bash^M: bad interpreter`;
- en un `Dockerfile`, una línea que termina en `\` seguida de `\r` ya no continúa en la siguiente;
- `gofmt` (paso 8) marcaría todos los archivos como mal formateados;
- si un integrante usa Windows y otro macOS, cada commit mostraría archivos enteros "cambiados" solo por los saltos de línea.

**Línea por línea:**
- `* text=auto eol=lf`: para todos los archivos (`*`), Git detecta si son texto (`text=auto`) y, si lo son, los guarda **y** los entrega siempre con `LF` (`eol=lf`), sin importar el sistema operativo ni la configuración `core.autocrlf` de cada persona.
- `*.png binary` y `*.pdf binary`: estos archivos **no son texto**. `binary` equivale a `-text -diff -merge`: nunca convertir sus bytes, no mostrar diferencias línea a línea y no intentar combinarlos en un merge. Es importante porque una imagen o un PDF contienen bytes `\r\n` que son datos, no saltos de línea. Un PDF incluso empieza con texto (`%PDF-1.7`) y podría confundirse. Si Git los "corrigiera", quedarían dañados.

### 3.3 `.env.example`

```dotenv
# --- Aplicación ---------------------------------------------------------------
# Entorno: desarrollo | produccion
APP_ENTORNO=desarrollo
APP_PUERTO=8080
APP_ZONA_HORARIA=America/Bogota
```

- Es una **plantilla**: sí se sube al repositorio, pero solo con valores de ejemplo. Cada persona ejecuta `cp .env.example .env` y edita su `.env`.
- Las variables de entorno son la forma estándar de configurar una aplicación sin tocar el código: el mismo programa se comporta distinto en tu computador, en GitHub Actions o en la nube según las variables que reciba.
- **Todavía nadie lee estas variables.** El programa las empieza a usar en el paso 1. Se dejan aquí para fijar desde el inicio el formato del archivo.
- `APP_ENTORNO`: `desarrollo` o `produccion` (cambiará el nivel de detalle de los logs del framework web).
- `APP_PUERTO`: puerto en el que escuchará la API.
- `APP_ZONA_HORARIA`: zona horaria para decidir en qué semana cae cada fecha (las semanas van de lunes a domingo en hora de Bogotá).
- Regla importante: **los comentarios van en su propia línea**. Docker Compose no admite un comentario después de un valor (`CLAVE=valor  # comentario` dejaría el comentario como parte del valor).

### 3.4 `backend/go.mod`

```
module seguimiento

go 1.26.0
```

- `go.mod` define un **módulo**: el conjunto de paquetes de Go que se versionan juntos. Es el equivalente al `package.json` de Node o al `pom.xml` de Maven.
- `module seguimiento` es la **ruta del módulo**: el prefijo con el que se importan sus paquetes. Desde el paso 1 verá `import "seguimiento/internal/config"`. Si el código fuera una librería que otros proyectos descargan, la ruta debería ser la URL del repositorio (`github.com/usuario/repo`); como es una aplicación que nadie importa, basta un nombre corto.
- `go 1.26.0` es la **versión mínima de Go**. Un Go más antiguo se niega a compilar el proyecto. Se eligió 1.26 porque es una versión con soporte y porque algunas dependencias que se agregarán más adelante (por ejemplo `golang.org/x/crypto`, paso 12) la exigen.
- Se crea con `go mod init seguimiento`. Más adelante, cuando se agreguen librerías, aparecerán aquí líneas `require` y un archivo `go.sum`.

### 3.5 `backend/cmd/api/main.go`

**Por qué en `cmd/api/`.** Es la convención de Go para proyectos con varios ejecutables (guía oficial *Organizing a Go module*, go.dev/doc/modules/layout): cada programa va en `cmd/<nombre>/`. El nombre de la carpeta es el nombre del ejecutable. En el paso 55 llegará `cmd/worker`, un segundo programa. El código reutilizable irá en `backend/internal/`, una carpeta especial que el compilador protege: sus paquetes solo pueden importarse desde dentro de este mismo módulo.

**Línea por línea:**

```go
// Comando api: servidor HTTP de la plataforma de seguimiento.
package main
```
- El comentario justo encima de `package` es la documentación del paquete (la muestra `go doc`).
- `package main` es un nombre especial: indica que este paquete se compila como **programa ejecutable** y no como librería. Debe tener una función llamada `main`.

```go
import (
	"log/slog"
	"os"
)
```
- Importa dos paquetes de la **librería estándar** (vienen con Go, no hay que descargarlos):
  - `log/slog`: registro de eventos (logs) **estructurado**: cada mensaje es un registro con campos (`clave=valor`), no un texto libre.
  - `os`: acceso al sistema operativo (salida estándar, código de salida del proceso).

```go
func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("servicio", "api")
```
- `func main()` es el punto de entrada: lo primero que se ejecuta.
- `slog.NewJSONHandler(os.Stdout, nil)` crea un "manejador" que escribe cada mensaje como **una línea JSON** en la salida estándar. El `nil` significa "opciones por defecto" (nivel mínimo Info).
  - **Por qué la salida estándar:** en contenedores, Docker recoge lo que el programa escribe ahí; así funciona `docker compose logs`.
  - **Por qué JSON:** en la nube (Entrega 2), los servicios de logs lo interpretan directamente y permiten filtrar por campo.
- `.With("servicio", "api")` crea un logger que agrega ese par clave-valor a **todas** sus líneas. Cuando exista el worker (`"servicio": "worker"`) se podrán distinguir sus logs.

```go
	if err := ejecutar(log); err != nil {
		log.Error("la API terminó con error", "error", err)
		os.Exit(1)
	}
}
```
- Llama a `ejecutar`. Si devuelve un error, lo registra y termina el proceso con código `1` ("falló"). Docker y GitHub Actions usan ese código para saber que algo salió mal.
- **Por qué no poner toda la lógica en `main`:** `os.Exit` termina el programa **sin ejecutar los `defer` pendientes** (instrucciones de limpieza, como cerrar la conexión a la base de datos, que aparecerán en el paso 3). Con este patrón, toda la lógica vive en `ejecutar`, que devuelve un error; sus `defer` se ejecutan normalmente y solo `main` decide el código de salida.

```go
// ejecutar arma la aplicación. En los siguientes pasos se agregan la
// configuración, la base de datos y el servidor HTTP.
func ejecutar(log *slog.Logger) error {
	log.Info("API iniciada")
	return nil
}
```
- Recibe el logger y devuelve un `error`. En este paso solo escribe un mensaje y devuelve `nil` ("sin error").

---

## 4. Conceptos de Go de este paso

| Concepto | Qué significa | Ejemplo |
|---|---|---|
| Paquete | Carpeta con archivos `.go` que comparten `package <nombre>`. Es la unidad de código de Go. | `package main` |
| `import` | Trae otros paquetes. Los de la librería estándar se escriben por su ruta corta. | `import "os"` |
| `func` | Declara una función. Los parámetros llevan el tipo **después** del nombre. | `func ejecutar(log *slog.Logger) error` |
| `:=` | **Declara** una variable nueva y le asigna un valor; Go deduce el tipo. Solo dentro de funciones. | `log := slog.New(...)` |
| `=` | Solo **asigna** a una variable que ya existe. | `x = 5` |
| `if x := f(); cond` | `if` con inicialización: primero ejecuta `x := f()` y luego evalúa la condición. La variable solo existe dentro del `if`. El `;` separa las dos partes. | `if err := ejecutar(log); err != nil` |
| `error` | Tipo que representa un error. En Go los errores **son valores** que se devuelven, no excepciones. | `return nil` |
| `nil` | "Nada" (sin valor). Un `error` igual a `nil` significa que no hubo error. | `err != nil` |
| `*T` | Un **puntero** a un valor de tipo `T`: se comparte el mismo objeto en lugar de copiarlo. | `*slog.Logger` |
| Mayúscula inicial | Un nombre que empieza en mayúscula se **exporta** (se ve desde otros paquetes); en minúscula es privado del paquete. | `slog.New` (exportada), `ejecutar` (privada) |

---

## 5. Cómo comprobarlo

### Opción A · Con Go instalado

```bash
cd backend
go run ./cmd/api
```

Salida esperada (una línea JSON):

```json
{"time":"2026-10-11T07:34:06.16Z","level":"INFO","msg":"API iniciada","servicio":"api"}
```

Para instalar Go en Windows: `winget install --id GoLang.Go` (o el instalador de go.dev/dl). Verifique con `go version`.

### Opción B · Sin instalar Go, con Docker

Desde la raíz del repositorio:

```bash
# PowerShell
docker run --rm -v "${PWD}/backend:/src" -w /src golang:1.26 go run ./cmd/api

# Git Bash (en Windows hace falta MSYS_NO_PATHCONV=1 y la ruta en formato Windows)
MSYS_NO_PATHCONV=1 docker run --rm -v "$(pwd -W)/backend:/src" -w /src golang:1.26 go run ./cmd/api
```

- `docker run --rm`: crea un contenedor temporal y lo borra al terminar.
- `-v ".../backend:/src"`: monta la carpeta `backend` de su equipo dentro del contenedor en `/src`.
- `-w /src`: hace que el comando se ejecute en esa carpeta.
- `golang:1.26`: imagen oficial con Go 1.26 instalado.

> En los pasos siguientes, los comandos de Go se escriben como `go ...` "desde `backend/`". Si no tiene Go instalado, anteponga `docker run --rm -v "${PWD}/backend:/src" -w /src golang:1.26` como en este ejemplo. Desde el paso 8 también existe `make pruebas`.

---

## 6. Pasos para lograr esta versión desde cero

1. Crear (o clonar) el repositorio vacío y entrar en él:
   ```bash
   git clone https://github.com/<usuario>/<repo>.git
   cd <repo>
   ```
2. Crear `.gitignore` con el contenido de la sección 3.1.
3. Crear `.gitattributes` con el contenido de la sección 3.2. Si el repositorio ya tenía archivos, aplicar la normalización una vez: `git add --renormalize .`
4. Crear `.env.example` con el contenido de la sección 3.3 y, para uso local, copiarlo: `cp .env.example .env`
5. Crear el módulo de Go:
   ```bash
   mkdir -p backend/cmd/api
   cd backend
   go mod init seguimiento
   ```
   Abrir `backend/go.mod` y dejar la línea de versión como `go 1.26.0`.
6. Crear `backend/cmd/api/main.go` con el código de la sección 3.5.
7. Comprobar que compila y se ejecuta: `go run ./cmd/api` (desde `backend/`).
8. Revisar qué se va a subir y hacer el commit:
   ```bash
   cd ..
   git status            # no debe aparecer .env
   git add .
   git commit -m "Paso 0: estructura inicial del proyecto"
   ```

---

## 7. Preparación para las próximas entregas

- **Configuración por variables de entorno desde el inicio:** en la Entrega 2 (máquinas virtuales en GCP) y en la Entrega 4 (Cloud Run), la configuración se inyectará como variables de entorno. El mismo binario servirá sin cambios.
- **Logs JSON en la salida estándar:** Cloud Logging de GCP los interpreta automáticamente, con nivel y campos filtrables.
- **`cmd/` para varios programas:** el worker de la Entrega 1 y cualquier proceso adicional de entregas futuras (por ejemplo, una función serverless en la Entrega 4) tendrán su propio `cmd/<nombre>` y compartirán el código de `internal/`.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Lineamientos 1.2: repositorio único y organizado | Estructura con `backend/` separado del resto. |
| Lineamientos 10: sin secretos en el repositorio | `.env` ignorado; solo `.env.example` con valores de ejemplo. |
| Stack: Go | Módulo `seguimiento` con Go 1.26. |
