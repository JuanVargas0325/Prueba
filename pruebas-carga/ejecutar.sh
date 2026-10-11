#!/usr/bin/env bash
# Pruebas de carga con Apache Benchmark (ab), siguiendo el tutorial de análisis de
# capacidad del curso: se incrementa la concurrencia por niveles y cada nivel se
# repite varias veces. Durante cada corrida se muestrea CPU y memoria de la API y
# de PostgreSQL con `docker stats`.
#
# Requisitos: el sistema levantado (docker compose up -d) y los datos de carga:
#   docker compose exec -T postgres psql -U seguimiento -d seguimiento < pruebas-carga/datos_carga.sql
#
# Uso (desde la raíz del repositorio):
#   bash pruebas-carga/ejecutar.sh
# Variables opcionales: NIVELES="10 50 100" REPETICIONES=3 DURACION=15 ESCENARIOS="login seguimiento tareas"
set -euo pipefail
export MSYS_NO_PATHCONV=1 # Git Bash en Windows: no convertir rutas de los contenedores

cd "$(dirname "$0")"
DIR=$(pwd -W 2>/dev/null || pwd)

NIVELES=${NIVELES:-"10 50 100 150 200"}
REPETICIONES=${REPETICIONES:-3}
DURACION=${DURACION:-15} # segundos por corrida
ESCENARIOS=${ESCENARIOS:-"login seguimiento tareas"}
API_HOST=${API_HOST:-http://localhost:${NGINX_PUERTO:-8080}}
RED=${RED:-seguimiento_default}
OBJETIVO=http://nginx
IMAGEN_AB=httpd:2.4-bookworm

RESULTADOS=resultados
mkdir -p "$RESULTADOS/ab" "$RESULTADOS/stats" cuerpos
RESUMEN="$RESULTADOS/resumen.csv"
echo "escenario,concurrencia,repeticion,completadas,errores,rps,media_ms,p50_ms,p90_ms,p95_ms,p99_ms,max_ms,cpu_api_max,cpu_db_max,mem_api_max_mb,mem_db_max_mb" > "$RESUMEN"

contenedor() { docker compose ps -q "$1"; }
API=$(contenedor api)
DB=$(contenedor postgres)

token() {
	curl -s -X POST "$API_HOST/api/v1/auth/login" -H 'Content-Type: application/json' \
		-d "{\"correo\":\"$1\",\"password\":\"Carga123*\"}" | sed -E 's/.*"token":"([^"]+)".*/\1/'
}

psql() { docker compose exec -T postgres psql -U seguimiento -d seguimiento -tAc "$1"; }

# Cada ejecución parte del mismo estado: se eliminan las tareas creadas por ejecuciones anteriores.
echo "Restaurando los datos de carga..."
psql "DELETE FROM tareas WHERE titulo = 'Tarea de carga'" >/dev/null

echo "Obteniendo tokens..."
TOKEN_PROFESORA=$(token profesora.carga@uniandes.edu.co)
TOKEN_MONITOR=$(token monitor.carga.1@uniandes.edu.co)
VINCULACION=$(psql "SELECT md5('vinculacion-carga-1')::uuid")

printf '{"correo":"monitor.carga.2@uniandes.edu.co","password":"Carga123*"}' > cuerpos/login.json
printf '{"vinculacion_id":"%s","titulo":"Tarea de carga","descripcion":"Registro generado por la prueba de carga.","estado":"abierto","horas_invertidas":1}' \
	"$VINCULACION" > cuerpos/tarea.json

# muestrear escribe "cpu_api,cpu_db,mem_api_mb,mem_db_mb" hasta que exista el archivo $1.
muestrear() {
	while [ ! -f "$1" ]; do
		docker stats --no-stream --format '{{.ID}} {{.CPUPerc}} {{.MemUsage}}' "$API" "$DB" 2>/dev/null |
			awk -v api="${API:0:12}" '
				function mb(v) { if (v ~ /GiB/) return v * 1024; if (v ~ /KiB/) return v / 1024; return v + 0 }
				{ cpu = $2; sub("%", "", cpu); mem = mb($3)
				  if ($1 == api) { ca = cpu; ma = mem } else { cd = cpu; md = mem } }
				END { printf "%s,%s,%.1f,%.1f\n", ca, cd, ma, md }'
	done
}

# ejecutar_ab escenario concurrencia repeticion
ejecutar_ab() {
	local escenario=$1 c=$2 r=$3 args=()
	case $escenario in
	login)
		args=(-p /cuerpos/login.json -T application/json "$OBJETIVO/api/v1/auth/login") ;;
	seguimiento)
		args=(-H "Authorization: Bearer $TOKEN_PROFESORA" "$OBJETIVO/api/v1/seguimiento/personas") ;;
	tareas)
		args=(-H "Authorization: Bearer $TOKEN_MONITOR" -p /cuerpos/tarea.json -T application/json "$OBJETIVO/api/v1/tareas") ;;
	esac
	local nombre="${escenario}_c${c}_r${r}"
	local parar="$RESULTADOS/stats/.parar"
	rm -f "$parar"
	muestrear "$parar" > "$RESULTADOS/stats/$nombre.csv" &
	local muestreo=$!

	# -t: duración fija; -n alto para que el límite sea el tiempo; -k: keep-alive; -r: no abortar por errores.
	docker run --rm --network "$RED" -v "$DIR/cuerpos:/cuerpos:ro" "$IMAGEN_AB" \
		ab -t "$DURACION" -n 1000000 -c "$c" -k -r "${args[@]}" > "$RESULTADOS/ab/$nombre.txt" 2>&1 || true
	touch "$parar"
	wait "$muestreo" # espera a que termine la muestra en curso

	# Los "Failed requests" por longitud no son errores: las respuestas JSON varían de tamaño.
	awk -v e="$escenario" -v c="$c" -v r="$r" -v stats="$RESULTADOS/stats/$nombre.csv" '
		/^Complete requests:/ { completadas = $3 }
		/^Failed requests:/ && $3 > 0 { getline; gsub(/[(),]/, " "); fallos = $2 + $4 + $8 }
		/^Non-2xx responses:/ { no2xx = $3 }
		/^Requests per second:/ { rps = $4 }
		/^Time per request:.*\(mean\)$/ { media = $4 }
		/^  50%/ { p50 = $2 } /^  90%/ { p90 = $2 } /^  95%/ { p95 = $2 } /^  99%/ { p99 = $2 } /^ 100%/ { pmax = $2 }
		END {
			while ((getline linea < stats) > 0) {
				split(linea, v, ",")
				if (v[1] + 0 > ca) ca = v[1] + 0; if (v[2] + 0 > cd) cd = v[2] + 0
				if (v[3] + 0 > ma) ma = v[3] + 0; if (v[4] + 0 > md) md = v[4] + 0
			}
			printf "%s,%s,%s,%d,%d,%s,%s,%s,%s,%s,%s,%s,%.1f,%.1f,%.1f,%.1f\n",
				e, c, r, completadas, fallos + no2xx, rps, media, p50, p90, p95, p99, pmax, ca, cd, ma, md
		}' "$RESULTADOS/ab/$nombre.txt" >> "$RESUMEN"
	tail -n 1 "$RESUMEN"
}

for escenario in $ESCENARIOS; do
	for c in $NIVELES; do
		for r in $(seq 1 "$REPETICIONES"); do
			ejecutar_ab "$escenario" "$c" "$r"
			sleep 3 # deja que el sistema se estabilice entre corridas
		done
	done
done

echo "Resultados en pruebas-carga/$RESUMEN"
