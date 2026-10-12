# Paso 43 · Servicio de seguimiento

> Rama `paso-a-paso` · paso 43 de 65 · anterior: **Paso 42 · Consulta de tareas por vinculaciones** · siguiente: **Paso 44 · Endpoints de seguimiento**

---

## 1. Qué se hizo en este paso

Se creó el caso de uso que arma las **vistas de seguimiento** combinando vinculaciones, tareas y los cálculos del dominio (paso 41).

| Archivo | Cambio |
|---|---|
| `backend/internal/servicio/seguimiento.go` | **Nuevo.** Servicio `Seguimiento` con `PersonasACargo`, `SemanasPersona` y `Consolidado`, más `vinculacionesVisibles`, `rangoSemanas`, `tareasPorPersona` e `idsDe`. |
| `backend/internal/servicio/modulos_test.go` | **Modificado.** Listador falso de tareas y dos pruebas. |

---

## 2. Por qué

| Vista | Requisito | Quién la usa |
|---|---|---|
| **Personas a cargo:** cada monitor y asistente con sus vinculaciones y cómo va en la semana actual y la anterior | RF-10.1, RF-10.2 | El profesor (vista principal) y el administrador |
| **Semanas de una persona:** detalle semana a semana con tareas y estado (incluido "sin reporte") | RF-10.3, RF-10.6, RF-09, RF-11.3 | El profesor, el administrador y la propia persona (su historial) |
| **Consolidado de una persona:** acumulado por vinculación | RF-10.4 | El profesor y el administrador |

La visibilidad (RF-12, RN-03 a RN-05) se aplica eligiendo **qué vinculaciones** se consultan.

---

## 3. Explicación detallada

### 3.1 Dependencias mínimas

```go
type ListadorVinculaciones interface {
	Listar(ctx context.Context, f dominio.FiltroVinculaciones) ([]dominio.Vinculacion, error)
}

type ListadorTareasPorVinculacion interface {
	ListarPorVinculaciones(ctx context.Context, vinculacionIDs []string, desde, hasta *time.Time) ([]dominio.Tarea, error)
}

type BuscadorUsuarios interface {
	BuscarPorID(ctx context.Context, id string) (dominio.Usuario, error)
}
```
- Tres interfaces de **un solo método** cada una: el servicio solo lee.

### 3.2 `PersonasACargo`: la vista principal del profesor

```go
type ResumenPersona struct {
	dominio.PersonaACargo
	SemanaActual   *dominio.SemanaPersona
	SemanaAnterior *dominio.SemanaPersona
}

func (s *Seguimiento) PersonasACargo(ctx context.Context, actor auth.Actor, f FiltroPersonas) ([]ResumenPersona, error) {
	filtro := dominio.FiltroVinculaciones{EspacioID: f.EspacioID, Paginacion: dominio.SinPaginar}
	switch {
	case actor.EsAdministrador():
		filtro.ProfesorID = f.ProfesorID
	case actor.TieneRol(dominio.RolProfesor):
		filtro.ProfesorID = actor.UsuarioID // RN-03
	default:
		return nil, apperr.Prohibido("solo profesores y administradores consultan personas a cargo")
	}

	vinculaciones, err := s.vinculaciones.Listar(ctx, filtro)
	...
	actual := s.reloj.SemanaActual()
	anterior := actual.AddDate(0, 0, -7)
	tareas, err := s.tareas.ListarPorVinculaciones(ctx, idsDe(vinculaciones), &anterior, &actual)
	...
	personas := dominio.AgruparPorPersona(vinculaciones)
	tareasDe := tareasPorPersona(vinculaciones, tareas)
	resultado := make([]ResumenPersona, 0, len(personas))
	for _, p := range personas {
		r := ResumenPersona{PersonaACargo: p}
		for _, semana := range dominio.ConstruirSemanas(p.Vinculaciones, tareasDe[p.UsuarioID], anterior, actual, s.reloj.Ahora(), s.reloj.Zona()) {
			semana := semana
			if semana.SemanaInicio.Equal(actual) {
				r.SemanaActual = &semana
			} else {
				r.SemanaAnterior = &semana
			}
		}
		resultado = append(resultado, r)
	}
	return resultado, nil
}
```
- **Struct embebido:** `ResumenPersona` incluye `dominio.PersonaACargo` sin nombre de campo. Sus campos (`Nombre`, `Vinculaciones`...) se usan como propios: `r.Nombre`.
- **Visibilidad:** el profesor solo ve sus personas (RN-03; se **ignora** el `ProfesorID` que pida); el administrador puede filtrar por cualquier profesor; los demás roles → 403.
- **Dos consultas en total**, sin importar cuántas personas haya: las vinculaciones y las tareas de las dos últimas semanas (paso 42).
- `SinPaginar` (paso 11): aquí sí se necesitan todas las vinculaciones del profesor.
- Para cada persona se construyen solo dos semanas: la **actual** (qué lleva reportado) y la **anterior** (¿quedó sin reporte?). Es lo que un profesor necesita ver de un vistazo.
- `semana := semana` crea una **copia** de la variable del ciclo antes de tomar su dirección con `&semana`, para que cada puntero apunte a su propia semana. (Desde Go 1.22 cada vuelta ya tiene su propia variable, pero la copia explícita deja clara la intención.)
- Un puntero `nil` en `SemanaActual` o `SemanaAnterior` significa "no estaba vinculada esa semana".

