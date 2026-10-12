# Paso 9 · Errores de aplicación (`apperr`)

> Rama `paso-a-paso` · paso 9 de 65 · anterior: **Paso 8 · Integración continua** · siguiente: **Paso 10 · Router por módulos y middleware de registro**

---

## 1. Qué se hizo en este paso

Se creó el paquete `apperr` con los **tipos de error** que usarán todas las capas del sistema.

| Archivo | Cambio |
|---|---|
| `backend/internal/apperr/apperr.go` | **Nuevo.** Tipo `Error` con una categoría (`Tipo`) y un mensaje para el usuario, más funciones para crearlos y reconocerlos. |

Todavía nadie usa el paquete. Lo usarán el dominio (paso 11), los repositorios (paso 13), los servicios (paso 14) y el handler, que traduce cada tipo a un código HTTP (paso 17).

---

## 2. Por qué

La aplicación tendrá tres capas (sección 4): **presentación** (handler, HTTP), **servicio** (reglas y casos de uso) y **datos** (repositorios, SQL). Cuando algo sale mal en una capa interna, la capa HTTP tiene que decidir qué código responder:

- un correo inválido → **400** (el cliente envió datos incorrectos);
- una tarea que no existe → **404**;
- superar las 22 horas de asistencia graduada → **422** (los datos son válidos, pero violan una regla de negocio);
- un fallo de la base de datos → **500**.

Sin un vocabulario común, el servicio tendría que conocer los códigos HTTP (y entonces ya no sería independiente de HTTP), o el handler tendría que adivinar a partir del texto del error. `apperr` resuelve eso: las capas internas dicen **qué clase de problema** ocurrió y solo el handler decide el código HTTP.

---

## 3. Explicación detallada de `apperr.go`

### 3.1 Los tipos de error

```go
type Tipo string

const (
	TipoValidacion    Tipo = "validacion"     // 400
	TipoNoAutenticado Tipo = "no_autenticado" // 401
	TipoProhibido     Tipo = "prohibido"      // 403
	TipoNoEncontrado  Tipo = "no_encontrado"  // 404
	TipoConflicto     Tipo = "conflicto"      // 409
	TipoReglaNegocio  Tipo = "regla_negocio"  // 422
	TipoInterno       Tipo = "interno"        // 500
)
```

- `type Tipo string` crea un **tipo nuevo** basado en `string`. Aunque por dentro es texto, el compilador no deja mezclarlo con cualquier string por accidente.
- `const ( ... )` agrupa constantes. El texto de cada una es el que verá el cliente en el campo `"error"` del JSON.

| Tipo | Cuándo se usa | Ejemplo en este proyecto |
|---|---|---|
| `validacion` (400) | Los datos de la petición son incorrectos. | Fecha con formato inválido, correo mal escrito, título vacío. |
| `no_autenticado` (401) | No se sabe quién es el usuario. | Credenciales incorrectas, token vencido. |
| `prohibido` (403) | Se sabe quién es, pero no tiene permiso. | Un monitor intenta crear un usuario. |
| `no_encontrado` (404) | El recurso no existe **o el usuario no debe saber que existe**. | Un profesor consulta el reporte de otro profesor. |
| `conflicto` (409) | Choca con el estado actual. | Correo ya registrado, vinculación duplicada, lote ya en proceso. |
| `regla_negocio` (422) | Datos válidos que violan una regla del dominio. | Más de 22 h de asistencia, editar una tarea de una semana cerrada. |
| `interno` (500) | Fallo inesperado. | La base de datos no responde. |

### 3.2 La estructura `Error`

```go
type Error struct {
	Tipo    Tipo
	Mensaje string
	Causa   error
}

func (e *Error) Error() string {
	if e.Causa != nil {
		return fmt.Sprintf("%s: %s: %v", e.Tipo, e.Mensaje, e.Causa)
	}
	return fmt.Sprintf("%s: %s", e.Tipo, e.Mensaje)
}

func (e *Error) Unwrap() error { return e.Causa }
```

- **En Go, `error` es una interfaz** con un único método: `Error() string`. Cualquier tipo que tenga ese método **es** un error (sección 4). `*Error` lo tiene, así que se puede devolver donde se espera un `error`.
- `Mensaje` es un texto **apto para el usuario** ("la persona superaría el máximo de 22 horas...").
- `Causa` guarda el error técnico original (por ejemplo, el de la base de datos) para los logs, **sin mostrárselo al cliente**.
- `Unwrap()` permite que `errors.Is` y `errors.As` "miren adentro" y encuentren la causa.
- `%v` en `Sprintf` imprime un valor en su forma por defecto.

### 3.3 Funciones constructoras

