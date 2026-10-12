package servicio

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

// RepositorioTareas es lo que la gestión de tareas necesita de la capa de datos.
type RepositorioTareas interface {
	Crear(ctx context.Context, t dominio.Tarea, adjuntos []dominio.Adjunto) (dominio.Tarea, error)
	Obtener(ctx context.Context, id string) (dominio.Tarea, error)
	Listar(ctx context.Context, f dominio.FiltroTareas) ([]dominio.Tarea, error)
	Actualizar(ctx context.Context, t dominio.Tarea) (dominio.Tarea, error)
	Eliminar(ctx context.Context, id string) ([]string, error)
}

// LectorVinculaciones permite consultar una vinculación.
type LectorVinculaciones interface {
	Obtener(ctx context.Context, id string) (dominio.Vinculacion, error)
}

// NuevaTarea son los datos para registrar una tarea semanal (RF-06).
type NuevaTarea struct {
	VinculacionID string
	Semana        time.Time // cualquier fecha de la semana (se normaliza al lunes); cero = semana en curso
	Datos         dominio.DatosTarea
}

// Tareas gestiona el registro semanal de tareas (RF-06 a RF-08, RN-20 a RN-34).
// Los adjuntos se agregan en el paso 39.
type Tareas struct {
	repo          RepositorioTareas
	vinculaciones LectorVinculaciones
	reloj         Reloj
	log           *slog.Logger
}

// NuevoTareas crea el servicio de tareas.
func NuevoTareas(repo RepositorioTareas, vinculaciones LectorVinculaciones, reloj Reloj, log *slog.Logger) *Tareas {
	return &Tareas{repo: repo, vinculaciones: vinculaciones, reloj: reloj, log: log}
}

// Crear registra una tarea en una vinculación propia. Si la semana ya cerró,
// la tarea queda marcada como reporte tardío (RF-08.3, RF-08.4).
func (s *Tareas) Crear(ctx context.Context, actor auth.Actor, n NuevaTarea) (dominio.Tarea, error) {
	v, err := s.vinculaciones.Obtener(ctx, n.VinculacionID)
	if apperr.Es(err, apperr.TipoNoEncontrado) || (err == nil && v.UsuarioID != actor.UsuarioID) {
		// Solo se registran tareas en vinculaciones propias (RN-04, RN-21).
		return dominio.Tarea{}, apperr.Validacion("la vinculación indicada no existe o no le pertenece")
	}
	if err != nil {
		return dominio.Tarea{}, err
	}
	espacio := espacioDe(v)
	if err := espacio.PermiteRegistros(); err != nil {
		return dominio.Tarea{}, err
	}

	semana := s.reloj.SemanaActual()
	if !n.Semana.IsZero() {
		semana = dominio.InicioSemana(n.Semana)
	}
	if err := dominio.ValidarSemanaDeRegistro(semana, s.reloj.Ahora(), s.reloj.Zona()); err != nil {
		return dominio.Tarea{}, err
	}
	if !espacio.IncluyeSemana(semana) {
		return dominio.Tarea{}, apperr.Validacion("la semana del %s está fuera de las fechas del %s (%s a %s)",
			semana.Format(dominio.FormatoFecha), espacio.Tipo,
			espacio.FechaInicio.Format(dominio.FormatoFecha), espacio.FechaCierre.Format(dominio.FormatoFecha))
	}
	datos, err := n.Datos.Normalizar()
	if err != nil {
		return dominio.Tarea{}, err
	}

	t := dominio.Tarea{
		ID:            uuid.NewString(),
		VinculacionID: v.ID,
		SemanaInicio:  semana,
		Tardia:        dominio.SemanaCerrada(semana, s.reloj.Ahora(), s.reloj.Zona()),
	}
	t.Aplicar(datos)

	creada, err := s.repo.Crear(ctx, t, nil)
	if err != nil {
		return dominio.Tarea{}, err
	}
	s.log.Info("tarea registrada", "tarea_id", creada.ID, "vinculacion_id", v.ID,
		"semana", semana.Format(dominio.FormatoFecha), "tardia", creada.Tardia)
	return creada, nil
}

