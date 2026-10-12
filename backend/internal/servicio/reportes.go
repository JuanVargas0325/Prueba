package servicio

import (
	"context"
	"io"
	"log/slog"
	"time"

	"seguimiento/internal/apperr"
	"seguimiento/internal/auth"
	"seguimiento/internal/dominio"
)

// RepositorioReportes es lo que el módulo de reportes necesita de la capa de datos.
type RepositorioReportes interface {
	CrearLote(ctx context.Context, profesorID string, semana time.Time) (dominio.LoteReportes, error)
	ExisteLoteActivo(ctx context.Context, profesorID string, semana time.Time) (bool, error)
	ObtenerLote(ctx context.Context, id string) (dominio.LoteReportes, error)
	ListarLotes(ctx context.Context, f dominio.FiltroLotes) ([]dominio.LoteReportes, error)
	IniciarLote(ctx context.Context, id string, total int) error
	MarcarLoteFallido(ctx context.Context, id, motivo string) error

	ListarPersonasParaReporte(ctx context.Context, profesorID string, inicio, fin time.Time) ([]string, error)
	CrearReportesPendientes(ctx context.Context, loteID, profesorID string, semana time.Time, usuarioIDs []string) ([]string, error)
	ObtenerReporte(ctx context.Context, id string) (dominio.ReporteSemanal, error)
	ListarReportes(ctx context.Context, f dominio.FiltroReportes) ([]dominio.ReporteSemanal, error)
	MarcarReporteEnProceso(ctx context.Context, id string) error
	CompletarReporte(ctx context.Context, id string, r dominio.ResultadoReporte) error
	FallarReporte(ctx context.Context, id, motivo string) error

	ObtenerDatosSemana(ctx context.Context, profesorID, usuarioID string, semana time.Time) (dominio.DatosSemanaPersona, error)
}

// Encolador publica trabajos en la cola de tareas asíncronas.
type Encolador interface {
	EncolarLote(ctx context.Context, loteID string) error
	EncolarReporte(ctx context.Context, reporteID string) error
}

// Almacenamiento guarda y recupera archivos binarios.
type Almacenamiento interface {
	Guardar(ctx context.Context, ruta string, contenido io.Reader) (int64, error)
	Abrir(ctx context.Context, ruta string) (io.ReadCloser, error)
	Eliminar(ctx context.Context, ruta string) error
}

// Reportes atiende las operaciones de la API sobre reportes semanales:
// solicitar la generación por lotes y consultar o descargar los resultados.
type Reportes struct {
	repo           RepositorioReportes
	encolador      Encolador
	almacenamiento Almacenamiento
	reloj          Reloj
	log            *slog.Logger
}

// NuevoReportes crea el servicio de reportes.
func NuevoReportes(repo RepositorioReportes, encolador Encolador, almacenamiento Almacenamiento, reloj Reloj, log *slog.Logger) *Reportes {
	return &Reportes{repo: repo, encolador: encolador, almacenamiento: almacenamiento, reloj: reloj, log: log}
}

// SolicitarLote registra un lote de generación para la semana que contiene la
// fecha indicada y lo envía a la cola. La generación ocurre en el worker
// (RF-14.1, RN-41): la API responde de inmediato sin esperar al modelo de IA.
func (s *Reportes) SolicitarLote(ctx context.Context, actor auth.Actor, fechaSemana string) (dominio.LoteReportes, error) {
	if !actor.TieneRol(dominio.RolProfesor) {
		return dominio.LoteReportes{}, apperr.Prohibido("solo un profesor puede generar reportes")
	}
	fecha, err := dominio.ParsearFecha(fechaSemana)
	if err != nil {
		return dominio.LoteReportes{}, apperr.Validacion("semana debe ser una fecha con formato AAAA-MM-DD")
	}
	semana := dominio.InicioSemana(fecha)
	if semana.After(s.reloj.SemanaActual()) {
		return dominio.LoteReportes{}, apperr.Validacion("no se pueden generar reportes de semanas futuras")
	}

	activo, err := s.repo.ExisteLoteActivo(ctx, actor.UsuarioID, semana)
	if err != nil {
		return dominio.LoteReportes{}, err
	}
	if activo {
		return dominio.LoteReportes{}, apperr.Conflicto("ya hay un lote en proceso para la semana del %s", semana.Format(dominio.FormatoFecha))
	}

	lote, err := s.repo.CrearLote(ctx, actor.UsuarioID, semana)
	if err != nil {
		return dominio.LoteReportes{}, err
	}
	if err := s.encolador.EncolarLote(ctx, lote.ID); err != nil {
		s.log.Error("no fue posible encolar el lote", "lote_id", lote.ID, "error", err)
		_ = s.repo.MarcarLoteFallido(context.WithoutCancel(ctx), lote.ID, "no fue posible encolar el lote: "+err.Error())
		return dominio.LoteReportes{}, apperr.Interno(err, "no fue posible iniciar la generación de reportes, intente de nuevo")
	}
	return lote, nil
}

