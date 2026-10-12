# Paso 1 · Configuración por variables de entorno

> Rama `paso-a-paso` · paso 1 de 65 · anterior: **Paso 0 · Estructura inicial** · siguiente: **Paso 2 · Servidor HTTP con Gin y /salud**

---

## 1. Qué se hizo en este paso

Se creó el paquete `config`, el único lugar del programa que lee variables de entorno, y la API lo usa al arrancar.

| Archivo | Cambio |
|---|---|
| `backend/internal/config/config.go` | **Nuevo.** Define la estructura `Config` y la función `Cargar()`, que lee `APP_ENTORNO`, `APP_PUERTO` y `APP_ZONA_HORARIA`, aplica valores por defecto y valida. |
| `backend/internal/config/config_test.go` | **Nuevo.** Primeras pruebas unitarias del proyecto. |
| `backend/cmd/api/main.go` | **Modificado.** Llama a `config.Cargar()` y registra la configuración en el log. |

Ahora `go run ./cmd/api` imprime:

```json
{"time":"...","level":"INFO","msg":"configuración cargada","servicio":"api","entorno":"desarrollo","puerto":"8080","zona_horaria":"America/Bogota"}
```

---

## 2. Por qué

- **Un solo lugar para la configuración.** Si cada parte del código leyera `os.Getenv` por su cuenta, sería difícil saber qué variables existen, cuáles son obligatorias y qué valores por defecto tienen. Con un paquete `config`:
  - todas las variables se ven en un archivo;
  - se validan **al arrancar** (si falta algo, el programa se detiene con un mensaje claro en lugar de fallar más tarde en medio de una petición);
  - el resto del código recibe un `struct` ya validado y se puede probar sin tocar el entorno.
- **Sin secretos en el código** (lineamientos, sección 10): los valores reales vienen del `.env` de cada persona o de la plataforma donde se despliegue.
- **La zona horaria importa para las reglas de negocio:** las semanas del proyecto van de lunes a domingo (RF-08.1). Un registro hecho el domingo a las 11 p. m. en Bogotá ya es lunes en UTC. Por eso la zona es configurable y por defecto es `America/Bogota`.

---

## 3. Explicación detallada

### 3.1 `internal/config/config.go`

#### Por qué en `internal/`

`internal` es una carpeta con significado especial para el compilador de Go: los paquetes que están dentro solo pueden ser importados por código del mismo módulo (`seguimiento`). Así se deja claro que es código interno de esta aplicación y no una librería pública.

El paquete se llama `config` (igual que su carpeta, que es la convención) y se importa como `seguimiento/internal/config`, es decir, la ruta del módulo (`go.mod`) más la carpeta.

#### La estructura `Config`

```go
type Config struct {
	Entorno     string // desarrollo | produccion
	PuertoHTTP  string
	ZonaHoraria *time.Location
}
```

- Un `struct` agrupa datos con nombre, como una clase sin métodos de otros lenguajes.
- Los campos empiezan en mayúscula: se **exportan**, así que `main` puede leer `cfg.PuertoHTTP`.
- `*time.Location` es un puntero a la zona horaria ya interpretada (no el texto `"America/Bogota"`, sino el objeto que sabe calcular la hora local).
- `PuertoHTTP` es texto porque se usará para armar la dirección `":8080"`.

#### Un método

```go
func (c Config) EsDesarrollo() bool { return c.Entorno == "desarrollo" }
```

- `(c Config)` es el **receptor**: convierte la función en un método de `Config`. Se usa como `cfg.EsDesarrollo()`.
- Se usará en el paso 2 para decidir el modo del framework web.

#### `Cargar()`

```go
func Cargar() (Config, error) {
	var errs []error
	c := Config{
		Entorno:    texto("APP_ENTORNO", "desarrollo"),
		PuertoHTTP: texto("APP_PUERTO", "8080"),
	}
```

