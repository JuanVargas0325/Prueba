# Guía: orden de implementación y división del trabajo

> **Grupo N° _completar_** · _Integrantes_
>
> Cómo se construiría el sistema desde cero y cómo repartirlo en un equipo de 3 o 4 personas. Sirve también como mapa para que cada integrante sepa qué partes del código debe poder explicar en la sustentación.

## 1. Orden de implementación desde cero

Cada paso deja algo funcionando y probado, y solo depende de los anteriores.

| Paso | Qué se construye | Archivos principales | Resultado verificable |
|---|---|---|---|
| 0 | Repositorio, módulo Go, `.gitignore`, `.env.example`, estructura de carpetas | `backend/go.mod`, `backend/cmd/`, `backend/internal/` | `go build ./...` compila |
| 1 | Configuración y conexión a la base de datos con migraciones | `config/`, `basedatos/`, `migraciones/0001_esquema_inicial.sql` | La API arranca y crea las tablas |
| 2 | Contenedores: `Dockerfile`, `docker-compose.yml` con PostgreSQL, Redis y Nginx | `backend/Dockerfile`, `docker-compose.yml`, `nginx/nginx.conf` | `docker compose up` y `GET /salud` responde |
| 3 | Pipeline de CI mínimo (formato, lint y pruebas) | `.github/workflows/ci.yml`, `.golangci.yml` | Cada push queda validado desde el inicio |
| 4 | Esqueleto de capas: errores tipados, respuestas HTTP, enrutador por módulos | `apperr/`, `handler/respuestas.go`, `handler/router.go` | Formato de error único |
| 5 | Usuarios y autenticación: bcrypt, JWT, middleware de roles, administrador inicial | `auth/`, `dominio/usuario.go`, `servicio/autenticacion.go`, `servicio/usuarios.go`, `handler/middleware.go` | Login y CRUD de usuarios (RF-01, RF-02) |
| 6 | Períodos y espacios (cursos y proyectos), cierre y visibilidad | `dominio/espacio.go`, `servicio/periodos.go`, `servicio/espacios.go`, `servicio/acceso.go` | RF-03 |
| 7 | Vinculaciones con las reglas de horas, transacción con bloqueo y rectificación | `dominio/vinculacion.go`, `servicio/vinculaciones.go`, `repositorio/vinculaciones.go` | RF-04, RF-05 (la parte más delicada) |
| 8 | Semanas y tareas: semana activa, tardanza, inmutabilidad y adjuntos | `dominio/semana.go`, `dominio/tarea.go`, `servicio/tareas.go`, `almacenamiento/` | RF-06 a RF-08 |
| 9 | Seguimiento: personas a cargo, semanas, "sin reporte", consolidado, historial | `dominio/seguimiento.go`, `servicio/seguimiento.go` | RF-09 a RF-12 |
| 10 | Reportes asíncronos: cola, worker, IA, PDF | `cola/`, `cmd/worker/`, `ia/`, `pdf/`, `servicio/reportes.go`, `servicio/generacion_reportes.go` | RF-14 |
| 11 | Pruebas de integración y colección de Postman en CI | `repositorio/integracion_test.go`, `postman/` | Regresiones detectadas automáticamente |
| 12 | SonarQube en el pipeline y corrección de hallazgos | `sonar-project.properties`, etapa del pipeline | Quality Gate aprobado |
| 13 | Pruebas de carga y análisis | `pruebas-carga/` | Capacidad medida |
| 14 | Documentación en la Wiki, video, tag y envío | `docs/wiki/` | Entrega cerrada |

**Por qué este orden:**

- **La infraestructura va primero (pasos 1 a 3):** si el `docker compose` y el pipeline existen desde el día uno, todo lo demás se integra y se prueba de forma continua, no al final.
- **Los módulos siguen las dependencias del modelo de datos:** usuario → período → espacio → vinculación → tarea → seguimiento → reporte. Ninguno se puede probar sin el anterior.
- **Dentro de cada módulo se avanza de adentro hacia afuera:** primero la regla pura en `dominio` con sus pruebas unitarias, después el servicio con dobles de prueba, luego el repositorio SQL y por último el handler. Así las reglas de negocio se validan antes de existir la base de datos o el HTTP.
- **Lo asíncrono va al final** porque consume todo lo anterior (tareas y vinculaciones de una semana), pero su infraestructura (Redis) ya estaba en el paso 2.

