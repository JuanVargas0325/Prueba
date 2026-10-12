# Paso 13 · Repositorio de usuarios

> Rama `paso-a-paso` · paso 13 de 65 · anterior: **Paso 12 · bcrypt y JWT** · siguiente: **Paso 14 · Servicio de autenticación**

---

## 1. Qué se hizo en este paso

Se creó la **capa de datos** (`repositorio`) con su primer repositorio: el de usuarios. Es el único lugar del sistema que escribe SQL sobre las tablas `usuarios` y `usuario_roles`.

| Archivo | Cambio |
|---|---|
| `backend/internal/repositorio/comun.go` | **Nuevo.** Utilidades compartidas por todos los repositorios: transacciones, traducción de errores de PostgreSQL, constructor de filtros `WHERE`, paginación y lectura de filas. |
| `backend/internal/repositorio/usuarios.go` | **Nuevo.** `BuscarPorCorreo`, `BuscarPorID`, `Listar`, `Crear`, `ReemplazarRoles`, `TieneVinculacionesActivas` y `TieneEspaciosActivos`. |

---

## 2. Por qué

- **Patrón repositorio:** las capas superiores piden "búscame el usuario con este correo" sin saber que hay SQL detrás. Si mañana cambia la consulta, o incluso la base de datos, solo cambia este paquete.
- **Errores de PostgreSQL → errores de la aplicación:** un correo repetido produce en PostgreSQL el código `23505`. El servicio no debería conocer ese número; el repositorio lo traduce a `apperr.Conflicto` (paso 9), que el handler convertirá en 409.
- **Utilidades comunes desde el primer repositorio:** en los pasos siguientes llegarán cinco repositorios más (períodos, espacios, vinculaciones, tareas y reportes). Todos necesitan transacciones, filtros y paginación; se escriben una vez.

---

## 3. Explicación detallada

### 3.1 `comun.go`

#### Nombres de recursos

```go
const (
	recursoUsuario       = "usuario"
	recursoUsuarios      = "usuarios"
	recursoVinculaciones = "vinculaciones"
)
```
- Textos que aparecen en los mensajes de error ("usuario no encontrado"). Se definen como constantes para no repetir el mismo literal en muchos lugares (SonarQube lo reportaría en el paso 62). Cada repositorio nuevo agregará las suyas.

#### Transacciones

```go
func enTransaccion(ctx context.Context, db *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("iniciando transacción: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no hace nada si ya se confirmó

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("confirmando transacción: %w", err)
	}
	return nil
}
```
- Recibe una **función** con el trabajo a hacer dentro de la transacción.
- El truco del `defer tx.Rollback`: si `fn` falla (o entra en pánico), al salir se revierte todo. Si ya se hizo `Commit`, el `Rollback` posterior no hace nada. Así es imposible olvidar revertir.
- Se usa, por ejemplo, para crear un usuario y sus roles: o se guardan ambos o ninguno.

#### Traducción de errores

```go
const (
	codigoUnico         = "23505"
	codigoLlaveForanea  = "23503"
	codigoRestriccion   = "23514"
	codigoTextoInvalido = "22P02"
)

func traducir(err error, recurso string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.NoEncontrado("%s no encontrado", recurso)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case codigoUnico:
			return apperr.Conflicto("%s ya existe", recurso)
		case codigoLlaveForanea:
			return apperr.Validacion("%s hace referencia a un registro que no existe", recurso)
		case codigoRestriccion, codigoTextoInvalido:
			return apperr.Validacion("los datos de %s no son válidos", recurso)
		}
	}
	return fmt.Errorf("%s: %w", recurso, err)
}
```

| Situación | PostgreSQL | Se traduce a |
|---|---|---|
| La consulta no devolvió filas | `pgx.ErrNoRows` | `NoEncontrado` (404) |
| Violación de `UNIQUE` | `23505` | `Conflicto` (409) |
| Llave foránea a un registro inexistente | `23503` | `Validacion` (400) |
| Violación de un `CHECK` | `23514` | `Validacion` (400) |
| Texto que no es un UUID válido, etc. | `22P02` | `Validacion` (400) |
| Cualquier otro | — | se envuelve y queda como error interno (500) |

- Si `err` es `nil`, devuelve `nil`. Eso permite escribir `return u, traducir(err, recursoUsuario)` siempre, haya o no error.

#### Constructor de filtros

```go
type filtro struct {
	condiciones []string
	args        []any
}

func (f *filtro) agregar(expresion string, valor any) {
	f.args = append(f.args, valor)
	f.condiciones = append(f.condiciones, strings.ReplaceAll(expresion, "?", fmt.Sprintf("$%d", len(f.args))))
}

func (f *filtro) where() string {
	if len(f.condiciones) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(f.condiciones, " AND ")
}
```
- Muchos listados tienen filtros **opcionales** (por rol, por búsqueda...). Armar el `WHERE` a mano con los números `$1, $2...` es propenso a errores. `filtro` lo hace solo:
  ```go
  var w filtro
  w.agregar("rol = ?", "monitor")                         // condición "rol = $1", args = ["monitor"]
  w.agregar("(nombre ILIKE ? OR correo ILIKE ?)", "%an%") // "(nombre ILIKE $2 OR correo ILIKE $2)"
  w.where()  // " WHERE rol = $1 AND (nombre ILIKE $2 OR correo ILIKE $2)"
  ```
