# Paso 16 · Middleware de autenticación y roles

> Rama `paso-a-paso` · paso 16 de 65 · anterior: **Paso 15 · Administrador inicial** · siguiente: **Paso 17 · Endpoints de inicio de sesión**

---

## 1. Qué se hizo en este paso

Se agregaron al paquete `handler` los **guardianes** de las rutas protegidas:

| Middleware / función | Qué hace |
|---|---|
| `Autenticar(tokens)` | Exige un token `Bearer` válido. Si falta o es inválido responde **401**; si es válido, guarda el `Actor` en la petición. |
| `RequiereRol(roles...)` | Deja pasar solo si el actor tiene **alguno** de los roles indicados. Si no, responde **403**. |
| `ActorDe(c)` | Recupera el actor autenticado dentro de un handler. |

| Archivo | Cambio |
|---|---|
| `backend/internal/handler/middleware.go` | **Modificado.** Se agregan `ValidadorTokens`, `Autenticar`, `RequiereRol` y `ActorDe`. |

En este paso todavía no hay rutas protegidas. En el paso 17 se usan por primera vez y se prueban.

---

## 2. Por qué

- **RF-01.2 Control de acceso por rol:** cada endpoint debe estar disponible solo para ciertos roles (por ejemplo, crear usuarios solo para el administrador; registrar tareas solo para monitores y asistentes).
- **Un middleware, no código repetido:** si cada handler verificara el token por su cuenta, bastaría con olvidarlo en uno para abrir un agujero de seguridad. Con un middleware aplicado a un **grupo de rutas**, todas quedan protegidas por defecto.
- **Dos niveles distintos:**
  - **401 No autenticado:** "no sé quién eres" (sin token, token vencido o falsificado);
  - **403 Prohibido:** "sé quién eres, pero tu rol no puede hacer esto".

---

## 3. Explicación detallada

### 3.1 La interfaz `ValidadorTokens`

```go
type ValidadorTokens interface {
	Validar(token string) (auth.Actor, error)
}
```
- El middleware solo necesita **validar** tokens. `*auth.GestorTokens` (paso 12) cumple esta interfaz. En las pruebas se puede usar el gestor real con un secreto de prueba.

### 3.2 `Autenticar`

```go
const claveActor = "actor"

func Autenticar(tokens ValidadorTokens) gin.HandlerFunc {
	return func(c *gin.Context) {
		cabecera := c.GetHeader("Authorization")
		token, ok := strings.CutPrefix(cabecera, "Bearer ")
		if !ok || strings.TrimSpace(token) == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, RespuestaError{Error: "no_autenticado", Mensaje: "se requiere un token de acceso"})
			return
		}
		actor, err := tokens.Validar(strings.TrimSpace(token))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, RespuestaError{Error: "no_autenticado", Mensaje: "token inválido o expirado"})
			return
		}
		c.Set(claveActor, actor)
		c.Next()
	}
}
```
- El cliente envía el token en la cabecera estándar: `Authorization: Bearer eyJhbGciOi...`. *Bearer* ("portador") significa "quien porta este token tiene acceso".
- `strings.CutPrefix(cabecera, "Bearer ")` quita el prefijo y devuelve `ok = false` si la cabecera no empieza así.
- `c.AbortWithStatusJSON(...)` responde de inmediato y **corta la cadena**: el handler de la ruta nunca se ejecuta.
- Si el token es válido, `c.Set(claveActor, actor)` guarda el actor en el **contexto de la petición** (un pequeño diccionario que vive mientras dura la petición) y `c.Next()` continúa.
- El formato de error es el mismo `RespuestaError` del paso 10.

### 3.3 `RequiereRol`

```go
func RequiereRol(roles ...dominio.Rol) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !ActorDe(c).TieneAlgunRol(roles...) {
			c.AbortWithStatusJSON(http.StatusForbidden, RespuestaError{Error: "prohibido", Mensaje: "no tiene permiso para esta operación"})
			return
		}
		c.Next()
	}
}
```
- Siempre se usa **después** de `Autenticar` (necesita el actor).
- Se aplicará por ruta o por grupo, por ejemplo:
  ```go
  r := g.Group("/usuarios", RequiereRol(dominio.RolAdministrador))   // todo el grupo
  r.POST("", RequiereRol(dominio.RolProfesor), h.Crear)               // solo una ruta
  ```

### 3.4 `ActorDe`

```go
func ActorDe(c *gin.Context) auth.Actor {
	if v, ok := c.Get(claveActor); ok {
		if actor, ok := v.(auth.Actor); ok {
			return actor
		}
	}
	return auth.Actor{}
}
```
- `c.Get` devuelve un valor de tipo `any` (puede ser cualquier cosa). `v.(auth.Actor)` es una **aserción de tipo**: "afirmo que `v` es un `auth.Actor`". La forma con dos resultados (`actor, ok := ...`) no entra en pánico si no lo es; devuelve `ok = false`.
- Si no hay actor (ruta pública), devuelve un actor vacío, sin roles. Así `RequiereRol` simplemente niega el acceso.

### 3.5 ¿Cómo se usan juntos? (vista previa del paso 17)

```go
api := r.Group("/api/v1")
api.POST("/auth/login", ...)                                    // pública
protegidas := api.Group("", Autenticar(d.Tokens))               // todo lo de aquí exige token
protegidas.GET("/usuarios", RequiereRol(dominio.RolAdministrador), ...)
```

```
petición ─► Recovery ─► RegistrarPeticiones ─► Autenticar ─► RequiereRol ─► handler
                                                  │ 401           │ 403
```

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Aserción de tipo | `x.(T)` obtiene el valor concreto de una interfaz; con `v, ok := x.(T)` no hay pánico si no coincide. |
| `strings.CutPrefix` | Quita un prefijo e indica si estaba presente. |
| Contexto de Gin | `c.Set` / `c.Get` guardan datos durante una petición. |
| Middleware que corta | `AbortWithStatusJSON` responde y detiene la cadena. |

---

## 5. Cómo comprobarlo

En este paso solo se puede compilar y pasar el lint:

```bash
make lint
```

Las pruebas que ejercitan estos middleware llegan en el paso 17 (`TestRutasProtegidasExigenToken`), cuando exista la primera ruta protegida.

---

## 6. Pasos para lograr esta versión desde el paso 15

1. En `backend/internal/handler/middleware.go` agregar:
   - imports `net/http`, `strings`, `seguimiento/internal/auth` y `seguimiento/internal/dominio`;
   - la constante `claveActor`;
   - la interfaz `ValidadorTokens`;
   - las funciones `Autenticar`, `RequiereRol` y `ActorDe` (sección 3).
2. `make lint`
3. Commit: `git add . && git commit -m "Paso 16: middleware de autenticación y roles"`

---

## 7. Preparación para las próximas entregas

- **La validación del token no consulta la base de datos**, así que no agrega carga por petición; escala con el número de instancias (Entrega 3).
- **El frontend de la Entrega 2** solo tendrá que guardar el token del inicio de sesión y enviarlo en la cabecera `Authorization` de cada petición.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-01.2 Control de acceso por rol | `RequiereRol` protege rutas por rol (403). |
| RNF-03 Autenticación local | `Autenticar` exige un JWT válido (401). |
| RN-03 a RN-05 Visibilidad | El `Actor` queda disponible para que los servicios filtren la información. |
