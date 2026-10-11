# Entrega 1 – Reporte de deuda técnica

> **Grupo N° _completar_** · _Integrantes_
>
> Deudas identificadas durante la construcción, el análisis de SonarQube y las pruebas de carga, con su estado y su prioridad (lineamientos, sección 8).

## Deudas identificadas

### DT-01 – Pruebas de integración de los repositorios SQL

| Campo | Detalle |
|---|---|
| Descripción | Las consultas de `internal/repositorio` no tenían pruebas automáticas contra una base de datos real. |
| Componente | Capa de datos (`internal/repositorio`, `internal/basedatos`) |
| Origen | Se priorizaron primero las pruebas unitarias de la lógica de negocio. |
| Impacto | Pruebas y confiabilidad |
| Prioridad | Alta |
| Riesgo | Regresiones en la persistencia que el pipeline no detectaría. |
| Acción tomada | Se agregaron pruebas de integración (`-tags integracion`) contra PostgreSQL, incluida una prueba de concurrencia de las reglas de horas. Se ejecutan en CI con un contenedor de PostgreSQL, junto con una prueba de punta a punta (Newman sobre Docker Compose). |
| Estado | **Resuelta** |

### DT-02 – Almacenamiento de archivos en un volumen local

| Campo | Detalle |
|---|---|
| Descripción | Los PDF y adjuntos se guardan en un volumen de Docker que solo existe en una máquina. |
| Componente | `internal/almacenamiento`, volumen `archivos` |
| Origen | Decisión deliberada para la Entrega 1 (despliegue local). |
| Impacto | Escalabilidad y disponibilidad: con varias instancias en distintas máquinas (Entrega 3), los archivos no estarían compartidos. |
| Prioridad | Media (alta para la Entrega 2 o 3) |
| Riesgo | Reportes o adjuntos registrados cuyo archivo no está en la instancia que atiende la descarga. |
| Acción propuesta | Implementar la interfaz `servicio.Almacenamiento` sobre Cloud Storage. |
| Estado | Identificada |

### DT-03 – Envío de datos de estudiantes a un proveedor externo de IA

| Campo | Detalle |
|---|---|
| Descripción | Las descripciones y observaciones de las tareas se envían al servicio LLM configurado. Se excluyen el nombre y el correo, pero el texto libre podría contener datos personales. |
| Componente | `internal/ia` |
| Origen | Requerimiento RF-14 con un proveedor externo. |
| Impacto | Seguridad y privacidad |
| Prioridad | Media |
| Riesgo | Exposición de información personal a un tercero según sus políticas de retención. |
| Acción propuesta | Documentar el proveedor elegido y sus políticas; evaluar un modelo local (Ollama) o anonimizar el texto libre. |
| Estado | Parcialmente resuelta (se excluyen nombre, correo y horas contratadas) |

### DT-04 – Tokens JWT sin revocación ni renovación

| Campo | Detalle |
|---|---|
| Descripción | Los tokens son válidos hasta que expiran (`JWT_EXPIRACION`, 8 h). No hay cierre de sesión en el servidor. Si el administrador cambia los roles de un usuario, el cambio aplica en su siguiente inicio de sesión. |
| Componente | `internal/auth`, `internal/handler/middleware.go` |
| Origen | Simplificación deliberada; el enunciado pide autenticación local básica (RNF-03). |
| Impacto | Seguridad |
| Prioridad | Baja |
| Riesgo | Un token filtrado o un rol retirado siguen siendo válidos hasta la expiración. |
| Acción propuesta | Expiraciones cortas con *refresh token*, o una versión de sesión por usuario verificada en cada petición. |
| Estado | Identificada |

### DT-05 – Sin límite de intentos de inicio de sesión

| Campo | Detalle |
|---|---|
| Descripción | `/api/v1/auth/login` no limita los intentos fallidos por IP o por usuario. |
| Componente | Nginx / `internal/handler` |
| Origen | Fuera del alcance mínimo de la Entrega 1. |
| Impacto | Seguridad (fuerza bruta) |
| Prioridad | Media |
| Riesgo | Adivinación de contraseñas. |
| Acción propuesta | `limit_req` en Nginx para la ruta de login o un limitador en la API. |
| Estado | Identificada |

