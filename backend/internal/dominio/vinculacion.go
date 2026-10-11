package dominio

import (
	"time"

	"seguimiento/internal/apperr"
)

// Límites de dedicación definidos por las reglas de negocio (RN-14 a RN-18).
const (
	MaxHorasAsistenteGraduado   = 22 // RN-14
	MaxMonitoriasSimultaneas    = 3  // RN-15
	MaxHorasMonitoria           = 12 // RN-16
	PorcentajeMaxMonitoriaMixta = 40 // RN-17: % de las horas de asistencia graduada
)

// Vinculacion asocia una persona a un curso o proyecto con un rol y unas horas
// semanales contratadas (RF-04, RN-11). Incluye datos del usuario y del espacio
// para las consultas; el profesor responsable es el del espacio.
type Vinculacion struct {
	ID                 string
	UsuarioID          string
	UsuarioNombre      string
	UsuarioCorreo      string
	EspacioID          string
	EspacioNombre      string
	EspacioTipo        TipoEspacio
	EspacioEstado      EstadoEspacio
	EspacioFechaInicio time.Time
	EspacioFechaCierre time.Time
	ProfesorID         string
	ProfesorNombre     string
	Rol                Rol
	HorasSemanales     int
	CreadoEn           time.Time
	ActualizadoEn      time.Time
}

// InicioVigencia es la primera fecha en que la vinculación exige reporte:
// la mayor entre el inicio del espacio y el día en que se creó la vinculación.
func (v Vinculacion) InicioVigencia(zona *time.Location) time.Time {
	creada := Fecha(v.CreadoEn, zona)
	if creada.After(v.EspacioFechaInicio) {
		return creada
	}
	return v.EspacioFechaInicio
}

// VigenteEnSemana indica si la persona debía reportar en esa semana.
func (v Vinculacion) VigenteEnSemana(semanaInicio time.Time, zona *time.Location) bool {
	return seCruzan(v.InicioVigencia(zona), v.EspacioFechaCierre, InicioSemana(semanaInicio), FinSemana(semanaInicio))
}

// ValidarHoras verifica las horas semanales contratadas de una vinculación.
func ValidarHoras(horas int) error {
	if horas <= 0 {
		return apperr.Validacion("las horas semanales contratadas deben ser mayores que cero")
	}
	return nil
}

// LimiteMonitoriaMixta devuelve el máximo de horas de monitoría para quien
// también es asistente graduado: el 40 % de sus horas de asistencia,
// redondeado hacia arriba (RN-17, RN-18). Ejemplo: 22 h → 9 h; 12 h → 5 h.
func LimiteMonitoriaMixta(horasAsistencia int) int {
	return (horasAsistencia*PorcentajeMaxMonitoriaMixta + 99) / 100
}

// ValidarCargaHoraria aplica las reglas de dedicación (RN-14 a RN-19) sobre el
// conjunto completo de vinculaciones simultáneas de una persona, incluida la
// que se quiere crear o modificar. Las reglas son bloqueantes.
func ValidarCargaHoraria(vinculaciones []Vinculacion) error {
	horasAsistencia, horasMonitoria, monitorias := 0, 0, 0
	for _, v := range vinculaciones {
		switch v.Rol {
		case RolAsistenteGraduado:
			horasAsistencia += v.HorasSemanales
		case RolMonitor:
			horasMonitoria += v.HorasSemanales
			monitorias++
		}
	}

	if horasAsistencia > MaxHorasAsistenteGraduado {
		return apperr.ReglaNegocio("la persona superaría el máximo de %d horas semanales como asistente graduado (quedaría con %d)",
			MaxHorasAsistenteGraduado, horasAsistencia)
	}
	if monitorias > MaxMonitoriasSimultaneas {
		return apperr.ReglaNegocio("la persona superaría el máximo de %d monitorías simultáneas", MaxMonitoriasSimultaneas)
	}
	if horasMonitoria > MaxHorasMonitoria {
		return apperr.ReglaNegocio("la persona superaría el máximo de %d horas semanales de monitoría (quedaría con %d)",
			MaxHorasMonitoria, horasMonitoria)
	}
	if horasAsistencia > 0 && monitorias > 0 {
		if limite := LimiteMonitoriaMixta(horasAsistencia); horasMonitoria > limite {
			return apperr.ReglaNegocio("con %d horas como asistente graduado, las horas de monitoría no pueden superar %d (40 %% redondeado hacia arriba); quedaría con %d",
				horasAsistencia, limite, horasMonitoria)
		}
	}
	return nil
}

// ValidadorCarga recibe las vinculaciones simultáneas de una persona y la
// vinculación que se quiere guardar, y devuelve un error si se viola una regla.
// El repositorio lo invoca dentro de la transacción que bloquea al usuario.
type ValidadorCarga func(simultaneas []Vinculacion, objetivo Vinculacion) error

// Rectificacion registra una corrección administrativa de una vinculación (RF-04.5).
type Rectificacion struct {
	ID              string
	VinculacionID   string
	AdministradorID string
	RolAnterior     Rol
	RolNuevo        Rol
	HorasAnteriores int
	HorasNuevas     int
	Motivo          string
	CreadoEn        time.Time
}

// FiltroVinculaciones delimita la consulta de vinculaciones.
type FiltroVinculaciones struct {
	// VisiblePara limita el resultado a las vinculaciones de los espacios del
	// profesor o a las propias del usuario. Vacío = sin restricción.
	VisiblePara string
	// ProfesorID limita el resultado a los espacios de ese profesor.
	ProfesorID string
	UsuarioID  string
	EspacioID  string
	// SoloEspaciosActivos excluye las vinculaciones de espacios cerrados.
	SoloEspaciosActivos bool
	Paginacion
}
