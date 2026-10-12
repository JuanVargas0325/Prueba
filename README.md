# Paso 6 · Docker Compose con PostgreSQL y la API

> Rama `paso-a-paso` · paso 6 de 65 · anterior: **Paso 5 · Dockerfile de la API** · siguiente: **Paso 7 · Nginx como proxy inverso**

---

## 1. Qué se hizo en este paso

Con **un solo comando** se levanta la base de datos y la API, ya conectadas entre sí.

| Archivo | Cambio |
|---|---|
| `docker-compose.yml` | **Nuevo.** Define dos servicios (`postgres` y `api`), la red entre ellos y un volumen para los datos. |
| `Makefile` | **Nuevo.** Atajos: `make arriba`, `make abajo`, `make logs`, `make ayuda`. |

```bash
cp .env.example .env          # una sola vez; edite las contraseñas
docker compose up --build -d --wait
curl http://localhost:8080/salud
# {"estado":"ok","componentes":{"postgres":"ok"}}
```

---

## 2. Por qué

- **Entregable de la Entrega 1:** el enunciado pide un `docker-compose.yml` que permita "levantar el sistema de forma local y controlada".
- **Sin Compose** había que crear la red, el volumen y los dos contenedores a mano, con el orden y las variables correctas (paso 5). Compose describe todo eso **en un archivo versionado**: cualquier integrante obtiene exactamente el mismo entorno.
- **Un solo `.env` para todo:** las mismas variables (`POSTGRES_USER`, etc.) configuran el contenedor de PostgreSQL y la API (paso 3).

---

## 3. Explicación detallada

### 3.1 `docker-compose.yml`

```yaml
name: seguimiento
```
- Nombre del proyecto. Compose lo usa como prefijo de contenedores, redes y volúmenes (`seguimiento-api-1`, `seguimiento_datos-postgres`).

#### Servicio `postgres`

```yaml
  postgres:
    image: postgres:16-bookworm
    restart: unless-stopped
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: ${POSTGRES_DB}
```
- Imagen oficial de PostgreSQL 16 sobre Debian 12.
- `restart: unless-stopped`: si el contenedor se cae, Docker lo reinicia, salvo que usted lo haya detenido a propósito.
- `${POSTGRES_USER}`: **interpolación**. Compose lee el `.env` de la misma carpeta y reemplaza el valor. Con estas tres variables, la imagen crea el usuario y la base de datos la primera vez que arranca.

```yaml
    volumes:
      - datos-postgres:/var/lib/postgresql/data
```
- Un **volumen con nombre** guarda los archivos de la base de datos **fuera** del contenedor. Si el contenedor se borra o se recrea, los datos siguen ahí.

```yaml
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U $${POSTGRES_USER} -d $${POSTGRES_DB}"]
      interval: 5s
      timeout: 3s
      retries: 10
```
- `pg_isready` es una herramienta de PostgreSQL que responde si la base de datos acepta conexiones.
- `$${...}` con **doble `$`**: le dice a Compose "no reemplaces esto tú"; queda como `${POSTGRES_USER}` y lo resuelve el shell **dentro** del contenedor, que tiene esas variables.

```yaml
    # ports:
    #   - "5433:5432"
```
- La base de datos **no se publica** en su computador: solo la API la alcanza, por la red interna. Si quiere explorarla con DBeaver o pgAdmin, descomente estas líneas (se usa 5433 para no chocar con un PostgreSQL que ya tenga instalado).

#### Servicio `api`

```yaml
  api:
    build:
      context: ./backend
      target: api
    image: seguimiento-api:local
```
- `build`: Compose construye la imagen con el `Dockerfile` de `./backend`, hasta la etapa `api` (paso 5).
- `image`: el nombre con el que se etiqueta la imagen construida.

```yaml
    env_file: .env
    environment:
      POSTGRES_HOST: postgres
```
- `env_file` pasa **todas** las variables del `.env` al contenedor de la API.
- `environment` **sobrescribe** algunas: en su `.env`, `POSTGRES_HOST=localhost` (para ejecutar la API sin Docker), pero dentro de Compose la base de datos se llama `postgres`, el nombre del servicio. Compose crea una red donde **cada servicio es alcanzable por su nombre**.

```yaml
    depends_on:
      postgres:
        condition: service_healthy
```
- La API no arranca hasta que PostgreSQL esté **sano** según su healthcheck. Los reintentos del paso 3 siguen siendo útiles: en la nube no siempre existe este orden de arranque.

