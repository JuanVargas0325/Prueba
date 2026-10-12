package servicio

import (
	"context"
	"fmt"
	"io"
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
	AgregarAdjuntos(ctx context.Context, tareaID string, adjuntos []dominio.Adjunto, maximo int) error
	ObtenerAdjunto(ctx context.Context, tareaID, adjuntoID string) (dominio.Adjunto, error)
	EliminarAdjunto(ctx context.Context, tareaID, adjuntoID string) (string, error)
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

// ArchivoAdjunto es un archivo recibido para adjuntar a una tarea.
type ArchivoAdjunto struct {
	Nombre    string
	Contenido io.Reader
}

// Tareas gestiona el registro semanal de tareas y sus adjuntos
// (RF-06 a RF-08, RN-20 a RN-34).
type Tareas struct {
	repo           RepositorioTareas
	vinculaciones  LectorVinculaciones
	almacenamiento Almacenamiento
	reloj          Reloj
	maxBytes       int64
	log            *slog.Logger
}

// NuevoTareas crea el servicio de tareas. maxBytesAdjunto limita el tamaño de cada archivo.
func NuevoTareas(repo RepositorioTareas, vinculaciones LectorVinculaciones, almacenamiento Almacenamiento,
	reloj Reloj, maxBytesAdjunto int64, log *slog.Logger) *Tareas {
	return &Tareas{repo: repo, vinculaciones: vinculaciones, almacenamiento: almacenamiento,
		reloj: reloj, maxBytes: maxBytesAdjunto, log: log}
}

// Crear registra una tarea en una vinculación propia. Si la semana ya cerró,
// la tarea queda marcada como reporte tardío (RF-08.3, RF-08.4). Los adjuntos
// pueden enviarse en la misma operación, lo que permite incluir evidencia
// también en los reportes tardíos, que no admiten cambios posteriores.
func (s *Tareas) Crear(ctx context.Context, actor auth.Actor, n NuevaTarea, archivos []ArchivoAdjunto) (dominio.Tarea, error) {
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
	if len(archivos) > dominio.MaxAdjuntosPorTarea {
		return dominio.Tarea{}, apperr.Validacion("una tarea admite máximo %d adjuntos", dominio.MaxAdjuntosPorTarea)
	}

	t := dominio.Tarea{
		ID:            uuid.NewString(),
		VinculacionID: v.ID,
		SemanaInicio:  semana,
		Tardia:        dominio.SemanaCerrada(semana, s.reloj.Ahora(), s.reloj.Zona()),
	}
	t.Aplicar(datos)

	adjuntos, err := s.guardarArchivos(ctx, t.ID, archivos)
	if err != nil {
		return dominio.Tarea{}, err
	}
	creada, err := s.repo.Crear(ctx, t, adjuntos)
	if err != nil {
		s.eliminarArchivos(ctx, adjuntos)
		return dominio.Tarea{}, err
	}
	s.log.Info("tarea registrada", "tarea_id", creada.ID, "vinculacion_id", v.ID,
		"semana", semana.Format(dominio.FormatoFecha), "tardia", creada.Tardia, "adjuntos", len(adjuntos))
	return creada, nil
}

// Listar devuelve las tareas visibles: las propias, las de los espacios del
// profesor o todas para el administrador (RF-10.5, RF-11.2, RF-11.3, RF-12).
func (s *Tareas) Listar(ctx context.Context, actor auth.Actor, f dominio.FiltroTareas) ([]dominio.Tarea, error) {
	f.VisiblePara = visiblePara(actor)
	return s.repo.Listar(ctx, f)
}

// Obtener devuelve una tarea con sus adjuntos si el actor puede verla.
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
	rutas, err := s.repo.Eliminar(ctx, id)
	if err != nil {
		return err
	}
	for _, ruta := range rutas {
		s.eliminarArchivo(ctx, ruta)
	}
	s.log.Info("tarea eliminada", "tarea_id", id)
	return nil
}

// AgregarAdjuntos añade archivos a una tarea propia modificable (RF-06.4).
func (s *Tareas) AgregarAdjuntos(ctx context.Context, actor auth.Actor, id string, archivos []ArchivoAdjunto) (dominio.Tarea, error) {
	if len(archivos) == 0 {
		return dominio.Tarea{}, apperr.Validacion("envíe al menos un archivo")
	}
	if _, err := s.obtenerModificable(ctx, actor, id); err != nil {
		return dominio.Tarea{}, err
	}
	adjuntos, err := s.guardarArchivos(ctx, id, archivos)
	if err != nil {
		return dominio.Tarea{}, err
	}
	if err := s.repo.AgregarAdjuntos(ctx, id, adjuntos, dominio.MaxAdjuntosPorTarea); err != nil {
		s.eliminarArchivos(ctx, adjuntos)
		return dominio.Tarea{}, err
	}
	return s.repo.Obtener(ctx, id)
}

