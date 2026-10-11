# Entrega 1 – Trazabilidad: requisito → código

> **Grupo N° _completar_** · _Integrantes_
>
> Para cada exigencia del enunciado se indica dónde está implementada y dónde se prueba. Las rutas son relativas a [`backend/internal/`](https://github.com/JuanVargas0325/Prueba/tree/main/backend/internal) salvo que se indique otra cosa.

**Cómo leer las capas:** el **dominio** contiene la regla pura (sin base de datos ni HTTP), el **servicio** la aplica en el caso de uso junto con los permisos, el **handler** expone el endpoint y el **repositorio** persiste. Las pruebas están junto al código (`*_test.go`), más la colección de Postman (`postman/`) y las pruebas de integración (`repositorio/integracion_test.go`).

## 1. Requerimientos funcionales

### RF-01 · Autenticación y acceso

| Requisito | Implementación | Endpoint | Pruebas |
|---|---|---|---|
| RF-01.1 Inicio de sesión local | `servicio/autenticacion.go` → `IniciarSesion`; `auth/auth.go` → `VerificarPassword` (bcrypt), `GestorTokens.Generar` (JWT) | `POST /api/v1/auth/login` | `servicio/autenticacion_test.go`, `auth/auth_test.go` |
| RF-01.2 Control de acceso por rol | `handler/middleware.go` → `Autenticar`, `RequiereRol`; cada handler declara sus roles en `Registrar` | todas las rutas protegidas | `handler/router_test.go` → `TestControlDeAccesoPorRol` |
| RF-01.3 Varios roles por persona | `dominio/usuario.go` → `Usuario.Roles`, `TieneRol`; tabla `usuario_roles` | — | `auth/auth_test.go` → `TestTokenIdaYVuelta` |

### RF-02 y RF-15.2 · Gestión de usuarios (administrador)

| Requisito | Implementación | Endpoint | Pruebas |
|---|---|---|---|
| RF-02.1 Crear usuarios | `servicio/usuarios.go` → `Crear` (valida nombre, correo, contraseña y roles) | `POST /api/v1/usuarios` | `servicio/modulos_test.go` → `TestCrearUsuario` |
| RF-02.2 Asignar roles | `servicio/usuarios.go` → `AsignarRoles` (impide quitar un rol en uso o el propio rol de administrador) | `PUT /api/v1/usuarios/{id}/roles` | `TestAsignarRoles` |
| RF-02.3 Consulta global | `servicio/usuarios.go` → `Listar`, `Obtener`; `repositorio/usuarios.go` → `Listar` | `GET /api/v1/usuarios` | `repositorio/integracion_test.go` → `TestUsuarios` |
| Administrador inicial | `servicio/autenticacion.go` → `AsegurarAdministrador` (variables `ADMIN_*`) | — | `TestAsegurarAdministrador` |

### RF-03 · Cursos y proyectos

| Requisito | Implementación | Endpoint | Pruebas |
|---|---|---|---|
| RF-03.1 Crear cursos y proyectos | `servicio/espacios.go` → `Crear` | `POST /api/v1/espacios` | `TestCrearEspacio` |
| RF-03.2 Solo en un período habilitado; períodos explícitos | `dominio/espacio.go` → `PermiteCrearEspacios`, `ValidarFechasEnPeriodo`; `servicio/periodos.go` | `POST/GET/PATCH /api/v1/periodos` | `dominio/reglas_test.go` → `TestPeriodoPermiteCrearEspacios`; `TestPeriodos` |
| RF-03.3 Información mínima | `dominio.Espacio` (nombre, período, fechas, profesor, observaciones) | — | `TestCrearEspacio` |
| RF-03.4 Consultar sus espacios | `servicio/espacios.go` → `Listar` (filtro `VisiblePara`) | `GET /api/v1/espacios` | `TestVisibilidadYCierreDeEspacios` |
| RF-03.5 Cerrar | `servicio/espacios.go` → `Cerrar`; `repositorio/espacios.go` → `Cerrar` | `POST /api/v1/espacios/{id}/cerrar` | `TestEspaciosVisibilidadYCierre` |
| RF-03.6 Consultar cerrados (solo lectura) | `handler/espacios.go` → campo `solo_lectura` | `GET /api/v1/espacios/{id}` | Postman, carpeta 9 |
| RF-03.7 Sin registros en espacios cerrados | `dominio/espacio.go` → `PermiteRegistros`, usado al vincular y al crear o editar tareas | — | `TestCrearVinculacion`, `TestModificarTareas` |

### RF-04 y RF-05 · Vinculaciones y reglas de horas

| Requisito | Implementación | Endpoint | Pruebas |
|---|---|---|---|
| RF-04.1 a 04.3 Vinculación manual con rol y horas | `servicio/vinculaciones.go` → `Crear` (por `usuario_id` o `correo`) | `POST /api/v1/vinculaciones` | `TestCrearVinculacion` |
| RF-04.4 Horas no modificables | No existe ningún endpoint de edición para el profesor; solo la rectificación del administrador | — | Postman: "El profesor no puede modificar horas" |
| RF-04.5 Rectificación (administrador) | `servicio/vinculaciones.go` → `Rectificar`; `repositorio/vinculaciones.go` → `Rectificar` (guarda el historial) | `PUT /api/v1/vinculaciones/{id}`, `GET …/rectificaciones` | `TestRectificarVinculacion`; `TestVinculacionesReglasYConcurrencia` |
| RF-04.6 Vinculaciones independientes | Tabla `vinculaciones` con `UNIQUE (usuario_id, espacio_id, rol)` | — | `TestVinculacionesReglasYConcurrencia` |
| RF-05.1 Máximo 22 h de asistencia | `dominio/vinculacion.go` → `ValidarCargaHoraria` | — | `dominio/reglas_test.go` → `TestValidarCargaHoraria` |
| RF-05.2 Máximo 3 monitorías | `ValidarCargaHoraria` | — | `TestValidarCargaHoraria` |
| RF-05.3 Máximo 12 h de monitoría | `ValidarCargaHoraria` | — | `TestValidarCargaHoraria` |
| RF-05.4 y 05.5 Regla del 40 % con redondeo hacia arriba | `dominio/vinculacion.go` → `LimiteMonitoriaMixta` | — | `TestLimiteMonitoriaMixtaRedondeaHaciaArriba` (22 h → 9 h; 12 h → 5 h) |
| RF-05.6 Validaciones bloqueantes | `repositorio/vinculaciones.go` → `Crear` y `Rectificar`: transacción con `SELECT … FOR UPDATE` sobre el usuario y validador `dominio.ValidadorCarga` | responde 422 | `TestVinculacionesReglasYConcurrencia` (dos vinculaciones simultáneas: solo una pasa) |

### RF-06 a RF-08 · Tareas semanales, estados y semanas

| Requisito | Implementación | Endpoint | Pruebas |
|---|---|---|---|
| RF-06.1 Crear tareas | `servicio/tareas.go` → `Crear` | `POST /api/v1/tareas` | `TestCrearTareaEnLaSemanaActual` |
| RF-06.2 Una sola vinculación | `tareas.vinculacion_id`; el servicio verifica que la vinculación sea propia | — | `TestCrearTareaValidaciones` |
| RF-06.3 Información mínima | `dominio/tarea.go` → `DatosTarea.Normalizar` | — | `TestDatosTareaNormalizar` |
| RF-06.4 Adjuntos | `servicio/tareas.go` → `guardarArchivos`, `AgregarAdjuntos`, `AbrirAdjunto`; `dominio/tarea.go` → `ValidarArchivoAdjunto`; `almacenamiento/local.go` | `POST/GET/DELETE /api/v1/tareas/{id}/adjuntos…` (también en la creación, como multipart) | `TestCrearTareaConAdjuntos`, `handler/modulos_test.go` → `TestCrearTareaConAdjuntosMultipart` |
| RF-06.5 Horas enteras | Campo `int` en JSON (un decimal responde 400) | — | `TestCrearTareaRechazaHorasNoEnterasYCamposDesconocidos` |
| RF-06.6 Varias tareas por semana | Sin restricción de unicidad por semana | — | Postman, carpeta 6 |
| RF-06.7 y 06.8 Editar y eliminar en la semana activa | `servicio/tareas.go` → `Actualizar`, `Eliminar`, `verificarModificable` | `PUT/DELETE /api/v1/tareas/{id}` | `TestModificarTareas` |
| RF-06.9 Nuevo registro para otra semana | La semana y la vinculación no se editan (`Tarea.Aplicar` solo cambia los datos) | — | `TestModificarTareas` |
| RF-07.1 y 07.2 Estados abierto, en desarrollo, finalizado | `dominio/tarea.go` → `EstadoTarea`; `CHECK` en la tabla | — | `TestDatosTareaNormalizar` |
| RF-08.1 Semana de lunes a domingo | `dominio/semana.go` → `InicioSemana`, `FinSemana`, `SemanaActual` (zona `APP_ZONA_HORARIA`) | — | `dominio/semana_test.go` |
| RF-08.2 Operaciones en la semana activa | `dominio/tarea.go` → `PuedeModificarse` | — | `TestTareaPuedeModificarse` |
| RF-08.3 Semanas anteriores | `dominio/tarea.go` → `ValidarSemanaDeRegistro` (solo rechaza semanas futuras) | — | `TestValidarSemanaDeRegistro` |
| RF-08.4 Marca de tardanza | `servicio/tareas.go` → `Crear` (`Tardia = SemanaCerrada(...)`) | — | `TestCrearTareaEnSemanaPasadaEsTardia` |
| RF-08.5 Tardía inmutable | `Tarea.PuedeModificarse`; además el repositorio solo actualiza o elimina con `tardia = FALSE` | — | `TestModificarTareas`, `TestTareasYAdjuntos` |

### RF-09 a RF-12 · Seguimiento y visibilidad

| Requisito | Implementación | Endpoint | Pruebas |
|---|---|---|---|
| RF-09 Estado "sin reporte" (adelantado) | `dominio/seguimiento.go` → `ConstruirSemanas`, `estadoSemana` | `GET /api/v1/seguimiento/personas/{id}/semanas` | `TestConstruirSemanasDetectaSinReporte` |
| RF-10.1 y 10.2 Personas a cargo y sus vinculaciones | `servicio/seguimiento.go` → `PersonasACargo`; `dominio.AgruparPorPersona` | `GET /api/v1/seguimiento/personas` | `TestPersonasACargo` |
| RF-10.3 Detalle semanal | `servicio/seguimiento.go` → `SemanasPersona` | `GET …/personas/{id}/semanas` | `TestSemanasPersonaVisibilidadYRango` |
| RF-10.4 Vista consolidada | `servicio/seguimiento.go` → `Consolidado`; `dominio.ConstruirConsolidado` | `GET …/personas/{id}/consolidado` | `TestConstruirConsolidado` |
| RF-10.5 Tareas de sus personas | `servicio/tareas.go` → `Listar` | `GET /api/v1/tareas?usuario_id=` | Postman, carpeta 6 |
| RF-10.6 Horas reportadas frente a contratadas | `dominio.SemanaPersona` (`HorasContratadas`, `HorasReportadas`) | `…/semanas` | `TestConstruirSemanasDetectaSinReporte` |
| RF-11.1 Listado unificado del monitor | `servicio/vinculaciones.go` → `Listar` y `servicio/espacios.go` → `Listar` | `GET /api/v1/vinculaciones`, `GET /api/v1/espacios` | Postman, carpeta 4 |
| RF-11.2 Solo sus tareas | `Tareas.Listar` con `VisiblePara` | `GET /api/v1/tareas` | `TestTareasYAdjuntos` |
| RF-11.3 Historial | `servicio/seguimiento.go` → `SemanasPersona` sobre el propio usuario | `GET /api/v1/seguimiento/mi-historial` | `handler/modulos_test.go` → `TestSeguimiento` |
| RF-12.1 a 12.3 Visibilidad por profesor, usuario y administrador | `servicio/acceso.go` → `visiblePara`, `puedeVerDeProfesor`; filtros `VisiblePara` en los repositorios | todas | `TestReporteDeOtroProfesorNoEsVisible`, `TestVisibilidadDeVinculaciones`, `TestSemanasPersonaVisibilidadYRango` |

### RF-14 · Reportes semanales con IA

| Requisito | Implementación | Endpoint | Pruebas |
|---|---|---|---|
| RF-14.1 Ejecución manual por lotes | `servicio/reportes.go` → `SolicitarLote`; `cola/cola.go` → `EncolarLote` | `POST /api/v1/reportes/lotes` (202) | `TestSolicitarLoteNormalizaLaSemanaYEncola` |
| RF-14.2 Un reporte por persona | `servicio/generacion_reportes.go` → `ProcesarLote` (worker) | — | `TestProcesarLoteCreaYEncolaUnReportePorPersona` |
| RF-14.3 Solo los insumos permitidos | `ia/prompt.go` → `ConstruirMensajeUsuario` (sin nombre, correo ni horas contratadas) | — | `ia/ia_test.go` → `TestMensajeUsuarioIncluyeSoloInsumosPermitidos` |
| RF-14.4 PDF | `pdf/generador.go` → `Generar` | — | `pdf/generador_test.go` |
| RF-14.5 Almacenar los PDF | `servicio/generacion_reportes.go` → `GenerarReporte` + `almacenamiento/local.go` | — | `TestGenerarReporteCompleto` |
| RF-14.6 Descarga posterior | `servicio/reportes.go` → `AbrirPDF` | `GET /api/v1/reportes/{id}/pdf` | `TestDescargarPDF` |
| RF-14.7 Una semana concreta | `dominio.InicioSemana` sobre la fecha solicitada | — | `TestSolicitarLoteNormalizaLaSemanaYEncola` |
| Proveedor de IA configurable | `ia/ia.go` → `Nuevo` (`simulado`, `openai`, `anthropic`) | — | `TestNuevoValidaConfiguracion`, `TestOpenAI…`, `TestAnthropic…` |
| Reintentos y fallos | `cola/cola.go` → `ejecutar`; `servicio.EsErrorPermanente` | — | `cola/cola_test.go`, `TestEsErrorPermanente` |

### RF-15 · Administración

RF-15.1 (visibilidad global) se cubre con `visiblePara` (vacío para el administrador); RF-15.2 con el módulo de usuarios; y RF-15.3 con `Vinculaciones.Rectificar`.

## 2. Reglas de negocio destacadas

| Regla | Dónde se aplica |
|---|---|
| RN-02 Rol por vinculación | `vinculaciones.rol`; `verificarPersonaVinculable` exige además el rol global |
| RN-07 Creación solo en la ventana del período | `PeriodoAcademico.PermiteCrearEspacios` (`creacion_habilitada`) |
| RN-09 Espacio cerrado = solo consulta | `Espacio.PermiteRegistros` |
| RN-12 Horas inmutables | Solo `Rectificar` (administrador), con motivo y registro |
| RN-14 a RN-19 Horas | `ValidarCargaHoraria` + transacción con bloqueo |
| RN-26 Continuidad entre semanas | La semana no se edita |
| RN-29 a RN-34 Semanas, tardanza e inmutabilidad | `dominio/semana.go`, `dominio/tarea.go` |
| RN-35 a RN-37 "Sin reporte" derivado | `dominio/seguimiento.go` (no se almacena) |
| RN-41 a RN-46 Reportes con IA | `servicio/generacion_reportes.go`, `ia/prompt.go` (instrucciones: no evaluar el desempeño) |

## 3. Requerimientos no funcionales y stack

| Exigencia | Evidencia |
|---|---|
| RNF-01 Aplicación web sencilla con backend sólido | Arquitectura en tres capas con dominio puro |
| RNF-03 Autenticación local y roles | `auth/`, `handler/middleware.go` |
| RNF-04 Persistencia de tareas, vinculaciones y PDF | PostgreSQL + volumen `archivos` |
| RNF-05 Mantenibilidad | Un archivo por módulo en cada capa; golangci-lint y SonarQube con 0 hallazgos |
| RNF-06 Consistencia del negocio | Reglas en el dominio, transacciones y `CHECK` en la base de datos |
| Go + Gin | `backend/go.mod`, `handler/` |
| PostgreSQL | `basedatos/`, `repositorio/` |
| Linux Debian | `backend/Dockerfile` (`debian:bookworm-slim`), imágenes `*-bookworm` |
| Docker y Docker Compose | `backend/Dockerfile`, `docker-compose.yml` |
| Nginx | `nginx/nginx.conf` |
| Asynq + Redis | `cola/`, `cmd/worker/` |
| LLM | `ia/` |

## 4. Entregables de la Entrega 1

| Entregable | Evidencia |
|---|---|
| Backend según la especificación | `backend/` (este documento) |
| `Dockerfile` y `docker-compose.yml` | `backend/Dockerfile`, `docker-compose.yml` |
| Pruebas unitarias | `backend/internal/**/*_test.go` |
| Pipeline de GitHub Actions | `.github/workflows/ci.yml` |
| SonarQube | Etapa "Análisis SonarQube" del pipeline (artefacto `reporte-sonarqube`) |
| Documento de arquitectura | [Entrega 1 – Arquitectura](Entrega-1-Arquitectura) |
| Documento de deuda técnica | [Entrega 1 – Deuda técnica](Entrega-1-Deuda-tecnica) |
| Pruebas con Postman | `postman/seguimiento.postman_collection.json` (también en CI) |
| Pruebas de carga | `pruebas-carga/` y [Entrega 1 – Pruebas de carga](Entrega-1-Pruebas-de-carga) |
