# Paso 64 · Resultados de las pruebas de carga

> Rama `paso-a-paso` · paso 64 de 65 · anterior: **Paso 63 · Scripts de pruebas de carga** · siguiente: **Paso 65 · README final**

---

## 1. Qué se hizo en este paso

Se ejecutó la medición completa con los scripts del paso 63 y se versionaron sus resultados: **45 corridas** (3 escenarios × 5 niveles de concurrencia × 3 repeticiones de 15 s).

| Archivo o carpeta | Contenido |
|---|---|
| `pruebas-carga/resultados/ab/*.txt` (45) | Salida completa de Apache Benchmark de cada corrida. |
| `pruebas-carga/resultados/stats/*.csv` (45) | Muestras de CPU y memoria de la API y PostgreSQL durante cada corrida. |
| `pruebas-carga/resultados/resumen.csv` | Una fila por corrida con las métricas principales. |
| `pruebas-carga/resultados/resumen.md` | Tablas (media ± desviación) y gráficas Mermaid que genera `analizar.py`. |

No hay código nuevo: son **evidencias**.

---

## 2. Por qué

- La entrega pide **resultados** del análisis de capacidad, no solo el plan. Versionar los archivos crudos (`ab/` y `stats/`) permite que cualquiera **verifique** las cifras del informe o las recalcule con otro criterio.
- Quedan como **línea base**: en la Entrega 2 se repiten los mismos escenarios en la nube y se comparan con estos números.

---

## 3. Explicación de los archivos

### 3.1 Un archivo de `ab/` (por ejemplo `login_c100_r1.txt` = login, 100 usuarios, repetición 1)

Las líneas importantes que produce Apache Benchmark:

| Línea | Significado |
|---|---|
| `Complete requests` | Peticiones completadas en los 15 s. |
| `Failed requests` | Cuidado: incluye respuestas de **distinto tamaño** (`Length`), que en una API JSON no son errores. El script solo cuenta `Connect`, `Receive` y `Exceptions`. |
| `Non-2xx responses` | Respuestas con código de error HTTP. Si no aparece, no hubo ninguna. |
| `Requests per second` | Throughput. |
| `Time per request (mean)` | Latencia media vista por cada cliente. |
| `Percentage of the requests served within a certain time` | Percentiles: la fila `95%` es el p95. |

### 3.2 Un archivo de `stats/`

Cada línea es una muestra de `docker stats`: `cpu_api,cpu_bd,mem_api_mb,mem_bd_mb`. La CPU está en porcentaje de **un núcleo** (1.600 % = 16 núcleos). Se toman los máximos.

### 3.3 Resultados principales (de `resumen.md`)

| Escenario | Throughput máximo | p95 con 100 usuarios | p95 con 200 usuarios | Errores |
|---|---|---|---|---|
| E1 · Inicio de sesión | ~239 peticiones/s (desde 50 usuarios) | 601 ms | 1.292 ms | 0 % |
| E2 · Vista del profesor | ~4.060 peticiones/s | 33 ms | 57 ms | 0 % |
| E3 · Registro de tareas | ~5.240 peticiones/s | 22 ms | 44 ms | 0 % |

### 3.4 Cómo leerlos

- **E1 está limitado por CPU.** Desde 50 usuarios el throughput no sube y la API usa ~1.600 % de CPU (todos los núcleos): cada login calcula **bcrypt**, que es lento a propósito para frenar ataques de fuerza bruta (paso 12). A partir de ahí solo crece la espera. Lo confirma la **ley de Little**: usuarios = throughput × tiempo de respuesta → 200 / 237 ≈ 0,84 s, igual a la media medida (843 ms).
- **E2 escala bien gracias a los pasos 42 y 43.** Sigue subiendo hasta 200 usuarios (~4.060 peticiones/s, p95 de 57 ms) con la CPU de la API en ~690 % de los 1.600 % disponibles.
- **E3 es el más eficiente:** un `INSERT` simple con validaciones. Se estabiliza en ~5.200 peticiones/s desde 50 usuarios, con PostgreSQL en ~350 % de CPU por las escrituras.
- **La memoria no es un problema:** la API usa menos de 60 MB.