// AbrirAdjunto devuelve el contenido de un adjunto si el actor puede ver la tarea.
// El llamador debe cerrar el lector.
func (s *Tareas) AbrirAdjunto(ctx context.Context, actor auth.Actor, tareaID, adjuntoID string) (io.ReadCloser, dominio.Adjunto, error) {
	if _, err := s.Obtener(ctx, actor, tareaID); err != nil {
		return nil, dominio.Adjunto{}, err
	}
	a, err := s.repo.ObtenerAdjunto(ctx, tareaID, adjuntoID)
	if err != nil {
		return nil, a, err
	}
	contenido, err := s.almacenamiento.Abrir(ctx, a.RutaAlmacenamiento)
	return contenido, a, err
}

// EliminarAdjunto quita un adjunto de una tarea propia modificable.
func (s *Tareas) EliminarAdjunto(ctx context.Context, actor auth.Actor, tareaID, adjuntoID string) error {
	if _, err := s.obtenerModificable(ctx, actor, tareaID); err != nil {
		return err
	}
	ruta, err := s.repo.EliminarAdjunto(ctx, tareaID, adjuntoID)
	if err != nil {
		return err
	}
	s.eliminarArchivo(ctx, ruta)
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

// guardarArchivos valida y guarda los archivos en el almacenamiento y devuelve
// sus metadatos. Si alguno falla, elimina los que ya se habían guardado.
func (s *Tareas) guardarArchivos(ctx context.Context, tareaID string, archivos []ArchivoAdjunto) ([]dominio.Adjunto, error) {
	adjuntos := make([]dominio.Adjunto, 0, len(archivos))
	for _, archivo := range archivos {
		nombre, extension, tipo, err := dominio.ValidarArchivoAdjunto(archivo.Nombre)
		if err != nil {
			s.eliminarArchivos(ctx, adjuntos)
			return nil, err
		}
		id := uuid.NewString()
		ruta := fmt.Sprintf("adjuntos/%s/%s%s", tareaID, id, extension)
		// Se lee un byte más del máximo para detectar archivos demasiado grandes.
		tamano, err := s.almacenamiento.Guardar(ctx, ruta, io.LimitReader(archivo.Contenido, s.maxBytes+1))
		if err == nil && tamano > s.maxBytes {
			s.eliminarArchivo(ctx, ruta)
			err = apperr.Validacion("el archivo %q supera el tamaño máximo de %d MB", nombre, s.maxBytes>>20)
		}
		if err == nil && tamano == 0 {
			s.eliminarArchivo(ctx, ruta)
			err = apperr.Validacion("el archivo %q está vacío", nombre)
		}
		if err != nil {
			s.eliminarArchivos(ctx, adjuntos)
			return nil, err
		}
		adjuntos = append(adjuntos, dominio.Adjunto{
			ID: id, TareaID: tareaID, NombreOriginal: nombre, RutaAlmacenamiento: ruta, TipoContenido: tipo, TamanoBytes: tamano,
		})
	}
	return adjuntos, nil
}

func (s *Tareas) eliminarArchivos(ctx context.Context, adjuntos []dominio.Adjunto) {
	for _, a := range adjuntos {
		s.eliminarArchivo(ctx, a.RutaAlmacenamiento)
	}
}

// eliminarArchivo borra un archivo; un fallo solo se registra, porque el dato
// en la base de datos ya es consistente y el archivo huérfano no afecta al usuario.
func (s *Tareas) eliminarArchivo(ctx context.Context, ruta string) {
	if err := s.almacenamiento.Eliminar(context.WithoutCancel(ctx), ruta); err != nil {
		s.log.Warn("no fue posible eliminar el archivo", "ruta", ruta, "error", err)
	}
}

// espacioDe arma el espacio a partir de los datos incluidos en la vinculación.
func espacioDe(v dominio.Vinculacion) dominio.Espacio {
	return dominio.Espacio{
		ID: v.EspacioID, Nombre: v.EspacioNombre, Tipo: v.EspacioTipo, Estado: v.EspacioEstado,
		FechaInicio: v.EspacioFechaInicio, FechaCierre: v.EspacioFechaCierre, ProfesorID: v.ProfesorID,
	}
}
