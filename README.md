# Paso 58 · Pruebas de integración de la capa de datos

> Rama `paso-a-paso` · paso 58 de 65 · anterior: **Paso 57 · Modelo local con Ollama** · siguiente: **Paso 59 · Datos de demostración**

---

## 1. Qué se hizo en este paso

Se agregaron **pruebas de integración**: ejecutan los repositorios contra un **PostgreSQL real** y verifican las consultas SQL, las restricciones, las transacciones y la concurrencia. Se ejecutan en GitHub Actions y con `make`.

| Archivo | Cambio |
|---|---|
| `backend/internal/repositorio/integracion_test.go` | **Nuevo.** Siete pruebas contra PostgreSQL, compiladas solo con la etiqueta `integracion`. |
| `.github/workflows/ci.yml` | **Modificado.** El job de pruebas levanta un PostgreSQL como **servicio** y ejecuta también las de integración. |
| `Makefile` | **Modificado.** Objetivo `pruebas-integracion`. |

Con este paso, **todo `backend/` queda idéntico a la versión final**.

---

## 2. Por qué

- Las pruebas unitarias usan repositorios **falsos en memoria**: comprueban la lógica, pero no que el SQL sea correcto. Un error en una consulta, una columna mal escrita o un bloqueo que no funciona solo aparecen contra una base de datos real.
- Lo más delicado del sistema vive en SQL: la **concurrencia de las reglas de horas** (paso 31), la **idempotencia** de los reportes (paso 46) y las barreras de la base de datos (tareas tardías inmutables, semana en lunes).
- Esto saldaba la **deuda técnica DT-01** ("las consultas de `repositorio` no tenían pruebas automáticas contra una base de datos real").

---

## 3. Explicación detallada

### 3.1 Etiqueta de compilación

```go
//go:build integracion

// Pruebas de integración de la capa de datos contra un PostgreSQL real.
// Se ejecutan con:
//
//	DATABASE_URL_PRUEBAS=postgres://usuario:clave@localhost:5432/bd_pruebas?sslmode=disable \
//	  go test -tags integracion ./internal/repositorio/...
//
// ¡Borran todos los datos de esa base de datos! Use una base exclusiva para pruebas.
package repositorio_test
```
- `//go:build integracion` es una **etiqueta de compilación**: el archivo solo se compila con `go test -tags integracion`. Un `go test ./...` normal (rápido, sin base de datos) lo ignora.
- `package repositorio_test` (con `_test`): es un **paquete de prueba externo**. Solo puede usar lo **exportado** de `repositorio`, como lo haría cualquier otro paquete. Así se prueban los repositorios a través de su interfaz pública.
- La configuración `build-tags: integracion` de golangci-lint (paso 8) hace que el linter también lo revise.

### 3.2 Preparación

```go
func conectar(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL_PRUEBAS")
	if url == "" {
		t.Skip("DATABASE_URL_PRUEBAS no está definida")
	}
	...
	t.Cleanup(db.Close)
	if err := basedatos.Migrar(ctx, db, logPrueba); err != nil { ... }
	_, err = db.Exec(ctx, `TRUNCATE usuarios, usuario_roles, periodos_academicos, espacios, vinculaciones,
		vinculacion_rectificaciones, tareas, tarea_adjuntos, lotes_reportes, reportes_semanales CASCADE`)
	...
}
```
- Sin `DATABASE_URL_PRUEBAS`, la prueba se **omite** (`t.Skip`) en lugar de fallar.
- Se usa una variable **distinta** de `DATABASE_URL` a propósito: así es imposible correr las pruebas contra la base de datos de desarrollo por error, porque **`TRUNCATE` borra todo** para que cada prueba empiece limpia.
- `t.Cleanup(db.Close)` registra una limpieza que Go ejecuta al terminar la prueba.

