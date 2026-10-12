# Paso 11 · Dominio: usuarios, roles y validaciones

> Rama `paso-a-paso` · paso 11 de 65 · anterior: **Paso 10 · Router por módulos** · siguiente: **Paso 12 · Contraseñas (bcrypt) y tokens (JWT)**

---

## 1. Qué se hizo en este paso

Se creó el paquete **`dominio`**, el corazón del sistema: las entidades y las reglas de negocio **puras**, sin HTTP ni base de datos. Este paso agrega lo relativo a usuarios y las validaciones comunes.

| Archivo | Cambio |
|---|---|
| `backend/internal/dominio/usuario.go` | **Nuevo.** Tipo `Rol` con sus cuatro valores, entidad `Usuario` y filtro de consulta `FiltroUsuarios`. |
| `backend/internal/dominio/validacion.go` | **Nuevo.** Límites de longitud, paginación y validación de textos, correos y contraseñas. |
| `backend/internal/dominio/reglas_test.go` | **Nuevo.** Pruebas de correos, contraseñas y paginación. |

---

## 2. Por qué

### ¿Por qué una capa de dominio?

Las reglas de negocio (quién tiene qué rol, qué es un correo válido, cuántas horas puede tener un monitor) son lo más valioso del sistema y lo que más debe estar probado. Si viven mezcladas con SQL o con HTTP:
- para probarlas habría que levantar una base de datos o hacer peticiones HTTP;
- un cambio de tecnología (otra base de datos, una función serverless) obligaría a reescribirlas.

En `dominio` las reglas son funciones normales de Go que reciben datos y devuelven resultados o errores. Se prueban en milisegundos y no dependen de nada externo (solo de `apperr`).

### ¿Por qué empezar por usuarios?

Todo lo demás depende de ellos: los profesores crean cursos, los monitores registran tareas, el administrador gestiona a todos. Y el primer módulo funcional será el inicio de sesión (pasos 12 a 17).

---

## 3. Explicación detallada

### 3.1 `usuario.go`

```go
// Package dominio contiene las entidades, constantes y reglas puras del sistema.
// No depende de HTTP ni de la base de datos, por lo que sus reglas se prueban
// de forma aislada.
package dominio
```

#### Roles

```go
type Rol string

const (
	RolAdministrador     Rol = "administrador"
	RolProfesor          Rol = "profesor"
	RolMonitor           Rol = "monitor"
	RolAsistenteGraduado Rol = "asistente_graduado"
)
```

- Los cuatro roles del enunciado. Los valores en texto coinciden **exactamente** con el `CHECK` de la tabla `usuario_roles` (paso 4).

```go
var RolesValidos = []Rol{RolAdministrador, RolProfesor, RolMonitor, RolAsistenteGraduado}

var RolesOperativos = []Rol{RolMonitor, RolAsistenteGraduado}
```

- `RolesValidos`: el catálogo completo, para validar lo que llega del exterior.
- `RolesOperativos`: los roles de quienes **registran tareas** (monitores y asistentes graduados). Se usará para proteger las rutas de tareas (paso 37).
- Son `var` y no `const` porque Go no admite constantes de tipo slice.

```go
func (r Rol) EsValido() bool {
	for _, v := range RolesValidos {
		if r == v {
			return true
		}
	}
	return false
}
```

- Un **método sobre un tipo basado en string**: se usa como `dominio.Rol("rector").EsValido()` (devuelve `false`).

```go
func (r Rol) EsRolDeVinculacion() bool {
	return r == RolMonitor || r == RolAsistenteGraduado
}
```

- RN-02: solo se puede **vincular** a una persona a un curso o proyecto como monitor o como asistente graduado (paso 30). Un profesor o un administrador no se vinculan.

```go
func (r Rol) Texto() string {
	switch r {
	case RolAsistenteGraduado:
		return "asistente graduado"
	default:
		return string(r)
	}
}
```

- Versión legible para los mensajes de error ("no se puede quitar el rol asistente graduado...").
- `switch` en Go **no necesita `break`**: cada `case` termina solo. `default` cubre todo lo demás.
- `string(r)` convierte el `Rol` de vuelta a texto.

