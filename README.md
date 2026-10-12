# Paso 63 · Scripts de pruebas de carga

> Rama `paso-a-paso` · paso 63 de 65 · anterior: **Paso 62 · SonarQube** · siguiente: **Paso 64 · Resultados de las pruebas de carga**

---

## 1. Qué se hizo en este paso

Se agregaron las herramientas para medir la **capacidad** del sistema: cuántas peticiones por segundo atiende y cuánto tarda en responder a medida que aumentan los usuarios simultáneos. Siguen el tutorial de análisis de capacidad del curso.

| Archivo | Cambio |
|---|---|
| `pruebas-carga/datos_carga.sql` | **Nuevo.** Escenario de carga: 1 profesora, 40 monitores y 800 tareas. |
| `pruebas-carga/ejecutar.sh` | **Nuevo.** Ejecuta Apache Benchmark (`ab`) por niveles de concurrencia y mide CPU y memoria. |
| `pruebas-carga/analizar.py` | **Nuevo.** Resume los resultados en tablas y gráficas (Markdown + Mermaid). |
| `.gitignore` | **Modificado.** Ignora los archivos temporales que genera el script. |

En este paso solo están las herramientas. Los resultados de la corrida completa se suben en el paso 64.

---

## 2. Por qué

- **La entrega pide un análisis de capacidad**: escenarios, niveles de carga, métricas (throughput, tiempos de respuesta, errores, uso de recursos) y conclusiones.
- Con un script, la prueba es **reproducible**: cualquiera del equipo obtiene números comparables, y en las próximas entregas se puede repetir contra la nube para comparar.

### Conceptos

| Término | Significado |
|---|---|
| **Concurrencia** | Cuántos clientes envían peticiones **al mismo tiempo**. Cada uno envía otra apenas recibe la respuesta. |
| **Throughput** (peticiones/s, RPS) | Cuántas respuestas por segundo da el sistema. |
| **Latencia media** | Tiempo promedio de respuesta. |
| **Percentil 95 (p95)** | El 95 % de las peticiones tardó esto o menos. Representa la experiencia de "casi todos" mejor que la media, que esconden los casos lentos. |
| **Punto de saturación** | El nivel a partir del cual el throughput deja de crecer y solo crece la latencia: el sistema ya está al máximo y las peticiones hacen fila. |

---

## 3. Explicación detallada

### 3.1 `datos_carga.sql`: el escenario

Representa a **una profesora con un curso grande a mitad del semestre**: 40 monitores, cada uno con 2 tareas por semana durante 10 semanas (800 tareas). Es el caso más pesado de la vista principal del profesor.

```sql
INSERT INTO usuarios (id, nombre, correo, password_hash)
SELECT md5('monitor-carga-' || i)::uuid, 'Monitor de Carga ' || i, 'monitor.carga.' || i || '@uniandes.edu.co',
       crypt('Carga123*', gen_salt('bf', 10))
FROM generate_series(1, 40) AS i
ON CONFLICT (id) DO NOTHING;
```
- **`generate_series(1, 40)`** produce las filas 1, 2, …, 40. El `SELECT` crea un usuario por cada una. Así se generan muchos datos sin escribirlos uno a uno.
- `||` concatena texto en SQL.
- **`md5('monitor-carga-' || i)::uuid`**: un UUID **determinista** (siempre el mismo para el mismo `i`): el hash MD5 tiene 128 bits, igual que un UUID. Gracias a esto, el script es idempotente (`ON CONFLICT DO NOTHING`) y `ejecutar.sh` puede calcular el ID de la vinculación con `md5('vinculacion-carga-1')`.
- Contraseña de todos: `Carga123*`, con hash bcrypt (`pgcrypto`, como en el paso 59).

```sql
FROM generate_series(1, 40) AS i, generate_series(0, 9) AS s, generate_series(1, 2) AS n
```
Combina las tres series (**producto cartesiano**): 40 monitores × 10 semanas × 2 tareas = **800** filas. El estado y las horas varían con `%` (módulo) para que los datos no sean todos iguales.

