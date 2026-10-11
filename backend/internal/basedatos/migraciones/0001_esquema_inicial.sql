-- Esquema inicial de la plataforma de seguimiento de monitores y asistentes graduados.
-- Para cambiar el esquema NO edite este archivo: cree uno nuevo con el siguiente número
-- (por ejemplo 0002_descripcion.sql). Las migraciones se aplican en orden al iniciar la API.

-- Usuarios y roles (RF-01, RF-02, RN-01) ------------------------------------------

CREATE TABLE usuarios (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nombre          TEXT        NOT NULL,
    correo          TEXT        NOT NULL,
    password_hash   TEXT        NOT NULL,
    activo          BOOLEAN     NOT NULL DEFAULT TRUE,
    creado_en       TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX usuarios_correo_unico ON usuarios (lower(correo));

CREATE TABLE usuario_roles (
    usuario_id  UUID NOT NULL REFERENCES usuarios (id) ON DELETE CASCADE,
    rol         TEXT NOT NULL CHECK (rol IN ('administrador', 'profesor', 'monitor', 'asistente_graduado')),
    PRIMARY KEY (usuario_id, rol)
);

-- Períodos académicos (RF-03.2) ---------------------------------------------------

CREATE TABLE periodos_academicos (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    codigo               TEXT        NOT NULL UNIQUE,               -- ej. 2026-20
    fecha_inicio         DATE        NOT NULL,
    fecha_cierre         DATE        NOT NULL,
    estado               TEXT        NOT NULL DEFAULT 'activo' CHECK (estado IN ('activo', 'cerrado')),
    creacion_habilitada  BOOLEAN     NOT NULL DEFAULT TRUE,         -- ventana para crear cursos/proyectos (RN-07)
    creado_en            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (fecha_cierre > fecha_inicio)
);

-- Cursos y proyectos (RF-03, RN-06 a RN-09) ---------------------------------------

CREATE TABLE espacios (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tipo            TEXT        NOT NULL CHECK (tipo IN ('curso', 'proyecto')),
    nombre          TEXT        NOT NULL,
    periodo_id      UUID        NOT NULL REFERENCES periodos_academicos (id),
    profesor_id     UUID        NOT NULL REFERENCES usuarios (id),
    fecha_inicio    DATE        NOT NULL,
    fecha_cierre    DATE        NOT NULL,
    observaciones   TEXT        NOT NULL DEFAULT '',
    estado          TEXT        NOT NULL DEFAULT 'activo' CHECK (estado IN ('activo', 'cerrado')),
    creado_en       TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (fecha_cierre >= fecha_inicio)
);
CREATE INDEX espacios_profesor ON espacios (profesor_id);

-- Vinculaciones (RF-04, RN-10 a RN-13) --------------------------------------------
-- El profesor responsable (RN-11) se obtiene del espacio asociado.
-- Las reglas de horas (RN-14 a RN-19) dependen de varias vinculaciones, por lo que
-- se validan en la capa de servicio dentro de una transacción que bloquea al usuario.

CREATE TABLE vinculaciones (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    usuario_id       UUID        NOT NULL REFERENCES usuarios (id),
    espacio_id       UUID        NOT NULL REFERENCES espacios (id),
    rol              TEXT        NOT NULL CHECK (rol IN ('monitor', 'asistente_graduado')),
    horas_semanales  INTEGER     NOT NULL CHECK (horas_semanales > 0),
    creado_en        TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (usuario_id, espacio_id, rol)
);
CREATE INDEX vinculaciones_usuario ON vinculaciones (usuario_id);
CREATE INDEX vinculaciones_espacio ON vinculaciones (espacio_id);

-- Rectificaciones administrativas de vinculaciones (RF-04.5): conservan el valor
-- anterior y el nuevo para que toda corrección de horas o rol quede trazable.
CREATE TABLE vinculacion_rectificaciones (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vinculacion_id    UUID        NOT NULL REFERENCES vinculaciones (id) ON DELETE CASCADE,
    administrador_id  UUID        NOT NULL REFERENCES usuarios (id),
    rol_anterior      TEXT        NOT NULL,
    rol_nuevo         TEXT        NOT NULL,
    horas_anteriores  INTEGER     NOT NULL,
    horas_nuevas      INTEGER     NOT NULL,
    motivo            TEXT        NOT NULL,
    creado_en         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX vinculacion_rectificaciones_vinculacion ON vinculacion_rectificaciones (vinculacion_id);

-- Tareas semanales (RF-06 a RF-08, RN-20 a RN-34) ---------------------------------

CREATE TABLE tareas (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vinculacion_id    UUID        NOT NULL REFERENCES vinculaciones (id),
    semana_inicio     DATE        NOT NULL CHECK (EXTRACT(ISODOW FROM semana_inicio) = 1), -- siempre lunes
    titulo            TEXT        NOT NULL,
    descripcion       TEXT        NOT NULL DEFAULT '',
    estado            TEXT        NOT NULL DEFAULT 'abierto' CHECK (estado IN ('abierto', 'en_desarrollo', 'finalizado')),
    horas_invertidas  INTEGER     NOT NULL CHECK (horas_invertidas >= 0),                  -- horas enteras
    observaciones     TEXT        NOT NULL DEFAULT '',
    tardia            BOOLEAN     NOT NULL DEFAULT FALSE,                                   -- RN-32
    creado_en         TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tareas_vinculacion_semana ON tareas (vinculacion_id, semana_inicio);
CREATE INDEX tareas_semana ON tareas (semana_inicio);

CREATE TABLE tarea_adjuntos (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tarea_id             UUID        NOT NULL REFERENCES tareas (id) ON DELETE CASCADE,
    nombre_original      TEXT        NOT NULL,
    ruta_almacenamiento  TEXT        NOT NULL,
    tipo_contenido       TEXT        NOT NULL,
    tamano_bytes         BIGINT      NOT NULL CHECK (tamano_bytes >= 0),
    creado_en            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tarea_adjuntos_tarea ON tarea_adjuntos (tarea_id);

-- Reportes semanales con IA (RF-14, RN-41 a RN-46) --------------------------------

CREATE TABLE lotes_reportes (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profesor_id           UUID        NOT NULL REFERENCES usuarios (id),
    semana_inicio         DATE        NOT NULL CHECK (EXTRACT(ISODOW FROM semana_inicio) = 1),
    estado                TEXT        NOT NULL DEFAULT 'pendiente'
                          CHECK (estado IN ('pendiente', 'en_proceso', 'completado', 'completado_con_errores', 'fallido')),
    total_reportes        INTEGER     NOT NULL DEFAULT 0,
    reportes_completados  INTEGER     NOT NULL DEFAULT 0,
    reportes_fallidos     INTEGER     NOT NULL DEFAULT 0,
    error                 TEXT        NOT NULL DEFAULT '',
    creado_en             TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en        TIMESTAMPTZ NOT NULL DEFAULT now(),
    finalizado_en         TIMESTAMPTZ
);
CREATE INDEX lotes_reportes_profesor_semana ON lotes_reportes (profesor_id, semana_inicio);

CREATE TABLE reportes_semanales (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lote_id         UUID        NOT NULL REFERENCES lotes_reportes (id) ON DELETE CASCADE,
    profesor_id     UUID        NOT NULL REFERENCES usuarios (id),
    usuario_id      UUID        NOT NULL REFERENCES usuarios (id),
    semana_inicio   DATE        NOT NULL,
    estado          TEXT        NOT NULL DEFAULT 'pendiente'
                    CHECK (estado IN ('pendiente', 'en_proceso', 'completado', 'fallido')),
    resumen         TEXT        NOT NULL DEFAULT '',
    ruta_pdf        TEXT        NOT NULL DEFAULT '',
    tamano_bytes    BIGINT      NOT NULL DEFAULT 0,
    intentos        INTEGER     NOT NULL DEFAULT 0,
    error           TEXT        NOT NULL DEFAULT '',
    proveedor_ia    TEXT        NOT NULL DEFAULT '',
    modelo_ia       TEXT        NOT NULL DEFAULT '',
    creado_en       TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en  TIMESTAMPTZ NOT NULL DEFAULT now(),
    generado_en     TIMESTAMPTZ,
    UNIQUE (lote_id, usuario_id)
);
CREATE INDEX reportes_semanales_profesor_semana ON reportes_semanales (profesor_id, semana_inicio);
CREATE INDEX reportes_semanales_usuario ON reportes_semanales (usuario_id);
