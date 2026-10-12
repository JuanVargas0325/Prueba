# Paso 48 · Servicio de reportes (solicitud por lotes)

> Rama `paso-a-paso` · paso 48 de 65 · anterior: **Paso 47 · Cola con Asynq y Redis** · siguiente: **Paso 49 · Endpoints de reportes**

---

## 1. Qué se hizo en este paso

Se creó la parte del módulo de reportes que corre **en la API**: solicitar un lote (que se encola y se procesa después) y consultar o descargar los resultados.

| Archivo | Cambio |
|---|---|
| `backend/internal/servicio/reportes.go` | **Nuevo.** Interfaces `RepositorioReportes`, `Encolador` y `Almacenamiento`; servicio `Reportes` con `SolicitarLote`, `ListarLotes`, `ObtenerLote`, `ListarReportes`, `ObtenerReporte` y `AbrirPDF`. |
| `backend/internal/servicio/reportes_test.go` | **Nuevo.** Dobles del repositorio, del encolador y del almacenamiento, el bloque de dobles compartidos del paquete y ocho pruebas. |
| `backend/internal/servicio/tareas.go` | **Modificado.** La interfaz `Almacenamiento` se traslada a `reportes.go`. |
| `backend/internal/servicio/autenticacion_test.go` | **Modificado.** El bloque de dobles compartidos (`logPrueba`, actores, `ahoraPrueba`) y `almacenamientoFalso` se trasladan a `reportes_test.go`. |

---

## 2. Por qué

- **RF-14.1 / RN-41:** el profesor dispara **manualmente** la generación de los reportes de una semana. La API solo registra el lote y lo encola: **no espera a la IA**.
- **RF-14.6:** los PDF generados se consultan y descargan después.
- **RN-03:** cada profesor solo ve sus lotes y reportes; el administrador ve todos.
- **¿Por qué mover `Almacenamiento` y los dobles?** Desde este paso hay varias piezas de infraestructura que comparten el servicio de tareas y el de reportes (y el generador del paso 54): el almacenamiento, el encolador y la IA. Sus interfaces quedan juntas en `reportes.go`, y sus dobles de prueba juntos en `reportes_test.go`. En Go cualquier archivo del paquete ve todo lo declarado en los demás, así que mover una declaración entre archivos del mismo paquete **no cambia el comportamiento**.

---

## 3. Explicación detallada de `reportes.go`

### 3.1 Interfaces de infraestructura

```go
type Encolador interface {
	EncolarLote(ctx context.Context, loteID string) error
	EncolarReporte(ctx context.Context, reporteID string) error
}

type Almacenamiento interface {
	Guardar(ctx context.Context, ruta string, contenido io.Reader) (int64, error)
	Abrir(ctx context.Context, ruta string) (io.ReadCloser, error)
	Eliminar(ctx context.Context, ruta string) error
}
```
- `*cola.Cliente` (paso 47) cumple `Encolador` y `*almacenamiento.Local` (paso 38) cumple `Almacenamiento`. El servicio no conoce Redis ni el disco.

```go
type RepositorioReportes interface {
	CrearLote(...); ExisteLoteActivo(...); ObtenerLote(...); ListarLotes(...); IniciarLote(...); MarcarLoteFallido(...)
	ListarPersonasParaReporte(...); CrearReportesPendientes(...); ObtenerReporte(...); ListarReportes(...)
	MarcarReporteEnProceso(...); CompletarReporte(...); FallarReporte(...)
	ObtenerDatosSemana(...)
}
```
- Todo lo del repositorio del paso 46. Una sola interfaz porque la API y el worker (paso 54) usan el mismo repositorio, cada uno con sus métodos.

### 3.2 `SolicitarLote`

```go
func (s *Reportes) SolicitarLote(ctx context.Context, actor auth.Actor, fechaSemana string) (dominio.LoteReportes, error) {
	if !actor.TieneRol(dominio.RolProfesor) {
		return dominio.LoteReportes{}, apperr.Prohibido("solo un profesor puede generar reportes")
	}
	fecha, err := dominio.ParsearFecha(fechaSemana)
	if err != nil {
		return dominio.LoteReportes{}, apperr.Validacion("semana debe ser una fecha con formato AAAA-MM-DD")
	}
	semana := dominio.InicioSemana(fecha)
	if semana.After(s.reloj.SemanaActual()) {
		return dominio.LoteReportes{}, apperr.Validacion("no se pueden generar reportes de semanas futuras")
	}

	activo, err := s.repo.ExisteLoteActivo(ctx, actor.UsuarioID, semana)
	...
	if activo {
		return dominio.LoteReportes{}, apperr.Conflicto("ya hay un lote en proceso para la semana del %s", semana.Format(dominio.FormatoFecha))
	}

	lote, err := s.repo.CrearLote(ctx, actor.UsuarioID, semana)
	...
	if err := s.encolador.EncolarLote(ctx, lote.ID); err != nil {
		s.log.Error("no fue posible encolar el lote", "lote_id", lote.ID, "error", err)
		_ = s.repo.MarcarLoteFallido(context.WithoutCancel(ctx), lote.ID, "no fue posible encolar el lote: "+err.Error())
		return dominio.LoteReportes{}, apperr.Interno(err, "no fue posible iniciar la generación de reportes, intente de nuevo")
	}
	return lote, nil
}
```
1. **Solo profesores** (ni siquiera el administrador genera: los reportes son del profesor sobre **sus** personas).
2. **RF-14.7:** cualquier fecha se normaliza al **lunes** de su semana. Se permite la semana en curso y las pasadas; las futuras no.
3. **Un lote activo por semana:** si ya hay uno pendiente o en proceso para esa semana → **409**. Evita duplicar trabajo y gasto en la IA por doble clic.
4. Se **registra** el lote (estado `pendiente`) y se **encola**.
5. **Si Redis no responde,** el lote quedaría pendiente para siempre. Por eso se marca **fallido** (con el motivo) y se responde 500 con un mensaje para reintentar. El lote fallido no bloquea uno nuevo (no está activo).

