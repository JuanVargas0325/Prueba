# Paso 57 · Modelo local con Ollama

> Rama `paso-a-paso` · paso 57 de 65 · anterior: **Paso 56 · Panel de la cola** · siguiente: **Paso 58 · Pruebas de integración de la capa de datos**

---

## 1. Qué se hizo en este paso

Se agregó la opción de ejecutar un **modelo de lenguaje local** con **Ollama**, como perfil opcional de Docker Compose. Con este paso, `docker-compose.yml` y `.env.example` quedan en su **versión final**.

| Archivo | Cambio |
|---|---|
| `docker-compose.yml` | **Modificado.** Servicio `ollama` en el perfil `ollama`, volumen `modelos-ollama` y nota en el comentario inicial. |
| `.env.example` | **Modificado.** Ejemplo de configuración para Ollama. |

No se modificó código Go: Ollama expone una API **compatible con OpenAI**, así que funciona con el proveedor `openai` del paso 52.

---

## 2. Por qué

- **El stack del curso** pide "un modelo pequeño" para los reportes, y el profesor indicó que puede ser **local** o en la nube.
- **Ventajas de un modelo local:**
  - **privacidad:** las descripciones de las tareas no salen de la máquina (mitiga la deuda técnica DT-03);
  - **sin costo por uso** y **sin clave**;
  - funciona sin internet.
- **Desventajas:** consume CPU y memoria de la máquina (un modelo de 3 mil millones de parámetros pesa ~1,9 GB y necesita unos 3–4 GB de RAM) y es más lento y menos preciso que un modelo grande en la nube.
- Al ser un perfil opcional, quien no lo use no descarga nada.

---

## 3. Explicación detallada

### 3.1 `docker-compose.yml`

```yaml
  ollama:
    profiles: ["ollama"]
    image: ollama/ollama:latest
    volumes:
      - modelos-ollama:/root/.ollama
...
volumes:
  ...
  modelos-ollama:
```
- `profiles: ["ollama"]`: solo se levanta con `--profile ollama` (como Asynqmon, paso 56).
- **No publica puertos:** el worker lo alcanza por la red interna en `http://ollama:11434`.
- El volumen `modelos-ollama` guarda los modelos descargados (pueden pesar varios GB): no se vuelven a descargar al recrear el contenedor.

### 3.2 Configuración (`.env.example`)

```dotenv
# Ollama local (docker compose --profile ollama up -d
#               docker compose exec ollama ollama pull qwen2.5:3b)
# Use al menos un modelo de 3b: con qwen2.5:1.5b los resúmenes inventan datos.
#   LLM_PROVEEDOR=openai
#   LLM_URL_BASE=http://ollama:11434/v1
#   LLM_API_KEY=
#   LLM_MODELO=qwen2.5:3b
```
- `LLM_PROVEEDOR=openai` porque Ollama implementa `/v1/chat/completions` (paso 52).
- `LLM_API_KEY` vacía: Ollama local no pide clave (el cliente del paso 52 no envía la cabecera si está vacía).
- **`qwen2.5:3b` es el modelo recomendado:** es pequeño, tiene buen soporte para español y, en la prueba de la sección 5, sus resúmenes fueron fieles a las tareas. Se puede usar cualquier modelo del catálogo de Ollama, pero **no uno más pequeño** (ver sección 5.1).
- "3b" significa 3 mil millones (*billions*) de **parámetros**: los números internos que el modelo aprendió. Más parámetros = más capacidad de seguir instrucciones, pero también más memoria y más lentitud.

**La sección de IA completa de `.env.example`** ahora documenta cuatro formas de configurar el modelo: simulado (por defecto), un servicio compatible con OpenAI (Gemini, OpenAI, Groq), Ollama local y Anthropic.

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Modelo local | El LLM corre en la misma máquina, sin enviar datos a terceros. |
| API compatible | Un mismo cliente (paso 52) sirve para servicios en la nube y para Ollama. |
| Perfiles de Compose | Servicios opcionales que no se levantan por defecto. |

---

## 5. Cómo comprobarlo

Se probó de punta a punta con los datos de demostración (paso 59). **No hubo que cambiar código**: solo el `.env`.