- Devuelve **dos valores**: la configuración y un error. En Go es la forma normal de informar errores.
- `var errs []error` declara una lista (slice) vacía de errores. La idea es **acumular todos los problemas** y reportarlos juntos, para que quien configura vea de una vez todo lo que falta en lugar de corregir un error, volver a arrancar y descubrir el siguiente.
- `Config{Campo: valor, ...}` crea el struct con los valores que se conocen ya.

```go
	zona, err := time.LoadLocation(texto("APP_ZONA_HORARIA", "America/Bogota"))
	if err != nil {
		errs = append(errs, fmt.Errorf("APP_ZONA_HORARIA: %w", err))
	}
	c.ZonaHoraria = zona
```

- `time.LoadLocation` busca la zona en la base de datos de zonas horarias. Falla si el nombre no existe (`Marte/Monte_Olimpo`).
- `fmt.Errorf("APP_ZONA_HORARIA: %w", err)` crea un error nuevo que **envuelve** al original (`%w`): el mensaje dice qué variable está mal y conserva la causa.
- `append` agrega un elemento a la lista y devuelve la lista nueva.

```go
	return c, errors.Join(errs...)
}
```

- `errors.Join` une varios errores en uno solo (un mensaje por línea). Si la lista está vacía, devuelve `nil`, es decir, "sin error".
- `errs...` "desempaca" la lista como argumentos separados.

#### La función auxiliar `texto`

```go
func texto(clave, porDefecto string) string {
	if v := strings.TrimSpace(os.Getenv(clave)); v != "" {
		return v
	}
	return porDefecto
}
```

- `os.Getenv` devuelve el valor de la variable de entorno, o `""` si no existe.
- `strings.TrimSpace` quita espacios al inicio y al final. Así, `APP_PUERTO= 8080 ` funciona igual que `APP_PUERTO=8080`.
- Si queda vacía, usa el valor por defecto.
- Empieza en minúscula: es **privada** del paquete `config`.
- `clave, porDefecto string`: cuando parámetros seguidos tienen el mismo tipo, basta escribirlo una vez.

### 3.2 `internal/config/config_test.go`

Las pruebas de Go viven junto al código, en archivos que terminan en `_test.go` (no se incluyen en el programa compilado).

```go
func TestCargarUsaValoresPorDefecto(t *testing.T) {
	t.Setenv("APP_ENTORNO", "")
	t.Setenv("APP_PUERTO", "")
	t.Setenv("APP_ZONA_HORARIA", "")

	c, err := Cargar()
	if err != nil {
		t.Fatal(err)
	}
	...
}
```

- Toda función `TestXxx(t *testing.T)` es una prueba; `go test` las encuentra y ejecuta automáticamente.
- `t.Setenv` cambia una variable de entorno **solo durante esa prueba** y la restaura al final. Así la prueba no depende de lo que haya en su computador.
- `t.Fatal` marca la prueba como fallida y la detiene; `t.Errorf` la marca como fallida pero sigue ejecutando para reportar más problemas.
- `%+v` en `t.Errorf` imprime un struct con los nombres de sus campos, útil para entender qué salió mal.

La segunda prueba comprueba que una zona inexistente produce un error que **nombra la variable** (`APP_ZONA_HORARIA`), que es lo que necesita quien configura el sistema.

### 3.3 `cmd/api/main.go`

```go
import (
	"log/slog"
	"os"
	_ "time/tzdata" // incluye las zonas horarias en el binario

	"seguimiento/internal/config"
)
```

- `_ "time/tzdata"` es un **import en blanco**: no se usa nada del paquete por nombre, pero al importarlo se incluye dentro del ejecutable la base de datos completa de zonas horarias. Sin esto, `time.LoadLocation("America/Bogota")` dependería de que el sistema operativo tenga esa información instalada, y la imagen mínima de Linux que se usará en el paso 5 podría no tenerla.
- Los imports se agrupan por convención: primero la librería estándar y, separados por una línea en blanco, los paquetes propios.

