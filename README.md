# Paso 18 · Servicio de gestión de usuarios

> Rama `paso-a-paso` · paso 18 de 65 · anterior: **Paso 17 · Endpoints de inicio de sesión** · siguiente: **Paso 19 · Endpoints de usuarios**

---

## 1. Qué se hizo en este paso

Se creó el caso de uso con el que el **administrador** gestiona usuarios: crear, listar, consultar y asignar roles.

| Archivo | Cambio |
|---|---|
| `backend/internal/servicio/usuarios.go` | **Nuevo.** Servicio `Usuarios` con `Crear`, `Listar`, `Obtener` y `AsignarRoles`. |
| `backend/internal/servicio/acceso.go` | **Nuevo.** Primeras reglas de acceso compartidas: `exigirRol` y `noEncontrado`. |
| `backend/internal/servicio/modulos_test.go` | **Nuevo.** Pruebas de los servicios de gestión (por ahora, usuarios) con un repositorio falso. |
| `backend/internal/servicio/autenticacion_test.go` | **Modificado.** Agrega los actores de prueba (`admin`, `profesor`, `monitor`). |

---

## 2. Por qué

- **RF-02.1, RF-02.2, RF-02.3 y RF-15.2:** el administrador crea usuarios, les asigna uno o varios roles y consulta a todos los usuarios del sistema.
- **Los permisos se verifican en el servicio**, no solo en el router. El router (paso 19) también los exigirá, pero si mañana se agrega otra forma de llamar al servicio, la regla sigue protegida. Es **defensa en profundidad**.
- **Evitar inconsistencias:** quitarle a alguien el rol de monitor mientras tiene monitorías activas dejaría vinculaciones huérfanas. El servicio lo impide.

---

## 3. Explicación detallada

### 3.1 `acceso.go`

```go
// Reglas de visibilidad compartidas por los servicios (RF-12, RN-03 a RN-05).
// La capa de presentación ya filtra por rol; aquí se verifica la pertenencia
// de cada dato, para que la regla se cumpla aunque cambie la capa HTTP.

func exigirRol(actor auth.Actor, roles ...dominio.Rol) error {
	if !actor.TieneAlgunRol(roles...) {
		return apperr.Prohibido("no tiene permiso para esta operación")
	}
	return nil
}

func noEncontrado(recurso string) error {
	return apperr.NoEncontrado("%s no encontrado", recurso)
}
```
- `exigirRol`: 403 si el actor no tiene ninguno de los roles.
- `noEncontrado`: se usa **en lugar de 403** cuando el actor ni siquiera debe saber que el recurso existe. Si un monitor pide `/usuarios/{id}` de otra persona, recibe "usuario no encontrado", no "no tiene permiso" (que confirmaría que ese ID existe).
- Este archivo crecerá con las reglas de visibilidad por profesor (paso 28).

### 3.2 `usuarios.go`

#### La interfaz que necesita

```go
type RepositorioGestionUsuarios interface {
	BuscarPorID(ctx context.Context, id string) (dominio.Usuario, error)
	Crear(ctx context.Context, u dominio.Usuario) (dominio.Usuario, error)
	Listar(ctx context.Context, f dominio.FiltroUsuarios) ([]dominio.Usuario, error)
	ReemplazarRoles(ctx context.Context, usuarioID string, roles []dominio.Rol) error
	TieneVinculacionesActivas(ctx context.Context, usuarioID string, rol dominio.Rol) (bool, error)
	TieneEspaciosActivos(ctx context.Context, usuarioID string) (bool, error)
}
```
- Es **otra** interfaz sobre el mismo repositorio del paso 13, con otros métodos. Cada servicio pide solo lo que usa.

```go
type NuevoUsuario struct {
	Nombre   string
	Correo   string
	Password string
	Roles    []dominio.Rol
}
```
- Datos de entrada del caso de uso (todavía sin validar).

#### `Crear`