// Listar devuelve las tareas visibles: las propias, las de los espacios del
// profesor o todas para el administrador (RF-10.5, RF-11.2, RF-11.3, RF-12).
func (s *Tareas) Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroTareas) ([]dominio.Tarea, error) {
	f.VisiblePara = visiblePara(actor)
	return s.repo.Listar(ctx, f)
}

// Obtener devuelve una tarea si el actor puede verla.
func (s *Tareas) Obtener(ctx context.Context, actor auth.Actor, id string) (dominio.Tarea, error) {
	t, err := s.repo.Obtener(ctx, id)
	if err != nil {
		return t, err
	}
	if t.UsuarioID != actor.UsuarioID && !puedeVerDeProfesor(actor, t.ProfesorID) {
		return dominio.Tarea{}, noEncontrado("tarea")
	}
	return t, nil
}

// Actualizar modifica una tarea propia mientras su semana esté activa
// (RF-06.7, RF-07.2, RF-08.5). La semana y la vinculación no cambian (RN-26).
func (s *Tareas) Actualizar(ctx context.Context, actor auth.Actor, id string, datos dominio.DatosTarea) (dominio.Tarea, error) {
	t, err := s.obtenerModificable(ctx, actor, id)
	if err != nil {
		return t, err
	}
	datos, err = datos.Normalizar()
	if err != nil {
		return t, err
	}
	t.Aplicar(datos)
	actualizada, err := s.repo.Actualizar(ctx, t)
	if err == nil {
		s.log.Info("tarea actualizada", "tarea_id", id)
	}
	return actualizada, err
}

// Eliminar borra una tarea propia mientras su semana esté activa (RF-06.8).
func (s *Tareas) Eliminar(ctx context.Context, actor auth.Actor, id string) error {
	if _, err := s.obtenerModificable(ctx, actor, id); err != nil {
		return err
	}
	if _, err := s.repo.Eliminar(ctx, id); err != nil {
		return err
	}
	s.log.Info("tarea eliminada", "tarea_id", id)
	return nil
}

// Modificable indica si el actor puede editar o eliminar la tarea en este
// momento. Permite que la interfaz muestre u oculte esas acciones.
func (s *Tareas) Modificable(actor auth.Actor, t dominio.Tarea) bool {
	return s.verificarModificable(actor, t) == nil
}

// obtenerModificable obtiene la tarea y verifica que el actor pueda modificarla.
func (s *Tareas) obtenerModificable(ctx context.Context, actor auth.Actor, id string) (dominio.Tarea, error) {
	t, err := s.Obtener(ctx, actor, id)
	if err != nil {
		return t, err
	}
	return t, s.verificarModificable(actor, t)
}

// verificarModificable aplica las reglas para editar o eliminar: la tarea es
// propia, su semana sigue activa, no es tardía y el espacio no está cerrado.
func (s *Tareas) verificarModificable(actor auth.Actor, t dominio.Tarea) error {
	if t.UsuarioID != actor.UsuarioID {
		return apperr.Prohibido("solo la persona que registró la tarea puede modificarla")
	}
	if t.EspacioEstado == dominio.EspacioCerrado {
		return apperr.ReglaNegocio("el %s %q está cerrado y solo admite consultas", t.EspacioTipo, t.EspacioNombre)
	}
	return t.PuedeModificarse(s.reloj.Ahora(), s.reloj.Zona())
}

// espacioDe arma el espacio a partir de los datos incluidos en la vinculación.
func espacioDe(v dominio.Vinculacion) dominio.Espacio {
	return dominio.Espacio{
		ID: v.EspacioID, Nombre: v.EspacioNombre, Tipo: v.EspacioTipo, Estado: v.EspacioEstado,
		FechaInicio: v.EspacioFechaInicio, FechaCierre: v.EspacioFechaCierre, ProfesorID: v.ProfesorID,
	}
}