### DT-06 – Reportes de lotes anteriores sin limpieza

| Campo | Detalle |
|---|---|
| Descripción | Cada nuevo lote de la misma semana genera PDF nuevos. Los anteriores se conservan para trazabilidad, sin una política de retención. |
| Componente | Módulo de reportes, almacenamiento |
| Origen | Decisión provisional. |
| Impacto | Operación y costo de almacenamiento |
| Prioridad | Baja |
| Riesgo | Crecimiento indefinido del volumen. |
| Acción propuesta | Conservar solo el último lote por semana o aplicar reglas de ciclo de vida en Cloud Storage. |
| Estado | Identificada |

### DT-07 – Adjuntos validados solo por extensión

| Campo | Detalle |
|---|---|
| Descripción | Los adjuntos se aceptan según su extensión (lista blanca), sin inspeccionar el contenido. Un archivo renombrado pasaría la validación. |
| Componente | `internal/dominio/tarea.go`, `internal/servicio/tareas.go` |
| Origen | Simplificación de la primera versión. |
| Impacto | Seguridad. Mitigado: se descargan siempre como adjunto con `nosniff`, así el navegador no los ejecuta. |
| Prioridad | Baja |
| Riesgo | Almacenamiento de contenido no deseado. |
| Acción propuesta | Verificar la firma del archivo (*magic bytes*) y, en la nube, analizarlo con un antivirus. |
| Estado | Identificada |

### DT-08 – Seguimiento semanal calculado en cada consulta

| Campo | Detalle |
|---|---|
| Descripción | El estado "sin reporte" y los consolidados se calculan en memoria en cada consulta, a partir de vinculaciones y tareas. No se guardan. |
| Componente | `internal/dominio/seguimiento.go`, `internal/servicio/seguimiento.go` |
| Origen | Diseño deliberado: el estado es derivado (RN-36) y así siempre es consistente. |
| Impacto | Rendimiento. **Medido en las pruebas de carga (escenario E2):** la vista principal del profesor se estabiliza en ~765 peticiones/s sin saturar la CPU. Con ~3.000 tareas adicionales en la semana consultada cayó de 567 a 51 peticiones/s con 10 usuarios: el costo crece con el volumen de tareas, porque se traen todas las filas para agregarlas en la aplicación. |
| Prioridad | Media (subió de Baja después de medirla) |
| Riesgo | Latencia alta en `/seguimiento/personas` para profesores con muchas personas o mucha actividad semanal. |
| Acción propuesta | Agregar en SQL (horas y cantidad de tareas por persona y semana) para la vista de personas a cargo, y ajustar el pool de conexiones (`pool_max_conns`) según la CPU de la máquina de la Entrega 2. Repetir el escenario E2 para comparar. |
| Estado | Identificada y medida |

### DT-09 – Notificaciones internas pendientes

| Campo | Detalle |
|---|---|
| Descripción | El recordatorio de los viernes y el centro de notificaciones (RF-13) no están implementados. |
| Componente | Nuevo módulo |
| Origen | Fuera del alcance de la Entrega 1 según el enunciado. |
| Impacto | Funcionalidad |
| Prioridad | Media (según el alcance de las siguientes entregas) |
| Riesgo | Ninguno para la Entrega 1. |
| Acción propuesta | Implementarlo como un módulo nuevo con el patrón documentado en el README y un *scheduler* de Asynq para el recordatorio de los viernes. |
| Estado | Identificada |

### DT-10 – Hallazgos de SonarQube

El pipeline analiza el código con SonarQube (Community Build 26.9, perfil "Sonar way" para Go) en cada push. El reporte completo de cada ejecución queda en el artefacto `reporte-sonarqube` de GitHub Actions.

**Primer análisis:** 0 bugs, 0 vulnerabilidades, 0 *security hotspots*, calificación A en confiabilidad, seguridad y mantenibilidad, y **15 code smells**, que se interpretaron así:

| Regla | Cantidad | Interpretación | Decisión |
|---|---|---|---|
| `go:S3776` complejidad cognitiva > 15 | 4 | Deuda real: `Migrar`, `ConstruirSemanas`, `ConstruirConsolidado` y una prueba de integración concentraban varias responsabilidades. | Se refactorizaron en funciones pequeñas con nombre propio (`aplicarMigracion`, `construirSemana`, `totalizarTareas`, `contarSemanas`...). |
| `go:S1192` literal repetido | 5 | Deuda menor: nombres de recursos repetidos en los mensajes de error de los repositorios. | Se reemplazaron por constantes en `repositorio/comun.go`. |
| `go:S1186` función vacía | 1 | Era intencional (función de cierre sin archivos abiertos), pero no estaba explicada. | Se agregó el comentario que la justifica. |
| `go:S1135` comentario "TODO" | 5 | **Falso positivo:** la regla interpreta la palabra española "todo" ("todo lo que se necesita") como un pendiente. | Se reescribieron los comentarios para evitar la ambigüedad. |

**Estado actual:** 0 hallazgos, 0 bugs, 0 vulnerabilidades, cobertura del 79 %, duplicación del 1 % y **Quality Gate aprobado**.

**Limitación conocida:** sin una cuenta de SonarQube Cloud, el pipeline usa un servidor temporal que analiza cada vez desde cero, así que las condiciones sobre *código nuevo* del Quality Gate no comparan contra el análisis anterior. Para tener historial y medir la evolución entre entregas, se recomienda conectar SonarQube Cloud (el pipeline ya lo soporta con `SONAR_TOKEN`).

| Campo | Detalle |
|---|---|
| Prioridad | Baja |
| Estado | **Resuelta** (los 15 hallazgos se atendieron); la limitación del historial queda identificada |

## Tabla consolidada de priorización

| ID | Deuda | Prioridad | Estado | ¿Atender antes de la Entrega 2? |
|---|---|---|---|---|
| DT-01 | Pruebas de integración de repositorios | Alta | Resuelta | — |
| DT-02 | Archivos en volumen local | Media | Identificada | Sí (despliegue en GCP) |
| DT-03 | Datos enviados al proveedor de IA | Media | Parcialmente resuelta | Documentar la decisión |
| DT-05 | Sin límite de intentos de login | Media | Identificada | Recomendable |
| DT-08 | Seguimiento calculado por consulta | Media | Identificada y medida | Sí (mejora de rendimiento medible) |
| DT-09 | Notificaciones pendientes | Media | Identificada | Según el alcance de la Entrega 2 |
| DT-04 | JWT sin revocación | Baja | Identificada | No |
| DT-06 | Sin retención de PDF antiguos | Baja | Identificada | No |
| DT-07 | Adjuntos validados por extensión | Baja | Identificada | No |
| DT-10 | Hallazgos de SonarQube | Baja | Resuelta (falta historial en SonarQube Cloud) | Opcional |

**Lectura técnica:** la base de calidad es sólida: las pruebas de integración (DT-01) y el análisis de SonarQube (DT-10) quedaron resueltos, con 0 bugs, 0 vulnerabilidades y 0 code smells. Las deudas que más condicionan la siguiente entrega son dos:

- **DT-02, archivos en un volumen local:** impide escalar horizontalmente (Entrega 3) y debe resolverse al desplegar en GCP con Cloud Storage. La interfaz `servicio.Almacenamiento` ya permite el cambio sin tocar la lógica.
- **DT-08, seguimiento calculado por consulta:** es la única con impacto medido en las pruebas de carga. Es la mejora de rendimiento más rentable y la que debe compararse contra esta línea base.

DT-03 (privacidad del texto enviado al LLM) debe documentarse cuando el equipo elija el proveedor. DT-05 (límite de intentos de login) es recomendable porque, según el escenario E1, el inicio de sesión es la operación más costosa en CPU. Las demás (DT-04, DT-06, DT-07 y DT-09) pueden diferirse sin comprometer la calidad de la Entrega 2.
