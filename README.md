# Paso 14 · Servicio de autenticación

> Rama `paso-a-paso` · paso 14 de 65 · anterior: **Paso 13 · Repositorio de usuarios** · siguiente: **Paso 15 · Administrador inicial al arrancar**

---

## 1. Qué se hizo en este paso

Se creó la **capa de servicio** (`servicio`) con su primer caso de uso: la autenticación.

| Archivo | Cambio |
|---|---|
| `backend/internal/servicio/autenticacion.go` | **Nuevo.** `IniciarSesion` (valida credenciales y emite un token), `UsuarioActual` y `AsegurarAdministrador` (crea el primer administrador si no existe). |
| `backend/internal/servicio/autenticacion_test.go` | **Nuevo.** Pruebas con un repositorio **falso en memoria**: no necesitan base de datos. |

---

## 2. Por qué

### La capa de servicio

Es la capa del medio: recibe pedidos de la capa HTTP, aplica las reglas (permisos, validaciones, orden de los pasos) y usa los repositorios para leer y guardar. **No conoce HTTP ni SQL.**

```
handler (HTTP) ──► servicio (casos de uso) ──► repositorio (SQL)
                          │
                          └──► auth (bcrypt, JWT), dominio (reglas)
```

### Interfaces declaradas por quien las usa

El servicio no depende del repositorio concreto (`*repositorio.Usuarios`), sino de una **interfaz** que él mismo declara con **solo los métodos que necesita**:

```go
type RepositorioUsuarios interface {
	BuscarPorCorreo(ctx context.Context, correo string) (dominio.Usuario, error)
	BuscarPorID(ctx context.Context, id string) (dominio.Usuario, error)
	Crear(ctx context.Context, u dominio.Usuario) (dominio.Usuario, error)
}
```

Ventajas:
- **Pruebas sin base de datos:** en la prueba se pasa un repositorio falso que guarda los usuarios en un mapa. Las pruebas corren en milisegundos.
- **Bajo acoplamiento:** `*repositorio.Usuarios` cumple la interfaz automáticamente (tiene esos métodos), sin que el paquete `repositorio` sepa que el servicio existe.
- Es una convención de Go: *"acepta interfaces, devuelve structs"*, y las interfaces se declaran **del lado de quien las consume**.

---

## 3. Explicación detallada de `autenticacion.go`

```go
// Package servicio contiene la capa de lógica de negocio: casos de uso,
// reglas del dominio y control de acceso a la información. No conoce HTTP ni
// SQL; depende de interfaces que implementan las capas de datos e infraestructura.
package servicio
```

### Dependencias del servicio

```go
type EmisorTokens interface {
	Generar(u dominio.Usuario) (string, time.Time, error)
}

type Autenticacion struct {
	usuarios RepositorioUsuarios
	tokens   EmisorTokens
	log      *slog.Logger
	// hashFicticio se compara cuando el correo no existe, para que el tiempo de
	// respuesta no revele qué correos están registrados.
	hashFicticio string
}

type Sesion struct {
	Token    string
	ExpiraEn time.Time
	Usuario  dominio.Usuario
}
```
- `EmisorTokens` es otra interfaz mínima: el servicio solo necesita **generar** tokens; `*auth.GestorTokens` (paso 12) la cumple.
- `Sesion` es lo que devuelve un inicio de sesión exitoso.

```go
func NuevaAutenticacion(usuarios RepositorioUsuarios, tokens EmisorTokens, log *slog.Logger) *Autenticacion {
	hash, _ := auth.HashPassword("contraseña-ficticia-para-comparar")
	return &Autenticacion{usuarios: usuarios, tokens: tokens, log: log, hashFicticio: hash}
}
```
- Calcula **una vez**, al crear el servicio, el hash de una contraseña ficticia. Su uso se explica abajo.

### `IniciarSesion`

```go
func (s *Autenticacion) IniciarSesion(ctx context.Context, correo, password string) (Sesion, error) {
	correo = strings.TrimSpace(correo)
	if correo == "" || password == "" {
		return Sesion{}, apperr.Validacion("correo y contraseña son obligatorios")
	}
	u, err := s.usuarios.BuscarPorCorreo(ctx, correo)
	if err != nil && !apperr.Es(err, apperr.TipoNoEncontrado) {
		return Sesion{}, err
	}
	hash := s.hashFicticio
	if err == nil {
		hash = u.PasswordHash
	}
	if !auth.VerificarPassword(hash, password) || err != nil || !u.Activo {
		return Sesion{}, apperr.NoAutenticado("credenciales inválidas")
	}
	token, expira, err := s.tokens.Generar(u)
	if err != nil {
		return Sesion{}, apperr.Interno(err, "no fue posible iniciar sesión")
	}
	return Sesion{Token: token, ExpiraEn: expira, Usuario: u}, nil
}
```

Paso a paso:
1. Sin correo o sin contraseña → **400** (validación).
2. Busca el usuario. Si falla por algo distinto de "no existe" (por ejemplo, la base de datos cayó), devuelve ese error tal cual (500).
3. **Siempre calcula bcrypt**, exista o no el usuario: si no existe, compara contra `hashFicticio`.
4. Si la contraseña no coincide, el usuario no existe o está inactivo → **el mismo error 401** "credenciales inválidas".
5. Emite el token. Si eso fallara, es un error interno con un mensaje genérico.

**Dos detalles de seguridad:**
- **Mismo mensaje para todo.** Si el sistema dijera "ese correo no existe" frente a "contraseña incorrecta", un atacante podría averiguar qué correos están registrados (*enumeración de usuarios*).
- **Mismo tiempo de respuesta.** bcrypt tarda unos 70 ms. Si con un correo inexistente se respondiera de inmediato (sin calcular bcrypt), midiendo el tiempo se sabría igual qué correos existen (*ataque de tiempo*). Por eso se compara contra el hash ficticio.

