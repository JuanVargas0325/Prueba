# Paso 46 · Repositorio de reportes

> Rama `paso-a-paso` · paso 46 de 65 · anterior: **Paso 45 · Dominio de lotes y reportes** · siguiente: **Paso 47 · Cola de tareas con Asynq y Redis**

---

## 1. Qué se hizo en este paso

Se creó el repositorio de lotes y reportes (tablas `lotes_reportes` y `reportes_semanales`) y la consulta que reúne los datos de la semana de una persona para su reporte.

| Archivo | Cambio |
|---|---|
| `backend/internal/repositorio/reportes.go` | **Nuevo.** Lotes (`CrearLote`, `ExisteLoteActivo`, `ObtenerLote`, `ListarLotes`, `IniciarLote`, `MarcarLoteFallido`), reportes (`ListarPersonasParaReporte`, `CrearReportesPendientes`, `ObtenerReporte`, `ListarReportes`, `MarcarReporteEnProceso`, `CompletarReporte`, `FallarReporte`) y datos de la semana (`ObtenerDatosSemana`). |
| `backend/internal/repositorio/comun.go` | **Modificado.** Constantes `recursoLote`, `recursoLotes`, `recursoReporte` y `recursoReportes` (el bloque queda completo). |

---

## 2. Por qué

La generación asíncrona (pasos 48 a 55) tiene una dificultad especial: **varios workers** pueden procesar reportes del mismo lote **al mismo tiempo**, y la cola puede **reintentar** o **entregar dos veces** el mismo trabajo. El repositorio debe garantizar que, pase lo que pase:
- no se creen reportes duplicados;
- un reporte terminado no se cuente dos veces;
- el lote llegue a su estado final correcto exactamente una vez.

---

## 3. Explicación detallada

### 3.1 Lotes

```go
func (r *Reportes) ExisteLoteActivo(ctx context.Context, profesorID string, semana time.Time) (bool, error) {
	... SELECT EXISTS (... WHERE profesor_id = $1 AND semana_inicio = $2 AND estado IN ('pendiente', 'en_proceso')) ...
}
```
- Para impedir que un profesor lance dos lotes de la misma semana a la vez (paso 48).

```go
func (r *Reportes) IniciarLote(ctx context.Context, id string, total int) error {
	_, err := r.db.Exec(ctx,
		`UPDATE lotes_reportes SET
			total_reportes = $2,
			estado = CASE WHEN $2 = 0 THEN 'completado' ELSE 'en_proceso' END,
			finalizado_en = CASE WHEN $2 = 0 THEN now() ELSE NULL END,
			actualizado_en = now()
		 WHERE id = $1 AND estado IN ('pendiente', 'en_proceso')`, id, total)
	...
}
```
- `CASE WHEN ... THEN ... ELSE ... END` es el "if" de SQL. Un lote sin personas queda **completado** de inmediato.
- `WHERE ... estado IN ('pendiente', 'en_proceso')`: no modifica lotes ya terminados (si el trabajo se reintenta).

### 3.2 ¿A quién se le hace reporte?

```go
func (r *Reportes) ListarPersonasParaReporte(ctx context.Context, profesorID string, inicio, fin time.Time) ([]string, error) {
	... SELECT DISTINCT v.usuario_id::text
	    FROM vinculaciones v JOIN espacios e ON e.id = v.espacio_id
	    WHERE e.profesor_id = $1 AND e.fecha_inicio <= $3 AND e.fecha_cierre >= $2 ...
}
```
- **RF-14.2:** un reporte por cada persona vinculada a algún espacio del profesor que estuvo vigente **esa semana** (intersección de fechas). `DISTINCT` evita repetir a quien está en dos espacios del mismo profesor.
- `::text` convierte el UUID a texto en SQL.

### 3.3 Crear reportes de forma idempotente

