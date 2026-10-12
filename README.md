# Paso 32 · Servicio de vinculaciones

> Rama `paso-a-paso` · paso 32 de 65 · anterior: **Paso 31 · Repositorio de vinculaciones** · siguiente: **Paso 33 · Endpoints de vinculaciones**

---

## 1. Qué se hizo en este paso

Se creó el caso de uso de las vinculaciones: el **profesor** vincula personas a sus cursos y proyectos, todos consultan las que pueden ver y el **administrador** rectifica errores.

| Archivo | Cambio |
|---|---|
| `backend/internal/servicio/vinculaciones.go` | **Nuevo.** Servicio `Vinculaciones` con `Crear`, `Listar`, `Obtener`, `Rectificar` y `ListarRectificaciones`, más `buscarPersona`, `verificarPersonaVinculable` y `validarCarga`. |
| `backend/internal/dominio/validacion.go` | **Modificado.** Constante `LongitudMaxMotivo` (1000 caracteres). |
| `backend/internal/servicio/modulos_test.go` | **Modificado.** Repositorio falso de vinculaciones y cuatro pruebas. |

---

## 2. Por qué

- **RF-04.1 a RF-04.3 / RN-10:** la vinculación es **manual** y la hace el profesor responsable del espacio, indicando rol y horas.
- **RF-04.4 / RN-12:** las horas **no se modifican** después; solo el administrador **rectifica** (RF-04.5), con motivo.
- **RF-05 / RN-14 a RN-19:** las reglas de horas se aplican al crear y al rectificar, de forma atómica (paso 31).
- **RN-02:** solo se vincula como monitor o asistente graduado, y la persona debe tener ese rol asignado por el administrador.

---

## 3. Explicación detallada

### 3.1 Dependencias

```go
type RepositorioVinculaciones interface {
	Crear(ctx context.Context, v dominio.Vinculacion, validar dominio.ValidadorCarga) (dominio.Vinculacion, error)
	Rectificar(ctx context.Context, id string, rol dominio.Rol, horas int, administradorID, motivo string,
		validar dominio.ValidadorCarga) (dominio.Vinculacion, error)
	Obtener(ctx context.Context, id string) (dominio.Vinculacion, error)
	Listar(ctx context.Context, f dominio.FiltroVinculaciones) ([]dominio.Vinculacion, error)
	ListarRectificaciones(ctx context.Context, vinculacionID string) ([]dominio.Rectificacion, error)
}

type LectorUsuarios interface {
	BuscarPorID(ctx context.Context, id string) (dominio.Usuario, error)
	BuscarPorCorreo(ctx context.Context, correo string) (dominio.Usuario, error)
}

type LectorEspacios interface {
	Obtener(ctx context.Context, id string) (dominio.Espacio, error)
}
```
- El servicio necesita tres repositorios distintos; de cada uno pide solo lo que usa.

```go
type NuevaVinculacion struct {
	EspacioID      string
	UsuarioID      string
	Correo         string
	Rol            dominio.Rol
	HorasSemanales int
}
```
- La persona se identifica por `UsuarioID` **o** por `Correo`. Un profesor normalmente conoce el correo de su monitor, no su identificador interno.

### 3.2 `Crear`

```go
func (s *Vinculaciones) Crear(ctx context.Context, actor auth.Actor, n NuevaVinculacion) (dominio.Vinculacion, error) {
	if err := exigirRol(actor, dominio.RolProfesor); err != nil { ... }
	if !n.Rol.EsRolDeVinculacion() {
		return dominio.Vinculacion{}, apperr.Validacion("rol inválido: use monitor o asistente_graduado")
	}
	if err := dominio.ValidarHoras(n.HorasSemanales); err != nil { ... }

	espacio, err := s.espacios.Obtener(ctx, n.EspacioID)
	if apperr.Es(err, apperr.TipoNoEncontrado) {
		return dominio.Vinculacion{}, apperr.Validacion("el curso o proyecto indicado no existe")
	}
	...
	if espacio.ProfesorID != actor.UsuarioID {
		return dominio.Vinculacion{}, apperr.Prohibido("solo el profesor responsable puede vincular personas a este %s", espacio.Tipo)
	}
	if err := espacio.PermiteRegistros(); err != nil { ... }

	persona, err := s.buscarPersona(ctx, n.UsuarioID, n.Correo)
	...
	if err := verificarPersonaVinculable(persona, n.Rol); err != nil { ... }

	objetivo := dominio.Vinculacion{
		UsuarioID: persona.ID, EspacioID: espacio.ID, Rol: n.Rol, HorasSemanales: n.HorasSemanales,
		EspacioEstado: espacio.Estado, EspacioFechaInicio: espacio.FechaInicio, EspacioFechaCierre: espacio.FechaCierre,
	}
	v, err := s.repo.Crear(ctx, objetivo, validarCarga)
	...
}
```
Validaciones en orden, de la más barata a la más costosa:
1. Solo profesores (403).
2. Rol de vinculación (monitor o asistente) y horas positivas (400).
3. El espacio existe (si no, 400), es **del profesor** que pide (si no, 403) y **no está cerrado** (RN-09, 422).
4. La persona existe, está activa y **tiene el rol** (3.4).
5. Las reglas de horas, en la transacción con bloqueo (paso 31), usando `validarCarga` (3.5).

