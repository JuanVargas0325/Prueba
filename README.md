# Paso 8 · Integración continua con GitHub Actions

> Rama `paso-a-paso` · paso 8 de 65 · anterior: **Paso 7 · Nginx** · siguiente: **Paso 9 · Errores de aplicación**

---

## 1. Qué se hizo en este paso

Cada `push` y cada *pull request* ejecutan automáticamente en GitHub una revisión del código: formato, análisis estático y pruebas.

| Archivo | Cambio |
|---|---|
| `.github/workflows/ci.yml` | **Nuevo.** Pipeline con dos trabajos: `calidad` (gofmt + golangci-lint) y `pruebas` (go test con detector de carreras y cobertura). |
| `backend/.golangci.yml` | **Nuevo.** Configuración del linter. |
| `Makefile` | **Modificado.** Nuevos objetivos `pruebas`, `lint` y `formato`, que ejecutan lo mismo localmente con Docker. |

En la pestaña **Actions** del repositorio aparece una ejecución por cada push, en verde (todo bien) o en rojo (algo falló), con el detalle de cada paso.

---

## 2. Por qué

- **Entregable de la Entrega 1:** "pipeline de GitHub Actions para validación automática de pruebas" (enunciado, `06-pruebas.md` y `07-alcance.md`).
- **Desde el principio y no al final:** si el pipeline existe desde el paso 8, cada funcionalidad que se agregue en los pasos siguientes queda validada en el momento. Un error se detecta en el commit que lo introdujo, cuando es fácil de corregir.
- **Disciplina de equipo:** nadie depende de "en mi computador funciona". El pipeline corre en una máquina limpia de GitHub.
- **Lint y formato** evitan discusiones de estilo y detectan errores comunes antes de ejecutar el código.

---

## 3. Explicación detallada

### 3.1 `.github/workflows/ci.yml`

GitHub Actions ejecuta los archivos `.yml` de `.github/workflows/`. Cada archivo es un *workflow* compuesto por *jobs* (trabajos), y cada job por *steps* (pasos).

```yaml
name: CI

on:
  push:
    branches: ["**"]
  pull_request:
    branches: [main]
```
- `on` define **cuándo** se ejecuta: en cada push a cualquier rama (`"**"`) y en cada *pull request* hacia `main`.

```yaml
permissions:
  contents: read
```
- El pipeline solo necesita **leer** el código. Dar el mínimo de permisos es una buena práctica de seguridad.

#### Job `calidad`

```yaml
  calidad:
    name: Formato y análisis estático
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: backend/go.mod
          cache-dependency-path: backend/go.sum
```
- `runs-on: ubuntu-latest`: GitHub presta una máquina virtual Linux limpia para cada ejecución.
- `actions/checkout` descarga el código del commit.
- `actions/setup-go` instala Go **en la versión que dice `go.mod`** (1.26) y guarda en caché las dependencias según `go.sum`. Así hay una sola fuente de verdad para la versión de Go.

```yaml
      - name: Verificar formato (gofmt)
        working-directory: backend
        run: |
          sin_formato=$(gofmt -l .)
          if [ -n "$sin_formato" ]; then
            echo "Archivos sin formato (ejecute gofmt -w .):"
            echo "$sin_formato"
            exit 1
          fi
```
- `gofmt` es el formateador oficial de Go. `gofmt -l .` **lista** los archivos que no están formateados. Si la lista no está vacía (`-n`), el paso falla (`exit 1`) y muestra cuáles son.
- En Go el formato no se discute: todo el código del mundo se ve igual. VS Code con la extensión de Go formatea al guardar.

```yaml
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v8
        with:
          version: latest
          working-directory: backend
```
- **golangci-lint** ejecuta varios analizadores estáticos a la vez (usa la configuración de `backend/.golangci.yml`).

#### Job `pruebas`

```yaml
  pruebas:
    name: Pruebas unitarias
    ...
      - name: Pruebas con cobertura
        run: go test -race -covermode=atomic -coverprofile=coverage.out ./...

      - name: Resumen de cobertura
        run: go tool cover -func=coverage.out | tail -n 1
```
- `go test ./...` ejecuta todas las pruebas de todos los paquetes.
- `-race` activa el **detector de condiciones de carrera**: encuentra accesos simultáneos a la misma memoria desde varias goroutines sin sincronización. Es importante en un servidor web, donde muchas peticiones corren en paralelo.
- `-coverprofile=coverage.out` registra qué líneas ejecutaron las pruebas; `-covermode=atomic` es el modo correcto cuando se usa `-race`.
- `go tool cover -func` imprime la cobertura por función; `tail -n 1` muestra solo el total (por ejemplo, `total: (statements) 60.0%`).

Los dos jobs corren **en paralelo** en máquinas distintas.

### 3.2 `backend/.golangci.yml`

