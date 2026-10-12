# Paso 59 · Datos de demostración

> Rama `paso-a-paso` · paso 59 de 65 · anterior: **Paso 58 · Pruebas de integración** · siguiente: **Paso 60 · Colección de Postman**

---

## 1. Qué se hizo en este paso

Se agregó un script SQL con un **escenario de demostración** listo para usar: una profesora, tres personas a cargo, un período, un curso, un proyecto, vinculaciones y tareas repartidas en varias semanas.

| Archivo | Cambio |
|---|---|
| `scripts/datos_demo.sql` | **Nuevo.** Escenario de demostración idempotente. |
| `Makefile` | **Modificado.** Objetivo `make demo`. |

| Usuario (contraseña `Demo123*`) | Rol | Situación |
|---|---|---|
| `profesora.rojas@uniandes.edu.co` | profesora | Responsable del curso y del proyecto. |
| `andres.monitor@uniandes.edu.co` | monitor | 6 h; reporta algunas semanas y otras no. |
| `laura.asistente@uniandes.edu.co` | asistente graduada y monitora | 12 h + 4 h (dentro del 40 %); tiene un reporte **tardío**. |
| `pedro.sintareas@uniandes.edu.co` | monitor | 3 h; **nunca reporta**: todas sus semanas cerradas aparecen `sin_reporte`. |

Vista de la profesora después de cargarlo:

```
Andrés Gómez   | 1 vinculación  |  6 h | semana anterior: sin_reporte
Laura Martínez | 2 vinculaciones| 16 h | semana anterior: con_reporte
Pedro Sánchez  | 1 vinculación  |  3 h | semana anterior: sin_reporte
```

---

## 2. Por qué

- **Para el video y la sustentación:** el seguimiento (paso 44) y los reportes (paso 55) solo se aprecian con **historia**: semanas con y sin reporte, tareas tardías, horas en varios espacios. Crearla a mano por la API tomaría mucho tiempo y, además, la API fecha todo "hoy" (no se puede crear una vinculación del mes pasado).
- **Para probar a mano** cualquier funcionalidad con datos realistas, sin preparar nada.

---

## 3. Explicación detallada de `datos_demo.sql`

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto; -- solo para generar los hash bcrypt de demostración

BEGIN;

INSERT INTO usuarios (id, nombre, correo, password_hash) VALUES
  ('11111111-1111-4111-8111-111111111111', 'Carolina Rojas', 'profesora.rojas@uniandes.edu.co', crypt('Demo123*', gen_salt('bf', 10))),
  ...
ON CONFLICT (id) DO NOTHING;
```
- **`pgcrypto`** es una extensión de PostgreSQL con funciones criptográficas. `crypt('Demo123*', gen_salt('bf', 10))` genera un hash **bcrypt** (`bf` = Blowfish, costo 10): el mismo formato que usa la API (paso 12). Así se puede iniciar sesión con estos usuarios.
- **IDs fijos y legibles** (`1111…`, `2222…`): permiten relacionar las filas en el script y reconocerlas en las pruebas.
- **`BEGIN … COMMIT`**: todo el escenario se carga en una transacción: o se crea completo o no se crea nada.
- **`ON CONFLICT … DO NOTHING`** en cada `INSERT`: el script es **idempotente**; ejecutarlo dos veces no duplica ni falla.

```sql
-- Laura: 12 h como asistente graduada y 4 h como monitora (máximo permitido: 40 % de 12 = 4,8 → 5 h).
-- Las vinculaciones se fechan al inicio del semestre para que exijan reporte desde entonces.
INSERT INTO vinculaciones (id, usuario_id, espacio_id, rol, horas_semanales, creado_en, actualizado_en) VALUES ...
```
- **Respeta las reglas de horas**: los datos de demostración son un ejemplo **válido** del negocio.
- `creado_en` = 3 de agosto: la vigencia de cada vinculación empieza ahí (paso 30), así que las semanas cerradas sin tareas salen como `sin_reporte`.

```sql
INSERT INTO tareas (..., tardia) VALUES
  -- Semana del 21 de septiembre
  ...
  -- Semana del 28 de septiembre (Laura reporta tarde; Andrés no reporta)
  ... 'Registrada después del cierre de la semana.', true),
  -- Semana del 5 de octubre
  ...
```
- Las tareas están en las semanas del 21/09, 28/09 y 05/10 de 2026, con distintos estados, observaciones y dos reportes **tardíos**.
- **Este script escribe directo en la base de datos y se salta las reglas de la API** (por ejemplo, puede crear tareas tardías o fechar vinculaciones en el pasado). Solo es para demostración; **nunca** se debe usar en producción.

---

## 4. `make demo`

```makefile
demo: ## Carga los datos de demostración
	docker compose exec -T postgres psql -U $$(grep '^POSTGRES_USER=' .env | cut -d= -f2) -d $$(grep '^POSTGRES_DB=' .env | cut -d= -f2) < scripts/datos_demo.sql
```
- Ejecuta `psql` **dentro** del contenedor de PostgreSQL y le pasa el script por la entrada estándar (`<`). `-T` desactiva la terminal interactiva, necesario para redirigir un archivo.
- Lee el usuario y la base de datos de su `.env` (`grep … | cut -d= -f2` toma lo que está después del `=`). En un `Makefile`, `$$(...)` produce `$(...)` para el shell.

---

## 5. Cómo comprobarlo

```bash
docker compose up --build -d --wait
make demo
# o: docker compose exec -T postgres psql -U seguimiento -d seguimiento < scripts/datos_demo.sql

# Iniciar sesión como la profesora y ver a sus personas
TOKEN=$(curl -s -X POST localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"correo":"profesora.rojas@uniandes.edu.co","password":"Demo123*"}' | python -c "import sys,json;print(json.load(sys.stdin)['token'])")
curl -s localhost:8080/api/v1/seguimiento/personas -H "Authorization: Bearer $TOKEN"
```

Ideas para la demostración:
- detalle semanal de Pedro (todo `sin_reporte`) y de Laura (con su tarea tardía);
- un lote de reportes de la semana del 28/09 y la descarga de los PDF;
- intentar vincular a Laura a otra monitoría de 2 h: se rechaza (4 + 2 = 6 > 5 h).

Ejecutar `make demo` otra vez no cambia nada (idempotente).

---

## 6. Pasos para lograr esta versión desde el paso 58

1. Crear `scripts/datos_demo.sql` (sección 3).
2. En el `Makefile`: el objetivo `demo` y agregarlo a `.PHONY`.
3. `make demo` sobre el sistema levantado.
4. Commit: `git add . && git commit -m "Paso 59: datos de demostración"`

---

## 7. Preparación para las próximas entregas

- Sirve igual contra la base de datos de la nube (Entrega 2): basta con ejecutarlo con `psql` contra Cloud SQL para tener datos con los cuales probar el frontend.
- Si las fechas de la demostración quedan muy atrás, se pueden ajustar las de las tareas y las vinculaciones; las reglas de semanas se calculan sobre la fecha real del sistema.

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Sustentación en video (lineamientos §3) | Escenario listo para mostrar las funcionalidades. |
| RF-09 / RF-10 | Datos con semanas con reporte, sin reporte y tardías. |
| RF-05 | Vinculaciones que respetan las reglas de horas. |
