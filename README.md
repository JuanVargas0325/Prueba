# Paso 26 · Repositorio de espacios

> Rama `paso-a-paso` · paso 26 de 65 · anterior: **Paso 25 · Dominio de cursos y proyectos** · siguiente: **Paso 27 · Reloj inyectable**

---

## 1. Qué se hizo en este paso

Se creó el repositorio de cursos y proyectos sobre la tabla `espacios`.

| Archivo | Cambio |
|---|---|
| `backend/internal/repositorio/espacios.go` | **Nuevo.** `Crear`, `Obtener`, `Listar` (con visibilidad por usuario), `Cerrar` y `EsParticipante`. |
| `backend/internal/repositorio/comun.go` | **Modificado.** Constantes `recursoEspacio`, `recursoEspacios` y `recursoVinculacion`. |

---

## 2. Por qué

- El servicio de espacios (paso 28) necesita guardarlos, consultarlos y cerrarlos.
- **La visibilidad se aplica en la consulta SQL:** un profesor solo ve sus espacios y un monitor solo aquellos en los que está vinculado (RF-03.4, RF-11.1, RF-12). Hacerlo en SQL es eficiente (la base de datos devuelve solo lo necesario) y seguro (el servicio nunca recibe datos que el actor no debe ver).

---

## 3. Explicación detallada de `espacios.go`

### 3.1 Consulta base con `JOIN`

```go
const selectEspacio = `
	SELECT e.id, e.tipo, e.nombre, e.periodo_id, p.codigo, e.profesor_id, u.nombre,
	       e.fecha_inicio, e.fecha_cierre, e.observaciones, e.estado, e.creado_en
	FROM espacios e
	JOIN periodos_academicos p ON p.id = e.periodo_id
	JOIN usuarios u ON u.id = e.profesor_id`
```
- Un **`JOIN`** combina filas de varias tablas. Aquí trae, junto con cada espacio, el **código del período** y el **nombre del profesor**. Por eso la entidad del dominio tiene `PeriodoCodigo` y `ProfesorNombre` (paso 25).
- Las letras `e`, `p`, `u` son **alias** de las tablas, para escribir menos.

### 3.2 `Crear`

```go
func (r *Espacios) Crear(ctx context.Context, e dominio.Espacio) (dominio.Espacio, error) {
	var id string
	err := r.db.QueryRow(ctx,
		`INSERT INTO espacios (tipo, nombre, periodo_id, profesor_id, fecha_inicio, fecha_cierre, observaciones)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		...
	).Scan(&id)
	if err != nil {
		return e, traducir(err, recursoEspacio)
	}
	return r.Obtener(ctx, id)
}
```
- Inserta y luego **vuelve a leer** con `Obtener`, para devolver el espacio completo con los datos de los `JOIN` (código del período y nombre del profesor), que el `INSERT` no tiene.
- El estado no se envía: la columna tiene `DEFAULT 'activo'`.

### 3.3 `Listar` con visibilidad

```go
func (r *Espacios) Listar(ctx context.Context, f dominio.FiltroEspacios) ([]dominio.Espacio, error) {
	var w filtro
	if f.VisiblePara != "" {
		w.agregar(`(e.profesor_id = ? OR EXISTS (SELECT 1 FROM vinculaciones v WHERE v.espacio_id = e.id AND v.usuario_id = ?))`, f.VisiblePara)
	}
	if f.PeriodoID != "" { w.agregar("e.periodo_id = ?", f.PeriodoID) }
	if f.Estado != "" { w.agregar("e.estado = ?", string(f.Estado)) }
	if f.Tipo != "" { w.agregar("e.tipo = ?", string(f.Tipo)) }
	sql := selectEspacio + w.where() + ` ORDER BY e.estado, lower(e.nombre)` + w.paginar(f.Paginacion)
	...
}
```
- **La regla de visibilidad en una línea de SQL:** el espacio es visible si el usuario es su **profesor** o si tiene alguna **vinculación** en él. Aquí se aprovecha que `filtro.agregar` reemplaza **los dos** `?` por el mismo parámetro (paso 13).
- `EXISTS (SELECT 1 ...)` pregunta "¿existe al menos una fila?" sin traerlas.
- Orden: primero los activos (`activo` < `cerrado` alfabéticamente) y luego por nombre.

### 3.4 `Cerrar`

```go
func (r *Espacios) Cerrar(ctx context.Context, id string) (dominio.Espacio, error) {
	etiqueta, err := r.db.Exec(ctx,
		`UPDATE espacios SET estado = 'cerrado', actualizado_en = now() WHERE id = $1 AND estado = 'activo'`, id)
	if err != nil {
		return dominio.Espacio{}, traducir(err, recursoEspacio)
	}
	if etiqueta.RowsAffected() == 0 {
		return dominio.Espacio{}, apperr.Conflicto("el curso o proyecto ya está cerrado")
	}
	return r.Obtener(ctx, id)
}
```
- El `WHERE ... AND estado = 'activo'` hace que la operación sea **segura ante concurrencia**: si dos peticiones intentan cerrar a la vez, solo una cambia la fila.
- `RowsAffected()` dice cuántas filas cambió el `UPDATE`. Si fue 0, el espacio ya estaba cerrado → **409**. (Si el espacio no existiera, el servicio lo detecta antes con `Obtener`.)

### 3.5 `EsParticipante`

```go
func (r *Espacios) EsParticipante(ctx context.Context, espacioID, usuarioID string) (bool, error)
```
- `SELECT EXISTS` sobre `vinculaciones`. El servicio lo usa para decidir si un monitor puede ver el detalle de un espacio.

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| `JOIN` | Combina filas relacionadas de varias tablas. |
| `EXISTS` | Subconsulta que solo pregunta si hay filas. |
| `RowsAffected()` | Filas modificadas por un `UPDATE`/`DELETE`. |
| `UPDATE ... WHERE estado = 'activo'` | Cambio condicional, seguro ante peticiones simultáneas. |
| Visibilidad en SQL | Filtrar en la base de datos según quién pregunta. |

---

## 5. Cómo comprobarlo

```bash
make lint
```

Se ejercita por HTTP en el paso 29 y con pruebas de integración en el paso 58 (`TestEspaciosVisibilidadYCierre`).

---

## 6. Pasos para lograr esta versión desde el paso 25

1. En `repositorio/comun.go`, agregar al bloque de constantes `recursoEspacio = "curso o proyecto"`, `recursoEspacios = "cursos y proyectos"` y `recursoVinculacion = "vinculación"`.
2. Crear `backend/internal/repositorio/espacios.go` con `Espacios`, `NuevoEspacios`, `selectEspacio`, `escanearEspacio`, `Crear`, `Obtener`, `Listar`, `Cerrar` y `EsParticipante`.
3. `make lint`
4. Commit: `git add . && git commit -m "Paso 26: repositorio de espacios"`

---

## 7. Preparación para las próximas entregas

- **La visibilidad filtrada en SQL** mantiene las respuestas pequeñas aunque crezca el número de espacios. Si hiciera falta, un índice sobre `vinculaciones(espacio_id, usuario_id)` acelera el `EXISTS` (ya existen índices por `usuario_id` y por `espacio_id`).

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-03.1 Crear cursos y proyectos | `Crear`. |
| RF-03.4 / RF-11.1 Consultar sus espacios | `Listar` con `VisiblePara`. |
| RF-03.5 Cerrar | `Cerrar`, sin doble cierre. |
| RF-12 Visibilidad | Filtro por profesor o participación en SQL. |