```yaml
version: "2"

run:
  # Incluye las pruebas de integración en el análisis.
  build-tags:
    - integracion

linters:
  # errcheck, govet, ineffassign, staticcheck y unused.
  default: standard
```
- `version: "2"`: formato de configuración de golangci-lint v2.
- `build-tags: integracion`: las pruebas de integración (paso 58) solo se compilan con esa etiqueta; así también se analizan.
- `default: standard` activa el conjunto estándar:

| Linter | Qué detecta |
|---|---|
| `errcheck` | Errores que se ignoran sin querer (por ejemplo, no revisar lo que devuelve `Close()`). |
| `govet` | Construcciones sospechosas (formatos de `Printf` incorrectos, copias de candados...). |
| `ineffassign` | Asignaciones que nunca se usan. |
| `staticcheck` | Cientos de reglas: código obsoleto, condiciones imposibles, errores comunes. |
| `unused` | Funciones, tipos, constantes o campos que nadie usa. |

`unused` es la razón de una regla que se sigue en toda esta rama: **cada paso solo agrega código que ya se usa** (o que ya tiene pruebas). Una función escrita "por si acaso" haría fallar el lint.

### 3.3 `Makefile`

```makefile
GO_IMG      := golang:1.26
LINT_IMG    := golangci/golangci-lint:latest
DOCKER_GO   := docker run --rm -v "$(CURDIR)/backend:/src" -v gomod:/go/pkg/mod -v gocache:/root/.cache/go-build -w /src
```
- Variables de `make`. `DOCKER_GO` es el prefijo para ejecutar comandos de Go dentro de Docker sobre la carpeta `backend`:
  - `$(CURDIR)` es la carpeta actual;
  - los volúmenes `gomod` y `gocache` guardan las dependencias descargadas y la caché de compilación entre ejecuciones, así la segunda vez es mucho más rápida.

| Objetivo | Qué hace |
|---|---|
| `make pruebas` | `go test -race ./...` en un contenedor con Go 1.26. |
| `make lint` | Verifica el formato y ejecuta golangci-lint, igual que el job `calidad`. |
| `make formato` | Aplica `gofmt -w .` para corregir el formato. |

Recomendación: ejecutar `make formato lint pruebas` antes de cada push. Si pasa localmente, pasará en GitHub.

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| CI (integración continua) | Validar automáticamente cada cambio que llega al repositorio. |
| Workflow / job / step | Archivo del pipeline / trabajo en una máquina / instrucción dentro del trabajo. |
| Action | Paso reutilizable publicado por terceros (`uses: actions/checkout@v4`). |
| Análisis estático | Revisar el código sin ejecutarlo. |
| Condición de carrera | Dos goroutines acceden a la misma memoria a la vez y al menos una escribe. |
| Cobertura | Porcentaje del código que ejecutan las pruebas. |

---

## 5. Cómo comprobarlo

Localmente (Git Bash, WSL o Linux):

```bash
make formato
make lint       # "0 issues."
make pruebas    # ok seguimiento/internal/config, ok seguimiento/internal/handler
```

En GitHub: haga push y abra la pestaña **Actions**. Verá el workflow **CI** con los jobs "Formato y análisis estático" y "Pruebas unitarias".

Para ver un fallo a propósito: agregue espacios extra en cualquier `.go` y haga push. El paso "Verificar formato" fallará indicando el archivo.

---

## 6. Pasos para lograr esta versión desde el paso 7

1. Crear `.github/workflows/ci.yml` con los jobs `calidad` y `pruebas` (sección 3.1).
2. Crear `backend/.golangci.yml` (sección 3.2).
3. Agregar al `Makefile` las variables `GO_IMG`, `LINT_IMG` y `DOCKER_GO`, los objetivos `pruebas`, `lint` y `formato`, y sumarlos a `.PHONY` (sección 3.3).
4. Comprobar localmente: `make lint pruebas`.
5. Commit y push:
   ```bash
   git add . && git commit -m "Paso 8: integración continua con GitHub Actions"
   git push
   ```
6. Revisar la pestaña **Actions** en GitHub.

---

## 7. Preparación para las próximas entregas

- El pipeline crecerá en esta misma rama: PostgreSQL para pruebas de integración (paso 58), despliegue con Docker Compose y Newman (paso 61) y SonarQube (paso 62).
- En entregas futuras se le puede agregar un job de **entrega continua**: construir la imagen y publicarla en Artifact Registry de GCP cuando se hace merge a `main`.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Entregable: pipeline de GitHub Actions | Workflow `CI` en cada push y pull request. |
| 06-pruebas: ejecución automática de pruebas unitarias | Job `pruebas` con `-race` y cobertura. |
| RNF-05 Mantenibilidad | Formato uniforme y análisis estático obligatorio. |