```go
type escenario struct {
	profesor, laura, andres dominio.Usuario
	periodo                 dominio.PeriodoAcademico
	curso, proyecto         dominio.Espacio
}

func crearEscenario(t *testing.T, db *pgxpool.Pool) escenario { ... }
```
- Un **escenario** reutilizable: una profesora, Laura (monitora y asistente), Andrés (monitor), un período 2026-20, un curso y un proyecto. Se crea **con los repositorios reales**, que así también quedan probados.

### 3.3 Las pruebas

| Prueba | Qué verifica contra PostgreSQL |
|---|---|
| `TestMigracionesSobreBaseVacia` | Sobre un esquema vacío (`DROP SCHEMA public CASCADE`), migrar **dos veces**: 1 migración registrada y 11 tablas, sin errores la segunda vez (paso 4). |
| `TestUsuarios` | Búsqueda por correo sin distinguir mayúsculas y con sus 2 roles; correo repetido → conflicto; filtro por rol y búsqueda parcial (`ndr`); reemplazo de roles; ID inexistente → no encontrado. |
| `TestEspaciosVisibilidadYCierre` | La profesora ve 2 espacios, Laura solo el curso donde está vinculada y Andrés ninguno; vienen el código del período y el nombre de la profesora (`JOIN`); cerrar dos veces → conflicto. |
| `TestVinculacionesReglasYConcurrencia` | **Concurrencia:** dos goroutines vinculan a Laura como asistente con 12 h en dos espacios **a la vez** (24 h > 22 h): **exactamente una** falla con regla de negocio. Luego: duplicado → conflicto; rectificación 12 → 10 h con su historial; una rectificación a 23 h se rechaza y **no** cambia las horas. |
| `TestTareasYAdjuntos` | Tarea con adjunto (trae persona, profesor y conteo de adjuntos); `semana_inicio` en miércoles → validación (el `CHECK` del paso 4); visibilidad; actualización; máximo de adjuntos; eliminar devuelve la ruta del archivo; una tarea **tardía** no se actualiza ni se elimina. |
| `TestListarPorVinculaciones` | La consulta liviana del seguimiento (paso 42): con 3 tareas en 3 semanas, el rango 05/10–12/10 devuelve 2, de la más reciente a la más antigua y con sus datos propios (semana, estado, horas, título); sin rango devuelve las 3; sin vinculaciones, ninguna. Verifica contra PostgreSQL real que el SQL propio de esa consulta es correcto. |
| `TestCreacionDeLoteDeReportes` | 2 personas y 2 reportes; el lote cuenta como activo; repetir la creación **no duplica**; los datos de la semana de Laura (1 tarea, 3 h, nombres). |
| `TestFinalizacionDeReportes` | Un reporte completado (completarlo dos veces **no cuenta dos veces**) y otro fallido → lote `completado_con_errores` con contadores 1 y 1 y fecha de finalización. |

La prueba de concurrencia usa `sync.WaitGroup`: `wg.Add(1)` por cada goroutine, `defer wg.Done()` dentro de cada una y `wg.Wait()` para esperar a que terminen las dos.

### 3.4 En GitHub Actions

```yaml
  pruebas:
    name: Pruebas unitarias y de integración
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-bookworm
        env:
          POSTGRES_USER: pruebas
          POSTGRES_PASSWORD: pruebas
          POSTGRES_DB: pruebas
        ports:
          - 5432:5432
        options: >-
          --health-cmd "pg_isready -U pruebas -d pruebas"
          ...
    env:
      DATABASE_URL_PRUEBAS: postgres://pruebas:pruebas@localhost:5432/pruebas?sslmode=disable
    ...
      - name: Pruebas con cobertura
        run: go test -race -tags integracion -covermode=atomic -coverprofile=coverage.out ./...
```
- `services:` levanta un contenedor de PostgreSQL **al lado** del job, accesible en `localhost:5432`. Las `options` le dan un healthcheck: el job espera a que esté listo.
- Las credenciales son de un contenedor desechable que solo existe durante el job; no son secretos reales.
- Ahora el job ejecuta **unitarias e integración** juntas (`-tags integracion`).