- Cada `?` de una expresión se reemplaza por **el mismo** parámetro; así una condición puede usar el valor dos veces.
- Los valores **nunca** se pegan en el SQL: van en `args` como parámetros. Esto elimina la **inyección SQL**.
- `var w filtro` crea un filtro vacío listo para usar: en Go los structs tienen un **valor cero** útil (listas vacías).
- Receptor puntero `(f *filtro)`: `agregar` **modifica** el filtro.

```go
func (f *filtro) paginar(p dominio.Paginacion) string {
	if p == dominio.SinPaginar {
		return ""
	}
	p = p.Normalizar()
	f.args = append(f.args, p.Limite, p.Desplazamiento)
	return fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(f.args)-1, len(f.args))
}
```
- Agrega `LIMIT`/`OFFSET` con los valores normalizados del paso 11 (máximo 200). `SinPaginar` no agrega nada.

```go
func patronBusqueda(texto string) string {
	escapado := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(texto)
	return "%" + escapado + "%"
}
```
- En `LIKE`/`ILIKE`, `%` significa "cualquier texto" y `_` "un carácter". Si un usuario busca `50%`, el `%` debe tratarse como literal. Se **escapan** los comodines y luego se envuelve con `%...%` ("contiene").

#### Leer filas con genéricos

```go
func recolectar[T any](filas pgx.Rows, escanear func(pgx.Row) (T, error)) ([]T, error) {
	defer filas.Close()
	resultado := []T{}
	for filas.Next() {
		v, err := escanear(filas)
		if err != nil {
			return nil, err
		}
		resultado = append(resultado, v)
	}
	return resultado, filas.Err()
}
```
- `[T any]` es un **parámetro de tipo (genéricos)**: la misma función sirve para leer usuarios, tareas o reportes. `T` se deduce de la función `escanear` que se le pasa.
- `defer filas.Close()` libera la conexión aunque haya un error a mitad del recorrido.
- Devuelve `[]T{}` (lista vacía) y no `nil` cuando no hay filas: en JSON eso se ve como `[]` y no como `null`, más cómodo para los clientes.
- `filas.Err()` informa errores que ocurrieron durante la lectura.

### 3.2 `usuarios.go`

```go
type Usuarios struct {
	db *pgxpool.Pool
}

func NuevoUsuarios(db *pgxpool.Pool) *Usuarios { return &Usuarios{db: db} }
```
- El repositorio guarda el pool (paso 3). `NuevoUsuarios` es su **constructor** (en Go, una función normal por convención `NuevoX`).

```go
const selectUsuario = `
	SELECT u.id, u.nombre, u.correo, u.password_hash, u.activo, u.creado_en,
	       COALESCE(array_agg(r.rol ORDER BY r.rol) FILTER (WHERE r.rol IS NOT NULL), '{}') AS roles
	FROM usuarios u
	LEFT JOIN usuario_roles r ON r.usuario_id = u.id`
```
- Trae cada usuario **con sus roles en una sola consulta**:
  - `LEFT JOIN`: incluye al usuario aunque no tenga roles;
  - `array_agg(r.rol ...)` junta los roles en un arreglo de PostgreSQL (requiere `GROUP BY u.id`, que agrega cada consulta);
  - `FILTER (WHERE r.rol IS NOT NULL)` evita un arreglo `{NULL}` para usuarios sin roles, y `COALESCE(..., '{}')` lo convierte en arreglo vacío.

```go
func escanearUsuario(fila pgx.Row) (dominio.Usuario, error) {
	var u dominio.Usuario
	var roles []string
	if err := fila.Scan(&u.ID, &u.Nombre, &u.Correo, &u.PasswordHash, &u.Activo, &u.CreadoEn, &roles); err != nil {
		return u, err
	}
	for _, r := range roles {
		u.Roles = append(u.Roles, dominio.Rol(r))
	}
	return u, nil
}
```
- `Scan` copia cada columna en su campo, en el mismo orden del `SELECT`. pgx convierte el arreglo de PostgreSQL en `[]string` y aquí se pasa a `[]dominio.Rol`.

```go
func (r *Usuarios) BuscarPorCorreo(ctx context.Context, correo string) (dominio.Usuario, error) {
	u, err := escanearUsuario(r.db.QueryRow(ctx,
		selectUsuario+` WHERE lower(u.correo) = lower($1) GROUP BY u.id`, strings.TrimSpace(correo)))
	return u, traducir(err, recursoUsuario)
}
```
- Compara en minúsculas: coincide con el índice único `lower(correo)` del paso 4 y PostgreSQL puede usar ese índice.

