# Paso 42 · Consulta de tareas por vinculaciones

> Rama `paso-a-paso` · paso 42 de 65 · anterior: **Paso 41 · Dominio del seguimiento** · siguiente: **Paso 43 · Servicio de seguimiento**

---

## 1. Qué se hizo en este paso

Se agregó al repositorio de tareas una consulta pensada para el seguimiento: **todas** las tareas de un conjunto de vinculaciones, opcionalmente entre dos semanas.

| Archivo | Cambio |
|---|---|
| `backend/internal/repositorio/tareas.go` | **Modificado.** Nuevo método `ListarPorVinculaciones`, con su propia consulta liviana (`selectTareaPropia` y `escanearTareaPropia`). |

---

## 2. Por qué

El seguimiento (paso 43) necesita, para una persona o para todas las personas de un profesor, las tareas de **varias vinculaciones** a la vez, en un rango de semanas, **sin paginar** (para calcular estados y totales hacen falta todas). `Listar` (paso 35) no sirve: pagina, filtra por una sola vinculación y aplica visibilidad por actor. Una consulta específica y en **una sola ida** a la base de datos evita el problema "N+1" (una consulta por vinculación).

Además, es la consulta que **más filas** procesa del sistema: la vista principal del profesor lee las tareas de todas sus personas en cada carga. Por eso solo trae lo que el seguimiento usa.

---

## 3. Explicación detallada

### 3.1 Una consulta liviana

```go
const selectTareaPropia = `
	SELECT t.id, t.vinculacion_id, t.semana_inicio, t.titulo, t.descripcion, t.estado,
	       t.horas_invertidas, t.observaciones, t.tardia
	FROM tareas t`

func escanearTareaPropia(fila pgx.Row) (dominio.Tarea, error) {
	var t dominio.Tarea
	var estado string
	err := fila.Scan(&t.ID, &t.VinculacionID, &t.SemanaInicio, &t.Titulo, &t.Descripcion, &estado,
		&t.HorasInvertidas, &t.Observaciones, &t.Tardia)
	t.Estado = dominio.EstadoTarea(estado)
	return t, err
}
```
- `selectTarea` (paso 35) une `tareas` con `vinculaciones`, `usuarios` y `espacios` y cuenta los adjuntos de **cada** fila con una subconsulta. Eso es útil para mostrar una tarea suelta, pero el seguimiento ya tiene esos datos: los obtuvo al leer las **vinculaciones**. Repetirlos en cada una de cientos o miles de tareas es trabajo inútil para PostgreSQL y más datos viajando por la red.
- `selectTareaPropia` lee **solo la tabla `tareas`**, con las columnas que usan el cálculo semanal, el detalle y el consolidado.
- Los campos que no se leen (nombre del usuario, del espacio, cantidad de adjuntos...) quedan con su **valor cero** en la `dominio.Tarea`. El seguimiento no los usa.
- **Efecto medido:** con 3.000 tareas extra en una semana, este cambio duplicó las peticiones por segundo de la vista del profesor (de 217 a 423 por segundo, junto con el cambio del paso 43). El detalle está en el paso 64.

### 3.2 El método

```go
func (r *Tareas) ListarPorVinculaciones(ctx context.Context, vinculacionIDs []string, desde, hasta *time.Time) ([]dominio.Tarea, error) {
	if len(vinculacionIDs) == 0 {
		return []dominio.Tarea{}, nil
	}
	var w filtro
	w.agregar("t.vinculacion_id = ANY(?::uuid[])", vinculacionIDs)
	if desde != nil {
		w.agregar("t.semana_inicio >= ?", *desde)
	}
	if hasta != nil {
		w.agregar("t.semana_inicio <= ?", *hasta)
	}
	filas, err := r.db.Query(ctx, selectTareaPropia+w.where()+` ORDER BY t.semana_inicio DESC, t.creado_en`, w.args...)
	if err != nil {
		return nil, traducir(err, recursoTareas)
	}
	tareas, err := recolectar(filas, escanearTareaPropia)
	return tareas, traducir(err, recursoTareas)
}
```

- **Sin vinculaciones, sin consulta:** devuelve una lista vacía de inmediato. Una persona sin vinculaciones no tiene tareas.
- **`= ANY(?::uuid[])`:** pgx envía la lista de Go (`[]string`) como un **arreglo** de PostgreSQL, y `::uuid[]` lo convierte a arreglo de UUID. `ANY` compara contra cada elemento. Es la forma eficiente y segura de hacer un `IN (...)` con una cantidad variable de valores **en un solo parámetro**, sin armar el SQL a mano.
- **Rango opcional:** `desde` y `hasta` son punteros; `nil` = sin límite (el consolidado del paso 43 necesita todo el historial).
- Usa el índice `tareas_vinculacion_semana (vinculacion_id, semana_inicio)` del paso 4, que cubre justamente este filtro.
- No pagina y no aplica `VisiblePara`: la visibilidad ya se resolvió al elegir **qué vinculaciones** se consultan (paso 43).

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Problema N+1 | Hacer una consulta por elemento en lugar de una para todos. |
| `= ANY($1::uuid[])` | Comparar contra una lista enviada como un único parámetro de tipo arreglo. |
| Índice compuesto | `(vinculacion_id, semana_inicio)` acelera filtros por ambas columnas. |
| Retorno temprano | Si no hay nada que buscar, se responde sin tocar la base de datos. |
| Valor cero | Un campo de un `struct` que no se asigna vale `""`, `0`, `false` o `nil`, según su tipo. |

---

## 5. Cómo comprobarlo

```bash
make lint
```

Se ejercita a través del servicio de seguimiento (paso 43, con un doble en memoria), por HTTP (paso 44) y con las pruebas de integración (paso 58).

---

## 6. Pasos para lograr esta versión desde el paso 41

1. En `backend/internal/repositorio/tareas.go`, después de `Listar`, agregar `selectTareaPropia`, `escanearTareaPropia` y `ListarPorVinculaciones` (sección 3).
2. `make lint`
3. Commit: `git add . && git commit -m "Paso 42: consulta de tareas por vinculaciones"`

---

## 7. Preparación para las próximas entregas

- Esta consulta es la base del escenario "vista del profesor" de las pruebas de carga de todas las entregas. Si en la nube siguiera siendo el cuello de botella, el siguiente paso sería una consulta que devuelva directamente horas y cantidad de tareas por vinculación y semana (`GROUP BY`), a costa de un poco más de código en el dominio.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-10.3 / RF-10.4 Detalle y consolidado | Proporciona las tareas de todas las vinculaciones de una persona. |
| RF-10.1 Personas a cargo | Tareas de todas las vinculaciones del profesor en dos semanas. |
| RNF-01 Backend sólido | Una consulta por pantalla, no una por vinculación. |
