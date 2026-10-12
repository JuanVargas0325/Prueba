# Paso 54 · Generación asíncrona de reportes

> Rama `paso-a-paso` · paso 54 de 65 · anterior: **Paso 53 · Proveedor Anthropic** · siguiente: **Paso 55 · Worker**

---

## 1. Qué se hizo en este paso

Se escribió la lógica que el worker ejecutará para cada trabajo de la cola: **repartir un lote** en un trabajo por persona y **generar el reporte** de cada una (IA → PDF → almacenamiento → base de datos).

| Archivo | Cambio |
|---|---|
| `backend/internal/servicio/generacion_reportes.go` | **Nuevo.** `GeneradorReportes` con `ProcesarLote`, `GenerarReporte`, `RegistrarFalloReporte` y `RegistrarFalloLote` (cumple `cola.Procesador`, paso 47), más `EsErrorPermanente`, `RutaPDFReporte`, `MensajeFallo` y la interfaz `GeneradorPDF`. |
| `backend/internal/servicio/reportes_test.go` | **Modificado.** Dobles de la IA (`resumidorFalso`) y del PDF (`pdfFalso`) y ocho pruebas del generador. |

Con este paso, **todo el código de `backend/internal`** queda igual a la versión final, salvo las pruebas de integración (paso 58).

---

## 2. Por qué

- **RF-14.2:** un reporte **por persona**. Un lote se reparte en trabajos independientes: si el reporte de una persona falla, los demás siguen; y varios reportes pueden generarse en paralelo.
- **RF-14.4 y RF-14.5:** cada reporte termina como un **PDF guardado**.
- **Robustez:** los trabajos pueden reintentarse o entregarse dos veces (paso 47); cada paso debe ser **idempotente**.

---

## 3. Explicación detallada

### 3.1 Dependencias

```go
type GeneradorPDF interface {
	Generar(c pdf.Contenido) ([]byte, error)
}

type GeneradorReportes struct {
	repo           RepositorioReportes
	encolador      Encolador
	almacenamiento Almacenamiento
	resumidor      ia.Resumidor
	pdf            GeneradorPDF
	reloj          Reloj
	log            *slog.Logger
}
```
- Todo llega como **interfaz**: el repositorio (paso 46), el encolador (paso 47), el almacenamiento (paso 38), la IA (pasos 51 a 53) y el PDF (paso 50). Las pruebas los reemplazan por dobles; nada toca la red ni el disco.

### 3.2 `ProcesarLote`: repartir

```go
func (g *GeneradorReportes) ProcesarLote(ctx context.Context, loteID string) error {
	lote, err := g.repo.ObtenerLote(ctx, loteID)
	...
	if !lote.Estado.EstaActivo() {
		g.log.Info("lote ya procesado, se omite", "lote_id", loteID, "estado", lote.Estado)
		return nil
	}
	personas, err := g.repo.ListarPersonasParaReporte(ctx, lote.ProfesorID, lote.SemanaInicio, dominio.FinSemana(lote.SemanaInicio))
	...
	reportes, err := g.repo.CrearReportesPendientes(ctx, lote.ID, lote.ProfesorID, lote.SemanaInicio, personas)
	...
	if err := g.repo.IniciarLote(ctx, lote.ID, len(reportes)); err != nil { ... }
	for _, reporteID := range reportes {
		if err := g.encolador.EncolarReporte(ctx, reporteID); err != nil {
			g.log.Error("no fue posible encolar el reporte", "reporte_id", reporteID, "error", err)
			if errFallo := g.repo.FallarReporte(ctx, reporteID, "no fue posible encolar el reporte: "+err.Error()); errFallo != nil {
				return errFallo
			}
		}
	}
	return nil
}
```
1. Un lote ya terminado **se omite** (si la cola lo entrega dos veces).
2. Busca a las personas vinculadas al profesor esa semana (RF-14.2).
3. Crea un reporte **pendiente** por persona (idempotente, paso 46).
4. Fija el total y pasa el lote a **en proceso** (o completado, si no hay nadie).
5. Encola un trabajo `generar_reporte` por persona (con `TaskID`, sin duplicados, paso 47). Si uno no se puede encolar, ese reporte se marca **fallido** para que el lote pueda terminar.

