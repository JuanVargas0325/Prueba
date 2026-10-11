-- Datos para las pruebas de carga (NO usar en producción). Son independientes de
-- los datos de demostración y el script es idempotente.
--
-- Escenario: una profesora con 40 monitores en un curso, cada uno con 2 tareas por
-- semana durante 10 semanas (800 tareas). Representa la carga de un profesor con
-- un curso grande en la mitad del semestre.
--
-- Usuarios (contraseña de todos: Carga123*):
--   profesora.carga@uniandes.edu.co                     profesor
--   monitor.carga.1@uniandes.edu.co ... monitor.carga.40 monitor

CREATE EXTENSION IF NOT EXISTS pgcrypto; -- solo para generar los hash bcrypt

BEGIN;

INSERT INTO usuarios (id, nombre, correo, password_hash)
VALUES ('c0000000-0000-4000-8000-000000000001', 'Profesora de Carga', 'profesora.carga@uniandes.edu.co', crypt('Carga123*', gen_salt('bf', 10)))
ON CONFLICT (id) DO NOTHING;
INSERT INTO usuario_roles (usuario_id, rol)
VALUES ('c0000000-0000-4000-8000-000000000001', 'profesor')
ON CONFLICT DO NOTHING;

INSERT INTO usuarios (id, nombre, correo, password_hash)
SELECT md5('monitor-carga-' || i)::uuid, 'Monitor de Carga ' || i, 'monitor.carga.' || i || '@uniandes.edu.co',
       crypt('Carga123*', gen_salt('bf', 10))
FROM generate_series(1, 40) AS i
ON CONFLICT (id) DO NOTHING;
INSERT INTO usuario_roles (usuario_id, rol)
SELECT md5('monitor-carga-' || i)::uuid, 'monitor' FROM generate_series(1, 40) AS i
ON CONFLICT DO NOTHING;

INSERT INTO periodos_academicos (id, codigo, fecha_inicio, fecha_cierre)
VALUES ('c0000000-0000-4000-8000-000000000002', '2026-90', '2026-08-01', '2026-12-15')
ON CONFLICT (id) DO NOTHING;

INSERT INTO espacios (id, tipo, nombre, periodo_id, profesor_id, fecha_inicio, fecha_cierre)
VALUES ('c0000000-0000-4000-8000-000000000003', 'curso', 'Curso de carga', 'c0000000-0000-4000-8000-000000000002',
        'c0000000-0000-4000-8000-000000000001', '2026-08-03', '2026-12-04')
ON CONFLICT (id) DO NOTHING;

INSERT INTO vinculaciones (id, usuario_id, espacio_id, rol, horas_semanales, creado_en, actualizado_en)
SELECT md5('vinculacion-carga-' || i)::uuid, md5('monitor-carga-' || i)::uuid, 'c0000000-0000-4000-8000-000000000003',
       'monitor', 4, '2026-08-03', '2026-08-03'
FROM generate_series(1, 40) AS i
ON CONFLICT (id) DO NOTHING;

-- 2 tareas por monitor y semana, desde la semana del 3 de agosto hasta la del 5 de octubre.
INSERT INTO tareas (id, vinculacion_id, semana_inicio, titulo, descripcion, estado, horas_invertidas, observaciones)
SELECT md5('tarea-carga-' || i || '-' || s || '-' || n)::uuid,
       md5('vinculacion-carga-' || i)::uuid,
       DATE '2026-08-03' + (s * 7),
       'Tarea ' || n || ' de la semana ' || (s + 1),
       'Actividad de apoyo al curso registrada para las pruebas de carga.',
       (ARRAY['abierto', 'en_desarrollo', 'finalizado'])[1 + (i + s + n) % 3],
       1 + (i + n) % 3,
       ''
FROM generate_series(1, 40) AS i, generate_series(0, 9) AS s, generate_series(1, 2) AS n
ON CONFLICT (id) DO NOTHING;

COMMIT;
