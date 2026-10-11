# Entrega 1 – Reporte de deuda técnica

> **Grupo N° _completar_** · _Integrantes_
>
> Borrador con las deudas identificadas durante la construcción. ✏️ El equipo debe complementarlo con los hallazgos de **SonarQube**, interpretados y no solo exportados (lineamientos, sección 8).

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
| Impacto | Rendimiento con muchos usuarios o rangos largos. Mitigado: el rango máximo es de 53 semanas y hay índices por vinculación y semana. |
| Prioridad | Baja (revisar con las pruebas de carga de la Entrega 2) |
| Riesgo | Latencia alta en `/seguimiento/personas` con profesores que tengan muchas personas. |
| Acción propuesta | Medir en las pruebas de carga; si hace falta, agregar en SQL o guardar un resumen semanal. |
| Estado | Identificada |

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

### DT-10 – Hallazgos de SonarQube ✏️

_Completar con el análisis del primer escaneo: principales code smells, vulnerabilidades y duplicaciones, cuáles son deuda real y cuáles se aceptan como decisión consciente._

## Tabla consolidada de priorización

| ID | Deuda | Prioridad | Estado | ¿Atender antes de la Entrega 2? |
|---|---|---|---|---|
| DT-01 | Pruebas de integración de repositorios | Alta | Resuelta | — |
| DT-02 | Archivos en volumen local | Media | Identificada | Sí (despliegue en GCP) |
| DT-03 | Datos enviados al proveedor de IA | Media | Parcialmente resuelta | Documentar la decisión |
| DT-05 | Sin límite de intentos de login | Media | Identificada | Recomendable |
| DT-09 | Notificaciones pendientes | Media | Identificada | Según el alcance de la Entrega 2 |
| DT-04 | JWT sin revocación | Baja | Identificada | No |
| DT-06 | Sin retención de PDF antiguos | Baja | Identificada | No |
| DT-07 | Adjuntos validados por extensión | Baja | Identificada | No |
| DT-08 | Seguimiento calculado por consulta | Baja | Identificada | Medir en las pruebas de carga |

**Lectura técnica:** ✏️ _Resumir las deudas más críticas. Por ejemplo, DT-02 bloquea el escalamiento horizontal de la Entrega 3 y DT-08 debe medirse en las pruebas de carga de la Entrega 2._