```go
func (r *Usuarios) Listar(ctx context.Context, f dominio.FiltroUsuarios) ([]dominio.Usuario, error) {
	var w filtro
	if f.Rol != "" {
		w.agregar(`EXISTS (SELECT 1 FROM usuario_roles x WHERE x.usuario_id = u.id AND x.rol = ?)`, string(f.Rol))
	}
	if b := strings.TrimSpace(f.Busqueda); b != "" {
		w.agregar(`(u.nombre ILIKE ? OR u.correo ILIKE ?)`, patronBusqueda(b))
	}
	sql := selectUsuario + w.where() + ` GROUP BY u.id ORDER BY lower(u.nombre), u.correo` + w.paginar(f.Paginacion)
	...
}
```
- El filtro por rol usa `EXISTS` (subconsulta) para no perder los **demás** roles del usuario en el `array_agg`.
- `ILIKE` es `LIKE` sin distinguir mayúsculas.
- Orden alfabético por nombre (sin distinguir mayúsculas) y, si empatan, por correo.

```go
func (r *Usuarios) Crear(ctx context.Context, u dominio.Usuario) (dominio.Usuario, error) {
	err := enTransaccion(ctx, r.db, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`INSERT INTO usuarios (nombre, correo, password_hash, activo) VALUES ($1, $2, $3, $4)
			 RETURNING id, creado_en`,
			u.Nombre, u.Correo, u.PasswordHash, u.Activo,
		).Scan(&u.ID, &u.CreadoEn)
		if esViolacionUnica(err) {
			return apperr.Conflicto("ya existe un usuario con el correo %s", u.Correo)
		}
		if err != nil {
			return traducir(err, recursoUsuario)
		}
		return insertarRoles(ctx, tx, u.ID, u.Roles)
	})
	return u, err
}
```
- Usuario y roles en **una transacción**.
- `RETURNING id, creado_en` devuelve los valores que generó PostgreSQL (el UUID y la fecha) sin otra consulta.
- Un correo repetido produce un mensaje específico ("ya existe un usuario con el correo ...") en lugar del genérico.

```go
func (r *Usuarios) ReemplazarRoles(ctx context.Context, usuarioID string, roles []dominio.Rol) error
```
- Borra los roles actuales e inserta los nuevos en una transacción (RF-02.2). `insertarRoles` usa `ON CONFLICT DO NOTHING` para ignorar repetidos.

```go
func (r *Usuarios) TieneVinculacionesActivas(ctx context.Context, usuarioID string, rol dominio.Rol) (bool, error)
func (r *Usuarios) TieneEspaciosActivos(ctx context.Context, usuarioID string) (bool, error)
```
- Consultas `SELECT EXISTS (...)` (devuelven verdadero o falso) sobre `vinculaciones` y `espacios`. Las tablas existen desde el paso 4 aunque sus módulos lleguen después. Las usará el servicio de usuarios (paso 18) para impedir que se le quite a alguien un rol que está usando en un curso activo.

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Genéricos | `func recolectar[T any](...)`: una función para muchos tipos. |
| Valor cero útil | `var w filtro` ya es un filtro vacío utilizable. |
| Función como parámetro | `enTransaccion(ctx, db, func(tx pgx.Tx) error { ... })`. |
| `errors.As` con tipos de librería | Detecta `*pgconn.PgError` para leer el código de PostgreSQL. |
| Constructor `NuevoX` | Convención para crear un valor ya configurado. |

---

## 5. Cómo comprobarlo

Este repositorio todavía no tiene consumidores. Se valida compilando y con el lint:

```bash
make lint      # o: cd backend && go vet ./...
```

Se ejercitará de verdad en el paso 15 (creación del administrador inicial al arrancar la API) y tendrá pruebas automáticas contra un PostgreSQL real en el paso 58 (pruebas de integración).

---

## 6. Pasos para lograr esta versión desde el paso 12

1. Crear `backend/internal/repositorio/comun.go` con: comentario del paquete, constantes `recursoUsuario`, `recursoUsuarios` y `recursoVinculaciones`, `enTransaccion`, códigos de error, `traducir`, `esViolacionUnica`, `filtro` (`agregar`, `where`, `paginar`), `patronBusqueda` y `recolectar`.
2. Crear `backend/internal/repositorio/usuarios.go` con el struct `Usuarios`, `NuevoUsuarios`, `selectUsuario`, `escanearUsuario` y los métodos descritos en 3.2.
3. `make lint`
4. Commit: `git add . && git commit -m "Paso 13: repositorio de usuarios"`

---

## 7. Preparación para las próximas entregas

- **Solo el paquete `repositorio` conoce PostgreSQL.** Al pasar a Cloud SQL (Entrega 2) no cambia nada; si algún día se usara otra base de datos, solo se reescribiría este paquete.
- **Paginación obligatoria en los listados:** limita el costo de cada consulta, algo que se medirá en las pruebas de carga.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-02.1 Crear usuarios | `Crear` con usuario y roles en una transacción. |
| RF-02.2 Asignar roles | `ReemplazarRoles`. |
| RF-02.3 Consulta global de usuarios | `Listar` con filtros por rol y búsqueda, paginado. |
| RNF-06 Consistencia | Transacciones y traducción de las restricciones de la base de datos. |
