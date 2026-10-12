package servicio

import (
	"context"
	"log/slog"
	"strings"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

// RepositorioVinculaciones es lo que la gestión de vinculaciones necesita de la capa de datos.
type RepositorioVinculaciones interface {
	Crear(ctx context.Context, v dominio.Vinculacion, validar dominio.ValidadorCarga) (dominio.Vinculacion, error)
	Rectificar(ctx context.Context, id string, rol dominio.Rol, horas int, administradorID, motivo string,
		validar dominio.ValidadorCarga) (dominio.Vinculacion, error)
	Obtener(ctx context.Context, id string) (dominio.Vinculacion, error)
	Listar(ctx context.Context, f dominio.FiltroVinculaciones) ([]dominio.Vinculacion, error)
	ListarRectificaciones(ctx context.Context, vinculacionID string) ([]dominio.Rectificacion, error)
}

// LectorUsuarios permite consultar usuarios.
type LectorUsuarios interface {
	BuscarPorID(ctx context.Context, id string) (dominio.Usuario, error)
	BuscarPorCorreo(ctx context.Context, correo string) (dominio.Usuario, error)
}

// LectorEspacios permite consultar cursos y proyectos.
type LectorEspacios interface {
	Obtener(ctx context.Context, id string) (dominio.Espacio, error)
}

// NuevaVinculacion son los datos para vincular a una persona (RF-04.1 a RF-04.3).
// La persona se identifica con UsuarioID o, si se desconoce, con su Correo.
type NuevaVinculacion struct {
	EspacioID      string
	UsuarioID      string
	Correo         string
	Rol            dominio.Rol
	HorasSemanales int
}

// SolicitudRectificacion son los cambios que el administrador aplica a una
// vinculación (RF-04.5); nil = sin cambio. El motivo es obligatorio.
type SolicitudRectificacion struct {
	Rol            *dominio.Rol
	HorasSemanales *int
	Motivo         string
}

// Vinculaciones gestiona las vinculaciones y aplica las reglas de dedicación
// (RF-04, RF-05, RN-10 a RN-19).
type Vinculaciones struct {
	repo     RepositorioVinculaciones
	usuarios LectorUsuarios
	espacios LectorEspacios
	log      *slog.Logger
}

// NuevoVinculaciones crea el servicio de vinculaciones.
func NuevoVinculaciones(repo RepositorioVinculaciones, usuarios LectorUsuarios, espacios LectorEspacios, log *slog.Logger) *Vinculaciones {
	return &Vinculaciones{repo: repo, usuarios: usuarios, espacios: espacios, log: log}
}

// Crear vincula manualmente a una persona a un curso o proyecto del profesor
// (RN-10). Las reglas de horas se validan de forma bloqueante y atómica (RN-19).
func (s *Vinculaciones) Crear(ctx context.Context, actor auth.Actor, n NuevaVinculacion) (dominio.Vinculacion, error) {
	if err := exigirRol(actor, dominio.RolProfesor); err != nil {
		return dominio.Vinculacion{}, err
	}
	if !n.Rol.EsRolDeVinculacion() {
		return dominio.Vinculacion{}, apperr.Validacion("rol inválido: use monitor o asistente_graduado")
	}
	if err := dominio.ValidarHoras(n.HorasSemanales); err != nil {
		return dominio.Vinculacion{}, err
	}

	espacio, err := s.espacios.Obtener(ctx, n.EspacioID)
	if apperr.Es(err, apperr.TipoNoEncontrado) {
		return dominio.Vinculacion{}, apperr.Validacion("el curso o proyecto indicado no existe")
	}
	if err != nil {
		return dominio.Vinculacion{}, err
	}
	if espacio.ProfesorID != actor.UsuarioID {
		return dominio.Vinculacion{}, apperr.Prohibido("solo el profesor responsable puede vincular personas a este %s", espacio.Tipo)
	}
	if err := espacio.PermiteRegistros(); err != nil {
		return dominio.Vinculacion{}, err
	}

	persona, err := s.buscarPersona(ctx, n.UsuarioID, n.Correo)
	if err != nil {
		return dominio.Vinculacion{}, err
	}
	if err := verificarPersonaVinculable(persona, n.Rol); err != nil {
		return dominio.Vinculacion{}, err
	}

	objetivo := dominio.Vinculacion{
		UsuarioID: persona.ID, EspacioID: espacio.ID, Rol: n.Rol, HorasSemanales: n.HorasSemanales,
		EspacioEstado: espacio.Estado, EspacioFechaInicio: espacio.FechaInicio, EspacioFechaCierre: espacio.FechaCierre,
	}
	v, err := s.repo.Crear(ctx, objetivo, validarCarga)
	if err == nil {
		s.log.Info("vinculación creada", "vinculacion_id", v.ID, "usuario_id", v.UsuarioID, "rol", v.Rol,
			"horas", v.HorasSemanales, "por", actor.UsuarioID)
	}
	return v, err
}

// Listar devuelve las vinculaciones visibles: el administrador ve todas; los
// demás, las de sus espacios como profesor y las propias (RF-10.2, RF-11.1).
func (s *Vinculaciones) Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroVinculaciones) ([]dominio.Vinculacion, error) {
	f.VisiblePara = visiblePara(actor)
	return s.repo.Listar(ctx, f)
}