```go
func (r *Reportes) CrearReportesPendientes(ctx context.Context, loteID, profesorID string, semana time.Time, usuarioIDs []string) ([]string, error) {
	if len(usuarioIDs) > 0 {
		_, err := r.db.Exec(ctx,
			`INSERT INTO reportes_semanales (lote_id, profesor_id, usuario_id, semana_inicio)
			 SELECT $1, $2, u, $3 FROM unnest($4::uuid[]) AS u
			 ON CONFLICT (lote_id, usuario_id) DO NOTHING`, ...)
		...
	}
	filas, err := r.db.Query(ctx, `SELECT id::text FROM reportes_semanales WHERE lote_id = $1 ORDER BY creado_en, id`, loteID)
	...
}
```
- `unnest($4::uuid[])` convierte el arreglo de IDs en filas; un solo `INSERT` crea todos los reportes.
- `ON CONFLICT (lote_id, usuario_id) DO NOTHING`: si el trabajo se ejecuta **dos veces**, la segunda no crea duplicados (usa el `UNIQUE` del paso 4). Esto es **idempotencia**: repetir la operación da el mismo resultado.
- Devuelve **todos** los reportes del lote (también los de una ejecución anterior), para encolarlos (paso 54).

### 3.4 Terminar un reporte y actualizar el lote en la misma transacción

```go
func (r *Reportes) CompletarReporte(ctx context.Context, id string, res dominio.ResultadoReporte) error {
	return r.finalizarReporte(ctx,
		`UPDATE reportes_semanales SET estado = 'completado', resumen = $2, ruta_pdf = $3, ...
		 WHERE id = $1 AND estado NOT IN ('completado', 'fallido')
		 RETURNING lote_id`,
		[]any{id, res.Resumen, ...}, 1, 0)
}

func (r *Reportes) FallarReporte(ctx context.Context, id, motivo string) error {
	return r.finalizarReporte(ctx, `UPDATE ... SET estado = 'fallido', error = $2 ... RETURNING lote_id`, ..., 0, 1)
}
```

```go
func (r *Reportes) finalizarReporte(ctx context.Context, sqlReporte string, args []any, completados, fallidos int) error {
	return enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		var loteID string
		err := tx.QueryRow(ctx, sqlReporte, args...).Scan(&loteID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // el reporte ya estaba en un estado final: nada que hacer
		}
		...
		_, err = tx.Exec(ctx,
			`UPDATE lotes_reportes l SET
				reportes_completados = l.reportes_completados + $2,
				reportes_fallidos    = l.reportes_fallidos + $3,
				estado = CASE
					WHEN l.estado = 'en_proceso'
					 AND l.reportes_completados + $2 + l.reportes_fallidos + $3 >= l.total_reportes THEN
						CASE
							WHEN l.reportes_fallidos + $3 = 0 THEN 'completado'
							WHEN l.reportes_completados + $2 = 0 THEN 'fallido'
							ELSE 'completado_con_errores'
						END
					ELSE l.estado
				END,
				finalizado_en = ...
			 WHERE l.id = $1`, loteID, completados, fallidos)
		...
	})
}
```
- **Paso 1:** cambia el estado del reporte **solo si no estaba terminado** (`estado NOT IN (...)`). Si ya lo estaba, `RETURNING` no devuelve filas → `ErrNoRows` → no hace nada. Así un trabajo repetido **no cuenta dos veces**.
- **Paso 2:** en la misma transacción suma 1 al contador correspondiente del lote y, si ya terminaron todos, fija el estado final: **completado** (ningún fallo), **fallido** (ningún éxito) o **completado con errores**.
- **¿Por qué no hay carreras con varios workers?** El `UPDATE` del lote **bloquea su fila** hasta el final de la transacción. Si dos workers terminan reportes del mismo lote a la vez, PostgreSQL ejecuta esas dos actualizaciones **una después de la otra**, y cada una ve los contadores ya sumados por la anterior. Es el mismo principio del paso 31, pero con el bloqueo implícito del `UPDATE`.
- Todo el cálculo ocurre **dentro de la base de datos** (`l.reportes_completados + $2`): no se lee el valor en Go para sumarle 1 y escribirlo (eso sí tendría carreras).
- Los mensajes de error se recortan con `recortarTexto` (paso 31).