### 3.2 `ejecutar.sh`: la medición

**Configuración** (cada valor se puede cambiar con variables de entorno):

```bash
NIVELES=${NIVELES:-"10 50 100 150 200"}    # usuarios concurrentes
REPETICIONES=${REPETICIONES:-3}             # corridas por nivel
DURACION=${DURACION:-15}                    # segundos por corrida
ESCENARIOS=${ESCENARIOS:-"login seguimiento tareas"}
RED=${RED:-seguimiento_default}
```
`${VAR:-valor}` significa "usa `$VAR` si existe; si no, `valor`". En total son 3 escenarios × 5 niveles × 3 repeticiones = **45 corridas** (~15 minutos).

**Los tres escenarios:**

| Escenario | Petición | Por qué |
|---|---|---|
| E1 `login` | `POST /api/v1/auth/login` | Lo más intensivo en CPU: bcrypt es lento **a propósito** (paso 12). |
| E2 `seguimiento` | `GET /api/v1/seguimiento/personas` (profesora) | La vista más consultada y la de más trabajo en la base de datos. |
| E3 `tareas` | `POST /api/v1/tareas` (monitor) | La escritura más frecuente. |

**Preparación:**
```bash
set -euo pipefail
export MSYS_NO_PATHCONV=1
```
- `set -e`: detener el script ante el primer error. `-u`: error si se usa una variable no definida. `-o pipefail`: un `a | b` falla si falla `a`.
- `MSYS_NO_PATHCONV=1`: en Git Bash de Windows, evita que rutas como `/cuerpos` se conviertan en `C:/Program Files/Git/cuerpos`.

```bash
psql "DELETE FROM tareas WHERE titulo = 'Tarea de carga'" >/dev/null
TOKEN_PROFESORA=$(token profesora.carga@uniandes.edu.co)
```
- Borra las tareas creadas por corridas anteriores de E3: cada ejecución parte del **mismo estado**.
- Obtiene los tokens JWT una sola vez. `sed -E 's/.*"token":"([^"]+)".*/\1/'` extrae el token del JSON sin depender de otras herramientas.

**Una corrida (`ejecutar_ab`):**
```bash
docker run --rm --network "$RED" -v "$DIR/cuerpos:/cuerpos:ro" "$IMAGEN_AB" \
    ab -t "$DURACION" -n 1000000 -c "$c" -k -r "${args[@]}"
```
- **Apache Benchmark (`ab`)** se ejecuta desde la imagen `httpd` (el servidor Apache la incluye), así que no hay que instalarlo.
- **`--network "$RED"`**: el contenedor de `ab` entra a la red interna de Docker Compose y le pega a `http://nginx`. Así **no mide la red de Windows/Docker Desktop**, que agregaría ruido.
- `-t 15 -n 1000000`: dura 15 s (el `-n` alto hace que el límite sea el tiempo). `-c`: concurrencia. `-k`: keep-alive (reutiliza conexiones, como un navegador). `-r`: no abortar si hay errores de conexión.
- `"${args[@]}"`: expande el arreglo de argumentos del escenario, cada uno como palabra separada.

**Medición de recursos en paralelo:**
```bash
muestrear "$parar" > "$RESULTADOS/stats/$nombre.csv" &
...
touch "$parar"
wait "$muestreo"
```
- `&` ejecuta `muestrear` **en segundo plano** mientras corre `ab`. `muestrear` repite `docker stats --no-stream` (CPU y memoria de la API y PostgreSQL) hasta que aparece el archivo `.parar`.
- `awk` convierte las unidades (`GiB`, `MiB`, `KiB`) a MB.
- La CPU puede superar el 100 %: **100 % = un núcleo**. 800 % significa 8 núcleos ocupados.