### 3.5 Localmente con `make`

```makefile
pruebas-integracion: ## Pruebas unitarias y de integración contra un PostgreSQL temporal
	-docker network create red-pruebas >/dev/null 2>&1
	-docker rm -f pg-pruebas >/dev/null 2>&1
	docker run -d --name pg-pruebas --network red-pruebas -e POSTGRES_USER=pruebas ... postgres:16-bookworm
	sleep 5
	$(DOCKER_GO) --network red-pruebas -e DATABASE_URL_PRUEBAS=postgres://pruebas:pruebas@pg-pruebas:5432/pruebas?sslmode=disable \
		$(GO_IMG) go test -race -tags integracion -coverpkg=./internal/... -cover ./... ; status=$$?; docker rm -f pg-pruebas >/dev/null; exit $$status
```
- Crea una red y un PostgreSQL **temporal**, ejecuta las pruebas en un contenedor de Go en esa red y **siempre** borra el PostgreSQL al final, guardando el código de salida de las pruebas (`status=$$?` ... `exit $$status`) para que `make` informe si fallaron.
- El `-` al inicio de una línea le dice a `make` que ignore su error (por ejemplo, si la red ya existía).
- `-coverpkg=./internal/...` cuenta la cobertura **entre paquetes**: por ejemplo, las migraciones de `basedatos` que ejecutan las pruebas de `repositorio`.

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Prueba de integración | Prueba varios componentes reales juntos (aquí, código + PostgreSQL). |
| `//go:build etiqueta` | El archivo solo se compila si se pide esa etiqueta. |
| Paquete `_test` externo | Prueba solo la API pública del paquete. |
| `t.Skip` / `t.Cleanup` | Omitir una prueba / registrar una limpieza. |
| `sync.WaitGroup` | Esperar a que terminen varias goroutines. |
| Servicios en GitHub Actions | Contenedores auxiliares (bases de datos) para un job. |

---

## 5. Cómo comprobarlo

```bash
make pruebas-integracion
# ok  seguimiento/internal/repositorio  ...
```

O a mano, con un PostgreSQL de pruebas:

```bash
DATABASE_URL_PRUEBAS=postgres://pruebas:pruebas@localhost:5432/pruebas?sslmode=disable \
  go test -tags integracion -v ./internal/repositorio/
```

Resultado esperado: las 7 pruebas en `PASS`. En GitHub, el job se llama "Pruebas unitarias y de integración".

---

## 6. Pasos para lograr esta versión desde el paso 57

1. Crear `backend/internal/repositorio/integracion_test.go` (sección 3).
2. En `.github/workflows/ci.yml`: el servicio `postgres`, la variable `DATABASE_URL_PRUEBAS`, `-tags integracion` en el comando de pruebas, el nuevo nombre del job y el comentario inicial.
3. En el `Makefile`: el objetivo `pruebas-integracion` y agregarlo a `.PHONY`.
4. `make pruebas-integracion`
5. Commit: `git add . && git commit -m "Paso 58: pruebas de integración de la capa de datos"`

---

## 7. Preparación para las próximas entregas

- Antes de migrar a **Cloud SQL** (Entrega 2) se pueden ejecutar estas mismas pruebas contra una instancia de prueba en la nube: basta con apuntar `DATABASE_URL_PRUEBAS` a ella.
- Cualquier optimización SQL futura queda cubierta por estas pruebas. Por ejemplo, la consulta liviana del seguimiento (paso 42, deuda técnica DT-08) tiene su propia prueba: si alguien cambia sus columnas o su filtro, el pipeline lo detecta.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| 06-pruebas: pruebas automáticas en el pipeline | Integración con PostgreSQL en GitHub Actions. |
| RF-05.6 / RN-19 Reglas atómicas | Prueba de concurrencia real. |
| RNF-06 Consistencia | Restricciones, transacciones e idempotencia verificadas en la base de datos. |
| Deuda técnica DT-01 | Resuelta. |
