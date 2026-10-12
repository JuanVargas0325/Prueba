# Paso 31 · Repositorio de vinculaciones (transacción con bloqueo)

> Rama `paso-a-paso` · paso 31 de 65 · anterior: **Paso 30 · Dominio de vinculaciones** · siguiente: **Paso 32 · Servicio de vinculaciones**

---

## 1. Qué se hizo en este paso

Se creó el repositorio de vinculaciones. Su parte central garantiza que las reglas de horas se cumplan **aunque dos personas vinculen a la misma persona al mismo tiempo**.

| Archivo | Cambio |
|---|---|
| `backend/internal/repositorio/vinculaciones.go` | **Nuevo.** `Crear` y `Rectificar` (con bloqueo y validación dentro de la transacción), `Obtener`, `Listar`, `ListarRectificaciones` y las auxiliares `bloquearUsuario` y `listarSimultaneas`. |
| `backend/internal/repositorio/comun.go` | **Modificado.** La interfaz `consultor`, el método `filtro.agregarFija` y la función `recortarTexto`. |

---

## 2. Por qué: el problema de concurrencia

Laura tiene 12 h de asistencia graduada. Dos profesores, al mismo tiempo, la vinculan como asistente: uno con 6 h y otro con 8 h.

```
Profesor A                                   Profesor B
lee: Laura tiene 12 h                        lee: Laura tiene 12 h
12 + 6 = 18 ≤ 22 ✔                           12 + 8 = 20 ≤ 22 ✔
guarda 6 h                                   guarda 8 h
                       → Laura queda con 26 h ✘ (RN-14)
```

Cada validación por separado fue correcta, pero el resultado viola la regla. Es una **condición de carrera** entre transacciones. RN-19 exige que las reglas sean bloqueantes; la solución es **serializar** las operaciones sobre la misma persona:

```
Profesor A                                   Profesor B
BEGIN; bloquea la fila de Laura              BEGIN; intenta bloquear la fila de Laura → ESPERA
lee 12 h; 18 ≤ 22 ✔; guarda 6 h; COMMIT     (se libera) lee 18 h; 26 > 22 ✘ → ROLLBACK, 422
```

---

## 3. Explicación detallada

### 3.1 `Crear`