Para probarlo:

```bash
# 1. Levantar Ollama y descargar un modelo pequeño (una sola vez)
docker compose --profile ollama up -d
docker compose exec ollama ollama pull qwen2.5:3b   # ~1,9 GB, una sola vez

# 2. En su .env
#    LLM_PROVEEDOR=openai
#    LLM_URL_BASE=http://ollama:11434/v1
#    LLM_API_KEY=
#    LLM_MODELO=qwen2.5:3b

# 3. Reiniciar el worker para que lea la nueva configuración
docker compose up -d worker
docker compose logs worker | grep "proveedor de IA"
# {"msg":"proveedor de IA configurado","proveedor":"openai","modelo":"qwen2.5:3b"}
```

Luego solicite un lote de reportes (paso 55). El resumen del PDF lo escribirá el modelo local; en CPU puede tardar decenas de segundos por reporte (el tiempo queda en el log `resumen generado`, campo `duracion_ms`). Si tarda más que `LLM_TIMEOUT` (2 min por defecto), auméntelo.

### 5.1 Resultado de la prueba: el tamaño del modelo importa

Se generaron los lotes de las semanas del 21/09 y del 28/09 de los datos de demostración con dos modelos (equipo de desarrollo, solo CPU):

| Modelo | Descarga | Lote de 3 reportes | Calidad del resumen |
|---|---|---|---|
| `qwen2.5:1.5b` | ~1 GB | 10–15 s | **Mala.** Inventó datos: dijo que el lunes 21/09 era domingo, mencionó un reporte tardío que no existía y agregó detalles que no estaban en las tareas. En otro reporte **evaluó** a la persona ("debe ser motivo de consideración…"), lo que prohíbe RN-46. |
| `qwen2.5:3b` | ~1,9 GB | 13–36 s | **Buena.** Fechas, horas, rol, estado y tardanza correctos. Los detalles que agrega salen de las descripciones de las tareas y no evalúa. |

Ejemplo real con `qwen2.5:3b` (Laura Martínez, semana del 28/09):

> Durante la semana del 2026-09-28 al 2026-10-04, la persona trabajó en el proyecto "Plataforma de analítica académica" en su rol de asistente graduado. Totalizaron 8 horas reportadas en este proyecto. La tarea principal consistía en el desarrollo del prototipo del tablero, que se enmarca en un estado de desarrollo con 8 horas de trabajo y se reportó tardía. […]

**Por qué pasa:** el prompt (paso 51) le pide al modelo no inventar ni evaluar, pero un modelo muy pequeño no siempre sigue bien las instrucciones. Por eso el PDF incluye siempre la tabla de horas y el detalle de las tareas, que salen de la base de datos y no del modelo: aunque el resumen tuviera un error, los datos del reporte son exactos.

Si una persona no registró tareas en la semana, **no se llama al modelo** y se usa un texto fijo ("No se registraron tareas…"). Por eso esos reportes no muestran proveedor ni modelo.

---

## 6. Pasos para lograr esta versión desde el paso 56

1. En `docker-compose.yml`: el servicio `ollama` (después de `asynqmon`), el volumen `modelos-ollama` y la línea `--profile ollama` en el comentario inicial.
2. En `.env.example`: el bloque de ejemplo de Ollama, entre el de Groq y el de Anthropic.
3. `docker compose --profile ollama config --services` (debe listar `ollama`).
4. Commit: `git add docker-compose.yml .env.example README.md && git commit -m "Paso 57: modelo local con Ollama"`

---

## 7. Preparación para las próximas entregas

- **En la nube** un modelo local exige una máquina con suficiente memoria (o GPU); con `qwen2.5:3b`, unos 4 GB libres solo para el modelo. Un proveedor externo (Gemini, OpenAI, Anthropic) da mejor calidad sin consumir recursos de la máquina virtual. Es una decisión de costo frente a privacidad que el equipo debe documentar en la arquitectura (sección de IA) y en la deuda técnica (DT-03).

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Stack: LLM pequeño (local o en la nube) | Ollama como opción local, configurable sin cambiar código. |
| RF-14.3 / privacidad | Con el modelo local los datos de las tareas no salen de la máquina. |