#### La entidad `Usuario`

```go
type Usuario struct {
	ID           string
	Nombre       string
	Correo       string
	PasswordHash string
	Activo       bool
	Roles        []Rol
	CreadoEn     time.Time
}
```

- Refleja la tabla `usuarios` más sus roles (tabla `usuario_roles`). Una persona puede tener **varios roles** (RF-01.3, RN-01): por ejemplo, ser monitor en un curso y asistente graduado en un proyecto.
- `PasswordHash`: nunca la contraseña, sino su *hash* (paso 12).
- Las entidades del dominio **no llevan etiquetas JSON**: cómo se ven en la API lo decide la capa HTTP (paso 17). Así, por ejemplo, el hash nunca puede terminar en una respuesta por accidente.

```go
func (u Usuario) TieneRol(rol Rol) bool { ... }
```

#### Filtro de consulta

```go
type FiltroUsuarios struct {
	Rol      Rol
	Busqueda string // coincide con nombre o correo
	Paginacion
}
```

- Lo que el administrador puede pedir al listar usuarios (paso 19).
- `Paginacion` aparece **sin nombre de campo**: es un **campo embebido**. Sus campos se "suben" al struct que lo contiene: se puede escribir `f.Limite` en lugar de `f.Paginacion.Limite`. Todos los filtros del sistema embeberán `Paginacion`.

### 3.2 `validacion.go`

```go
const (
	LongitudMaxNombre        = 150
	LongitudMaxObservaciones = 4000
	LongitudMinPassword      = 8
	LongitudMaxPassword      = 72 // límite de bcrypt
)
```

- Límites razonables para que nadie envíe un nombre de un millón de caracteres.
- **72 en la contraseña:** bcrypt (el algoritmo del paso 12) solo usa los primeros 72 bytes. Una contraseña más larga se truncaría en silencio; es mejor rechazarla y avisar.

#### Paginación

```go
const (
	LimitePorDefecto = 50
	LimiteMaximo     = 200
)

type Paginacion struct {
	Limite         int
	Desplazamiento int
}

var SinPaginar = Paginacion{Limite: -1}

func (p Paginacion) Normalizar() Paginacion {
	if p.Limite <= 0 {
		p.Limite = LimitePorDefecto
	}
	if p.Limite > LimiteMaximo {
		p.Limite = LimiteMaximo
	}
	if p.Desplazamiento < 0 {
		p.Desplazamiento = 0
	}
	return p
}
```

- Los listados de la API nunca devuelven "todo": como máximo 200 elementos por página (50 si no se indica). Así un listado grande no puede tumbar el servidor ni la red.
- `Desplazamiento` es cuántos elementos saltar (página 2 = desplazamiento 50).
- `Normalizar` tiene un receptor **por valor** `(p Paginacion)`: trabaja sobre una **copia** y devuelve la copia corregida, sin modificar el original.
- `SinPaginar` (límite −1) es una señal **interna** para pedir todos los resultados. La usarán los servicios que necesitan calcular con todos los datos (por ejemplo, el seguimiento semanal, paso 43). Las peticiones HTTP nunca pueden pedirlo: el handler solo acepta límites positivos.

#### Validar textos

```go
func Texto(campo, valor string, obligatorio bool, maximo int) (string, error) {
	valor = strings.TrimSpace(valor)
	if obligatorio && valor == "" {
		return "", apperr.Validacion("%s es obligatorio", campo)
	}
	if utf8.RuneCountInString(valor) > maximo {
		return "", apperr.Validacion("%s no puede superar %d caracteres", campo, maximo)
	}
	return valor, nil
}
```

- Limpia los espacios de los extremos y valida obligatoriedad y longitud. Devuelve el texto limpio, listo para guardar.
- `campo` es el nombre que verá el usuario: `dominio.Texto("el nombre", n.Nombre, true, LongitudMaxNombre)` produce "el nombre es obligatorio".
- **`utf8.RuneCountInString` y no `len`:** en Go, `len("ñ")` es **2** (cuenta bytes en UTF-8). Para contar caracteres reales (letras con tilde, eñes) se cuentan *runas*.

#### Validar correos