`objetivo` lleva el estado y las fechas del espacio porque el repositorio los necesita para buscar las vinculaciones **simultáneas**.

### 3.3 `buscarPersona`

```go
func (s *Vinculaciones) buscarPersona(ctx context.Context, usuarioID, correo string) (dominio.Usuario, error) {
	...
	switch {
	case usuarioID != "":
		persona, err = s.usuarios.BuscarPorID(ctx, usuarioID)
	case strings.TrimSpace(correo) != "":
		persona, err = s.usuarios.BuscarPorCorreo(ctx, correo)
	default:
		return persona, apperr.Validacion("indique usuario_id o correo de la persona")
	}
	if apperr.Es(err, apperr.TipoNoEncontrado) {
		return persona, apperr.Validacion("la persona indicada no está registrada")
	}
	return persona, err
}
```
- Prioriza el ID; si no viene, usa el correo. Una persona inexistente es un **dato inválido** de la petición (400).

### 3.4 `verificarPersonaVinculable`

```go
func verificarPersonaVinculable(persona dominio.Usuario, rol dominio.Rol) error {
	if !persona.Activo {
		return apperr.ReglaNegocio("la persona está inactiva")
	}
	if !persona.TieneRol(rol) {
		return apperr.ReglaNegocio("la persona no tiene asignado el rol %s; el administrador debe asignárselo primero", rol.Texto())
	}
	return nil
}
```
- **Los roles globales los asigna el administrador** (RF-02.2) y los roles por vinculación los usa el profesor (RN-02). Un profesor no puede convertir a alguien en asistente graduado por su cuenta: el administrador debe haberle dado ese rol primero. El mensaje dice exactamente qué hacer.

### 3.5 `validarCarga`: la regla que se envía al repositorio

```go
func validarCarga(simultaneas []dominio.Vinculacion, objetivo dominio.Vinculacion) error {
	if objetivo.EspacioEstado != dominio.EspacioActivo {
		return nil
	}
	return dominio.ValidarCargaHoraria(append(simultaneas, objetivo))
}
```
- Cumple el tipo `dominio.ValidadorCarga` (paso 30), así que se puede pasar como argumento: `s.repo.Crear(ctx, objetivo, validarCarga)`. El repositorio la ejecuta **con el usuario bloqueado**.
- Une las simultáneas con la nueva y aplica las reglas del dominio. Una vinculación de un espacio cerrado no suma carga (se puede rectificar, por ejemplo, un dato histórico).

### 3.6 Consultar y rectificar

```go
func (s *Vinculaciones) Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroVinculaciones) ([]dominio.Vinculacion, error) {
	f.VisiblePara = visiblePara(actor)
	return s.repo.Listar(ctx, f)
}

func (s *Vinculaciones) Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.Vinculacion, error) {
	v, err := s.repo.Obtener(ctx, id)
	...
	if !puedeVerDeProfesor(actor, v.ProfesorID) && v.UsuarioID != actor.UsuarioID {
		return dominio.Vinculacion{}, noEncontrado("vinculación")
	}
	return v, nil
}
```
- Ven una vinculación: su profesor, el administrador y **la persona vinculada** (RN-04: cada monitor ve sus propias vinculaciones).

