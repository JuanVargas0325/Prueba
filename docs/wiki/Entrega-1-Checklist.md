# Entrega 1 – Lista de lo que se esperaba (semanas 1 y 2)

> **Grupo N° _completar_** · _Integrantes_
>
> Todo lo que pedían el enunciado de la Entrega 1, los lineamientos generales y las actividades de las semanas 1 y 2, con su estado y dónde está la evidencia. ✅ = hecho · ⏳ = pendiente del equipo.

## 1. Actividades del curso (individuales o de grupo)

| # | Actividad | Semana | Estado | Nota |
|---|---|---|---|---|
| 1 | Formulario de usuario de GitHub | 1 | ⏳ | Cada integrante; da acceso a la organización del curso. |
| 2 | Formulario de grupos de trabajo (3 a 4 integrantes) | 1 | ⏳ | Define el número de grupo que va en el repositorio, la Wiki y el video. |
| 3 | Lecturas: sistemas distribuidos, nube, centros de datos | 1 | ⏳ | Teoría; no se entrega. |
| 4 | Tutoriales: máquinas virtuales y Docker, OpenCode | 1 | ⏳ | Preparación técnica. |
| 5 | Tutoriales: Docker (comandos e imágenes), Docker Compose | 2 | ✅ aplicado | `backend/Dockerfile`, `docker-compose.yml` |
| 6 | Tutoriales: Postman, Newman | 2 | ✅ aplicado | `postman/`, etapa e2e del pipeline |
| 7 | Tutorial: análisis de capacidad (pruebas de rendimiento) | 2 | ✅ aplicado | [Pruebas de carga](Entrega-1-Pruebas-de-carga) |

## 2. Recomendaciones antes de construir (enunciado, "Recomendación de trabajo")

| # | Actividad | Estado | Evidencia |
|---|---|---|---|
| 8 | Lectura completa de la documentación | ✅ | [Trazabilidad](Entrega-1-Trazabilidad) |
| 9 | Reconocimiento de entidades y reglas de negocio | ✅ | [Arquitectura §5 Modelo de datos](Entrega-1-Arquitectura) |
| 10 | Formulación inicial de la arquitectura | ✅ | [Arquitectura §2 y §3](Entrega-1-Arquitectura) |
| 11 | Distribución de responsabilidades | ⏳ | Propuesta en [Orden de implementación y división del trabajo](Guia-de-implementacion-y-trabajo-en-equipo) |
| 12 | Estrategia de pruebas | ✅ | README → "Pruebas y calidad" |
| 13 | Pipeline mínimo de CI | ✅ | `.github/workflows/ci.yml` |

## 3. Entregables mínimos de la Entrega 1

| # | Entregable | Estado | Evidencia |
|---|---|---|---|
| 14 | Backend completo según la especificación (RF, RN, RNF y stack) | ✅ | `backend/` · detalle en [Trazabilidad](Entrega-1-Trazabilidad) |
| 15 | Fuera de alcance respetado (sin frontend, notificaciones, aprobaciones, reinicio de contraseña ni desactivación) | ✅ | "Sin reporte" se implementó por adelantado |
| 16 | `Dockerfile` | ✅ | `backend/Dockerfile` (destinos `api` y `worker`, Debian 12) |
| 17 | `docker-compose.yml` reproducible en local | ✅ | Nginx, API, worker, PostgreSQL, Redis |
| 18 | Pruebas unitarias de componentes clave y endpoints | ✅ | `backend/internal/**/*_test.go` |
| 19 | Pipeline de GitHub Actions que ejecute las pruebas | ✅ | Pestaña Actions, ejecución en verde |
| 20 | Análisis con SonarQube en el pipeline | ✅ | Etapa "Análisis SonarQube" (artefacto `reporte-sonarqube`) |
| 21 | Documento de arquitectura (vista general, componentes, despliegue, datos, flujo asíncrono, nube) | ✅ | [Arquitectura](Entrega-1-Arquitectura) |
| 22 | Documento de deuda técnica (incluye los hallazgos de SonarQube) | ✅ | [Deuda técnica](Entrega-1-Deuda-tecnica) |
| 23 | Pruebas de API con Postman | ✅ | `postman/seguimiento.postman_collection.json` |
| 24 | Pruebas de carga con análisis de capacidad (≥ 3 corridas por escenario) | ✅ | [Pruebas de carga](Entrega-1-Pruebas-de-carga) |

## 4. Lineamientos generales (lista de verificación antes de entregar)

| # | Verificación | Estado | Responsable / nota |
|---|---|---|---|
| 25 | Un solo repositorio para todo el proyecto | ✅ | |
| 26 | Profesor y tutores con acceso al repositorio y a la Wiki | ⏳ | Agregarlos como colaboradores si el repositorio no es de la organización del curso. |
| 27 | Número de grupo y nombres visibles (README, Wiki, video) | ⏳ | Reemplazar los `_completar_`. |
| 28 | Wiki completa y en Markdown (sin enlaces a Google Docs ni Word) | ✅ | Esta Wiki |
| 29 | Diagrama de despliegue final | ⏳ | Actualizar las etiquetas de la imagen a Debian 12 (`bookworm`). |
| 30 | Elección del proveedor de IA documentada | ⏳ | [Arquitectura §7](Entrega-1-Arquitectura) y DT-03 |
| 31 | Sin credenciales en el repositorio | ✅ | `.env` en `.gitignore`; solo `.env.example` |
| 32 | Cada integrante con commits propios | ⏳ | El historial se usa para valorar la participación. |
| 33 | Declaración del uso de herramientas de IA | ⏳ | Sección final del [Inicio](Home) |
| 34 | Video de 10 a 20 minutos (no listado) | ⏳ | Guion sugerido abajo |
| 35 | Tag `entrega-1` sobre el commit final | ⏳ | `git tag entrega-1 && git push origin entrega-1` |
| 36 | Envío marcado en Coursera | ⏳ | |

## 5. Guion sugerido para el video

1. Grupo, integrantes y entrega (30 s).
2. Problema y solución general (1 min).
3. Arquitectura: componentes, despliegue, modelo de datos y flujo asíncrono (4 min).
4. Demostración: `docker compose up`, inicio de sesión, crear período, espacio y vinculación; violar una regla de horas (422); registrar tareas, una tardía; seguimiento con "sin reporte"; lote de reportes y descarga del PDF (6 min).
5. Calidad: pipeline en verde, SonarQube y Newman (2 min).
6. Pruebas de carga: escenarios, resultados y cuello de botella (3 min).
7. Deuda técnica y próximos pasos (1 min).