```go
func NormalizarCorreo(correo string) (string, error) {
	correo = strings.ToLower(strings.TrimSpace(correo))
	direccion, err := mail.ParseAddress(correo)
	if correo == "" || err != nil || direccion.Address != correo {
		return "", apperr.Validacion("correo inválido")
	}
	return correo, nil
}
```

- Lo pasa a minúsculas (`Ana@Uniandes.edu.co` y `ana@uniandes.edu.co` son el mismo usuario, igual que en el índice único del paso 4).
- `mail.ParseAddress` (librería estándar) valida el formato según el estándar de correo.
- `direccion.Address != correo` rechaza formatos que el estándar acepta pero que no son un correo "simple", como `Ana <ana@x.co>`.

#### Validar contraseñas

```go
func ValidarPassword(password string) error {
	n := len(password)
	if n < LongitudMinPassword || n > LongitudMaxPassword {
		return apperr.Validacion("la contraseña debe tener entre %d y %d caracteres", LongitudMinPassword, LongitudMaxPassword)
	}
	return nil
}
```

- Aquí sí se usa `len` (bytes), porque el límite de bcrypt es en **bytes**.

### 3.3 `reglas_test.go`

Este archivo reunirá las pruebas de **todas** las reglas del dominio. Por ahora:
- `TestNormalizarCorreoYPassword`: un correo con mayúsculas y espacios se normaliza; cuatro formatos inválidos se rechazan; una contraseña de 5 caracteres y una de 73 se rechazan.
- `TestPaginacion`: sin límite se usa 50; con 5000 se recorta a 200; un desplazamiento negativo pasa a 0.

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Métodos sobre tipos simples | `func (r Rol) EsValido() bool`: cualquier tipo definido puede tener métodos. |
| `switch` | Cada `case` termina solo (sin `break`); `default` es el caso por omisión. |
| Campo embebido | `Paginacion` dentro de `FiltroUsuarios`: sus campos se acceden directamente. |
| Receptor por valor | `(p Paginacion)` trabaja sobre una copia. |
| `len` vs runas | `len` cuenta bytes; `utf8.RuneCountInString` cuenta caracteres. |
| `var` de slice | Las "constantes" de tipo slice se declaran con `var`. |

---

## 5. Cómo comprobarlo

```bash
cd backend
go test ./internal/dominio/ -v
# === RUN   TestNormalizarCorreoYPassword
# --- PASS: TestNormalizarCorreoYPassword
# === RUN   TestPaginacion
# --- PASS: TestPaginacion
```

`-v` (*verbose*) muestra cada prueba. Desde la raíz también sirve `make pruebas`.

---

## 6. Pasos para lograr esta versión desde el paso 10

1. Crear `backend/internal/dominio/usuario.go` con `Rol`, sus constantes, `RolesValidos`, `RolesOperativos`, los métodos `EsValido`, `EsRolDeVinculacion` y `Texto`, la entidad `Usuario` con `TieneRol` y `FiltroUsuarios`.
2. Crear `backend/internal/dominio/validacion.go` con las constantes de longitud y paginación, `Paginacion` con `Normalizar`, `SinPaginar`, `Texto`, `NormalizarCorreo` y `ValidarPassword`.
3. Crear `backend/internal/dominio/reglas_test.go` con `TestNormalizarCorreoYPassword` y `TestPaginacion`.
4. `make pruebas lint`
5. Commit: `git add . && git commit -m "Paso 11: dominio de usuarios, roles y validaciones"`

---

## 7. Preparación para las próximas entregas

- **El dominio no depende de nada.** Si en la Entrega 4 alguna parte se mueve a una función serverless, estas reglas se reutilizan tal cual.
- **Paginación con máximo** desde el primer listado: protege a la API cuando crezcan los datos y durante las pruebas de carga.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-01.3 / RN-01 Múltiples roles | `Usuario.Roles` es una lista. |
| RF-02.1 Crear usuarios (validaciones) | Nombre, correo y contraseña se validan en el dominio. |
| RN-02 Roles por vinculación | `EsRolDeVinculacion` (se usará en el paso 32). |
| RNF-05 Mantenibilidad | Reglas puras, probadas sin infraestructura. |
