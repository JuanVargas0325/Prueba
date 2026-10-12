# Paso 3 · Conexión a PostgreSQL

> Rama `paso-a-paso` · paso 3 de 65 · anterior: **Paso 2 · Servidor HTTP con Gin** · siguiente: **Paso 4 · Migraciones y esquema**

---

## 1. Qué se hizo en este paso

La API se conecta a **PostgreSQL** al arrancar y `/salud` informa si la base de datos responde.

| Archivo | Cambio |
|---|---|
| `backend/go.mod`, `backend/go.sum` | Se agrega el controlador **pgx** (`github.com/jackc/pgx/v5 v5.11.0`). |
| `backend/internal/basedatos/basedatos.go` | **Nuevo.** Función `Conectar`: crea el *pool* de conexiones y reintenta mientras PostgreSQL arranca. |
| `backend/internal/config/config.go` | **Modificado.** Nuevo campo `BaseDatos` y función `urlBaseDatos()` que arma la URL de conexión. |
| `backend/internal/config/config_test.go` | **Modificado.** Pruebas de la URL y de la variable obligatoria. |
| `backend/cmd/api/main.go` | **Modificado.** Conecta a la base de datos y registra el comprobador `postgres` en `/salud`. |
| `.env.example` | **Modificado.** Nueva sección `PostgreSQL`. |

Resultado:

```bash
curl http://localhost:8080/salud
# {"estado":"ok","componentes":{"postgres":"ok"}}            → 200
# (si PostgreSQL se cae)
# {"estado":"degradado","componentes":{"postgres":"error"}}  → 503
```

---

## 2. Por qué

- **PostgreSQL** es la base de datos que exige el stack del curso. Guardará usuarios, períodos, cursos y proyectos, vinculaciones, tareas y reportes (RNF-04, persistencia).
- **pgx** es el controlador de PostgreSQL más usado en Go. Se eligió frente a `database/sql` con `lib/pq` porque:
  - habla el protocolo nativo de PostgreSQL (más rápido y con mejor manejo de tipos como `DATE`, `UUID` y arreglos);
  - trae `pgxpool`, un *pool* de conexiones listo para usar;
  - expone los códigos de error de PostgreSQL (`23505` = violación de unicidad), que se usarán en el paso 13 para traducir errores.
- **Se conecta al arrancar y no en cada petición:** abrir una conexión cuesta varios milisegundos. Un *pool* mantiene conexiones abiertas y las reutiliza.
- **Reintentos al arrancar:** con Docker Compose (paso 6), la API y PostgreSQL arrancan casi al mismo tiempo; PostgreSQL tarda unos segundos en aceptar conexiones. Sin reintentos, la API fallaría al primer intento.
- **`/salud` refleja el estado real:** si la base de datos no responde, la API no puede trabajar y debe decirlo (503).

---

## 3. Explicación detallada

### 3.1 La URL de conexión (`config.go`)

PostgreSQL se identifica con una URL:

```
postgres://usuario:clave@host:puerto/basededatos?sslmode=disable
```

Se puede dar completa en `DATABASE_URL` o, más cómodo, por partes con **las mismas variables que usa la imagen oficial de PostgreSQL** (`POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`). Así, en el paso 6 un único `.env` configura a la vez el contenedor de la base de datos y la API.

```go
func urlBaseDatos() string {
	if v := strings.TrimSpace(os.Getenv("DATABASE_URL")); v != "" {
		return v
	}
	usuario, clave, bd := os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD"), os.Getenv("POSTGRES_DB")
	if usuario == "" || bd == "" {
		return ""
	}
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(usuario, clave),
		Host:     texto("POSTGRES_HOST", "localhost") + ":" + texto("POSTGRES_PUERTO", "5432"),
		Path:     "/" + bd,
		RawQuery: "sslmode=" + texto("POSTGRES_SSLMODE", "disable"),
	}
	return u.String()
}
```

