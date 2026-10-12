# Paso 5 · Dockerfile de la API

> Rama `paso-a-paso` · paso 5 de 65 · anterior: **Paso 4 · Migraciones y esquema** · siguiente: **Paso 6 · Docker Compose con PostgreSQL y la API**

---

## 1. Qué se hizo en este paso

La API se puede empaquetar como **imagen de Docker** basada en **Linux Debian 12 (bookworm)**.

| Archivo | Cambio |
|---|---|
| `backend/Dockerfile` | **Nuevo.** Construcción en varias etapas: compila con Go y copia solo el binario a una imagen Debian mínima. |
| `backend/.dockerignore` | **Nuevo.** Archivos que no se envían a Docker al construir. |

```bash
cd backend
docker build --target api -t seguimiento-api .
```

La imagen resultante pesa unos 150 MB, frente a unos 900 MB de la imagen con el compilador de Go. Ejecuta la API como un usuario sin privilegios y Docker sabe si está sana gracias a un *healthcheck* sobre `/salud`.

---

## 2. Por qué

- **Entregable de la Entrega 1:** el enunciado exige "archivos `Dockerfile`" para ejecutar el sistema de forma reproducible.
- **Stack del curso:** Linux **Debian** o Ubuntu. Todas las imágenes del proyecto usan Debian 12 (`bookworm`).
- **Reproducibilidad:** la imagen contiene el programa y todo lo que necesita. Funciona igual en su equipo, en el de sus compañeros, en GitHub Actions y en la nube.
- **Imagen multietapa:** para compilar se necesita Go (cientos de MB); para ejecutar, solo el binario. Separar las etapas produce una imagen final pequeña, que se descarga y arranca más rápido y tiene menos programas que puedan tener vulnerabilidades.

---

## 3. Explicación detallada del `Dockerfile`

Un `Dockerfile` es una receta: cada instrucción crea una **capa** de la imagen. Docker guarda las capas en caché y solo reconstruye desde la primera que cambió.

### Etapa 1 · Compilación

```dockerfile
FROM golang:1.26-bookworm AS compilacion
WORKDIR /src
```
- `FROM` parte de una imagen existente: la oficial de Go 1.26 sobre Debian 12. `AS compilacion` le da nombre a la etapa para referirse a ella después.
- `WORKDIR /src` crea la carpeta `/src` y se ubica en ella.

```dockerfile
COPY go.mod go.sum ./
RUN go mod download
```
- Primero se copian **solo** `go.mod` y `go.sum` y se descargan las dependencias.
- **Truco de caché:** las dependencias cambian poco y el código cambia mucho. Al copiarlas por separado, si solo cambió el código Docker reutiliza la capa de dependencias ya descargadas y la construcción tarda segundos en lugar de minutos.

```dockerfile
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /salida/api ./cmd/api
```
- Copia el resto del código (menos lo excluido en `.dockerignore`) y compila.
- `CGO_ENABLED=0`: compila sin depender de librerías de C del sistema. El binario es **estático** y funciona en cualquier Linux.
- `GOOS=linux`: el sistema operativo destino.
- `-trimpath`: quita del binario las rutas del equipo donde se compiló (reproducibilidad y privacidad).
- `-ldflags="-s -w"`: quita la información de depuración; el binario queda más pequeño (unos 18 MB).
- `-o /salida/api`: dónde dejar el ejecutable. `./cmd/api` es el programa a compilar.

### Etapa 2 · Base de ejecución

```dockerfile
FROM debian:bookworm-slim AS base
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates tzdata wget \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 10001 --no-create-home --shell /usr/sbin/nologin app
USER app
```
- `debian:bookworm-slim`: Debian 12 en su versión reducida (sin documentación ni herramientas extra).
- Se instalan solo tres paquetes:
  - `ca-certificates`: certificados de las autoridades de confianza, necesarios para llamadas **HTTPS** salientes (en el paso 52 la API de IA);
  - `tzdata`: zonas horarias del sistema (el binario ya trae las suyas desde el paso 1, pero así también las tienen otras herramientas);
  - `wget`: lo usa el *healthcheck* para consultar `/salud`.
- `--no-install-recommends` evita instalar paquetes "sugeridos" que no se necesitan.
- `rm -rf /var/lib/apt/lists/*` borra los índices de paquetes descargados (unos 20 MB que ya no sirven).
- Todo va en **un solo `RUN`** unido con `&&`. Si se hiciera en varios, los archivos borrados seguirían ocupando espacio en la capa anterior.
- `useradd ... app` crea un usuario sin contraseña, sin carpeta personal y sin shell. `USER app` hace que todo lo siguiente corra con ese usuario. **Por seguridad:** si alguien encontrara una falla en la API, no tendría permisos de administrador (`root`) dentro del contenedor.
- La etapa se llama `base` porque en el paso 55 la reutilizará también el worker.

### Etapa 3 · API

