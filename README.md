# Paso 15 · Administrador inicial al arrancar

> Rama `paso-a-paso` · paso 15 de 65 · anterior: **Paso 14 · Servicio de autenticación** · siguiente: **Paso 16 · Middleware de autenticación y roles**

---

## 1. Qué se hizo en este paso

Las piezas de los pasos 12 a 14 se conectan en `main`. Al arrancar, la API crea el **primer administrador** con los datos del `.env`, si todavía no existe.

| Archivo | Cambio |
|---|---|
| `backend/internal/config/config.go` | **Modificado.** Nuevas secciones `JWT` (secreto y expiración) y `AdminInicial` (nombre, correo y contraseña), y función `duracion`. |
| `backend/internal/config/config_test.go` | **Modificado.** Pruebas del secreto obligatorio, del valor por defecto de la expiración y de formatos inválidos. |
| `backend/cmd/api/main.go` | **Modificado.** Crea el repositorio de usuarios, el gestor de tokens y el servicio de autenticación, y llama a `AsegurarAdministrador`. |
| `.env.example` | **Modificado.** Nueva sección `Autenticación`. |

Al levantar el sistema por primera vez:

```json
{"level":"INFO","msg":"administrador inicial creado","servicio":"api","correo":"admin@uniandes.edu.co"}
```

---

## 2. Por qué

- Para crear usuarios (paso 19) hace falta un administrador, y para tener un administrador hace falta crearlo. Hacerlo automáticamente al arrancar evita pasos manuales en la base de datos.
- El **secreto de los JWT** es lo único que impide falsificar tokens. Por eso es **obligatorio** y debe ser largo: si alguien lo adivinara, podría fabricarse un token de administrador.
- La expiración de los tokens se configura (8 horas por defecto, una jornada de trabajo).

---

## 3. Explicación detallada

### 3.1 `config.go`

```go
type Config struct {
	Entorno     string // desarrollo | produccion
	PuertoHTTP  string
	BaseDatos   string // URL de conexión a PostgreSQL
	ZonaHoraria *time.Location

	JWT   JWT
	Admin AdminInicial
}

type JWT struct {
	Secreto    string
	Expiracion time.Duration
}

// AdminInicial es el administrador que se crea al arrancar si aún no existe.
type AdminInicial struct {
	Nombre   string
	Correo   string
	Password string
}
```
- La configuración se organiza en **sub-structs** por tema. Se lee `cfg.JWT.Secreto`, `cfg.Admin.Correo`...
- Un campo puede llamarse igual que su tipo (`JWT JWT`): el primero es el nombre del campo y el segundo el tipo.

```go
	c.JWT = JWT{
		Secreto:    os.Getenv("JWT_SECRETO"),
		Expiracion: duracion("JWT_EXPIRACION", 8*time.Hour, &errs),
	}
	if len(c.JWT.Secreto) < 32 {
		errs = append(errs, errors.New("JWT_SECRETO es obligatorio y debe tener al menos 32 caracteres"))
	}
```
- 32 caracteres es un mínimo razonable para la firma HS256. Se recomienda generarlo al azar: `openssl rand -base64 48`.
- No se aplica `TrimSpace` al secreto ni a la contraseña: los espacios podrían ser parte del valor.

```go
	c.Admin = AdminInicial{
		Nombre:   texto("ADMIN_NOMBRE", "Administrador"),
		Correo:   strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_CORREO"))),
		Password: os.Getenv("ADMIN_PASSWORD"),
	}
```
- El correo se normaliza a minúsculas, igual que en el dominio (paso 11).
- Correo y contraseña **no** son obligatorios aquí: si faltan, el servicio solo advierte (paso 14). Eso permite, por ejemplo, ejecutar el worker sin estos datos.

```go
func duracion(clave string, porDefecto time.Duration, errs *[]error) time.Duration {
	v := strings.TrimSpace(os.Getenv(clave))
	if v == "" {
		return porDefecto
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s debe ser una duración como 30s o 2m: %w", clave, err))
		return porDefecto
	}
	return d
}
```
- Lee duraciones en el formato de Go: `30s`, `2m`, `8h`, `1h30m`.
- `errs *[]error` es un **puntero a la lista de errores** de `Cargar`. Así la función puede **agregar** su error a esa lista (`*errs = append(*errs, ...)`): `append` puede crear una lista nueva, y con el puntero se actualiza la variable original. Se llama con `&errs`.
- Si el valor es inválido, registra el error y devuelve el valor por defecto, para seguir validando las demás variables y reportarlas todas juntas.

### 3.2 `main.go`: el punto de ensamblaje