- **Prioridad:** si existe `DATABASE_URL`, se usa tal cual. Si no, se arma con las partes.
- `usuario, clave, bd := a, b, c` asigna tres variables en una línea.
- **Se usa `url.URL` y no concatenación de texto** porque la contraseña puede tener caracteres especiales. Con `cl@ve:con/símbolos`, una URL armada a mano quedaría `postgres://app:cl@ve:con/símbolos@...` y sería imposible saber dónde termina la contraseña. `url.UserPassword` la **codifica** (`cl%40ve%3Acon%2Fs%C3%ADmbolos`). La prueba `TestCargarConstruyeLaURLDesdeVariablesDePostgres` comprueba justamente eso.
- `sslmode=disable`: dentro de la red de Docker no se cifra la conexión. En la nube (Entrega 2) se podrá cambiar con `POSTGRES_SSLMODE=require` sin tocar el código.

En `Cargar()` la base de datos es **obligatoria**:

```go
	c.BaseDatos = urlBaseDatos()
	if c.BaseDatos == "" {
		errs = append(errs, errors.New("defina DATABASE_URL o POSTGRES_USER, POSTGRES_PASSWORD y POSTGRES_DB"))
	}
```

### 3.2 `internal/basedatos/basedatos.go`

```go
func Conectar(ctx context.Context, url string, log *slog.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("configuración de base de datos inválida: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creando pool de conexiones: %w", err)
	}
```

- `pgxpool.ParseConfig` interpreta la URL (y detecta errores de formato).
- `pgxpool.NewWithConfig` crea el **pool**: un conjunto de conexiones que se abren cuando hacen falta y se reutilizan entre peticiones. Por defecto admite tantas conexiones como núcleos de CPU (mínimo 4). Las peticiones concurrentes de Gin toman una conexión libre, la usan y la devuelven.
- Crear el pool **no** abre ninguna conexión todavía; por eso el siguiente bloque hace *ping*.

```go
	const intentos = 15
	for i := 1; ; i++ {
		ctxPing, cancelar := context.WithTimeout(ctx, 3*time.Second)
		err = pool.Ping(ctxPing)
		cancelar()
		if err == nil {
			return pool, nil
		}
		if i == intentos {
			pool.Close()
			return nil, fmt.Errorf("no fue posible conectarse a PostgreSQL: %w", err)
		}
		log.Warn("esperando a PostgreSQL", "intento", i, "error", err)
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
```

- `for i := 1; ; i++` es un ciclo **sin condición de salida en la cabecera**: termina con los `return` de adentro.
- Cada *ping* tiene 3 segundos como máximo. Si responde, se devuelve el pool listo.
- Hasta **15 intentos con 2 segundos de espera** (unos 30 a 75 segundos en total). Si no se logra, se cierra el pool y se devuelve el error; la API termina con código 1.
- El `select` espera 2 segundos (`time.After`) **o** a que se cancele el contexto (Ctrl+C). Así, si se detiene la API mientras espera a la base de datos, sale de inmediato en lugar de seguir reintentando.
- `const intentos = 15`: una constante local; documenta el número en lugar de dejar un "15" suelto.

### 3.3 `cmd/api/main.go`

```go
	// Capa de datos
	db, err := basedatos.Conectar(ctx, cfg.BaseDatos, log)
	if err != nil {
		return err
	}
	defer db.Close()
```

- La conexión se abre **después** de crear el contexto de señales, para que Ctrl+C pueda interrumpir los reintentos.
- `defer db.Close()` cierra todas las conexiones del pool al terminar `ejecutar`. Este es el motivo del patrón `main → ejecutar` del paso 0: si la lógica estuviera en `main` y se llamara a `os.Exit`, este `defer` no se ejecutaría.

```go
	router := handler.NuevoRouter(handler.Dependencias{
		Salud: map[string]handler.ComprobadorSalud{
			"postgres": db.Ping,
		},
	})
```

- `db.Ping` es un **método usado como valor**: tiene la forma `func(ctx context.Context) error`, que es exactamente un `ComprobadorSalud` (paso 2). El router no conoce pgx; solo recibe "una función que comprueba algo".

### 3.4 Pruebas de configuración

- `TestCargarConstruyeLaURLDesdeVariablesDePostgres`: arma la URL con una contraseña llena de símbolos y verifica host, puerto, base de datos y que la contraseña esté codificada.
- `TestCargarExigeBaseDeDatos`: sin variables de base de datos, el error debe mencionar `DATABASE_URL`.
- `TestCargarRechazaZonaHorariaInvalida`: ahora define `DATABASE_URL`, porque la base de datos pasó a ser obligatoria y la prueba quiere aislar el error de la zona.