### 3.5 El efecto de la optimización del seguimiento (deuda técnica DT-08)

Antes de los pasos 42 y 43, la vista del profesor traía cada tarea con datos de otras tablas y cada persona reagrupaba las tareas de **todas** las demás. Para medir el efecto, se ejecutó también el escenario E2 con el código anterior, en el **mismo equipo y el mismo día**:

| Usuarios | Antes: peticiones/s | Después: peticiones/s | Antes: p95 | Después: p95 |
|---:|---:|---:|---:|---:|
| 10 | 984 | 3.174 | 13 ms | 4 ms |
| 50 | 1.249 | 3.627 | 73 ms | 20 ms |
| 100 | 1.307 | 3.781 | 105 ms | 33 ms |
| 200 | 1.378 | 4.063 | 173 ms | 57 ms |

- **Unas 3 veces más peticiones por segundo y un tercio del tiempo de respuesta** en todos los niveles.
- Con **3.000 tareas extra** en la semana (un profesor con mucha actividad), la diferencia es mayor: de 46 a 423 peticiones/s con 10 usuarios (de 216 a 24 ms por petición).

**Nota sobre la comparación con la primera medición.** La primera versión de estos resultados se tomó otro día, con el equipo más cargado: el inicio de sesión, cuyo código no cambió, daba ~190 peticiones/s y hoy da ~239. Por eso la comparación de DT-08 se hizo midiendo ambas versiones del código en las mismas condiciones, y no contra los números viejos. Es la misma regla que exige el curso para comparar entregas: misma herramienta, mismos parámetros y mismo entorno.
- Las desviaciones entre repeticiones son bajas (menos del 3 % en la mayoría): las mediciones son confiables.

El análisis completo (criterios de aceptación, capacidad máxima y siguientes pasos) está en la página de la wiki **Entrega 1 – Pruebas de carga**.

---

## 4. Cómo comprobarlo

- Abra `pruebas-carga/resultados/resumen.md` en GitHub: las gráficas Mermaid se dibujan solas.
- Recalcular el resumen desde los datos crudos:
  ```bash
  python pruebas-carga/analizar.py
  git diff pruebas-carga/resultados/resumen.md   # no debe cambiar
  ```

---

## 5. Pasos para lograr esta versión desde el paso 63

1. Levantar el sistema y cargar los datos:
   ```bash
   docker compose up --build -d --wait
   docker compose exec -T postgres psql -U seguimiento -d seguimiento < pruebas-carga/datos_carga.sql
   ```
2. Cerrar los programas pesados del equipo (afectan las mediciones) y ejecutar la medición completa (~17 minutos):
   ```bash
   bash pruebas-carga/ejecutar.sh
   python pruebas-carga/analizar.py
   ```
3. Commit: `git add . && git commit -m "Paso 64: resultados de las pruebas de carga"`

Los números dependen del equipo: si repite la medición, sus valores serán otros. Lo importante es la **forma** de las curvas y qué recurso se satura.

---

## 6. Preparación para las próximas entregas

- **Esta es la línea base.** En la Entrega 2 (máquina virtual en GCP + Cloud SQL) se repiten exactamente estos escenarios. Conviene guardar esos resultados en otra carpeta, por ejemplo `pruebas-carga/resultados-entrega2/`, para comparar lado a lado.
- Lo que los datos ya sugieren para las próximas entregas:
  1. **E1 (inicio de sesión)** es el límite real: está ligado a la CPU por bcrypt. Es el escenario ideal para demostrar el **autoscaling por CPU** de la Entrega 3: varias instancias de la API detrás del balanceador multiplican su capacidad (la API no guarda estado).
  2. En una VM pequeña, el pool de conexiones de pgx es de 4 (lo calcula según los núcleos); si E2 o E3 esperan por conexiones, se ajusta con `pool_max_conns` en `DATABASE_URL`.
- Desde la Entrega 2, el enunciado exige k6, Locust, Artillery o JMeter en lugar de `ab`. La comparación y la recomendación están en la wiki (**Entrega 1 – Pruebas de carga**, sección 6).

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Análisis de capacidad: resultados y evidencias (Entrega 1) | Archivos crudos y resumen versionados. |
| Comparación entre entregas | Línea base reproducible. |