// Obtener devuelve una vinculación si el actor puede verla.
func (s *Vinculaciones) Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.Vinculacion, error) {
	v, err := s.repo.Obtener(ctx, id)
	if err != nil {
		return v, err
	}
	if !puedeVerDeProfesor(actor, v.ProfesorID) && v.UsuarioID != actor.UsuarioID {
		return dominio.Vinculacion{}, noEncontrado("vinculación")
	}
	return v, nil
}

// Rectificar corrige el rol o las horas contratadas. Es la única forma de
// cambiar las horas después de creada la vinculación (RF-04.4, RF-04.5, RN-12)
// y queda registrada con su motivo.
func (s *Vinculaciones) Rectificar(ctx context.Context, actor auth.Actor, id string, r SolicitudRectificacion) (dominio.Vinculacion, error) {
	if err := exigirRol(actor, dominio.RolAdministrador); err != nil {
		return dominio.Vinculacion{}, err
	}
	motivo, err := dominio.Texto("el motivo", r.Motivo, true, dominio.LongitudMaxMotivo)
	if err != nil {
		return dominio.Vinculacion{}, err
	}
	actual, err := s.repo.Obtener(ctx, id)
	if err != nil {
		return actual, err
	}

	rol, horas := actual.Rol, actual.HorasSemanales
	if r.Rol != nil {
		rol = *r.Rol
	}
	if r.HorasSemanales != nil {
		horas = *r.HorasSemanales
	}
	if !rol.EsRolDeVinculacion() {
		return actual, apperr.Validacion("rol inválido: use monitor o asistente_graduado")
	}
	if err := dominio.ValidarHoras(horas); err != nil {
		return actual, err
	}
	if rol == actual.Rol && horas == actual.HorasSemanales {
		return actual, apperr.Validacion("la rectificación no cambia el rol ni las horas")
	}
	if rol != actual.Rol {
		persona, err := s.usuarios.BuscarPorID(ctx, actual.UsuarioID)
		if err != nil {
			return actual, err
		}
		if err := verificarPersonaVinculable(persona, rol); err != nil {
			return actual, err
		}
	}

	v, err := s.repo.Rectificar(ctx, id, rol, horas, actor.UsuarioID, motivo, validarCarga)
	if err == nil {
		s.log.Info("vinculación rectificada", "vinculacion_id", id, "rol", rol, "horas", horas, "por", actor.UsuarioID)
	}
	return v, err
}

// ListarRectificaciones devuelve el historial de correcciones de una vinculación.
func (s *Vinculaciones) ListarRectificaciones(ctx context.Context, actor auth.Actor, id string) ([]dominio.Rectificacion, error) {
	if _, err := s.Obtener(ctx, actor, id); err != nil {
		return nil, err
	}
	return s.repo.ListarRectificaciones(ctx, id)
}

func (s *Vinculaciones) buscarPersona(ctx context.Context, usuarioID, correo string) (dominio.Usuario, error) {
	var persona dominio.Usuario
	var err error
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

// verificarPersonaVinculable exige que la persona esté activa y que el
// administrador le haya asignado el rol con el que se la vincula (RF-02.2).
func verificarPersonaVinculable(persona dominio.Usuario, rol dominio.Rol) error {
	if !persona.Activo {
		return apperr.ReglaNegocio("la persona está inactiva")
	}
	if !persona.TieneRol(rol) {
		return apperr.ReglaNegocio("la persona no tiene asignado el rol %s; el administrador debe asignárselo primero", rol.Texto())
	}
	return nil
}

// validarCarga aplica las reglas de dedicación al conjunto de vinculaciones
// simultáneas más la que se crea o corrige. Una vinculación de un espacio
// cerrado no suma carga.
func validarCarga(simultaneas []dominio.Vinculacion, objetivo dominio.Vinculacion) error {
	if objetivo.EspacioEstado != dominio.EspacioActivo {
		return nil
	}
	return dominio.ValidarCargaHoraria(append(simultaneas, objetivo))
}