**Cada persona recibe solo sus tareas:**

```go
func tareasPorPersona(vinculaciones []dominio.Vinculacion, tareas []dominio.Tarea) map[string][]dominio.Tarea {
	duenoDe := make(map[string]string, len(vinculaciones))
	for _, v := range vinculaciones {
		duenoDe[v.ID] = v.UsuarioID
	}
	resultado := map[string][]dominio.Tarea{}
	for _, t := range tareas {
		persona := duenoDe[t.VinculacionID]
		resultado[persona] = append(resultado[persona], t)
	}
	return resultado
}
```
- La consulta del paso 42 trae las tareas de **todas** las personas del profesor juntas. `ConstruirSemanas` (paso 41) agrupa las tareas que recibe; si se le pasaran todas a cada persona, cada una volvería a recorrer y agrupar las tareas de todos los demás para luego no usarlas.
- Con 40 personas y 3.000 tareas eso eran unas **120.000** operaciones por petición; repartiéndolas una vez son unas **3.000**. Las pruebas de carga lo confirmaron: la vista del profesor pasó de ~1.270 a ~2.560 peticiones por segundo con datos normales, y de 46 a 217 con 3.000 tareas extra (paso 64). Es la mejora principal de la deuda técnica DT-08.
- `duenoDe` es un mapa "vinculación → persona": permite saber de quién es cada tarea con una sola búsqueda.
- `make(map[string]string, len(vinculaciones))` reserva desde el inicio espacio para todas las entradas.
- `append` sobre una clave que aún no existe funciona: el mapa devuelve una lista vacía (`nil`) y `append` crea la lista.

### 3.3 `SemanasPersona`: el detalle y el historial

```go
func (s *Seguimiento) SemanasPersona(ctx context.Context, actor auth.Actor, personaID string, desde, hasta *time.Time) (DetallePersona, error) {
	inicio, fin, err := s.rangoSemanas(desde, hasta)
	...
	persona, vinculaciones, err := s.vinculacionesVisibles(ctx, actor, personaID)
	...
	tareas, err := s.tareas.ListarPorVinculaciones(ctx, idsDe(vinculaciones), &inicio, &fin)
	...
	return DetallePersona{
		Persona: persona,
		Semanas: dominio.ConstruirSemanas(vinculaciones, tareas, inicio, fin, s.reloj.Ahora(), s.reloj.Zona()),
	}, nil
}
```

```go
func (s *Seguimiento) rangoSemanas(desde, hasta *time.Time) (time.Time, time.Time, error) {
	actual := s.reloj.SemanaActual()
	fin := actual
	if hasta != nil && dominio.InicioSemana(*hasta).Before(actual) {
		fin = dominio.InicioSemana(*hasta)
	}
	inicio := fin.AddDate(0, 0, -7*7) // 8 semanas por defecto
	if desde != nil {
		inicio = dominio.InicioSemana(*desde)
	}
	if inicio.After(fin) { ... 400 ... }
	if fin.Sub(inicio) > time.Duration(dominio.MaxSemanasConsulta-1)*7*24*time.Hour { ... 400 ... }
	return inicio, fin, nil
}
```
- Por defecto, **las últimas 8 semanas** (la actual y 7 anteriores). Nunca más allá de la semana actual. Máximo 53 semanas por consulta.

### 3.4 `vinculacionesVisibles`: quién puede ver a quién