```go
func (s *Usuarios) Crear(ctx context.Context, actor auth.Actor, n NuevoUsuario) (dominio.Usuario, error) {
	if err := exigirRol(actor, dominio.RolAdministrador); err != nil {
		return dominio.Usuario{}, err
	}
	nombre, err := dominio.Texto("el nombre", n.Nombre, true, dominio.LongitudMaxNombre)
	...
	correo, err := dominio.NormalizarCorreo(n.Correo)
	...
	if err := dominio.ValidarPassword(n.Password); err != nil { ... }
	roles, err := normalizarRoles(n.Roles)
	...
	hash, err := auth.HashPassword(n.Password)
	if err != nil {
		return dominio.Usuario{}, apperr.Interno(err, "no fue posible crear el usuario")
	}
	u, err := s.repo.Crear(ctx, dominio.Usuario{Nombre: nombre, Correo: correo, PasswordHash: hash, Activo: true, Roles: roles})
	...
	s.log.Info("usuario creado", "usuario_id", u.ID, "roles", roles, "por", actor.UsuarioID)
	return u, nil
}
```
- **Orden:** primero el permiso, luego las validaciones del dominio (paso 11), después el hash y por último la base de datos. Las validaciones baratas van antes que lo costoso (bcrypt).
- Un correo repetido lo detecta el repositorio (409, paso 13).
- El log de auditoría registra **quién** creó a quién, nunca la contraseña.

#### `normalizarRoles`

```go
func normalizarRoles(roles []dominio.Rol) ([]dominio.Rol, error) {
	var resultado []dominio.Rol
	for _, r := range roles {
		if !r.EsValido() {
			return nil, apperr.Validacion("rol inválido: %q", r)
		}
		if !slices.Contains(resultado, r) {
			resultado = append(resultado, r)
		}
	}
	if len(resultado) == 0 {
		return nil, apperr.Validacion("el usuario debe tener al menos un rol")
	}
	slices.Sort(resultado)
	return resultado, nil
}
```
- Rechaza roles que no existen, **quita duplicados** (`["monitor","monitor"]` → `["monitor"]`), exige al menos uno y los **ordena** para que el resultado sea siempre igual.
- `slices` es un paquete estándar (Go 1.21+) con funciones genéricas para listas: `Contains`, `Sort`, `Index`...

#### `Listar` y `Obtener`

```go
func (s *Usuarios) Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroUsuarios) ([]dominio.Usuario, error) {
	if err := exigirRol(actor, dominio.RolAdministrador); err != nil { ... }
	if f.Rol != "" && !f.Rol.EsValido() {
		return nil, apperr.Validacion("rol inválido")
	}
	return s.repo.Listar(ctx, f)
}

func (s *Usuarios) Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.Usuario, error) {
	if !actor.EsAdministrador() && actor.UsuarioID != id {
		return dominio.Usuario{}, noEncontrado("usuario")
	}
	return s.repo.BuscarPorID(ctx, id)
}
```
- Listar es solo para el administrador (RF-02.3).
- Obtener: el administrador ve a cualquiera; cualquier otro usuario solo a sí mismo; a los demás les "no existen".

#### `AsignarRoles`

```go
func (s *Usuarios) AsignarRoles(ctx context.Context, actor auth.Actor, id string, roles []dominio.Rol) (dominio.Usuario, error) {
	if err := exigirRol(actor, dominio.RolAdministrador); err != nil { ... }
	roles, err := normalizarRoles(roles)
	...
	u, err := s.repo.BuscarPorID(ctx, id)
	...
	if id == actor.UsuarioID && !slices.Contains(roles, dominio.RolAdministrador) {
		return u, apperr.ReglaNegocio("no puede quitarse a sí mismo el rol de administrador")
	}
	for _, anterior := range u.Roles {
		if slices.Contains(roles, anterior) {
			continue
		}
		if err := s.verificarRetiroDeRol(ctx, id, anterior); err != nil {
			return u, err
		}
	}
	if err := s.repo.ReemplazarRoles(ctx, id, roles); err != nil { ... }
	...
	return s.repo.BuscarPorID(ctx, id)
}
```
- **Reemplaza** el conjunto completo de roles (RF-02.2).
- **Protección contra quedarse sin administrador:** el administrador no puede quitarse a sí mismo su rol; de lo contrario, el sistema podría quedar sin nadie que gestione usuarios.
- Para cada rol que se **retira**, `verificarRetiroDeRol` revisa si está en uso:

```go
func (s *Usuarios) verificarRetiroDeRol(ctx context.Context, usuarioID string, rol dominio.Rol) error {
	var enUso bool
	var err error
	switch {
	case rol.EsRolDeVinculacion():
		enUso, err = s.repo.TieneVinculacionesActivas(ctx, usuarioID, rol)
	case rol == dominio.RolProfesor:
		enUso, err = s.repo.TieneEspaciosActivos(ctx, usuarioID)
	}
	...
	if enUso {
		return apperr.ReglaNegocio("no se puede quitar el rol %s: la persona lo usa en cursos o proyectos activos", rol.Texto())
	}
	return nil
}
```
- Monitor o asistente: ¿tiene vinculaciones con ese rol en espacios activos? Profesor: ¿es responsable de algún espacio activo? Si es así → **422**.
- Al final se vuelve a leer el usuario para devolver sus roles actualizados.

---

## 4. Las pruebas (`modulos_test.go`)

```go
func esperarTipo(t *testing.T, err error, tipo apperr.Tipo) { ... }
```
- Auxiliar: falla si el error no es del tipo esperado. Hace las pruebas cortas y legibles.

```go
type repoUsuariosFalso struct {
	usuarios       map[string]dominio.Usuario
	rolesEnUso     map[dominio.Rol]bool
	espaciosActivo bool
}
```
- Repositorio en memoria. `rolesEnUso` y `espaciosActivo` permiten simular "este rol está en uso" sin base de datos.

| Prueba | Qué verifica |
|---|---|
| `TestCrearUsuario` | El nombre se limpia, el correo se pasa a minúsculas, los roles repetidos se quitan, la contraseña se guarda hasheada y el usuario queda activo. Seis casos inválidos: un profesor intentando crear (403), nombre vacío, correo inválido, contraseña corta, rol inexistente y sin roles (400). |
| `TestAsignarRoles` | El administrador no puede quitarse su rol (422); no se retira un rol en uso (422); un cambio válido se aplica; no se quita el rol de profesor con espacios activos (422); un monitor no ve a otro usuario (404). |

Los actores (`admin`, `profesor`, `monitor`) se declaran junto a `logPrueba` en `autenticacion_test.go`. En el paso 48 este bloque de dobles compartidos se trasladará a `reportes_test.go`.

---

## 5. Conceptos de Go de este paso

| Concepto | Explicación |
|---|---|
| Paquete `slices` | Funciones genéricas para listas: `slices.Contains`, `slices.Sort`. |
| Bloque `var ( ... )` | Varias variables relacionadas en un bloque. |
| Defensa en profundidad | La regla se aplica en el router **y** en el servicio. |
| 404 en lugar de 403 | No revelar la existencia de recursos ajenos. |

---

## 6. Cómo comprobarlo

```bash
cd backend
go test ./internal/servicio/ -v -run 'TestCrearUsuario|TestAsignarRoles'
```

---

## 7. Pasos para lograr esta versión desde el paso 17

1. Crear `backend/internal/servicio/acceso.go` con el comentario de las reglas de visibilidad, `exigirRol` y `noEncontrado`.
2. Crear `backend/internal/servicio/usuarios.go` con `RepositorioGestionUsuarios`, `NuevoUsuario`, `Usuarios`, `NuevoUsuarios`, `Crear`, `Listar`, `Obtener`, `AsignarRoles`, `verificarRetiroDeRol` y `normalizarRoles`.
3. En `autenticacion_test.go`, convertir `logPrueba` en un bloque `var (...)` con `profesor`, `admin` y `monitor`.
4. Crear `backend/internal/servicio/modulos_test.go` con `esperarTipo`, el doble `repoUsuariosFalso` y las pruebas `TestCrearUsuario` y `TestAsignarRoles`.
5. `make pruebas lint`
6. Commit: `git add . && git commit -m "Paso 18: servicio de gestión de usuarios"`

---

## 8. Preparación para las próximas entregas

- **Los logs de auditoría ("usuario creado ... por ...")** quedarán en Cloud Logging en la nube. Sirven para saber quién hizo cada cambio administrativo.
- **Pendientes que el enunciado deja fuera de la Entrega 1:** reinicio de contraseñas y desactivación de usuarios. El campo `activo` y el login que ya rechaza usuarios inactivos (paso 14) dejan lista la base para agregarlos.

---

## 9. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-02.1 Crear usuarios | `Crear` con validaciones y contraseña hasheada. |
| RF-02.2 Asignar roles | `AsignarRoles`, sin dejar inconsistencias. |
| RF-02.3 Consulta global | `Listar` (solo administrador). |
| RF-15.2 Gestión de usuarios por el administrador | Permisos exigidos en el servicio. |
| RN-04 Cada usuario ve solo lo suyo | `Obtener` devuelve 404 para usuarios ajenos. |
