# Paso 60 · Colección de Postman (prueba de punta a punta)

> Rama `paso-a-paso` · paso 60 de 65 · anterior: **Paso 59 · Datos de demostración** · siguiente: **Paso 61 · Newman en la integración continua**

---

## 1. Qué se hizo en este paso

Se agregó una **colección de Postman** que recorre toda la API, del inicio de sesión a la descarga del PDF del reporte, y comprueba cada respuesta con pruebas automáticas. También se agregó `make e2e`, que la ejecuta con **Newman**, el ejecutor de colecciones de Postman por línea de comandos.

| Archivo | Cambio |
|---|---|
| `postman/seguimiento.postman_collection.json` | **Nuevo.** 73 peticiones en 10 carpetas, con 104 comprobaciones. |
| `postman/archivos/evidencia.txt` | **Nuevo.** Archivo que se sube como adjunto en la petición *multipart*. |
| `Makefile` | **Modificado.** Variables `NEWMAN_IMG` y `ADMIN_PASSWORD` y objetivo `e2e`. |

Resultado contra el sistema levantado con `docker compose`:

```
│  requests     │ 73  │ 0 │
│  assertions   │ 104 │ 0 │
```

---

## 2. Por qué

- **La entrega exige una colección de Postman** que demuestre el funcionamiento de la API (lineamientos de la Entrega 1).
- Las pruebas unitarias (pasos 9 a 55) y de integración (paso 58) prueban piezas por separado. Esta colección prueba el **sistema completo, tal como lo usaría un cliente**: Nginx → API → PostgreSQL → Redis → worker → PDF. Por eso se llama prueba **de punta a punta** (*end to end*, **e2e**).
- Es documentación ejecutable: quien abra la colección en Postman ve cada endpoint con un ejemplo real de petición.

---

## 3. Explicación detallada

### 3.1 ¿Qué es Postman? ¿Qué es Newman?

- **Postman** es una aplicación de escritorio para enviar peticiones HTTP a mano. Una **colección** es un archivo JSON con peticiones ordenadas en carpetas. Se importa con *File → Import*.
- Cada petición puede tener dos scripts de JavaScript:
  - **pre-request** (antes de enviarla): prepara datos;
  - **tests** (después de recibir la respuesta): comprueba la respuesta con `pm.test(...)` y guarda valores para las peticiones siguientes.
- **Newman** ejecuta la misma colección **sin interfaz gráfica**. Se usa en la terminal y en la integración continua (paso 61). Aquí se ejecuta con su imagen oficial de Docker (`postman/newman:ubuntu`), así que no hace falta instalar Node.js.

### 3.2 Las carpetas

| Carpeta | Qué prueba |
|---|---|
| 0. Preparación | `/salud`, inicio de sesión del administrador y contraseña incorrecta (401). |
| 1. Usuarios y roles | Crear profesora, monitora/asistente y monitor; correo inválido; un monitor no puede crear usuarios (403). |
| 2. Períodos | Crear, duplicado (409) y filtro por estado. |
| 3. Cursos y proyectos | Crear, fechas fuera del período, permisos y listado. |
| 4. Vinculaciones | Reglas de horas: 40 % (RN-17), máximo 22 h (RN-14), repetida, rol requerido. |
| 5. Rectificación | Solo el administrador cambia horas (RN-12) e historial. |
| 6. Tareas | Crear (JSON y *multipart* con adjunto), tardía, futura, validaciones, editar/eliminar, visibilidad. |
| 7. Seguimiento | Personas a cargo, detalle semanal, consolidado, historial propio y permisos. |
| 8. Reportes con IA | Lote → espera al worker → listar → detalle → **PDF**. |
| 9. Cierre | Curso cerrado (no admite tareas ni vinculaciones, pero se consulta) y cierre de la ventana de creación. |

Cada petición comprueba **el código HTTP** y, cuando aplica, **el contenido**. Por ejemplo, el caso del 40 %:

```js
pm.test('Responde 422', () => pm.response.to.have.status(422));
const r = (pm.response.headers.get('Content-Type') || '').includes('json') ? pm.response.json() : {};
pm.test("Explica la regla del 40 %", () => { pm.expect(r.mensaje).to.include('40'); });
```
- `pm` es el objeto de Postman disponible en los scripts. `pm.response` es la respuesta y `pm.expect` hace las comprobaciones (estilo de la librería Chai).
- `r` solo se interpreta como JSON si la respuesta es JSON (el PDF no lo es).

### 3.3 Cómo pasa datos de una petición a otra

```js
pm.collectionVariables.set('profesorId', r.id);   // en "Crear profesora"
```
y luego en otra petición: `"usuario_id": "{{profesorId}}"`. Las `{{variables}}` se reemplazan antes de enviar. Así funciona la cadena de tokens (`tokenAdmin`, `tokenProfesor`, `tokenMonitor`…) e IDs.

Variables propias de la colección (se pueden cambiar sin editar el archivo):

| Variable | Valor por defecto | Para qué |
|---|---|---|
| `baseUrl` | `http://localhost:8080` | Dirección de la API. |
| `adminCorreo` | `admin@uniandes.edu.co` | `ADMIN_EMAIL` del `.env`. |
| `adminPassword` | `cambie-esta-clave` | `ADMIN_PASSWORD` del `.env`. |
| `password` | `Demo12345*` | Contraseña de los usuarios que crea. |

### 3.4 Se puede ejecutar muchas veces

La primera petición genera **datos únicos** para cada ejecución:

