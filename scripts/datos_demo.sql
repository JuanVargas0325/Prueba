-- Datos de demostración para desarrollo y para el video (NO usar en producción).
-- Cargar con:
--   docker compose exec -T postgres psql -U seguimiento -d seguimiento < scripts/datos_demo.sql
--
-- Usuarios creados (contraseña de todos: Demo123*):
--   profesora.rojas@uniandes.edu.co   profesor
--   andres.monitor@uniandes.edu.co    monitor
--   laura.asistente@uniandes.edu.co   asistente_graduado y monitor
--   pedro.sintareas@uniandes.edu.co   monitor (no registra tareas: sus semanas aparecen "sin_reporte")
--
-- Escenario: período 2026-20, un curso y un proyecto desde agosto. Las tareas se
-- reparten en varias semanas para que el seguimiento muestre semanas con reporte,
-- sin reporte y reportes tardíos. El script es idempotente.

CREATE EXTENSION IF NOT EXISTS pgcrypto; -- solo para generar los hash bcrypt de demostración

BEGIN;

INSERT INTO usuarios (id, nombre, correo, password_hash) VALUES
  ('11111111-1111-4111-8111-111111111111', 'Carolina Rojas', 'profesora.rojas@uniandes.edu.co', crypt('Demo123*', gen_salt('bf', 10))),
  ('22222222-2222-4222-8222-222222222222', 'Andrés Gómez',   'andres.monitor@uniandes.edu.co',  crypt('Demo123*', gen_salt('bf', 10))),
  ('33333333-3333-4333-8333-333333333333', 'Laura Martínez', 'laura.asistente@uniandes.edu.co', crypt('Demo123*', gen_salt('bf', 10))),
  ('44444444-4444-4444-8444-444444444444', 'Pedro Sánchez',  'pedro.sintareas@uniandes.edu.co', crypt('Demo123*', gen_salt('bf', 10)))
ON CONFLICT (id) DO NOTHING;

INSERT INTO usuario_roles (usuario_id, rol) VALUES
  ('11111111-1111-4111-8111-111111111111', 'profesor'),
  ('22222222-2222-4222-8222-222222222222', 'monitor'),
  ('33333333-3333-4333-8333-333333333333', 'asistente_graduado'),
  ('33333333-3333-4333-8333-333333333333', 'monitor'),
  ('44444444-4444-4444-8444-444444444444', 'monitor')
ON CONFLICT DO NOTHING;

INSERT INTO periodos_academicos (id, codigo, fecha_inicio, fecha_cierre) VALUES
  ('55555555-5555-4555-8555-555555555555', '2026-20', '2026-08-01', '2026-12-15')
ON CONFLICT (id) DO NOTHING;

INSERT INTO espacios (id, tipo, nombre, periodo_id, profesor_id, fecha_inicio, fecha_cierre, observaciones) VALUES
  ('66666666-6666-4666-8666-666666666666', 'curso',    'Desarrollo de software en la nube',
   '55555555-5555-4555-8555-555555555555', '11111111-1111-4111-8111-111111111111', '2026-08-03', '2026-12-04', 'Curso de maestría MISW4204'),
  ('77777777-7777-4777-8777-777777777777', 'proyecto', 'Plataforma de analítica académica',
   '55555555-5555-4555-8555-555555555555', '11111111-1111-4111-8111-111111111111', '2026-08-03', '2026-12-04', 'Proyecto de investigación')
ON CONFLICT (id) DO NOTHING;

-- Laura: 12 h como asistente graduada y 4 h como monitora (máximo permitido: 40 % de 12 = 4,8 → 5 h).
-- Las vinculaciones se fechan al inicio del semestre para que exijan reporte desde entonces.
INSERT INTO vinculaciones (id, usuario_id, espacio_id, rol, horas_semanales, creado_en, actualizado_en) VALUES
  ('88888888-8888-4888-8888-888888888881', '22222222-2222-4222-8222-222222222222', '66666666-6666-4666-8666-666666666666', 'monitor',            6, '2026-08-03', '2026-08-03'),
  ('88888888-8888-4888-8888-888888888882', '33333333-3333-4333-8333-333333333333', '77777777-7777-4777-8777-777777777777', 'asistente_graduado', 12, '2026-08-03', '2026-08-03'),
  ('88888888-8888-4888-8888-888888888883', '33333333-3333-4333-8333-333333333333', '66666666-6666-4666-8666-666666666666', 'monitor',            4, '2026-08-03', '2026-08-03'),
  ('88888888-8888-4888-8888-888888888884', '44444444-4444-4444-8444-444444444444', '66666666-6666-4666-8666-666666666666', 'monitor',            3, '2026-08-03', '2026-08-03')
ON CONFLICT (id) DO NOTHING;

INSERT INTO tareas (id, vinculacion_id, semana_inicio, titulo, descripcion, estado, horas_invertidas, observaciones, tardia) VALUES
  -- Semana del 21 de septiembre
  ('99999999-9999-4999-8999-999999999911', '88888888-8888-4888-8888-888888888881', '2026-09-21',
   'Preparación del taller de Docker', 'Guía y ejemplos del taller de contenedores.', 'finalizado', 5, '', false),
  ('99999999-9999-4999-8999-999999999912', '88888888-8888-4888-8888-888888888882', '2026-09-21',
   'Levantamiento de requerimientos', 'Reuniones con la coordinación para definir las métricas.', 'finalizado', 10, '', false),
  -- Semana del 28 de septiembre (Laura reporta tarde; Andrés no reporta)
  ('99999999-9999-4999-8999-999999999913', '88888888-8888-4888-8888-888888888882', '2026-09-28',
   'Prototipo del tablero', 'Primer prototipo con datos de prueba.', 'en_desarrollo', 8, 'Registrada después del cierre de la semana.', true),
  -- Semana del 5 de octubre
  ('99999999-9999-4999-8999-999999999901', '88888888-8888-4888-8888-888888888881', '2026-10-05',
   'Calificación del taller de Docker', 'Revisión y retroalimentación de 28 entregas del taller de Docker Compose.', 'finalizado', 4, '', false),
  ('99999999-9999-4999-8999-999999999902', '88888888-8888-4888-8888-888888888881', '2026-10-05',
   'Atención de dudas en Slack', 'Respuesta a preguntas sobre la configuración de Nginx y variables de entorno.', 'en_desarrollo', 2, 'Quedan dos preguntas abiertas.', false),
  ('99999999-9999-4999-8999-999999999903', '88888888-8888-4888-8888-888888888882', '2026-10-05',
   'Modelo de datos del tablero', 'Diseño del esquema de tablas para las métricas académicas del tablero.', 'finalizado', 6, '', false),
  ('99999999-9999-4999-8999-999999999904', '88888888-8888-4888-8888-888888888882', '2026-10-05',
   'Integración con la API de notas', 'Conexión con el servicio de notas y pruebas de extracción de datos.', 'en_desarrollo', 5, 'Pendiente de credenciales del servicio.', false),
  ('99999999-9999-4999-8999-999999999905', '88888888-8888-4888-8888-888888888883', '2026-10-05',
   'Preparación del laboratorio de Postman', 'Guía paso a paso para el laboratorio de pruebas de API con Postman y Newman.', 'abierto', 3, '', true)
ON CONFLICT (id) DO NOTHING;

COMMIT;
