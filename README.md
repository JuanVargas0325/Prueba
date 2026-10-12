# Paso 22 · Repositorio de períodos

> Rama `paso-a-paso` · paso 22 de 65 · anterior: **Paso 21 · Dominio de períodos** · siguiente: **Paso 23 · Servicio de períodos**

---

## 1. Qué se hizo en este paso

Se creó el repositorio que guarda y lee los períodos académicos en la tabla `periodos_academicos` (creada en el paso 4).

| Archivo | Cambio |
|---|---|
| `backend/internal/repositorio/periodos.go` | **Nuevo.** `Crear`, `Obtener`, `Listar` y `Actualizar`. |
| `backend/internal/repositorio/comun.go` | **Modificado.** Constantes `recursoPeriodo` y `recursoPeriodos` para los mensajes de error. |

---

## 2. Por qué

El servicio de períodos (paso 23) necesita persistirlos. Siguiendo la arquitectura en capas, **solo** el repositorio escribe SQL.

---

## 3. Explicación detallada de `periodos.go`

```go
type Periodos struct {
	db *pgxpool.Pool
}

func NuevoPeriodos(db *pgxpool.Pool) *Periodos { return &Periodos{db: db} }

const columnasPeriodo = `id, codigo, fecha_inicio, fecha_cierre, estado, creacion_habilitada, creado_en`
```
- Mismo esqueleto que el repositorio de usuarios (paso 13).
- `columnasPeriodo` evita repetir la lista de columnas en cada consulta y garantiza que siempre coincida con el orden de `escanearPeriodo`.

```go
func escanearPeriodo(fila pgx.Row) (dominio.PeriodoAcademico, error) {
	var p dominio.PeriodoAcademico
	var estado string
	err := fila.Scan(&p.ID, &p.Codigo, &p.FechaInicio, &p.FechaCierre, &estado, &p.CreacionHabilitada, &p.CreadoEn)
	p.Estado = dominio.EstadoPeriodo(estado)
	return p, err
}
```
- pgx convierte las columnas `DATE` en `time.Time` a medianoche UTC, exactamente la representación de fechas del dominio (paso 20).
- El estado se lee como `string` y se convierte al tipo del dominio.

```go
func (r *Periodos) Crear(ctx context.Context, p dominio.PeriodoAcademico) (dominio.PeriodoAcademico, error) {
	creado, err := escanearPeriodo(r.db.QueryRow(ctx,
		`INSERT INTO periodos_academicos (codigo, fecha_inicio, fecha_cierre, estado, creacion_habilitada)
		 VALUES ($1, $2, $3, $4, $5) RETURNING `+columnasPeriodo,
		p.Codigo, p.FechaInicio, p.FechaCierre, string(p.Estado), p.CreacionHabilitada))
	if esViolacionUnica(err) {
		return creado, apperr.Conflicto("ya existe el período %s", p.Codigo)
	}
	return creado, traducir(err, recursoPeriodo)
}
```
- `INSERT ... RETURNING <columnas>` inserta y devuelve la fila completa en un solo viaje a la base de datos.
- El código del período es único (`codigo TEXT NOT NULL UNIQUE`): crear dos veces `2026-20` → **409** con un mensaje específico.

```go
func (r *Periodos) Listar(ctx context.Context, f dominio.FiltroPeriodos) ([]dominio.PeriodoAcademico, error) {
	var w filtro
	if f.Estado != "" {
		w.agregar("estado = ?", string(f.Estado))
	}
	sql := `SELECT ` + columnasPeriodo + ` FROM periodos_academicos` + w.where() +
		` ORDER BY fecha_inicio DESC` + w.paginar(f.Paginacion)
	...
}
```
- Filtro opcional por estado, del período más reciente al más antiguo, paginado.

```go
func (r *Periodos) Actualizar(ctx context.Context, p dominio.PeriodoAcademico) (dominio.PeriodoAcademico, error) {
	actualizado, err := escanearPeriodo(r.db.QueryRow(ctx,
		`UPDATE periodos_academicos SET estado = $2, creacion_habilitada = $3
		 WHERE id = $1 RETURNING `+columnasPeriodo,
		p.ID, string(p.Estado), p.CreacionHabilitada))
	return actualizado, traducir(err, recursoPeriodo)
}
```
- Solo se pueden cambiar el **estado** y la **ventana de creación**. El código y las fechas no se modifican después de creado el período (los cursos ya creados dependen de ellas).
- Si el `id` no existe, `RETURNING` no devuelve filas → `pgx.ErrNoRows` → `traducir` lo convierte en 404.

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| `INSERT/UPDATE ... RETURNING` | PostgreSQL devuelve las filas afectadas, sin otra consulta. |
| Lista de columnas compartida | Una constante para que la consulta y el `Scan` no se desalineen. |
| Conversión de tipos | `string(p.Estado)` al escribir, `dominio.EstadoPeriodo(estado)` al leer. |

---

## 5. Cómo comprobarlo

```bash
make lint
```

Se ejercita por HTTP en el paso 24 y con pruebas automáticas contra PostgreSQL en el paso 58.

---

## 6. Pasos para lograr esta versión desde el paso 21

1. En `repositorio/comun.go`, agregar al bloque de constantes:
   ```go
   recursoPeriodo  = "período"
   recursoPeriodos = "períodos"
   ```
2. Crear `backend/internal/repositorio/periodos.go` con `Periodos`, `NuevoPeriodos`, `columnasPeriodo`, `escanearPeriodo`, `Crear`, `Obtener`, `Listar` y `Actualizar`.
3. `make lint`
4. Commit: `git add . && git commit -m "Paso 22: repositorio de períodos"`

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-03.2 Períodos explícitos | Persistencia de código, fechas, estado y ventana de creación. |
| RNF-04 Persistencia | Tabla `periodos_academicos`. |