**Resumen de la corrida:** otro `awk` lee la salida de `ab` (`Requests per second`, percentiles 50/90/95/99/100) y los máximos de CPU/memoria, y agrega una línea a `resultados/resumen.csv`. Detalle: `ab` cuenta como *Failed requests* las respuestas de distinto **tamaño** que la primera. En una API JSON eso es normal, así que el script solo cuenta como errores los de conexión, excepciones y respuestas no 2xx.

### 3.3 `analizar.py`: el informe

- Agrupa las 3 repeticiones de cada (escenario, concurrencia) y calcula **media ± desviación estándar** (`statistics.mean` / `stdev`). Una desviación pequeña indica una medición estable.
- Escribe `resultados/resumen.md` con una tabla por escenario y dos gráficas **Mermaid** (`xychart-beta`): throughput y p95 frente a la concurrencia. GitHub dibuja las gráficas Mermaid directamente en el Markdown.
- Solo usa la biblioteca estándar de Python (no hay que instalar nada con `pip`).

### 3.4 `.gitignore`

```gitignore
# Cuerpos de petición que genera el script de pruebas de carga
pruebas-carga/cuerpos/
pruebas-carga/resultados/stats/.parar
```
`cuerpos/` (los JSON que envía `ab`) y la marca `.parar` son temporales; no se versionan.

---

## 4. Cómo comprobarlo

```bash
docker compose up --build -d --wait
docker compose exec -T postgres psql -U seguimiento -d seguimiento < pruebas-carga/datos_carga.sql

# Corrida corta para probar (≈ 15 s):
NIVELES="10" REPETICIONES=1 DURACION=3 bash pruebas-carga/ejecutar.sh
python pruebas-carga/analizar.py
```
Salida de la corrida corta al construir este paso (equipo de desarrollo, 10 usuarios):
```
login,10,1,399,0,132.46,...        → ~132 peticiones/s, 0 errores
seguimiento,10,1,1701,0,566.77,... → ~567 peticiones/s, 0 errores
tareas,10,1,8651,0,2882.84,...     → ~2883 peticiones/s, 0 errores
```
Para la medición completa, ejecute el script sin variables.

**Atención:** el script usa el proyecto de Compose `seguimiento` (el `name:` de `docker-compose.yml`). Si levantó el sistema con otro nombre (`docker compose -p otro`), use `COMPOSE_PROJECT_NAME=otro RED=otro_default bash pruebas-carga/ejecutar.sh`.

---

## 5. Pasos para lograr esta versión desde el paso 62

1. Crear la carpeta `pruebas-carga/` con `datos_carga.sql`, `ejecutar.sh` y `analizar.py` (sección 3).
2. Agregar al final de `.gitignore` las dos líneas de la sección 3.4.
3. Cargar los datos y hacer una corrida corta (sección 4). Borrar la carpeta `pruebas-carga/resultados` que genera la corrida de prueba; los resultados reales se agregan en el paso 64.
4. Commit: `git add . && git commit -m "Paso 63: scripts de pruebas de carga"`

---

## 6. Preparación para las próximas entregas

- **Comparar local vs. nube:** en las próximas entregas, `API_HOST` y `RED` permiten apuntar el mismo script a otra instalación. Desde una máquina virtual de prueba en la nube se usa `ab` directamente contra la IP del balanceador, con los mismos escenarios y niveles, para que los números sean comparables.
- **Escalado:** si al agregar instancias de la API (autoescalado) el punto de saturación de E1 se mueve hacia arriba, eso demuestra que el escalado horizontal funciona. La API no guarda estado (JWT, paso 12), por eso se puede escalar.
- `datos_carga.sql` usa IDs deterministas, así que también se puede cargar en Cloud SQL sin duplicar.

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Análisis de capacidad (Entrega 1) | Escenarios, niveles, repeticiones y métricas automatizadas. |
| Métricas: throughput, tiempos de respuesta, errores, CPU y memoria | Columnas de `resumen.csv`. |