```go
func (s *Seguimiento) vinculacionesVisibles(ctx context.Context, actor auth.Actor, personaID string) (dominio.Usuario, []dominio.Vinculacion, error) {
	filtro := dominio.FiltroVinculaciones{UsuarioID: personaID, Paginacion: dominio.SinPaginar}
	propio := actor.UsuarioID == personaID
	switch {
	case propio || actor.EsAdministrador():
	case actor.TieneRol(dominio.RolProfesor):
		filtro.ProfesorID = actor.UsuarioID
	default:
		return dominio.Usuario{}, nil, noEncontrado("persona")
	}
	persona, err := s.usuarios.BuscarPorID(ctx, personaID)
	...
	vinculaciones, err := s.vinculaciones.Listar(ctx, filtro)
	...
	if len(vinculaciones) == 0 && !propio && !actor.EsAdministrador() {
		return dominio.Usuario{}, nil, noEncontrado("persona")
	}
	return persona, vinculaciones, nil
}
```
- **La propia persona** ve **todas** sus vinculaciones (RF-11.3: su historial).
- **El administrador** ve todo (RN-05).
- **Un profesor** ve a la persona **solo a través de sus propios espacios** (RN-03): si Laura es monitora de dos profesores, cada uno ve únicamente su parte. Si no tiene ninguna vinculación con él → 404.
- Un `case` vacío en un `switch` significa "no hacer nada en este caso".

### 3.5 `Consolidado`

- Las vinculaciones visibles + **todas** sus tareas (`nil, nil` = sin rango) → `dominio.ConstruirConsolidado` (paso 41).

---

## 4. Las pruebas

El reloj marca el miércoles 14/10/2026 (semana actual: 12/10). Laura tiene una monitoría (4 h) con el profesor `prof-1` desde agosto y una tarea de 2 h esta semana.

| Prueba | Qué verifica |
|---|---|
| `TestPersonasACargo` | Aunque el profesor pida `prof-2`, el filtro usa su propio ID (RN-03). Laura aparece con la semana actual (2 h reportadas) y la anterior **sin reporte**. Un monitor → 403. |
| `TestSemanasPersonaVisibilidadYRango` | Por defecto 8 semanas, del 24/08 al 12/10. La propia Laura ve su historial (RF-11.3). Otro profesor → 404 (RN-03). Un monitor ajeno → 404 (RN-04). Rango invertido → 400. Rango de más de 53 semanas → 400. El consolidado del administrador suma las 2 h. |

---

## 5. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Struct embebido | `ResumenPersona` incluye `dominio.PersonaACargo`; sus campos se promueven. |
| Copia de la variable del ciclo | `semana := semana` antes de tomar `&semana`. |
| `case` vacío | En un `switch`, un caso que solo detiene la evaluación. |
| `t.Sub(u)` | Diferencia entre dos instantes (`time.Duration`). |
| `make(map[K]V, n)` | Crea un mapa con espacio reservado para `n` entradas. |
| `append` sobre un mapa | `m[k] = append(m[k], v)` funciona aunque `k` no exista todavía. |

---

## 6. Cómo comprobarlo

```bash
cd backend
go test ./internal/servicio/ -v -run 'PersonasACargo|SemanasPersona'
```

---

## 7. Pasos para lograr esta versión desde el paso 42

1. Crear `backend/internal/servicio/seguimiento.go` con las tres interfaces, `FiltroPersonas`, `ResumenPersona`, `DetallePersona`, `ConsolidadoPersona`, `Seguimiento`, `NuevoSeguimiento`, los tres casos de uso y las funciones auxiliares.
2. Al final de `modulos_test.go`, la sección `// --- seguimiento ---` con `listadorTareasFalso`, `entornoSeguimiento` y las dos pruebas.
3. `make pruebas lint`
4. Commit: `git add . && git commit -m "Paso 43: servicio de seguimiento"`

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-10.1 / RF-10.2 Personas a cargo | `PersonasACargo`. |
| RF-10.3 / RF-10.6 Detalle semanal | `SemanasPersona`. |
| RF-10.4 Consolidado | `Consolidado`. |
| RF-11.3 Historial propio | `SemanasPersona` sobre el propio usuario. |
| RF-09 Sin reporte | Semanas cerradas sin tareas. |
| RF-12 / RN-03 a RN-05 Visibilidad | `vinculacionesVisibles` y filtro por profesor. |