### 3.3 Consultas

```go
func (s *Reportes) ListarLotes(ctx context.Context, actor auth.Actor, f dominio.FiltroLotes) ([]dominio.LoteReportes, error) {
	switch {
	case actor.EsAdministrador():
	case actor.TieneRol(dominio.RolProfesor):
		f.ProfesorID = actor.UsuarioID // RN-03: solo sus propios lotes
	default:
		return nil, apperr.Prohibido("no tiene permiso para consultar reportes")
	}
	return s.repo.ListarLotes(ctx, f)
}
```
- El profesor siempre ve **solo lo suyo** (se sobrescribe cualquier `ProfesorID` que pida); el administrador puede filtrar por cualquiera. Igual en `ListarReportes`.
- `ObtenerLote` devuelve el lote **y** sus reportes (para ver el avance de cada persona) si el actor puede verlo.
- `ObtenerReporte` responde **404** (no 403) a otro profesor: no se revela que el reporte existe.

```go
func (s *Reportes) AbrirPDF(ctx context.Context, actor auth.Actor, id string) (io.ReadCloser, dominio.ReporteSemanal, error) {
	rep, err := s.ObtenerReporte(ctx, actor, id)
	...
	if rep.Estado != dominio.ReporteCompletado || rep.RutaPDF == "" {
		return nil, rep, apperr.Conflicto("el reporte aún no está disponible (estado: %s)", rep.Estado)
	}
	archivo, err := s.almacenamiento.Abrir(ctx, rep.RutaPDF)
	...
}
```
- Descargar un reporte que todavía se está generando → **409** con su estado.

---

## 4. Las pruebas (`reportes_test.go`)

Dobles:
- `repoFalso`: lotes y reportes en mapas; guarda los filtros recibidos y los lotes y reportes marcados como fallidos.
- `encoladorFalso`: anota lo encolado y puede simular un error de Redis.
- `almacenamientoFalso`: archivos en memoria (antes en `autenticacion_test.go`).
- El bloque `var (...)` con `logPrueba`, los actores y `ahoraPrueba` (miércoles 14/10/2026), ahora aquí.

| Prueba | Qué verifica |
|---|---|
| `TestSolicitarLoteNormalizaLaSemanaYEncola` | El jueves 08/10 se convierte en el lunes 05/10; el lote es del profesor y queda encolado. |
| `TestSolicitarLoteValidaciones` | Un monitor (403), el administrador (403), fecha inválida (400), semana futura (400) y lote activo (409). |
| `TestSolicitarLotePermiteLaSemanaEnCurso` | La semana actual sí se puede reportar. |
| `TestSolicitarLoteMarcaFallidoSiNoPuedeEncolar` | Con Redis caído: 500 y el lote queda fallido. |
| `TestListarLotesAplicaVisibilidad` | El profesor solo ve sus lotes; el administrador filtra por cualquier profesor; un monitor (403). |
| `TestListarReportesAplicaVisibilidad` | Lo mismo para los reportes. |
| `TestReporteDeOtroProfesorNoEsVisible` | Otro profesor recibe 404 en el reporte y en el lote; el dueño y el administrador sí los ven. |
| `TestAbrirPDF` | Un reporte en proceso → 409; uno completado entrega su contenido. |

---

## 5. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Archivos de un paquete | Todos comparten el mismo espacio de nombres: mover una declaración entre ellos no cambia nada. |
| Compensación | Si falla el encolado, se marca el lote como fallido para no dejarlo colgado. |
| `_ =` sobre un error | Se ignora a propósito: el error principal ya se está informando. |
| 409 por estado | El recurso existe pero su estado no permite la operación (aún no está listo). |

---

## 6. Cómo comprobarlo

```bash
cd backend
go test ./internal/servicio/ -v -run 'SolicitarLote|Lotes|Reportes|PDF|OtroProfesor'
```

---

## 7. Pasos para lograr esta versión desde el paso 47

1. Crear `backend/internal/servicio/reportes.go` con `RepositorioReportes`, `Encolador`, `Almacenamiento`, `Reportes`, `NuevoReportes` y los seis métodos.
2. Quitar de `servicio/tareas.go` la interfaz `Almacenamiento` (ahora está en `reportes.go`).
3. Crear `backend/internal/servicio/reportes_test.go` con los dobles (`repoFalso`, `encoladorFalso`, `almacenamientoFalso`), el bloque `var (...)` de dobles compartidos, `nuevoServicio` y las ocho pruebas.
4. Quitar de `autenticacion_test.go` el bloque de dobles compartidos y `almacenamientoFalso`.
5. `make pruebas lint`
6. Commit: `git add . && git commit -m "Paso 48: servicio de reportes"`

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-14.1 / RN-41 Ejecución manual por lotes | `SolicitarLote` registra y encola, sin esperar a la IA. |
| RF-14.6 Descarga posterior | `AbrirPDF`. |
| RF-14.7 Semana concreta | Normalización al lunes; sin semanas futuras. |
| RN-03 / RN-05 Visibilidad | Profesor: solo lo suyo; administrador: todo. |
