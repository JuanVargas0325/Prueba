# Paso 7 · Nginx como proxy inverso

> Rama `paso-a-paso` · paso 7 de 65 · anterior: **Paso 6 · Docker Compose** · siguiente: **Paso 8 · Integración continua con GitHub Actions**

---

## 1. Qué se hizo en este paso

**Nginx** pasa a ser la única puerta de entrada al sistema. Las peticiones llegan a Nginx y Nginx las reenvía a la API, que queda oculta en la red interna de Docker.

| Archivo | Cambio |
|---|---|
| `nginx/nginx.conf` | **Nuevo.** Configuración del proxy inverso hacia la API. |
| `docker-compose.yml` | **Modificado.** Nuevo servicio `nginx`; la API deja de publicar su puerto (`ports` → `expose`). |
| `.env.example` | **Modificado.** Nueva variable `NGINX_PUERTO`. |

```
Navegador / Postman ──► localhost:8080 ──► nginx:80 ──► api:8080 ──► postgres:5432
                        (su equipo)        (contenedor)  (red interna de Docker)
```

```bash
curl -i http://localhost:8080/salud
# HTTP/1.1 200 OK
# Server: nginx            ← respondió a través de Nginx
```

---

## 2. Por qué

### ¿Qué es un proxy inverso?

- Un **proxy** es un intermediario de las peticiones HTTP.
- Un proxy **normal** (o *forward proxy*) trabaja **para los clientes**: por ejemplo, el proxy de una empresa por el que salen todos los computadores hacia Internet. El sitio de destino ve al proxy, no a los empleados.
- Un proxy **inverso** trabaja **para los servidores**: recibe las peticiones de Internet y las reparte a los servidores de atrás. El cliente ve al proxy, no a los servidores.

### ¿Por qué ponerlo delante de la API?

- **Stack del curso:** Nginx es parte del stack obligatorio.
- **Un único punto de entrada:** la API, la base de datos y (más adelante) Redis quedan en una red privada. Solo Nginx tiene un puerto abierto hacia afuera. Es la misma arquitectura que se usará en la nube.
- **Responsabilidades que no le tocan a la API:** límite de tamaño de las peticiones (paso 40), tiempos máximos, ocultar la versión del servidor y, más adelante, HTTPS o reparto de carga entre varias instancias de la API.

---

## 3. Explicación detallada

### 3.1 `nginx/nginx.conf`

```nginx
upstream api_backend {
    server api:8080;
    keepalive 16;
}
```
- `upstream` define un grupo de servidores de destino con un nombre (`api_backend`). Hoy tiene uno: `api:8080`, el servicio de Compose. El nombre `api` lo resuelve el DNS interno de Docker.
- `keepalive 16`: Nginx mantiene hasta 16 conexiones abiertas y **reutilizables** hacia la API, en lugar de abrir una conexión TCP nueva por cada petición. Reduce la latencia bajo carga.
- Si hubiera varias instancias de la API, bastaría con agregar más líneas `server` y Nginx las repartiría (balanceo de carga).

```nginx
server {
    listen 80;
    server_name _;
```
- `server` es un "sitio". `listen 80`: escucha en el puerto 80 **dentro del contenedor** (el puerto HTTP estándar).
- `server_name _`: atiende cualquier nombre de dominio (aquí solo hay un sitio).

```nginx
    server_tokens off;
```
- Nginx no revela su versión en las cabeceras ni en las páginas de error (responde `Server: nginx` en lugar de `Server: nginx/1.27.x`). Es una medida básica de seguridad: no facilita buscar vulnerabilidades de una versión concreta.

```nginx
    location / {
        proxy_pass http://api_backend;
```
- `location /` aplica a **todas** las rutas. `proxy_pass` reenvía la petición al `upstream`.

```nginx
        proxy_http_version 1.1;
        proxy_set_header Connection "";
```
- Necesarios para que funcione `keepalive`: HTTP/1.1 permite conexiones persistentes, y vaciar la cabecera `Connection` evita que se reenvíe un `Connection: close` del cliente.

```nginx
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
```
- Para la API, todas las peticiones vienen "de Nginx". Estas cabeceras le cuentan los datos **originales** del cliente:
  - `Host`: el dominio al que llamó el cliente;
  - `X-Real-IP` y `X-Forwarded-For`: la IP real del cliente (útil en los logs; la API la leerá desde el paso 10);
  - `X-Forwarded-Proto`: si el cliente usó `http` o `https`.

```nginx
        proxy_connect_timeout 5s;
        proxy_read_timeout 120s;
        proxy_send_timeout 120s;
    }
}
```
- `proxy_connect_timeout 5s`: si la API no acepta la conexión en 5 s, Nginx responde 502 en lugar de dejar al cliente esperando.
- `proxy_read_timeout` y `proxy_send_timeout` (120 s): máximo entre dos lecturas o escrituras con la API. Hay margen para operaciones lentas, como subir archivos.