```js
// La API calcula las semanas en hora de Bogotá (UTC-5, sin horario de verano); se usa esa
// fecha y no la UTC para que la semana pasada lo sea también para la API.
const hoy = new Date(Date.now() - 5 * 3600000);
pm.collectionVariables.set('sufijo', Date.now().toString(36));
pm.collectionVariables.set('codigoPeriodo', (2100 + Math.floor(Math.random() * 7800)) + '-' + ...);
pm.collectionVariables.set('semanaPasada', fmt(sumar(hoy, -7)));
```
- `sufijo` se agrega a los correos (`profesora.<sufijo>@…`), así que nunca chocan con los de ejecuciones anteriores.
- El código de período es aleatorio (`2100-10` … `9899-98`).
- Las fechas son **relativas a hoy**, así que la colección no "caduca".

**Detalle importante de la zona horaria (corregido en este paso):** `new Date()` en JavaScript da la hora UTC. Entre las 19:00 y las 24:00 de un domingo en Colombia, en UTC ya es **lunes**. Entonces "hoy − 7 días" caía en la semana **actual** de Bogotá y la tarea "de la semana pasada" no quedaba marcada como tardía: fallaban 6 comprobaciones. Restar 5 horas (`- 5 * 3600000` milisegundos) hace que las fechas coincidan con las que calcula la API (`APP_ZONA_HORARIA=America/Bogota`, paso 1).

### 3.5 La espera al worker

```js
// Espera a que el worker procese el lote.
setTimeout(() => {}, 4000);
```
Generar reportes es **asíncrono** (paso 47): la API responde 202 enseguida y el worker trabaja después. Este *pre-request* espera 4 s antes de consultar el lote; Newman espera a que terminen los temporizadores pendientes. Con el proveedor de IA `simulado` sobra tiempo.

### 3.6 `make e2e`

```makefile
NEWMAN_IMG  := postman/newman:ubuntu
ADMIN_PASSWORD ?= $(shell grep '^ADMIN_PASSWORD=' .env 2>/dev/null | cut -d= -f2)

e2e: ## Ejecuta la colección de Postman contra el sistema levantado
	docker run --rm -v "$(CURDIR)/postman:/etc/newman" $(NEWMAN_IMG) run seguimiento.postman_collection.json \
		--env-var "baseUrl=http://host.docker.internal:8080" --env-var "adminPassword=$(ADMIN_PASSWORD)"
```
- `-v "$(CURDIR)/postman:/etc/newman"` monta la carpeta `postman/` en el contenedor; Newman busca allí la colección y el adjunto (`archivos/evidencia.txt`).
- `host.docker.internal` es el nombre con el que un contenedor llega **al equipo anfitrión** (donde Nginx publica el puerto 8080). En Linux puede requerir `--add-host=host.docker.internal:host-gateway`.
- `?=` asigna solo si la variable no viene de fuera: `make e2e ADMIN_PASSWORD=otra` la reemplaza.
- `$(shell …)` ejecuta un comando al leer el Makefile y toma su salida (aquí, la clave del `.env`).

---

## 4. Cómo comprobarlo

```bash
docker compose up --build -d --wait
make e2e
```

Sin `make` (por ejemplo en Git Bash de Windows):

```bash
MSYS_NO_PATHCONV=1 docker run --rm -v "$(pwd -W)/postman:/etc/newman" postman/newman:ubuntu \
  run seguimiento.postman_collection.json \
  --env-var "baseUrl=http://host.docker.internal:8080" --env-var "adminPassword=cambie-esta-clave"
```
(`MSYS_NO_PATHCONV=1` evita que Git Bash convierta `/etc/newman` en una ruta de Windows; `pwd -W` da la ruta en formato Windows que entiende Docker Desktop.)

En Postman: *Import* → el archivo `.json` → *Run collection*. Si cambió `ADMIN_PASSWORD`, edite la variable `adminPassword` de la colección.

Debe terminar con **0 fallos**.

---

## 5. Pasos para lograr esta versión desde el paso 59

1. Crear `postman/archivos/evidencia.txt` con una línea de texto.
2. Crear `postman/seguimiento.postman_collection.json`. Se puede construir en Postman (carpetas, peticiones, scripts) y exportar con *Export → Collection v2.1*. Aquí se copia la versión del repositorio.
3. En el `Makefile`: las variables `NEWMAN_IMG` y `ADMIN_PASSWORD` al inicio, `e2e` en `.PHONY` y el objetivo `e2e` al final.
4. `docker compose up --build -d --wait` y `make e2e`.
5. Commit: `git add . && git commit -m "Paso 60: colección de Postman"`

---

## 6. Preparación para las próximas entregas

- **Prueba de humo del despliegue en la nube:** la misma colección sirve contra la URL del balanceador de carga cambiando solo `baseUrl`:
  `newman run seguimiento.postman_collection.json --env-var baseUrl=http://<ip-del-balanceador> --env-var adminPassword=...`
  Si pasa, el despliegue funciona de punta a punta.
- Al agregar endpoints en futuras entregas, conviene agregar su petición aquí: el paso 61 hace que la integración continua la ejecute en cada push.

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Colección de Postman (entregables de la Entrega 1) | La colección completa. |
| RF-01 a RF-12 | Al menos una petición de éxito y otra de error por funcionalidad. |
| Reglas de negocio (RN-04, RN-07, RN-12, RN-14, RN-17, RN-24, RN-27, RN-32 a RN-34) | Casos con nombre de la regla. |