### 3.5 Los datos de la semana para el reporte

```go
func (r *Reportes) ObtenerDatosSemana(ctx context.Context, profesorID, usuarioID string, semana time.Time) (dominio.DatosSemanaPersona, error) {
	... 1) nombre y correo de la persona y nombre del profesor ...
	... 2) vinculaciones de la persona en espacios del profesor vigentes esa semana ...
	... 3) tareas de esas vinculaciones en esa semana ...
}
```
- **RF-14.3:** solo lee los insumos permitidos (título, descripción, estado, horas, observaciones, si fue tardía), en los tipos del paso 45.
- **Solo los espacios de ese profesor:** si Laura trabaja con dos profesores, cada uno recibe un reporte con **su** parte (RN-03).
- Para la tercera consulta usa `= ANY($1::uuid[])` (paso 42) y un **mapa** `idVinculación → posición` para repartir cada tarea en su vinculación en una sola pasada.
- `type fila struct{...}` dentro de la función: un **tipo local**, visible solo ahí, para leer la consulta intermedia.

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Idempotencia | Repetir un trabajo no duplica ni cuenta dos veces (`ON CONFLICT DO NOTHING`, `WHERE estado NOT IN`). |
| `CASE WHEN` | Condicionales dentro de SQL. |
| `unnest(arreglo)` | Convierte un arreglo en filas. |
| Bloqueo implícito del `UPDATE` | Serializa actualizaciones concurrentes de la misma fila. |
| Aritmética en la base de datos | `x = x + 1` en SQL, nunca "leer, sumar y escribir" en la aplicación. |
| Tipo local | `type fila struct{...}` declarado dentro de una función. |

---

## 5. Cómo comprobarlo

```bash
make lint
```

En el paso 58, contra PostgreSQL:
- `TestCreacionDeLoteDeReportes`: un lote con 2 personas crea 2 reportes; el lote en proceso cuenta como activo; repetir la creación **no duplica**; los datos de la semana de Laura traen 1 tarea, 3 horas, su nombre y el del profesor.
- `TestFinalizacionDeReportes`: un reporte completado (completarlo **dos veces** no cuenta dos veces) y otro fallido dejan el lote en `completado_con_errores`, con 1 y 1 en los contadores y fecha de finalización; el reporte completado tiene 1 intento y su ruta de PDF.

---

## 6. Pasos para lograr esta versión desde el paso 45

1. En `repositorio/comun.go`, completar el bloque con `recursoLote = "lote de reportes"`, `recursoLotes = "lotes de reportes"`, `recursoReporte = "reporte"` y `recursoReportes = "reportes"`.
2. Crear `backend/internal/repositorio/reportes.go` con las secciones de lotes, reportes y datos de la semana.
3. `make lint`
4. Commit: `git add . && git commit -m "Paso 46: repositorio de reportes"`

---

## 7. Preparación para las próximas entregas

- **Seguro con varios workers:** en la Entrega 3 se podrá escalar el número de workers (o usar más concurrencia) sin cambiar este código: la consistencia la garantiza PostgreSQL.
- **Idempotente ante reintentos:** encaja con cualquier cola con entrega "al menos una vez" (Asynq aquí; Pub/Sub o Cloud Tasks en la Entrega 4).

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-14.1 Ejecución por lotes | Lotes con estado y contadores, sin duplicar lotes activos. |
| RF-14.2 Un reporte por persona | `ListarPersonasParaReporte` + `CrearReportesPendientes` idempotente. |
| RF-14.3 / RN-43 Insumos permitidos | `ObtenerDatosSemana` solo lee esos campos. |
| RF-14.5 Almacenar | Ruta y tamaño del PDF en `CompletarReporte`. |
| RF-14.7 Semana concreta | Todas las consultas son por semana. |