### `UsuarioActual`

```go
func (s *Autenticacion) UsuarioActual(ctx context.Context, actor auth.Actor) (dominio.Usuario, error) {
	return s.usuarios.BuscarPorID(ctx, actor.UsuarioID)
}
```
- Devuelve los datos completos del usuario autenticado (lo usará `GET /auth/yo` en el paso 17).

### `AsegurarAdministrador`

```go
func (s *Autenticacion) AsegurarAdministrador(ctx context.Context, nombre, correo, password string) error {
	if correo == "" || password == "" {
		s.log.Warn("ADMIN_CORREO o ADMIN_PASSWORD no están definidos: no se crea el administrador inicial")
		return nil
	}
	_, err := s.usuarios.BuscarPorCorreo(ctx, correo)
	if err == nil {
		return nil
	}
	if !apperr.Es(err, apperr.TipoNoEncontrado) {
		return err
	}
	hash, err := auth.HashPassword(password)
	...
	_, err = s.usuarios.Crear(ctx, dominio.Usuario{
		Nombre: nombre, Correo: correo, PasswordHash: hash, Activo: true,
		Roles: []dominio.Rol{dominio.RolAdministrador},
	})
	if apperr.Es(err, apperr.TipoConflicto) {
		return nil // otra instancia lo creó al mismo tiempo
	}
	...
	s.log.Info("administrador inicial creado", "correo", correo)
	return nil
}
```
- **El problema del huevo y la gallina:** para crear usuarios hace falta un administrador, pero ¿quién crea al primero? Al arrancar, la API crea uno con los datos de la configuración (paso 15).
- Es **idempotente**: si ya existe, no hace nada. Se puede llamar en cada arranque.
- Si dos instancias arrancan a la vez, ambas podrían intentar crearlo; la segunda recibirá un conflicto (correo único) y lo ignora.
- Si no hay configuración, solo advierte en el log (útil en pruebas).

---

## 4. Las pruebas (`autenticacion_test.go`)

```go
var logPrueba = slog.New(slog.NewTextHandler(io.Discard, nil))

type usuariosFalsos struct {
	porCorreo map[string]dominio.Usuario
	creados   []dominio.Usuario
}

func (u *usuariosFalsos) BuscarPorCorreo(_ context.Context, correo string) (dominio.Usuario, error) { ... }
func (u *usuariosFalsos) BuscarPorID(_ context.Context, id string) (dominio.Usuario, error) { ... }
func (u *usuariosFalsos) Crear(_ context.Context, usuario dominio.Usuario) (dominio.Usuario, error) { ... }
```
- `usuariosFalsos` es un **doble de prueba**: cumple `RepositorioUsuarios` con un mapa en memoria y anota lo que se creó.
- `_ context.Context`: el parámetro existe para cumplir la interfaz, pero no se usa; `_` lo indica.
- `logPrueba` descarta los logs. **Nota:** más adelante (paso 48) esta variable se trasladará, junto con otros dobles compartidos del paquete, a `reportes_test.go`.

| Prueba | Qué verifica |
|---|---|
| `TestIniciarSesion` | El correo se acepta con espacios y mayúsculas; se emite un token. Contraseña incorrecta, correo inexistente y usuario inactivo → 401; correo vacío → 400. |
| `TestAsegurarAdministrador` | La primera vez crea un administrador activo con la contraseña **hasheada**; la segunda no lo duplica; sin configuración no crea nada. |

---

## 5. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Interfaz en el consumidor | El servicio declara qué necesita; los repositorios la cumplen sin saberlo. |
| Doble de prueba (fake) | Implementación en memoria de una interfaz para probar sin infraestructura. |
| `_` como nombre de parámetro | Parámetro obligatorio por la firma, pero que no se usa. |
| Valor cero de un struct | `Sesion{}`: struct vacío que se devuelve junto con un error. |

---

## 6. Cómo comprobarlo

```bash
cd backend
go test ./internal/servicio/ -v -run 'TestIniciarSesion|TestAsegurarAdministrador'
```

`-run` filtra las pruebas por nombre (expresión regular).

---

## 7. Pasos para lograr esta versión desde el paso 13

1. Crear `backend/internal/servicio/autenticacion.go` con el comentario del paquete, las interfaces `RepositorioUsuarios` y `EmisorTokens`, los structs `Autenticacion` y `Sesion`, `NuevaAutenticacion`, `IniciarSesion`, `UsuarioActual` y `AsegurarAdministrador`.
2. Crear `backend/internal/servicio/autenticacion_test.go` con `logPrueba`, el doble `usuariosFalsos`, `nuevaAutenticacionPrueba` y las dos pruebas.
3. `make pruebas lint`
4. Commit: `git add . && git commit -m "Paso 14: servicio de autenticación"`

---

## 8. Preparación para las próximas entregas

- **`AsegurarAdministrador` tolera arranques simultáneos**, como los de varias instancias detrás de un balanceador (Entrega 3).
- **El servicio no sabe de HTTP.** La misma lógica de inicio de sesión serviría si en el futuro se expusiera por otro medio.

---

## 9. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-01.1 Inicio de sesión local | `IniciarSesion` valida credenciales y emite un JWT. |
| RF-02 / RF-15.2 Gestión por el administrador | Existe siempre un administrador inicial. |
| RNF-03 Seguridad básica | Mensaje y tiempo de respuesta iguales ante credenciales inválidas. |
