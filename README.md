# Paso 23 · Servicio de períodos

> Rama `paso-a-paso` · paso 23 de 65 · anterior: **Paso 22 · Repositorio de períodos** · siguiente: **Paso 24 · Endpoints de períodos**

---

## 1. Qué se hizo en este paso

Se creó el caso de uso de los períodos académicos: el **administrador** los crea y los actualiza (cerrar o reabrir; abrir o cerrar la ventana de creación) y **cualquier usuario autenticado** puede consultarlos.

| Archivo | Cambio |
|---|---|
| `backend/internal/servicio/periodos.go` | **Nuevo.** Servicio `Periodos` con `Crear`, `Listar`, `Obtener` y `Actualizar`. |
| `backend/internal/servicio/modulos_test.go` | **Modificado.** Auxiliar `fechaP`, repositorio falso de períodos y `TestPeriodos`. |

---

## 2. Por qué

- **RF-03.2** exige que los períodos se gestionen de forma explícita (código, fechas, estado). Alguien tiene que crearlos: el administrador, que es quien gestiona la estructura académica.
- Los profesores necesitan **consultarlos** para elegir en qué período crean un curso (paso 28), por eso la lectura está abierta a todos los usuarios autenticados.
- **RN-07:** la ventana de creación se controla con `CreacionHabilitada`.

---

## 3. Explicación detallada de `periodos.go`

```go
type RepositorioPeriodos interface {
	Crear(ctx context.Context, p dominio.PeriodoAcademico) (dominio.PeriodoAcademico, error)
	Obtener(ctx context.Context, id string) (dominio.PeriodoAcademico, error)
	Listar(ctx context.Context, f dominio.FiltroPeriodos) ([]dominio.PeriodoAcademico, error)
	Actualizar(ctx context.Context, p dominio.PeriodoAcademico) (dominio.PeriodoAcademico, error)
}

type NuevoPeriodo struct {
	Codigo             string
	FechaInicio        time.Time
	FechaCierre        time.Time
	CreacionHabilitada bool
}

type CambiosPeriodo struct {
	Estado             *dominio.EstadoPeriodo
	CreacionHabilitada *bool
}
```
- `CambiosPeriodo` usa **punteros** para expresar "opcional": `nil` significa "no cambiar ese campo". Sin punteros no se podría distinguir entre "no envió `creacion_habilitada`" y "envió `false`", porque el valor cero de un `bool` es `false`. Es la forma habitual en Go de modelar actualizaciones parciales (como un `PATCH` HTTP).

### `Crear`

```go
func (s *Periodos) Crear(ctx context.Context, actor auth.Actor, n NuevoPeriodo) (dominio.PeriodoAcademico, error) {
	if err := exigirRol(actor, dominio.RolAdministrador); err != nil {
		return dominio.PeriodoAcademico{}, err
	}
	p := dominio.PeriodoAcademico{
		Codigo:             strings.TrimSpace(n.Codigo),
		FechaInicio:        n.FechaInicio,
		FechaCierre:        n.FechaCierre,
		Estado:             dominio.PeriodoActivo,
		CreacionHabilitada: n.CreacionHabilitada,
	}
	if err := dominio.ValidarCodigoPeriodo(p.Codigo); err != nil { ... }
	if err := p.ValidarFechas(); err != nil { ... }
	creado, err := s.repo.Crear(ctx, p)
	...
}
```
- Todo período nace **activo**. Las validaciones son las del dominio (paso 21).

### `Listar` y `Obtener`

```go
func (s *Periodos) Listar(ctx context.Context, _ auth.Actor, f dominio.FiltroPeriodos) ([]dominio.PeriodoAcademico, error) {
	if f.Estado != "" && !f.Estado.EsValido() {
		return nil, apperr.Validacion("estado inválido: use activo o cerrado")
	}
	return s.repo.Listar(ctx, f)
}
```
- No exigen rol: el actor se recibe por **uniformidad** con el resto de servicios (todos reciben el actor), pero no se usa (`_`).

### `Actualizar`

```go
func (s *Periodos) Actualizar(ctx context.Context, actor auth.Actor, id string, c CambiosPeriodo) (dominio.PeriodoAcademico, error) {
	if err := exigirRol(actor, dominio.RolAdministrador); err != nil { ... }
	if c.Estado == nil && c.CreacionHabilitada == nil {
		return dominio.PeriodoAcademico{}, apperr.Validacion("indique estado o creacion_habilitada")
	}
	p, err := s.repo.Obtener(ctx, id)
	...
	if c.Estado != nil {
		if !c.Estado.EsValido() { ... }
		p.Estado = *c.Estado
	}
	if c.CreacionHabilitada != nil {
		p.CreacionHabilitada = *c.CreacionHabilitada
	}
	if p.Estado == dominio.PeriodoCerrado {
		p.CreacionHabilitada = false
	}
	actualizado, err := s.repo.Actualizar(ctx, p)
	...
}
```
- Patrón **leer → modificar → guardar**: se obtiene el período, se aplican solo los cambios enviados y se guarda.
- `*c.Estado` **desreferencia** el puntero: obtiene el valor al que apunta.
- **Regla de consistencia:** un período **cerrado** no puede tener la ventana de creación abierta; al cerrarlo, se cierra también la ventana.

---

## 4. Las pruebas

```go
func fechaP(t *testing.T, texto string) time.Time { ... }
```
- Auxiliar para escribir fechas legibles (como `fecha` en el dominio, paso 20).

`repoPeriodosFalso` guarda los períodos en un mapa. `TestPeriodos` verifica:
- el administrador crea un período, que queda **activo**;
- un profesor no puede crear (403);
- código `2026` (sin `-NN`) → 400;
- cierre anterior al inicio → 400;
- **cerrar el período deshabilita la creación**;
- una actualización sin cambios → 400.

---

## 5. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Punteros como "opcional" | `*bool` distingue "no enviado" (`nil`) de `false`. |
| Desreferenciar | `*p` obtiene el valor apuntado por `p`. |
| Leer-modificar-guardar | Patrón para actualizaciones parciales. |

---

## 6. Cómo comprobarlo

```bash
cd backend
go test ./internal/servicio/ -v -run TestPeriodos
```

---

## 7. Pasos para lograr esta versión desde el paso 22

1. Crear `backend/internal/servicio/periodos.go` con `RepositorioPeriodos`, `NuevoPeriodo`, `CambiosPeriodo`, `Periodos`, `NuevoPeriodos`, `Crear`, `Listar`, `Obtener` y `Actualizar`.
2. En `modulos_test.go`: agregar `fechaP` (debajo de `esperarTipo`) y, después de la sección de usuarios, la sección `// --- períodos ---` con `repoPeriodosFalso` y `TestPeriodos`.
3. `make pruebas lint`
4. Commit: `git add . && git commit -m "Paso 23: servicio de períodos"`

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-03.2 Períodos académicos | Creación y consulta con código, fechas y estado. |
| RN-07 Ventana de creación | `CreacionHabilitada`, que se cierra junto con el período. |
| RF-15 Administración | Solo el administrador crea y modifica períodos. |