### 3.3 `GenerarReporte`: el reporte de una persona

```go
const ResumenSinTareas = "No se registraron tareas durante esta semana, por lo que no hay actividades para resumir."

func (g *GeneradorReportes) GenerarReporte(ctx context.Context, reporteID string) error {
	rep, err := g.repo.ObtenerReporte(ctx, reporteID)
	...
	if rep.Estado.EsFinal() {
		g.log.Info("reporte ya finalizado, se omite", ...)
		return nil
	}
	if err := g.repo.MarcarReporteEnProceso(ctx, reporteID); err != nil { ... }

	datos, err := g.repo.ObtenerDatosSemana(ctx, rep.ProfesorID, rep.UsuarioID, rep.SemanaInicio)
	...
	resumen := ResumenSinTareas
	proveedor, modelo := "", ""
	if datos.TotalTareas() > 0 {
		inicio := g.reloj.Ahora()
		resumen, err = g.resumidor.Resumir(ctx, datos)
		if err != nil {
			return fmt.Errorf("generando resumen con IA: %w", err)
		}
		proveedor, modelo = g.resumidor.Proveedor(), g.resumidor.Modelo()
		g.log.Info("resumen generado", ..., "duracion_ms", g.reloj.Ahora().Sub(inicio).Milliseconds())
	}

	contenido, err := g.pdf.Generar(pdf.Contenido{Datos: datos, Resumen: resumen, GeneradoEn: g.reloj.Ahora(), ProveedorIA: proveedor, ModeloIA: modelo})
	...
	ruta := RutaPDFReporte(rep)
	tamano, err := g.almacenamiento.Guardar(ctx, ruta, bytes.NewReader(contenido))
	...
	if err := g.repo.CompletarReporte(ctx, reporteID, dominio.ResultadoReporte{...}); err != nil { ... }
	...
}
```
1. **Idempotente:** si el reporte ya terminó, no hace nada.
2. Lo marca **en proceso** (suma un intento, paso 46).
3. Lee los datos de la semana (solo los insumos permitidos, RF-14.3).
4. **Sin tareas, no se llama a la IA:** se usa un texto fijo. Así el modelo no puede "inventar" actividades para una semana vacía (RN-46) y no se gasta dinero en una llamada inútil.
5. Con tareas, pide el resumen y registra cuánto tardó (útil para elegir modelo y para el análisis de capacidad).
6. Genera el PDF, lo guarda en `reportes/<semana>/<id>.pdf` y registra el resultado; `CompletarReporte` también actualiza el lote (paso 46).
7. Cualquier error se **devuelve**: la cola decide si reintenta (3.4).

```go
func RutaPDFReporte(rep dominio.ReporteSemanal) string {
	return fmt.Sprintf("reportes/%s/%s.pdf", rep.SemanaInicio.Format(dominio.FormatoFecha), rep.ID)
}
```
- Los PDF quedan organizados por semana. Si el trabajo se reintenta, se sobrescribe el mismo archivo (la escritura es atómica, paso 38).

### 3.4 ¿Reintentar o no?

```go
func EsErrorPermanente(err error) bool {
	switch apperr.TipoDe(err) {
	case apperr.TipoNoEncontrado, apperr.TipoValidacion, apperr.TipoProhibido:
		return true
	}
	return ia.EsErrorDeIA(err) && !ia.EsReintentable(err)
}
```
- Es la función `esPermanente` que se le entrega a la cola (paso 47). **Permanentes:** datos que no existen o son inválidos, y errores de la IA no transitorios (clave inválida, modelo inexistente, rechazo). **Transitorios:** todo lo demás (red, tiempo agotado, límite de uso, base de datos momentáneamente caída).
- `fmt.Errorf("...: %w", err)` en `GenerarReporte` **envuelve** el error de la IA; `errors.As` (dentro de `ia.EsErrorDeIA`) lo encuentra igual.