```go
func ejecutar(log *slog.Logger) error {
	cfg, err := config.Cargar()
	if err != nil {
		return err
	}
	log.Info("configuración cargada", "entorno", cfg.Entorno, "puerto", cfg.PuertoHTTP, "zona_horaria", cfg.ZonaHoraria.String())
	return nil
}
```

- Si la configuración es inválida, `ejecutar` devuelve el error; `main` lo registra y termina con código 1.
- `log.Info(mensaje, clave, valor, clave, valor...)` agrega campos al JSON del log.

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| `struct` | Tipo que agrupa campos con nombre. Se crea con `Config{Campo: valor}`. |
| Método | Función con receptor: `func (c Config) EsDesarrollo() bool`. |
| Varios valores de retorno | `func Cargar() (Config, error)`; se reciben con `cfg, err := config.Cargar()`. |
| Patrón `if err != nil` | Después de cada operación que puede fallar se revisa el error y, si existe, se devuelve hacia arriba. |
| Slice (`[]error`) | Lista de tamaño variable. `append` agrega elementos. |
| Envolver errores (`%w`) | `fmt.Errorf("contexto: %w", err)` añade contexto sin perder la causa original. |
| Import en blanco (`_`) | Importa un paquete solo por su efecto (aquí, incluir las zonas horarias). |
| Pruebas | Archivos `_test.go`, funciones `TestXxx(t *testing.T)`, ejecutadas con `go test`. |

---

## 5. Cómo comprobarlo

Desde `backend/` (o con Docker, como se explica en el paso 0):

```bash
go test ./...          # ejecuta las pruebas de todos los paquetes
go run ./cmd/api       # muestra la configuración cargada

APP_ZONA_HORARIA=Marte/Monte_Olimpo go run ./cmd/api
# {"level":"ERROR","msg":"la API terminó con error","error":"APP_ZONA_HORARIA: unknown time zone Marte/Monte_Olimpo"}
```

`./...` significa "este paquete y todos los que estén debajo".

Con Docker, para pasar una variable se usa `-e`:

```bash
docker run --rm -e APP_PUERTO=9000 -v "${PWD}/backend:/src" -w /src golang:1.26 go run ./cmd/api
```

---

## 6. Pasos para lograr esta versión desde el paso 0

1. Crear la carpeta del paquete: `mkdir -p backend/internal/config`
2. Crear `backend/internal/config/config.go` con:
   - el comentario de paquete y `package config`;
   - el struct `Config` con `Entorno`, `PuertoHTTP` y `ZonaHoraria`;
   - el método `EsDesarrollo()`;
   - la función `Cargar()` que acumula errores y usa `errors.Join`;
   - la función privada `texto(clave, porDefecto)`.
3. Crear `backend/internal/config/config_test.go` con las dos pruebas.
4. Modificar `backend/cmd/api/main.go`:
   - agregar los imports `_ "time/tzdata"` y `"seguimiento/internal/config"`;
   - en `ejecutar`, llamar a `config.Cargar()`, devolver el error si existe y registrar la configuración.
5. Desde `backend/`, ejecutar las pruebas y el programa:
   ```bash
   go test ./...
   go run ./cmd/api
   ```
6. Commit: `git add . && git commit -m "Paso 1: configuración por variables de entorno"`

---

## 7. Preparación para las próximas entregas

- Cuando la API corra en una máquina virtual (Entrega 2) o en Cloud Run (Entrega 4), bastará con definir las mismas variables en la plataforma. El código no cambia.
- Como `Cargar()` valida todo al arrancar, un despliegue mal configurado falla de inmediato con un mensaje claro. En la nube eso se ve como un contenedor que no inicia, mucho más fácil de diagnosticar que un error intermitente.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Lineamientos 10: configuración sin secretos en el código | Toda la configuración llega por variables de entorno. |
| RF-08.1 (semana de lunes a domingo) | Se prepara la zona horaria con la que se calcularán las semanas. |
| RNF-05 Mantenibilidad | Un único paquete responsable de la configuración, con pruebas. |