```go
func (r *Vinculaciones) Crear(ctx context.Context, v dominio.Vinculacion, validar dominio.ValidadorCarga) (dominio.Vinculacion, error) {
	var id string
	err := enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		if err := bloquearUsuario(ctx, tx, v.UsuarioID); err != nil {
			return err
		}
		// La duplicidad se informa antes que las reglas de horas: es la causa real del rechazo.
		var existe bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM vinculaciones WHERE usuario_id = $1 AND espacio_id = $2 AND rol = $3)`,
			v.UsuarioID, v.EspacioID, string(v.Rol)).Scan(&existe); err != nil {
			return traducir(err, recursoVinculacion)
		}
		if existe {
			return apperr.Conflicto("la persona ya está vinculada como %s en este curso o proyecto", v.Rol.Texto())
		}
		simultaneas, err := listarSimultaneas(ctx, tx, v.UsuarioID, v.EspacioFechaInicio, v.EspacioFechaCierre, "")
		if err != nil {
			return err
		}
		if err := validar(simultaneas, v); err != nil {
			return err
		}
		err = tx.QueryRow(ctx,
			`INSERT INTO vinculaciones (usuario_id, espacio_id, rol, horas_semanales) VALUES ($1, $2, $3, $4) RETURNING id`,
			v.UsuarioID, v.EspacioID, string(v.Rol), v.HorasSemanales).Scan(&id)
		if esViolacionUnica(err) {
			return apperr.Conflicto(...)
		}
		return traducir(err, recursoVinculacion)
	})
	if err != nil {
		return dominio.Vinculacion{}, err
	}
	return r.Obtener(ctx, id)
}
```

Todo ocurre **dentro de una transacción** (paso 13), en este orden:
1. **Bloquear** la fila del usuario (3.3). Desde aquí, cualquier otra operación sobre la carga de esta persona espera.
2. **¿Duplicada?** Si ya existe esa vinculación (misma persona, espacio y rol) → **409**. Se revisa **antes** que las horas: si se vincula dos veces a Laura con 12 h, el motivo real es la duplicidad, no "superaría las 22 h".
3. **Leer las vinculaciones simultáneas** de la persona (3.4).
4. **Ejecutar la regla del dominio** que llegó como parámetro (`validar`, un `dominio.ValidadorCarga`, paso 30). Si falla, la transacción se revierte → **422**.
5. **Insertar.** Si a pesar de todo se violara el `UNIQUE` (otra transacción insertó el mismo duplicado), también se responde 409.
6. Fuera de la transacción, se lee la vinculación completa con sus `JOIN`.

**El repositorio no conoce las reglas de horas.** Solo sabe "bloquea, lee, pregunta si es válido, guarda". La regla viene del dominio a través del servicio (paso 32).

### 3.2 `Rectificar`

```go
func (r *Vinculaciones) Rectificar(ctx context.Context, id string, rol dominio.Rol, horas int,
	administradorID, motivo string, validar dominio.ValidadorCarga) (dominio.Vinculacion, error) {

	err := enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		actual, err := escanearVinculacion(tx.QueryRow(ctx, selectVinculacion+` WHERE v.id = $1 FOR UPDATE OF v`, id))
		...
		if err := bloquearUsuario(ctx, tx, actual.UsuarioID); err != nil { ... }
		simultaneas, err := listarSimultaneas(ctx, tx, actual.UsuarioID, actual.EspacioFechaInicio, actual.EspacioFechaCierre, id)
		...
		corregida := actual
		corregida.Rol, corregida.HorasSemanales = rol, horas
		if err := validar(simultaneas, corregida); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE vinculaciones SET rol = $2, horas_semanales = $3, actualizado_en = now() WHERE id = $1`, ...)
		...
		_, err = tx.Exec(ctx,
			`INSERT INTO vinculacion_rectificaciones
			   (vinculacion_id, administrador_id, rol_anterior, rol_nuevo, horas_anteriores, horas_nuevas, motivo)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			id, administradorID, string(actual.Rol), string(rol), actual.HorasSemanales, horas, recortarTexto(motivo))
		return traducir(err, "rectificación")
	})
	...
}
```
- **RF-04.5:** el administrador corrige rol u horas. Se aplican las **mismas reglas** que al crear; una corrección no puede dejar a la persona por encima de los límites.
- `FOR UPDATE OF v` bloquea la fila de la vinculación que se corrige.
- `listarSimultaneas(..., id)` **excluye** la propia vinculación (si no, se contaría dos veces: con las horas viejas y con las nuevas).
- El cambio **y** su registro histórico (valores anteriores, nuevos, quién y por qué) van en la **misma transacción**: no puede existir una corrección sin su registro.

### 3.3 `bloquearUsuario`

```go
func bloquearUsuario(ctx context.Context, tx pgx.Tx, usuarioID string) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM usuarios WHERE id = $1 FOR UPDATE`, usuarioID).Scan(&id)
	return traducir(err, recursoUsuario)
}
```
- `SELECT ... FOR UPDATE` **bloquea la fila** leída hasta que termine la transacción. Otra transacción que pida el mismo bloqueo **espera**.
- Se bloquea la fila del **usuario** y no las vinculaciones porque la regla es sobre la carga total de la persona, y el conjunto de vinculaciones puede crecer (no se puede bloquear una fila que todavía no existe). La fila del usuario hace de "candado de la persona".
- Solo se serializan las operaciones **sobre la misma persona**; vincular a personas distintas sigue en paralelo.

### 3.4 `listarSimultaneas`

```go
func listarSimultaneas(ctx context.Context, c consultor, usuarioID string, inicio, cierre time.Time, excepto string) ([]dominio.Vinculacion, error) {
	var w filtro
	w.agregar("v.usuario_id = ?", usuarioID)
	w.agregarFija("e.estado = 'activo'")
	w.agregar("e.fecha_inicio <= ?", cierre)
	w.agregar("e.fecha_cierre >= ?", inicio)
	if excepto != "" {
		w.agregar("v.id <> ?", excepto)
	}
	...
}
```
- "Simultáneas" son las vinculaciones de la persona en espacios **activos** cuyas fechas **se cruzan** con las del espacio nuevo (misma lógica de intervalos que `seCruzan`, paso 25, pero en SQL). Una monitoría de un semestre pasado o de un curso cerrado no cuenta para los límites de este.
- Recibe un `consultor` (3.5) para poder ejecutarse con la transacción.

