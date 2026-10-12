# Paso 12 · Contraseñas (bcrypt) y tokens (JWT)

> Rama `paso-a-paso` · paso 12 de 65 · anterior: **Paso 11 · Dominio de usuarios** · siguiente: **Paso 13 · Repositorio de usuarios**

---

## 1. Qué se hizo en este paso

Se creó el paquete **`auth`** con las dos piezas técnicas de la autenticación local:

1. **Guardar y verificar contraseñas** de forma segura con **bcrypt**.
2. **Emitir y validar tokens JWT**, con los que el cliente demuestra quién es en cada petición.

Además define `Actor`: "quién está haciendo esta operación y con qué roles".

| Archivo | Cambio |
|---|---|
| `backend/go.mod`, `go.sum` | Nuevas dependencias: `github.com/golang-jwt/jwt/v5 v5.3.1` y `golang.org/x/crypto v0.58.0` (bcrypt). |
| `backend/internal/auth/auth.go` | **Nuevo.** `Actor`, `HashPassword`, `VerificarPassword` y `GestorTokens` (`Generar` / `Validar`). |
| `backend/internal/auth/auth_test.go` | **Nuevo.** Cinco pruebas: hash, token de ida y vuelta, token expirado, firma ajena y roles. |

---

## 2. Por qué

- **RF-01.1 y RNF-03:** el sistema debe tener **autenticación local** (usuario y contraseña propios, sin Google ni otros proveedores) y control de acceso por roles.
- **Nunca se guarda la contraseña.** Si alguien robara la base de datos, no debe poder leer las contraseñas. Se guarda un *hash*: un resultado del que no se puede volver a la contraseña original.
- **¿Por qué tokens y no sesiones en el servidor?** Con sesiones, el servidor guarda "este usuario inició sesión" en memoria o en una base de datos, y todas las instancias de la API tendrían que compartir esa información. Con un **JWT**, el propio token lleva la información firmada; cualquier instancia que conozca el secreto puede validarlo sin consultar nada. Eso facilita escalar a varias instancias (Entrega 3).

---

## 3. Explicación detallada

### 3.1 `Actor`

```go
type Actor struct {
	UsuarioID string
	Roles     []dominio.Rol
}

func (a Actor) TieneRol(rol dominio.Rol) bool { ... }
func (a Actor) TieneAlgunRol(roles ...dominio.Rol) bool { ... }
func (a Actor) EsAdministrador() bool { return a.TieneRol(dominio.RolAdministrador) }
```

- El `Actor` es lo que se obtiene de un token válido. Todos los servicios lo recibirán para decidir permisos (paso 18 en adelante).
- `TieneAlgunRol(dominio.RolProfesor, dominio.RolAdministrador)`: ¿tiene al menos uno de estos roles? Es variádica (acepta cualquier cantidad).
- `EsAdministrador` existe porque el administrador tiene **visibilidad global** (RN-05) y esa pregunta se hará muchas veces.

### 3.2 Contraseñas con bcrypt

```go
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("generando hash de contraseña: %w", err)
	}
	return string(hash), nil
}

func VerificarPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
```

Un *hash* es una función de una sola vía: de `Clave123*` se obtiene algo como `$2a$10$N9qo8uLOickgx2ZMRZoMye...`, pero del resultado no se puede obtener la contraseña. Para verificar, se vuelve a calcular y se compara.

**¿Por qué bcrypt y no un hash común como SHA-256?**
- **Sal (salt) automática:** bcrypt agrega un valor aleatorio distinto para cada contraseña, guardado dentro del propio resultado. Dos usuarios con la misma contraseña tienen hashes distintos, y no sirven las tablas precalculadas de hashes.
- **Es lento a propósito:** `DefaultCost` (10) significa 2¹⁰ rondas internas, unos 50–100 ms por cálculo. Para un usuario que inicia sesión es imperceptible; para un atacante que quiere probar millones de contraseñas es enorme.
- **El costo está en el hash** (`$2a$10$...`). Si en el futuro se sube, los hashes viejos se siguen verificando.

`[]byte(password)` convierte el texto a bytes, que es lo que pide bcrypt. En las pruebas de carga se verá el efecto de este costo: el inicio de sesión es la operación más intensiva en CPU de la API.

### 3.3 Tokens JWT

Un **JWT** (*JSON Web Token*) tiene tres partes separadas por puntos:

```
eyJhbGciOiJIUzI1NiIs...  .  eyJyb2xlcyI6WyJwcm9mZXNvciJdLCJzdWIiOiI2ZjFj...  .  kX3n8Q0...
      cabecera                             datos (claims)                              firma
```

- Las dos primeras partes son JSON codificado en **Base64**. **No están cifradas**: cualquiera puede leerlas. Por eso el token solo lleva el identificador del usuario y sus roles, nada secreto.
- La **firma** se calcula con un secreto que solo conoce el servidor. Si alguien cambia los datos (por ejemplo, se agrega el rol administrador), la firma deja de coincidir y el token se rechaza.

```go
const emisor = "seguimiento-api"

type claims struct {
	Roles []dominio.Rol `json:"roles"`
	jwt.RegisteredClaims
}
```

- Los *claims* son los datos del token. `jwt.RegisteredClaims` (embebido) trae los estándar: `sub` (sujeto, el ID del usuario), `iss` (emisor), `iat` (emitido en) y `exp` (expira en). Se agrega `roles`.