```go
func (s *Vinculaciones) Rectificar(ctx context.Context, actor auth.Actor, id string, r SolicitudRectificacion) (dominio.Vinculacion, error) {
	if err := exigirRol(actor, dominio.RolAdministrador); err != nil { ... }
	motivo, err := dominio.Texto("el motivo", r.Motivo, true, dominio.LongitudMaxMotivo)
	...
	actual, err := s.repo.Obtener(ctx, id)
	...
	rol, horas := actual.Rol, actual.HorasSemanales
	if r.Rol != nil { rol = *r.Rol }
	if r.HorasSemanales != nil { horas = *r.HorasSemanales }
	... validar rol y horas ...
	if rol == actual.Rol && horas == actual.HorasSemanales {
		return actual, apperr.Validacion("la rectificación no cambia el rol ni las horas")
	}
	if rol != actual.Rol {
		... la persona debe tener el nuevo rol ...
	}
	v, err := s.repo.Rectificar(ctx, id, rol, horas, actor.UsuarioID, motivo, validarCarga)
	...
}
```
- Solo el administrador (RF-04.4: el profesor **no** puede cambiar horas).
- **El motivo es obligatorio**: queda en el historial. Su largo máximo es la constante `dominio.LongitudMaxMotivo` (1000), junto a los demás límites de texto en `validacion.go`: un número con nombre explica qué significa y se cambia en un solo lugar.
- Cambios opcionales con punteros (como en el paso 23). Una rectificación que no cambia nada se rechaza.
- `ListarRectificaciones` verifica primero que el actor pueda ver la vinculación.

---

## 4. Las pruebas

`repoVinculacionesFalso` **ejecuta el validador recibido** contra una lista configurable de `simultaneas`: así se prueba que el servicio entrega la regla correcta al repositorio.

| Prueba | Qué verifica |
|---|---|
| `TestCrearVinculacion` | Vincular por correo (sin distinguir mayúsculas) y que al repositorio le llegan las fechas del espacio. 9 subpruebas de error: solo profesores, profesor no responsable, espacio cerrado (RN-09), espacio inexistente, rol inválido, horas en cero, persona sin el rol, persona inexistente y sin persona. |
| `TestCrearVinculacionAplicaLasReglasDeHoras` | **El ejemplo del enunciado:** con 12 h de asistencia y 3 h de monitoría, otra monitoría de 3 h se rechaza (6 > 5); una de 2 h se acepta (5 ≤ 5). |
| `TestRectificarVinculacion` | Un profesor no rectifica (403); sin motivo (400); sin cambios (400); 13 h de monitoría (422); cambiar a un rol que la persona no tiene (422); una corrección válida llega al repositorio con su motivo. |
| `TestVisibilidadDeVinculaciones` | Los listados se filtran por el actor; otro profesor recibe 404; la persona vinculada sí ve su vinculación. |

---

## 5. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| Función como argumento | `s.repo.Crear(ctx, objetivo, validarCarga)`. |
| Identificar por ID o por correo | `switch` sin expresión para elegir la forma de búsqueda. |
| Orden de las validaciones | De lo barato (formato) a lo costoso (transacción con bloqueo). |

---

## 6. Cómo comprobarlo

```bash
cd backend
go test ./internal/servicio/ -v -run 'Vinculacion'
```

---

## 7. Pasos para lograr esta versión desde el paso 31

1. Crear `backend/internal/servicio/vinculaciones.go` con las interfaces (`RepositorioVinculaciones`, `LectorUsuarios`, `LectorEspacios`), `NuevaVinculacion`, `SolicitudRectificacion`, `Vinculaciones`, `NuevoVinculaciones`, los cinco métodos públicos y las funciones `buscarPersona`, `verificarPersonaVinculable` y `validarCarga`.
2. En `backend/internal/dominio/validacion.go`, agregar `LongitudMaxMotivo = 1000` a las constantes de longitud.
3. En `modulos_test.go`, después de espacios, la sección `// --- vinculaciones ---` con `repoVinculacionesFalso`, `entornoVinculaciones` y las cuatro pruebas.
4. `make pruebas lint`
5. Commit: `git add . && git commit -m "Paso 32: servicio de vinculaciones"`

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-04.1 a RF-04.3 / RN-10 Vinculación manual por el profesor | `Crear` (solo el responsable del espacio). |
| RF-04.4 / RN-12 Horas no modificables | No hay edición; solo `Rectificar`. |
| RF-04.5 Rectificación | `Rectificar` (administrador, con motivo e historial). |
| RF-05 / RN-14 a RN-19 Reglas de horas | `validarCarga` aplicada dentro de la transacción. |
| RN-02 Rol por vinculación | `verificarPersonaVinculable`. |
| RN-09 Espacio cerrado | `PermiteRegistros`. |
| RN-04 Visibilidad propia | La persona ve sus vinculaciones. |