```go
	usuarios := repositorio.NuevoUsuarios(db)

	// Capa de servicio
	tokens := auth.NuevoGestorTokens(cfg.JWT.Secreto, cfg.JWT.Expiracion)
	servicioAuth := servicio.NuevaAutenticacion(usuarios, tokens, log)
	if err := servicioAuth.AsegurarAdministrador(ctx, cfg.Admin.Nombre, cfg.Admin.Correo, cfg.Admin.Password); err != nil {
		return err
	}
```

- `main` es el **único lugar** que conoce las implementaciones concretas y las conecta: el repositorio de PostgreSQL se entrega al servicio como su `RepositorioUsuarios` y el `*auth.GestorTokens` como su `EmisorTokens`. A esto se le llama **composición** o *composition root*.
- Las capas de abajo no se conocen entre sí: el servicio no sabe que hay PostgreSQL y el repositorio no sabe que existe un servicio.
- Si la creación falla (por ejemplo, la base de datos rechaza la operación), la API no arranca: es preferible un error claro al inicio que un sistema sin administrador.

### 3.3 `.env.example`

```dotenv
# --- Autenticación ----------------------------------------------------------------
# Mínimo 32 caracteres. Genere uno con: openssl rand -base64 48
JWT_SECRETO=reemplace-este-valor-por-un-secreto-aleatorio-largo
JWT_EXPIRACION=8h

# Administrador inicial: se crea al arrancar la API si aún no existe.
ADMIN_NOMBRE=Administrador
ADMIN_CORREO=admin@uniandes.edu.co
ADMIN_PASSWORD=cambie-esta-clave
```
- **Cambie `JWT_SECRETO` y `ADMIN_PASSWORD` en su `.env`.** Los valores del ejemplo son públicos (están en el repositorio).
- Cambiar `ADMIN_PASSWORD` **después** de creado el administrador no cambia su contraseña: `AsegurarAdministrador` solo crea, nunca modifica. Para empezar de cero: `docker compose down -v`.

### 3.4 Pruebas de configuración

| Prueba | Qué verifica |
|---|---|
| `TestCargarConstruyeLaURLDesdeVariablesDePostgres` | Ahora define `JWT_SECRETO` (obligatorio) y comprueba que la expiración por defecto es 8 h. |
| `TestCargarExigeSecretoYBaseDeDatos` | Sin base de datos y con un secreto corto, el error menciona **ambos** problemas a la vez. |
| `TestCargarValidaFormatos` | Una zona horaria y una duración inválidas se reportan juntas. |

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Sub-structs | Agrupar configuración por tema: `cfg.JWT.Secreto`. |
| Puntero a slice | `errs *[]error` permite que otra función agregue elementos a la lista original. |
| `time.ParseDuration` | Interpreta textos como `8h` o `90s`. |
| Composition root | `main` crea las implementaciones concretas y las conecta mediante interfaces. |

---

## 5. Cómo comprobarlo

```bash
cp .env.example .env          # o agregue la sección Autenticación a su .env
docker compose up --build -d --wait
docker compose logs api | grep administrador
docker compose exec postgres psql -U seguimiento -d seguimiento \
  -c "SELECT u.correo, r.rol, left(u.password_hash, 7) AS hash FROM usuarios u JOIN usuario_roles r ON r.usuario_id = u.id"
#         correo         |      rol      |  hash
#  admin@uniandes.edu.co | administrador | $2a$10$     ← bcrypt, costo 10
docker compose restart api    # no se crea un segundo administrador
```

Si `JWT_SECRETO` es corto, la API no arranca: `docker compose logs api` muestra el motivo.

---

## 6. Pasos para lograr esta versión desde el paso 14

1. `config.go`: agregar los structs `JWT` y `AdminInicial`, los campos `JWT` y `Admin` en `Config`, su carga en `Cargar()` y la función `duracion`.
2. `config_test.go`: constante `secretoValido` y las tres pruebas de la sección 3.4.
3. `main.go`: imports `auth`, `repositorio` y `servicio`; crear `usuarios`, `tokens` y `servicioAuth`; llamar a `AsegurarAdministrador`.
4. `.env.example`: agregar la sección `Autenticación` (y copiarla a su `.env` con valores propios).
5. `make pruebas lint` y `docker compose up --build -d --wait`.
6. Commit: `git add . && git commit -m "Paso 15: administrador inicial al arrancar"`

---

## 7. Preparación para las próximas entregas

- **Secretos por variables de entorno:** en la nube, `JWT_SECRETO` y `ADMIN_PASSWORD` irán a un gestor de secretos (Secret Manager de GCP) que los inyecta como variables. El código no cambia.
- **Todas las instancias deben compartir el mismo `JWT_SECRETO`** para validar los tokens que emiten las demás (Entrega 3).

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-15.2 / RF-02 Administración de usuarios | Siempre existe un administrador para crear los demás usuarios. |
| RNF-03 Seguridad básica | Secreto JWT obligatorio y largo; contraseña del administrador con bcrypt. |
| Lineamientos 10 | Credenciales iniciales por variables de entorno. |