```go
func (g *GeneradorReportes) RegistrarFalloReporte(ctx context.Context, reporteID string, causa error) error {
	return g.repo.FallarReporte(ctx, reporteID, MensajeFallo(causa))
}
```
- La cola llama a estos métodos cuando el fallo es definitivo; el reporte queda `fallido` **con el motivo**, visible para el profesor en el detalle del lote.

---

## 4. Las pruebas

| Prueba | Qué verifica |
|---|---|
| `TestProcesarLoteCreaYEncolaUnReportePorPersona` | 3 personas → lote en proceso con total 3 y 3 trabajos encolados. |
| `TestProcesarLoteOmiteLotesTerminados` | Un lote completado no se reprocesa. |
| `TestProcesarLoteMarcaFallidoElReporteQueNoSePudoEncolar` | Con Redis caído, el reporte queda fallido. |
| `TestGenerarReporteCompleto` | Llama una vez a la IA, el PDF usa ese resumen, se guarda en `reportes/2026-10-05/r1.pdf` y el resultado registra ruta, tamaño, proveedor y modelo; antes se marcó en proceso. |
| `TestGenerarReporteSinTareasNoConsultaLaIA` | Sin tareas: cero llamadas a la IA, resumen fijo y sin proveedor. |
| `TestGenerarReporteOmiteReportesFinalizados` | Un reporte completado no se regenera. |
| `TestGenerarReportePropagaElErrorDeIA` | Un 401 de la IA es **permanente**; el reporte no se completa; al registrar el fallo, el motivo incluye "clave inválida". |
| `TestEsErrorPermanente` | No encontrado y validación → permanentes; IA 400 → permanente; IA 429 y 503 (incluso envuelto) → transitorios; un error de red → transitorio. |

---

## 5. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Fan-out | Un trabajo (el lote) genera muchos trabajos pequeños (uno por persona). |
| Idempotencia | Cada trabajo comprueba el estado antes de actuar. |
| Errores permanentes y transitorios | Solo se reintenta lo que puede funcionar después. |
| `bytes.NewReader` | Convierte un `[]byte` en un `io.Reader` para guardarlo. |

---

## 6. Cómo comprobarlo

```bash
cd backend
go test ./internal/servicio/ -v -run 'ProcesarLote|GenerarReporte|EsErrorPermanente'
```

Por HTTP de punta a punta, en el paso 55 (con el worker en marcha).

---

## 7. Pasos para lograr esta versión desde el paso 53

1. Crear `backend/internal/servicio/generacion_reportes.go` con `GeneradorPDF`, `ResumenSinTareas`, `GeneradorReportes`, `NuevoGeneradorReportes`, los cuatro métodos, `RutaPDFReporte`, `EsErrorPermanente` y `MensajeFallo`.
2. En `reportes_test.go`: `resumidorFalso`, `pdfFalso`, `nuevoGenerador`, `semanaPrueba`, `datosConTareas` y las ocho pruebas.
3. `make pruebas lint`
4. Commit: `git add . && git commit -m "Paso 54: generación asíncrona de reportes"`

---

## 8. Preparación para las próximas entregas

- **El generador no sabe que existe Asynq:** recibe un ID y devuelve un error. En la Entrega 4 se podría invocar desde una función serverless disparada por Pub/Sub sin cambiarlo.
- **El tiempo de cada llamada a la IA queda en los logs** (`duracion_ms`): servirá para dimensionar workers y comparar proveedores.

---

## 9. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-14.2 Un reporte por persona | `ProcesarLote` crea y encola uno por persona. |
| RF-14.3 Insumos permitidos | `ObtenerDatosSemana` + prompt del paso 51. |
| RF-14.4 / RF-14.5 PDF almacenado | `GenerarReporte` genera y guarda el PDF. |
| RN-46 No inventar ni evaluar | Sin tareas no se consulta a la IA. |
| RNF-01 Backend sólido | Idempotencia, reintentos solo ante errores transitorios y registro del motivo de fallo. |