```go
func nuevo(tipo Tipo, formato string, args ...any) *Error {
	return &Error{Tipo: tipo, Mensaje: fmt.Sprintf(formato, args...)}
}

func Validacion(formato string, args ...any) *Error {
	return nuevo(TipoValidacion, formato, args...)
}
// ... NoAutenticado, Prohibido, NoEncontrado, Conflicto, ReglaNegocio
```

- Se usan así: `apperr.Validacion("el campo %s es obligatorio", "titulo")`.
- `args ...any` es un parámetro **variádico**: acepta cualquier cantidad de argumentos de cualquier tipo (`any`), como `Printf`.

```go
func Interno(causa error, formato string, args ...any) *Error {
	e := nuevo(TipoInterno, formato, args...)
	e.Causa = causa
	return e
}
```

- Para errores inesperados: guarda la causa técnica y un mensaje genérico para el usuario.

### 3.4 Reconocer errores

```go
func TipoDe(err error) Tipo {
	var e *Error
	if errors.As(err, &e) {
		return e.Tipo
	}
	return TipoInterno
}

func Es(err error, tipo Tipo) bool {
	var e *Error
	return errors.As(err, &e) && e.Tipo == tipo
}
```

- `errors.As(err, &e)` busca, dentro de la cadena de errores envueltos, uno que sea de tipo `*Error`; si lo encuentra, lo copia en `e` y devuelve `true`. Funciona aunque el error haya sido envuelto con `fmt.Errorf("...: %w", err)` varias veces.
- `TipoDe` devuelve la categoría (y considera **interno** cualquier error que no sea de `apperr`, por ejemplo uno de la base de datos que no se tradujo).
- `Es` responde preguntas como `apperr.Es(err, apperr.TipoNoEncontrado)`. Lo usarán mucho las pruebas y algunos servicios.

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Tipo definido | `type Tipo string`: un tipo nuevo con `string` por debajo. |
| Bloque `const` | Varias constantes relacionadas en `const ( ... )`. |
| Interfaz | Conjunto de métodos. Un tipo la **implementa automáticamente** si tiene esos métodos; no hay que declararlo (`implements` no existe en Go). |
| `error` | Interfaz estándar: `type error interface { Error() string }`. |
| Receptor puntero | `func (e *Error) Error()`: el método pertenece a `*Error` (puntero). |
| Variádicos | `args ...any` recibe 0 o más argumentos; se reenvían con `args...`. |
| `any` | Alias de `interface{}`: cualquier tipo. |
| `errors.As` / `errors.Is` | Buscan un tipo o un valor concreto en una cadena de errores envueltos. |

### La arquitectura en tres capas que se está construyendo

```
handler      (presentación: HTTP, JSON, códigos de estado)   ← traduce apperr → HTTP
   │
servicio     (casos de uso, permisos, reglas)                ← crea errores apperr
   │
repositorio  (SQL, PostgreSQL)                               ← traduce errores de PostgreSQL → apperr
   │
dominio      (entidades y reglas puras, sin dependencias)    ← crea errores apperr
```

`apperr` no depende de ninguna capa; todas pueden usarlo.

---

## 5. Cómo comprobarlo

El paquete aún no tiene consumidores; se valida compilando y con el lint:

```bash
cd backend
go build ./...
go vet ./...
```

o `make lint` desde la raíz. Sus funciones se ejercitan en las pruebas de los pasos 11 en adelante.

---

## 6. Pasos para lograr esta versión desde el paso 8

1. Crear `backend/internal/apperr/apperr.go` con:
   - el tipo `Tipo` y las 7 constantes;
   - el struct `Error` y sus métodos `Error()` y `Unwrap()`;
   - la función privada `nuevo` y las constructoras `Validacion`, `NoAutenticado`, `Prohibido`, `NoEncontrado`, `Conflicto`, `ReglaNegocio` e `Interno`;
   - `TipoDe` y `Es`.
2. `make lint` (o `go vet ./...`).
3. Commit: `git add . && git commit -m "Paso 9: errores de aplicación"`

---

## 7. Preparación para las próximas entregas

- Si en entregas futuras aparece otra forma de exponer el sistema (por ejemplo, funciones serverless en la Entrega 4, o colas que reciben mensajes), los servicios seguirán devolviendo los mismos errores y solo esa nueva capa decidirá cómo presentarlos.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RNF-05 Mantenibilidad | Errores con un vocabulario común entre capas. |
| RNF-06 Consistencia | Las reglas de negocio violadas se distinguen de los datos inválidos (422 frente a 400). |
| RNF-03 Seguridad básica | El mensaje para el cliente está separado de la causa técnica, que no se expone. |