// ListarLotes devuelve los lotes visibles para el actor.
func (s *Reportes) ListarLotes(ctx context.Context, actor auth.Actor, f dominio.FiltroLotes) ([]dominio.LoteReportes, error) {
	switch {
	case actor.EsAdministrador():
	case actor.TieneRol(dominio.RolProfesor):
		f.ProfesorID = actor.UsuarioID // RN-03: solo sus propios lotes
	default:
		return nil, apperr.Prohibido("no tiene permiso para consultar reportes")
	}
	return s.repo.ListarLotes(ctx, f)
}

// ObtenerLote devuelve un lote y sus reportes si el actor puede verlos.
func (s *Reportes) ObtenerLote(ctx context.Context, actor auth.Actor, id string) (dominio.LoteReportes, []dominio.ReporteSemanal, error) {
	lote, err := s.repo.ObtenerLote(ctx, id)
	if err != nil {
		return lote, nil, err
	}
	if !puedeVerDeProfesor(actor, lote.ProfesorID) {
		return dominio.LoteReportes{}, nil, noEncontrado("lote de reportes")
	}
	reportes, err := s.repo.ListarReportes(ctx, dominio.FiltroReportes{LoteID: id, Paginacion: dominio.SinPaginar})
	return lote, reportes, err
}

// ListarReportes devuelve los reportes visibles para el actor.
func (s *Reportes) ListarReportes(ctx context.Context, actor auth.Actor, f dominio.FiltroReportes) ([]dominio.ReporteSemanal, error) {
	switch {
	case actor.EsAdministrador():
	case actor.TieneRol(dominio.RolProfesor):
		f.ProfesorID = actor.UsuarioID
	default:
		return nil, apperr.Prohibido("no tiene permiso para consultar reportes")
	}
	return s.repo.ListarReportes(ctx, f)
}

// ObtenerReporte devuelve un reporte si el actor puede verlo.
func (s *Reportes) ObtenerReporte(ctx context.Context, actor auth.Actor, id string) (dominio.ReporteSemanal, error) {
	rep, err := s.repo.ObtenerReporte(ctx, id)
	if err != nil {
		return rep, err
	}
	if !puedeVerDeProfesor(actor, rep.ProfesorID) {
		// Se responde "no encontrado" para no revelar que el reporte existe.
		return dominio.ReporteSemanal{}, noEncontrado("reporte")
	}
	return rep, nil
}

// AbrirPDF devuelve el archivo PDF de un reporte completado (RF-14.6).
// El llamador debe cerrar el lector.
func (s *Reportes) AbrirPDF(ctx context.Context, actor auth.Actor, id string) (io.ReadCloser, dominio.ReporteSemanal, error) {
	rep, err := s.ObtenerReporte(ctx, actor, id)
	if err != nil {
		return nil, rep, err
	}
	if rep.Estado != dominio.ReporteCompletado || rep.RutaPDF == "" {
		return nil, rep, apperr.Conflicto("el reporte aún no está disponible (estado: %s)", rep.Estado)
	}
	archivo, err := s.almacenamiento.Abrir(ctx, rep.RutaPDF)
	if err != nil {
		return nil, rep, err
	}
	return archivo, rep, nil
}
