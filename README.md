# Paso 35 · Repositorio de tareas

> Rama `paso-a-paso` · paso 35 de 65 · anterior: **Paso 34 · Dominio de tareas** · siguiente: **Paso 36 · Servicio de tareas**

---

## 1. Qué se hizo en este paso

Se creó el repositorio de las tareas semanales y de los metadatos de sus adjuntos (tablas `tareas` y `tarea_adjuntos`).

| Archivo | Cambio |
|---|---|
| `backend/internal/repositorio/tareas.go` | **Nuevo.** `Crear`, `Obtener`, `Listar`, `Actualizar`, `Eliminar`, `AgregarAdjuntos`, `ObtenerAdjunto`, `EliminarAdjunto` e `insertarAdjuntos`. |
| `backend/internal/repositorio/comun.go` | **Modificado.** Constantes `recursoTarea`, `recursoTareas`, `recursoAdjunto` y `recursoAdjuntos`. |

---

## 2. Por qué

- El servicio de tareas (paso 36) y el de adjuntos (paso 39) necesitan persistir.
- **Defensa en profundidad para RN-33:** aunque el servicio ya impide modificar una tarea tardía, el repositorio **tampoco** la modifica ni la borra. Si algún día otro código llamara al repositorio sin pasar por el servicio, la regla se seguiría cumpliendo.
- **Operaciones atómicas:** una tarea y sus adjuntos se guardan juntos; el número máximo de adjuntos se controla con bloqueo para que dos subidas simultáneas no lo superen.

---

## 3. Explicación detallada de `tareas.go`

### 3.1 Consulta base

```go
const selectTarea = `
	SELECT t.id, t.vinculacion_id, v.usuario_id, u.nombre, v.espacio_id, e.nombre, e.tipo, e.estado, e.profesor_id, v.rol,
	       t.semana_inicio, t.titulo, t.descripcion, t.estado, t.horas_invertidas, t.observaciones, t.tardia,
	       (SELECT count(*) FROM tarea_adjuntos a WHERE a.tarea_id = t.id),
	       t.creado_en, t.actualizado_en
	FROM tareas t
	JOIN vinculaciones v ON v.id = t.vinculacion_id
	JOIN usuarios u ON u.id = v.usuario_id
	JOIN espacios e ON e.id = v.espacio_id`
```
- Desde la vinculación se llega a la **persona**, al **espacio** y a su **profesor**: lo necesario para mostrar la tarea y para aplicar la visibilidad.
- `(SELECT count(*) ...)` es una **subconsulta escalar**: cuenta los adjuntos de cada tarea sin traerlos (útil en los listados).

### 3.2 `Crear`

