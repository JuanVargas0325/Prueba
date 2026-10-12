# Imagen multietapa sobre Debian: compila en una imagen con Go y copia solo el
# binario a una imagen liviana. En el paso 55 se agrega el destino del worker.
#   docker build --target api -t seguimiento-api .

# --- Compilación -------------------------------------------------------------
FROM golang:1.26-bookworm AS compilacion
WORKDIR /src

# Las dependencias se descargan en una capa aparte para aprovechar la caché.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /salida/api ./cmd/api

# --- Base de ejecución: Debian 12 (bookworm) ---------------------------------
FROM debian:bookworm-slim AS base
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates tzdata wget \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 10001 --no-create-home --shell /usr/sbin/nologin app
USER app

# --- API ---------------------------------------------------------------------
FROM base AS api
COPY --from=compilacion /salida/api /usr/local/bin/api
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=20s --retries=5 \
  CMD wget -qO- http://127.0.0.1:8080/salud >/dev/null || exit 1
ENTRYPOINT ["/usr/local/bin/api"]