`Conectar` **no** tiene prueba unitaria porque necesita un PostgreSQL real. Se prueba con las pruebas de integración del paso 58 y con Docker Compose desde el paso 6.

---

## 4. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Asignación múltiple | `a, b, c := x, y, z`. |
| Ciclo sin condición | `for i := 1; ; i++ { ... }` se repite hasta un `return` o `break`. |
| `const` local | Valor fijo con nombre dentro de una función. |
| `time.After(d)` | Canal que recibe un valor cuando pasa el tiempo `d`; útil en `select`. |
| Método como valor | `db.Ping` se puede pasar como función, "atado" a `db`. |
| `nil` en punteros | `return nil, err`: no hay pool que devolver. |

---

## 5. Cómo comprobarlo

Necesita un PostgreSQL. La forma más rápida es con Docker (en el paso 6 esto lo hará Docker Compose):

```bash
docker run -d --name pg -p 5432:5432 -e POSTGRES_USER=seguimiento -e POSTGRES_PASSWORD=clave -e POSTGRES_DB=seguimiento postgres:16-bookworm
```

Con Go instalado, desde `backend/`:

```bash
POSTGRES_USER=seguimiento POSTGRES_PASSWORD=clave POSTGRES_DB=seguimiento go run ./cmd/api
curl http://localhost:8080/salud          # {"estado":"ok","componentes":{"postgres":"ok"}}
docker stop pg
curl -i http://localhost:8080/salud       # 503 {"estado":"degradado",...}
```

En PowerShell, las variables se definen antes: `$env:POSTGRES_USER="seguimiento"`, etc.

Si arranca la API antes que PostgreSQL, verá en el log `"msg":"esperando a PostgreSQL"` hasta que la base de datos acepte conexiones.

Pruebas unitarias: `go test ./...`

---

## 6. Pasos para lograr esta versión desde el paso 2

1. Agregar pgx (desde `backend/`):
   ```bash
   go get github.com/jackc/pgx/v5@v5.11.0
   go mod tidy
   ```
2. Modificar `backend/internal/config/config.go`:
   - import `net/url`;
   - campo `BaseDatos string` en `Config`;
   - en `Cargar()`, asignar `c.BaseDatos = urlBaseDatos()` y agregar el error si está vacío;
   - nueva función `urlBaseDatos()` (sección 3.1).
3. Actualizar `backend/internal/config/config_test.go` con las tres pruebas descritas en 3.4.
4. Crear `backend/internal/basedatos/basedatos.go` con `Conectar` (sección 3.2).
5. Modificar `backend/cmd/api/main.go`: import `seguimiento/internal/basedatos`, llamar a `Conectar`, `defer db.Close()` y pasar `db.Ping` en `Dependencias.Salud`.
6. Agregar a `.env.example` la sección `PostgreSQL` (`POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_HOST`, `POSTGRES_PUERTO` y el ejemplo comentado de `DATABASE_URL`).
7. Probar como en la sección 5 y hacer commit: `git commit -am "Paso 3: conexión a PostgreSQL"` (más `git add` de los archivos nuevos).

---

## 7. Preparación para las próximas entregas

- **`DATABASE_URL` tiene prioridad.** En la Entrega 2 la base de datos será un servicio administrado (Cloud SQL). Bastará con definir `DATABASE_URL` con la dirección privada de esa instancia y, si se requiere, `sslmode=require`.
- **Los reintentos al arrancar** también ayudan en la nube: si la API se inicia antes de que la red privada o la base de datos estén listas, espera en lugar de fallar.
- **`/salud` con el estado de PostgreSQL** permitirá que el balanceador (Entrega 3) saque de servicio una instancia que perdió la conexión.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Stack: PostgreSQL | Conexión con pgx y pool de conexiones. |
| RNF-04 Persistencia | Base para guardar todos los datos del sistema. |
| Lineamientos 10 | Credenciales solo por variables de entorno, contraseña codificada en la URL. |