### 3.2 Cambios en `docker-compose.yml`

```yaml
  api:
    ...
    expose:
      - "8080"
```
- `ports` publicaba el puerto en su computador; `expose` solo lo **documenta** dentro de la red de Docker. Desde su equipo ya no se llega a la API directamente; solo a través de Nginx.

```yaml
  nginx:
    image: nginx:1.27-bookworm
    restart: unless-stopped
    ports:
      - "${NGINX_PUERTO:-8080}:80"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/conf.d/default.conf:ro
    depends_on:
      api:
        condition: service_healthy
```
- Imagen oficial de Nginx sobre **Debian 12** (como el resto del sistema).
- `"${NGINX_PUERTO:-8080}:80"`: el puerto de su equipo sale de `NGINX_PUERTO`; si no está definido, usa 8080 (la sintaxis `:-valor` es "valor por defecto"). Así, si el 8080 de su equipo está ocupado, basta con cambiar el `.env`.
- El archivo `nginx.conf` se **monta** (no se copia) en la ruta donde Nginx busca la configuración de los sitios. `:ro` = solo lectura. Si edita `nginx.conf`, basta con `docker compose restart nginx`.
- Nginx espera a que la API esté sana. Así no responde 502 mientras la API arranca.

### 3.3 ¿Cómo se eligieron los puertos?

| Puerto | Dónde | Por qué |
|---|---|---|
| 8080 | Su equipo → Nginx | Puerto alternativo habitual de HTTP; no requiere permisos de administrador (los menores de 1024 pueden requerirlos) y no choca con un servidor web local en el 80. Configurable con `NGINX_PUERTO`. |
| 80 | Dentro del contenedor de Nginx | El puerto HTTP estándar; es el que espera la imagen oficial. |
| 8080 | Dentro del contenedor de la API | `APP_PUERTO` por defecto (paso 1). No choca con el 80 de Nginx porque cada contenedor tiene su propia red. |
| 5432 | Dentro del contenedor de PostgreSQL | El puerto estándar de PostgreSQL. No se publica. |

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Proxy inverso | Intermediario que recibe las peticiones de los clientes en nombre de los servidores. |
| `upstream` | Grupo de servidores de destino en Nginx. |
| `keepalive` | Conexiones reutilizables entre Nginx y la API. |
| Cabeceras `X-Forwarded-*` | Datos del cliente original que el proxy pasa al servidor. |
| `ports` vs `expose` | Publicar en el equipo anfitrión vs. solo dentro de la red de Docker. |
| `${VAR:-defecto}` | Valor por defecto en la interpolación de Compose. |

---

## 5. Cómo comprobarlo

```bash
docker compose up --build -d --wait
docker compose ps
# api      ... 8080/tcp                    ← sin publicar
# nginx    ... 0.0.0.0:8080->80/tcp        ← única entrada
# postgres ... 5432/tcp

curl -i http://localhost:8080/salud        # cabecera "Server: nginx"
docker compose logs nginx                  # registro de accesos de Nginx
```

Para usar otro puerto: en `.env`, `NGINX_PUERTO=9000`, y luego `docker compose up -d`.

---

## 6. Pasos para lograr esta versión desde el paso 6

1. Crear `nginx/nginx.conf` con el `upstream` y el `server` de la sección 3.1.
2. En `docker-compose.yml`:
   - en `api`, reemplazar `ports: ["8080:8080"]` por `expose: ["8080"]`;
   - agregar el servicio `nginx` (sección 3.2);
   - actualizar el comentario inicial ("...a través de Nginx").
3. En `.env.example`, debajo de `APP_ZONA_HORARIA`, agregar:
   ```dotenv
   # Puerto del equipo anfitrión donde Nginx expone la API.
   NGINX_PUERTO=8080
   ```
   y copiar la misma línea a su `.env`.
4. `docker compose up --build -d --wait` y comprobar (sección 5).
5. Commit: `git add . && git commit -m "Paso 7: Nginx como proxy inverso"`

---

## 7. Preparación para las próximas entregas

- **En la Entrega 2** (máquinas virtuales), Nginx puede quedarse en la VM web como único puerto abierto, igual que aquí.
- **En la Entrega 3** (escalado), el `upstream` admite varias instancias de la API; aunque lo más probable es que ese papel lo asuma el balanceador de carga de GCP, la API ya está preparada para vivir detrás de un proxy (cabeceras `X-Forwarded-*`).
- **HTTPS:** se puede terminar en Nginx (o en el balanceador) sin tocar la API.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Stack: Nginx | Proxy inverso delante de la API. |
| Arquitectura §4 Despliegue | Única entrada pública; API y base de datos en la red interna. |
| RNF-03 / seguridad básica | Servicios internos no expuestos; versión de Nginx oculta. |
