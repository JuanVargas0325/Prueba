# Despliegue y ejecución (Entrega 1, local)

> **Grupo N° _completar_** · _Integrantes_
>
> Cómo levantar y probar el sistema de forma reproducible. La Entrega 1 no exige nube: todo corre con Docker Compose sobre imágenes Linux Debian 12 (`bookworm`).

## Requisitos

- Docker Desktop (o Docker Engine) con Docker Compose v2.
- No hace falta instalar Go: la compilación ocurre dentro del `Dockerfile`.

## Servicios

| Servicio | Imagen | Puerto expuesto | Función |
|---|---|---|---|
| `nginx` | `nginx:1.27-bookworm` | **8080** → 80 | Proxy inverso: única entrada al sistema |
| `api` | `backend/Dockerfile` (destino `api`) | interno 8080 | API REST en Go + Gin |
| `worker` | `backend/Dockerfile` (destino `worker`) | — | Consume la cola Asynq y genera los reportes |
| `postgres` | `postgres:16-bookworm` | interno 5432 | Base de datos |
| `redis` | `redis:7-bookworm` | interno 6379 | Broker de la cola |

Volúmenes: `datos-postgres` (base de datos), `datos-redis` (cola) y `archivos` (adjuntos y PDF, compartido por la API y el worker).

## Pasos

```bash
# 1. Configuración (las credenciales nunca se versionan)
cp .env.example .env
#    Editar como mínimo JWT_SECRETO (32+ caracteres), POSTGRES_PASSWORD y ADMIN_PASSWORD.

# 2. Construir y levantar
docker compose up --build -d --wait

# 3. Verificar
curl http://localhost:8080/salud

# 4. (Opcional) datos de demostración
docker compose exec -T postgres psql -U seguimiento -d seguimiento < scripts/datos_demo.sql

# 5. Recorrido completo con Newman
docker run --rm -v "$(pwd)/postman:/etc/newman" postman/newman:ubuntu run seguimiento.postman_collection.json \
  --env-var "baseUrl=http://host.docker.internal:8080" --env-var "adminPassword=<ADMIN_PASSWORD>"
```

Al arrancar, la API aplica las migraciones y crea el administrador inicial (`ADMIN_CORREO`, `ADMIN_PASSWORD`).

**Usuarios de demostración** (contraseña `Demo123*`): `profesora.rojas@uniandes.edu.co`, `andres.monitor@uniandes.edu.co`, `laura.asistente@uniandes.edu.co`, `pedro.sintareas@uniandes.edu.co`.

## Modelo de IA

Se elige solo con el `.env` (`LLM_PROVEEDOR` = `simulado`, `openai` o `anthropic`, más `LLM_URL_BASE`, `LLM_API_KEY` y `LLM_MODELO`). Después de cambiarlo: `docker compose up -d worker`. La tabla completa de proveedores está en el `README.md`.

## Comandos útiles

| Comando | Para qué |
|---|---|
| `docker compose logs -f api worker` | Ver logs |
| `docker compose --profile monitoreo up -d` | Panel de la cola (Asynqmon) en `http://localhost:8081` |
| `docker compose down` | Detener conservando los datos |
| `docker compose down -v` | Detener y borrar datos y archivos |
| `make pruebas` / `make pruebas-integracion` / `make lint` / `make e2e` | Pruebas y calidad (Git Bash, WSL, Linux o macOS) |

## Evidencia en el pipeline

La etapa "Despliegue con Docker Compose y pruebas de punta a punta" de GitHub Actions repite estos pasos en cada push (construye las imágenes, levanta el sistema y ejecuta la colección de Postman), así que el despliegue local queda verificado de forma automática.