```yaml
    ports:
      - "8080:8080"
```
- Publica el puerto: `puerto_de_su_equipo:puerto_del_contenedor`. Es temporal: en el paso 7 Nginx será la única entrada y la API dejará de estar expuesta.

```yaml
volumes:
  datos-postgres:
```
- Declara el volumen con nombre que usa `postgres`.

### 3.2 `Makefile`

`make` es una herramienta clásica para guardar comandos con nombre. Cada objetivo tiene la forma:

```makefile
arriba: ## Construye y levanta todo el sistema (requiere .env)
	docker compose up --build -d --wait
```
- El texto después de `##` es la descripción que muestra `make ayuda`.
- **Las líneas de comandos deben empezar con un TAB**, no con espacios (regla de `make`).
- `.PHONY` declara que estos nombres son acciones y no archivos.
- `ayuda` busca con `grep` las líneas `objetivo: ## descripción` y las imprime con `awk`. En un `Makefile`, `$$` se escribe para obtener un `$` literal.

| Comando | Equivale a |
|---|---|
| `make arriba` | `docker compose up --build -d --wait`: construye, levanta en segundo plano (`-d`) y **espera** (`--wait`) a que todos los servicios estén sanos. |
| `make abajo` | `docker compose down`: detiene y borra los contenedores; **los datos del volumen se conservan**. |
| `make logs` | `docker compose logs -f api`: muestra los logs de la API en vivo (Ctrl+C para salir). |
| `make ayuda` | Lista los comandos disponibles. |

En Windows, `make` está disponible en Git Bash (si se instaló con los extras), en WSL o con `winget install GnuWin32.Make`. Si no lo tiene, copie el comando de la columna derecha.

---

## 4. Conceptos de Docker Compose de este paso

| Concepto | Explicación |
|---|---|
| Servicio | Un contenedor (o varios iguales) descrito en el archivo. |
| Red por defecto | Compose crea una red donde los servicios se encuentran por su nombre. |
| Volumen con nombre | Almacenamiento persistente que sobrevive a los contenedores. |
| Interpolación `${VAR}` | Compose reemplaza valores con el `.env`; `$$` escapa el `$`. |
| `env_file` / `environment` | Variables para el contenedor; `environment` tiene prioridad. |
| `depends_on` + `service_healthy` | Orden de arranque basado en healthchecks. |
| `--wait` | `up` no termina hasta que todo esté sano (o falle). |

---

## 5. Cómo comprobarlo

```bash
cp .env.example .env
# edite .env: al menos POSTGRES_PASSWORD

docker compose up --build -d --wait     # o: make arriba
docker compose ps                       # api y postgres "(healthy)"
curl http://localhost:8080/salud
docker compose logs api                 # incluye "migración aplicada"

docker compose down                     # o: make abajo
docker compose up -d --wait             # los datos siguen: no vuelve a migrar
docker compose down -v                  # -v borra también el volumen (¡todos los datos!)
```

---

## 6. Pasos para lograr esta versión desde el paso 5

1. Crear `docker-compose.yml` en la raíz con los servicios `postgres` y `api` y el volumen `datos-postgres` (sección 3.1).
2. Crear `Makefile` en la raíz con `ayuda`, `arriba`, `abajo` y `logs` (sección 3.2). Recuerde que los comandos van indentados con TAB.
3. Crear su `.env`: `cp .env.example .env` (no se sube a Git).
4. Levantar y comprobar (sección 5).
5. Commit: `git add docker-compose.yml Makefile README.md && git commit -m "Paso 6: Docker Compose con PostgreSQL y la API"`

---

## 7. Preparación para las próximas entregas

- **Cada servicio está aislado y se comunica por nombre y variables.** En la Entrega 2 la base de datos sale de Compose y pasa a ser un servicio administrado: la API seguirá igual, solo cambia `POSTGRES_HOST` o `DATABASE_URL`.
- **El volumen con nombre** separa datos de contenedores; es la misma idea que el disco persistente de una VM.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Entregable: `docker-compose.yml` | Levanta la base de datos y la API de forma reproducible. |
| Stack: Docker Compose, PostgreSQL | Servicios `postgres` y `api`. |
| Lineamientos 10: reproducibilidad | Un comando, un archivo de configuración y documentación de cómo ejecutarlo. |
