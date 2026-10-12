# Imagen multietapa sobre Debian: compila una sola vez y produce dos imágenes
# livianas, una para la API y otra para el worker.
#   docker build --target api    -t seguimiento-api .
#   docker build --target worker -t seguimiento-worker .

# --- Compilación -------------------------------------------------------------
FROM golang:1.26-bookworm AS compilacion
WORKDIR /src

# Las dependencias se descargan en una capa aparte para aprovechar la caché.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /salida/api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /salida/worker ./cmd/worker

# --- Base de ejecución: Debian 12 (bookworm) ---------------------------------
FROM debian:bookworm-slim AS base
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates tzdata wget \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 10001 --no-create-home --shell /usr/sbin/nologin app \
 && mkdir -p /data \
 && chown app:app /data
# /data es el punto de montaje del volumen de archivos (PDF y adjuntos).
VOLUME ["/data"]
USER app

# --- API ---------------------------------------------------------------------
FROM base AS api
COPY --from=compilacion /salida/api /usr/local/bin/api
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=5 \
  CMD wget -qO- http://127.0.0.1:8080/salud >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/api"]

# --- Worker ------------------------------------------------------------------
FROM base AS worker
COPY --from=compilacion /salida/worker /usr/local/bin/worker
ENTRYPOINT ["/usr/local/bin/worker"]