```dockerfile
FROM base AS api
COPY --from=compilacion /salida/api /usr/local/bin/api
EXPOSE 8080
```
- Parte de `base` y copia **únicamente el binario** desde la etapa de compilación. El compilador, el código fuente y las dependencias no llegan a la imagen final.
- `EXPOSE 8080` documenta en qué puerto escucha. No publica el puerto por sí solo; eso lo hace `docker run -p` o Docker Compose.

```dockerfile
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=5 \
  CMD wget -qO- http://127.0.0.1:8080/salud >/dev/null || exit 1
```
- Docker ejecuta este comando **dentro del contenedor** cada 10 segundos. Si `/salud` no responde 200 en 3 segundos, cuenta un fallo; con 5 fallos seguidos marca el contenedor como `unhealthy`.
- `--start-period=20s`: margen inicial (por ejemplo, mientras espera a PostgreSQL) en el que los fallos no cuentan.
- `wget -qO-` descarga la URL en silencio. Si la respuesta no es exitosa (por ejemplo, 503), `wget` termina con error y `|| exit 1` lo reporta.
- Se usa `127.0.0.1` porque se consulta a sí mismo.
- En el paso 6, Docker Compose esperará a que este estado sea `healthy` antes de dar por arrancado el sistema.

```dockerfile
ENTRYPOINT ["/usr/local/bin/api"]
```
- El programa que se ejecuta al iniciar el contenedor. La forma con corchetes (*exec form*) ejecuta el binario directamente, sin pasar por un shell; así recibe las señales de `docker stop` y puede hacer el apagado ordenado del paso 2.

### `.dockerignore`

```
*_test.go
coverage.out
*.pdf
.env
```
- Lo que **no** se envía a Docker al construir: las pruebas (no se necesitan para compilar el programa), los reportes de cobertura, los PDF y, sobre todo, cualquier `.env` con secretos, para que nunca termine dentro de una imagen.

---

## 4. Conceptos de Docker de este paso

| Concepto | Explicación |
|---|---|
| Imagen | Plantilla inmutable con el sistema de archivos y el programa. |
| Contenedor | Una ejecución de una imagen. |
| Capa | Resultado de cada instrucción; Docker las cachea. |
| Multietapa | Varias `FROM` en un archivo; la imagen final copia solo lo necesario (`COPY --from`). |
| `--target` | Construye hasta la etapa indicada (`api`; en el paso 55 también `worker`). |
| `HEALTHCHECK` | Comando que Docker ejecuta para saber si el contenedor está sano. |
| `USER` | Usuario con el que corre el proceso. |

---

## 5. Cómo comprobarlo

```bash
cd backend
docker build --target api -t seguimiento-api .
docker images seguimiento-api               # ~150 MB

# Red y base de datos de prueba
docker network create red-prueba
docker run -d --name pg --network red-prueba -e POSTGRES_USER=s -e POSTGRES_PASSWORD=s -e POSTGRES_DB=s postgres:16-bookworm

# La API en contenedor, apuntando al contenedor "pg"
docker run -d --name api --network red-prueba -p 8080:8080 \
  -e POSTGRES_USER=s -e POSTGRES_PASSWORD=s -e POSTGRES_DB=s -e POSTGRES_HOST=pg seguimiento-api

docker ps                       # después de unos segundos, la columna STATUS dice "(healthy)"
curl http://localhost:8080/salud
docker exec api id              # uid=10001(app): no corre como root

# Limpieza
docker rm -f api pg && docker network rm red-prueba
```

Dentro de una red de Docker, los contenedores se encuentran por su **nombre** (`POSTGRES_HOST=pg`). En el paso 6 Docker Compose crea esa red automáticamente.

---

## 6. Pasos para lograr esta versión desde el paso 4

1. Crear `backend/Dockerfile` con las tres etapas (`compilacion`, `base`, `api`) de la sección 3.
2. Crear `backend/.dockerignore` con las cuatro líneas de la sección 3.
3. Construir y probar como en la sección 5.
4. Commit: `git add . && git commit -m "Paso 5: Dockerfile de la API"`

---

## 7. Preparación para las próximas entregas

- **La misma imagen sirve para la nube.** En la Entrega 2 se puede construir y ejecutar en una VM; en la Entrega 4, publicar en Artifact Registry y desplegar en Cloud Run.
- **La etapa `base` compartida** permite agregar el worker (paso 55) u otros procesos sin duplicar la configuración del sistema operativo.
- **El `HEALTHCHECK`** documenta cómo saber si la instancia está sana. Los grupos de instancias de la Entrega 3 usarán la misma ruta `/salud`.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Entregable: `Dockerfile` | Imagen multietapa de la API. |
| Stack: Linux Debian | `golang:1.26-bookworm` y `debian:bookworm-slim`. |
| RNF-01 / seguridad básica | Usuario sin privilegios, imagen mínima, sin secretos dentro de la imagen. |