```go
func (r *Tareas) Crear(ctx context.Context, t dominio.Tarea, adjuntos []dominio.Adjunto) (dominio.Tarea, error) {
	err := enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO tareas (id, vinculacion_id, semana_inicio, titulo, descripcion, estado, horas_invertidas, observaciones, tardia)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`, ...)
		if err != nil {
			return traducir(err, recursoTarea)
		}
		return insertarAdjuntos(ctx, tx, adjuntos)
	})
	...
	return r.Obtener(ctx, t.ID)
}
```
- **El ID lo trae la tarea** (lo genera el servicio, paso 36). Así el servicio conoce el ID **antes** de guardar y puede ubicar los archivos de los adjuntos en una carpeta con ese ID (paso 39).
- Tarea y adjuntos en una transacción: no puede quedar una tarea con la mitad de sus adjuntos.
- Una `semana_inicio` que no sea lunes viola el `CHECK` → 400 (lo comprueba la prueba de integración).

### 3.3 `Obtener` y `Listar`

- `Obtener` trae la tarea **y** la lista de sus adjuntos (segunda consulta, ordenada por fecha de creación).
- `Listar` aplica `VisiblePara` (`e.profesor_id = ? OR v.usuario_id = ?`: el profesor del espacio o la propia persona) y los filtros por usuario, espacio, vinculación y semana. Orden: de la semana más reciente a la más antigua.

### 3.4 `Actualizar` y `Eliminar` con la regla RN-33 en SQL

```go
func (r *Tareas) Actualizar(ctx context.Context, t dominio.Tarea) (dominio.Tarea, error) {
	etiqueta, err := r.db.Exec(ctx,
		`UPDATE tareas SET titulo = $2, descripcion = $3, estado = $4, horas_invertidas = $5, observaciones = $6,
		        actualizado_en = now()
		 WHERE id = $1 AND tardia = FALSE`, ...)
	...
	if etiqueta.RowsAffected() == 0 {
		return t, apperr.ReglaNegocio("la tarea no se puede modificar")
	}
	return r.Obtener(ctx, t.ID)
}
```
- `AND tardia = FALSE`: una tarea tardía no se actualiza aunque se pida. Si no se modificó ninguna fila → 422.
- Solo cambian los campos **editables**; la semana y la vinculación no se tocan (RN-26).

```go
func (r *Tareas) Eliminar(ctx context.Context, id string) ([]string, error) {
	var rutas []string
	err := enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, `SELECT ruta_almacenamiento FROM tarea_adjuntos WHERE tarea_id = $1`, id)
		...
		rutas, err = pgx.CollectRows(filas, pgx.RowTo[string])
		...
		etiqueta, err := tx.Exec(ctx, `DELETE FROM tareas WHERE id = $1 AND tardia = FALSE`, id)
		...
		if etiqueta.RowsAffected() == 0 {
			return apperr.ReglaNegocio("la tarea no se puede eliminar")
		}
		return nil
	})
	return rutas, err
}
```
- Antes de borrar, recoge las **rutas de los archivos** adjuntos y las devuelve: el servicio los borrará del almacenamiento (la base de datos no puede borrar archivos). Los registros de adjuntos se borran solos por el `ON DELETE CASCADE` (paso 4).
- `pgx.CollectRows(filas, pgx.RowTo[string])`: utilidad genérica de pgx que convierte cada fila (de una sola columna) en un `string`.

### 3.5 Adjuntos

```go
func (r *Tareas) AgregarAdjuntos(ctx context.Context, tareaID string, adjuntos []dominio.Adjunto, maximo int) error {
	return enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, `SELECT id FROM tareas WHERE id = $1 FOR UPDATE`, tareaID).Scan(&id); err != nil {
			return traducir(err, recursoTarea)
		}
		var actuales int
		... SELECT count(*) FROM tarea_adjuntos WHERE tarea_id = $1 ...
		if actuales+len(adjuntos) > maximo {
			return apperr.ReglaNegocio("una tarea admite máximo %d adjuntos (ya tiene %d)", maximo, actuales)
		}
		return insertarAdjuntos(ctx, tx, adjuntos)
	})
}
```
- Mismo patrón que las vinculaciones (paso 31): **bloquear** la tarea (`FOR UPDATE`), contar, validar y escribir. Dos subidas simultáneas no pueden superar el máximo.
- `ObtenerAdjunto` y `EliminarAdjunto` filtran por adjunto **y** tarea: no se puede pedir el adjunto de otra tarea cambiando el ID en la URL. `DELETE ... RETURNING ruta_almacenamiento` borra y devuelve la ruta del archivo en una sola operación.

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Subconsulta escalar | `(SELECT count(*) ...)` como columna. |
| ID generado en la aplicación | El servicio crea el UUID antes de insertar. |
| Regla repetida en SQL | `WHERE ... AND tardia = FALSE` como segunda barrera. |
| `ON DELETE CASCADE` | Borrar la tarea borra los registros de sus adjuntos. |
| `DELETE ... RETURNING` | Borrar y obtener datos de la fila borrada. |
| `pgx.CollectRows` / `pgx.RowTo[T]` | Leer filas a una lista con genéricos. |

---

## 5. Cómo comprobarlo

```bash
make lint
```

En el paso 58, `TestTareasYAdjuntos` lo prueba contra PostgreSQL: creación con adjunto, `semana_inicio` que no es lunes (400), visibilidad por profesor y por persona, actualización, máximo de adjuntos (422), eliminación devolviendo las rutas y una tarea tardía que no se actualiza ni se elimina (422).

---

## 6. Pasos para lograr esta versión desde el paso 34

1. En `repositorio/comun.go`, agregar al bloque `recursoTarea = "tarea"`, `recursoTareas = "tareas"`, `recursoAdjunto = "adjunto"` y `recursoAdjuntos = "adjuntos"`.
2. Crear `backend/internal/repositorio/tareas.go` con `Tareas`, `NuevoTareas`, `selectTarea`, `escanearTarea`, `columnasAdjunto`, `escanearAdjunto` y los métodos de la sección 3.
3. `make lint`
4. Commit: `git add . && git commit -m "Paso 35: repositorio de tareas"`

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-06.1 a RF-06.3 Registrar tareas | `Crear`. |
| RF-06.4 Adjuntos (metadatos) | `insertarAdjuntos`, `AgregarAdjuntos`, `ObtenerAdjunto`, `EliminarAdjunto`. |
| RF-06.7 / RF-06.8 Editar y eliminar | `Actualizar`, `Eliminar`. |
| RF-08.5 / RN-33 Tardía inmutable | `tardia = FALSE` en el `UPDATE` y el `DELETE`. |
| RF-11.2 / RF-12 Visibilidad | `Listar` con `VisiblePara`. |