### 3.5 Novedades en `comun.go`

```go
type consultor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
```
- Lo que tienen **en común** el pool (`*pgxpool.Pool`) y una transacción (`pgx.Tx`). Una función que recibe un `consultor` sirve dentro y fuera de transacciones.

```go
func (f *filtro) agregarFija(expresion string) {
	f.condiciones = append(f.condiciones, expresion)
}

func recortarTexto(s string) string {
	const maximo = 1000
	if r := []rune(s); len(r) > maximo {
		return string(r[:maximo])
	}
	return s
}
```
- `agregarFija`: condición sin parámetros (`e.estado = 'activo'`).
- `recortarTexto`: limita a 1000 caracteres los textos libres que se guardan (el motivo de una rectificación; después, los errores de los reportes). Corta por **runas** para no partir una letra con tilde a la mitad.
- `r[:maximo]` es un **slice** de la lista: los primeros `maximo` elementos.

### 3.6 Lecturas

- `selectVinculacion` une `vinculaciones` + `usuarios` (la persona) + `espacios` + `usuarios` otra vez (el profesor, con alias `p`).
- `Listar` aplica `VisiblePara` (`e.profesor_id = ? OR v.usuario_id = ?`: el profesor del espacio o la propia persona), `ProfesorID`, `UsuarioID`, `EspacioID` y `SoloEspaciosActivos`.
- `ListarRectificaciones` devuelve el historial de una vinculación, de la más reciente a la más antigua, usando `recolectar` con una función de escaneo **anónima**.

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Condición de carrera entre transacciones | Validaciones correctas por separado que juntas violan una regla. |
| `SELECT ... FOR UPDATE` | Bloqueo de filas hasta el fin de la transacción. |
| Bloqueo pesimista | Esperar el turno en lugar de detectar el conflicto al final. |
| Regla inyectada | El repositorio ejecuta una función del dominio dentro de su transacción. |
| Interfaz común pool/transacción | `consultor`. |
| Slice de una lista | `r[:n]`. |

---

## 5. Cómo comprobarlo

```bash
make lint
```

Las pruebas automáticas contra PostgreSQL llegan en el paso 58. Ahí, `TestVinculacionesReglasYConcurrencia` lanza **dos goroutines** que vinculan a la misma persona como asistente con 12 h en dos espacios a la vez (24 h > 22 h) y comprueba que **exactamente una** se rechaza con una regla de negocio; además prueba la duplicidad (409), la rectificación con su historial y una rectificación rechazada que no cambia las horas.

---

## 6. Pasos para lograr esta versión desde el paso 30

1. En `repositorio/comun.go`: la interfaz `consultor`, `filtro.agregarFija` y `recortarTexto` (y el import `pgconn`).
2. Crear `backend/internal/repositorio/vinculaciones.go` con `Vinculaciones`, `NuevoVinculaciones`, `selectVinculacion`, `escanearVinculacion`, `Crear`, `Rectificar`, `Obtener`, `Listar`, `ListarRectificaciones`, `bloquearUsuario` y `listarSimultaneas`.
3. `make lint`
4. Commit: `git add . && git commit -m "Paso 31: repositorio de vinculaciones"`

---

## 7. Preparación para las próximas entregas

- **El bloqueo vive en PostgreSQL**, no en la memoria de la API. Por eso la regla se sigue cumpliendo con **varias instancias** de la API (Entrega 3): todas pasan por la misma fila bloqueada.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-04.1 a RF-04.3 Vincular | `Crear`. |
| RF-04.5 Rectificación con registro | `Rectificar` + `vinculacion_rectificaciones` en la misma transacción. |
| RF-04.6 Vinculaciones independientes | Duplicado = misma persona, espacio **y** rol (409). |
| RF-05.6 / RN-19 Reglas bloqueantes y atómicas | Bloqueo del usuario y validación dentro de la transacción. |