## 2. División del trabajo

La arquitectura por módulos permite repartir **por funcionalidad completa (vertical)**: cada persona es dueña de sus módulos en las tres capas, con sus pruebas. Repartir por capas (uno hace handlers, otro SQL) genera bloqueos constantes y nadie puede explicar un caso de uso completo.

### Propuesta para 4 integrantes

| Integrante | Responsabilidad | Código | Documentación |
|---|---|---|---|
| **A – Plataforma y calidad** | Configuración, migraciones, Docker, Nginx, pipeline, SonarQube, Postman | `config/`, `basedatos/`, `Dockerfile`, `docker-compose.yml`, `nginx/`, `.github/`, `postman/` | Arquitectura §4 Despliegue, deuda DT-10 |
| **B – Acceso y estructura académica** | Autenticación, usuarios, roles, períodos, espacios, visibilidad | `auth/`, `servicio/autenticacion.go`, `usuarios.go`, `periodos.go`, `espacios.go`, `acceso.go`, `handler/middleware.go` | Arquitectura §10 Seguridad, DT-04, DT-05 |
| **C – Reglas de carga y tareas** | Vinculaciones (reglas de horas y concurrencia), semanas, tareas, adjuntos, seguimiento | `dominio/vinculacion.go`, `semana.go`, `tarea.go`, `seguimiento.go` y sus servicios y repositorios | Arquitectura §5 Datos y §9 Reglas temporales, DT-07, DT-08 |
| **D – Reportes asíncronos y capacidad** | Cola, worker, IA, PDF, pruebas de carga | `cola/`, `cmd/worker/`, `ia/`, `pdf/`, `servicio/reportes.go`, `generacion_reportes.go`, `pruebas-carga/` | Arquitectura §6 y §7, página de pruebas de carga, DT-02, DT-03, DT-06 |

**Con 3 integrantes:** A asume también los reportes asíncronos (la cola es infraestructura) y D reparte las pruebas de carga entre todos: cada uno ejecuta y analiza el escenario de su módulo.

### Tareas compartidas

- **Arquitectura §1 a §3** (problema, estilo y componentes): en conjunto, al inicio. Fija los contratos entre módulos.
- **Revisión cruzada:** todo cambio entra por *pull request* con aprobación de otro integrante y el pipeline en verde.
- **Video:** cada integrante presenta su parte (asegura que todos sustenten).
- **Deuda técnica:** cada uno registra la de sus módulos; A consolida la tabla final.

### Calendario sugerido (2 semanas, ~240 min por persona a la semana)

| Momento | Todos | A | B | C | D |
|---|---|---|---|---|---|
| Semana 1, inicio | Leer el enunciado, modelo de datos, arquitectura inicial, convenciones | Esqueleto, compose, CI | Autenticación y usuarios | Dominio de semanas y reglas de horas (sin base de datos) | Cola, worker y prompt con datos simulados |
| Semana 1, cierre | Revisión de PR | Migraciones completas, Postman base | Períodos y espacios | Vinculaciones en SQL con bloqueo | IA configurable y PDF |
| Semana 2, inicio | Integración de módulos | SonarQube, e2e en CI | Visibilidad en todos los endpoints | Tareas, adjuntos y seguimiento | Lotes de reportes de punta a punta |
| Semana 2, cierre | Wiki, video, tag `entrega-1`, Coursera | Despliegue y calidad en la Wiki | Seguridad | Reglas en la Wiki | Pruebas de carga y su análisis |

### Puntos de integración que conviene acordar el primer día

1. El esquema SQL inicial completo (evita conflictos entre migraciones).
2. El formato de error (`apperr` → `{"error", "mensaje"}`) y los códigos HTTP.
3. El usuario autenticado (`dominio.Usuario` en el contexto de Gin) y la función de visibilidad por rol.
4. Las interfaces que cada servicio declara para sus repositorios: permiten trabajar en paralelo con dobles de prueba antes de que exista el SQL.