```go
type GestorTokens struct {
	secreto    []byte
	expiracion time.Duration
	ahora      func() time.Time
}

func NuevoGestorTokens(secreto string, expiracion time.Duration) *GestorTokens {
	return &GestorTokens{secreto: []byte(secreto), expiracion: expiracion, ahora: time.Now}
}
```

- `ahora` es una **función** guardada en un campo. En producción es `time.Now`; las pruebas la reemplazan por una que devuelve otra hora, para crear tokens ya vencidos sin tener que esperar. Es una técnica habitual para probar código que depende del tiempo.
- Campos en minúscula: nadie fuera del paquete puede leer el secreto.

```go
func (g *GestorTokens) Generar(u dominio.Usuario) (string, time.Time, error) {
	ahora := g.ahora()
	expira := ahora.Add(g.expiracion)
	c := claims{
		Roles: u.Roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			Issuer:    emisor,
			IssuedAt:  jwt.NewNumericDate(ahora),
			ExpiresAt: jwt.NewNumericDate(expira),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(g.secreto)
	...
	return token, expira, nil
}
```

- **HS256** = HMAC con SHA-256: firma simétrica con un secreto compartido. Basta porque la misma API emite y valida.
- Devuelve también la fecha de expiración, para que el cliente sepa cuándo debe volver a iniciar sesión.

```go
func (g *GestorTokens) Validar(token string) (Actor, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) {
		return g.secreto, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(emisor),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(g.ahora),
	)
	if err != nil || c.Subject == "" {
		return Actor{}, ErrTokenInvalido
	}
	return Actor{UsuarioID: c.Subject, Roles: c.Roles}, nil
}
```

Cada opción cierra un tipo de ataque o de error:
- `WithValidMethods(HS256)`: solo acepta tokens firmados con HS256. Evita el ataque clásico de enviar un token con `"alg":"none"` (sin firma) o con otro algoritmo.
- `WithIssuer`: el token debe haber sido emitido por esta API.
- `WithExpirationRequired`: un token **sin** fecha de expiración se rechaza.
- `WithTimeFunc(g.ahora)`: usa el mismo reloj que `Generar` (necesario para las pruebas).
- Ante cualquier problema se devuelve el mismo `ErrTokenInvalido`, sin detalles: no se le da pistas a un atacante.

```go
var ErrTokenInvalido = errors.New("token inválido o expirado")
```

- Un **error centinela**: una variable de error que otros paquetes pueden comparar con `errors.Is`.

### 3.4 Pruebas (`auth_test.go`)

| Prueba | Qué verifica |
|---|---|
| `TestHashYVerificacionDePassword` | La contraseña correcta se verifica y una incorrecta no. |
| `TestTokenIdaYVuelta` | Un token generado se valida y conserva el usuario y sus dos roles; la expiración está en el futuro. |
| `TestTokenExpiradoEsRechazado` | Se genera con el reloj una hora atrás (expiración de 1 minuto) y luego se valida con la hora real: debe rechazarse. |
| `TestTokenConOtraFirmaEsRechazado` | Un token firmado con otro secreto se rechaza. |
| `TestTieneAlgunRol` | La lógica de roles del `Actor`. |

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| `[]byte(s)` / `string(b)` | Conversión entre texto y bytes. |
| Función como campo | `ahora func() time.Time`: se puede reemplazar en pruebas. |
| Error centinela | `var ErrX = errors.New("...")`, comparable con `errors.Is`. |
| Struct embebido de una librería | `jwt.RegisteredClaims` dentro de `claims`. |
| Opciones funcionales | `jwt.WithIssuer(...)`, `jwt.WithValidMethods(...)`: configuración por argumentos opcionales. |

---

## 5. Cómo comprobarlo

```bash
cd backend
go test ./internal/auth/ -v
```

Puede pegar un token generado en <https://jwt.io> para ver su contenido (cabecera y claims), y comprobar que la firma solo es válida con el secreto correcto.

---

## 6. Pasos para lograr esta versión desde el paso 11

1. Agregar las dependencias (desde `backend/`):
   ```bash
   go get github.com/golang-jwt/jwt/v5@v5.3.1 golang.org/x/crypto@v0.58.0
   go mod tidy
   ```
2. Crear `backend/internal/auth/auth.go` con `Actor` y sus métodos, `HashPassword`, `VerificarPassword`, `ErrTokenInvalido`, `claims`, `GestorTokens`, `NuevoGestorTokens`, `Generar` y `Validar`.
3. Crear `backend/internal/auth/auth_test.go` con las cinco pruebas.
4. `make pruebas lint`
5. Commit: `git add . && git commit -m "Paso 12: contraseñas con bcrypt y tokens JWT"`

---

## 7. Preparación para las próximas entregas

- **JWT sin estado en el servidor:** varias instancias de la API (Entrega 3) o funciones serverless (Entrega 4) validan el mismo token si comparten `JWT_SECRETO` (paso 15). No hace falta una sesión compartida en Redis ni en la base de datos.
- **Limitación conocida (deuda técnica DT-04):** un token es válido hasta que expira; no hay cierre de sesión en el servidor. Si se necesitara, se puede agregar una lista de tokens revocados o tokens de corta duración con renovación.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-01.1 Inicio de sesión local | Base criptográfica: contraseñas y tokens. |
| RF-01.2 Control de acceso por rol | Los roles viajan firmados en el token y se exponen en `Actor`. |
| RNF-03 Autenticación local y seguridad básica | bcrypt con sal y costo; JWT con algoritmo, emisor y expiración obligatorios. |
