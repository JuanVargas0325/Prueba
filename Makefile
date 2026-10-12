# Comandos habituales del proyecto. Usan Docker, así que no requieren Go instalado.
# En Windows puede usarlos desde Git Bash o WSL, o copiar el comando de cada objetivo.

GO_IMG      := golang:1.26
LINT_IMG    := golangci/golangci-lint:latest
DOCKER_GO   := docker run --rm -v "$(CURDIR)/backend:/src" -v gomod:/go/pkg/mod -v gocache:/root/.cache/go-build -w /src

.PHONY: ayuda arriba abajo logs pruebas pruebas-integracion lint formato

ayuda: ## Muestra esta ayuda
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "} {printf "  %-22s %s\n", $$1, $$2}'

arriba: ## Construye y levanta todo el sistema (requiere .env)
	docker compose up --build -d --wait

abajo: ## Detiene el sistema (conserva los datos)
	docker compose down

logs: ## Muestra los logs de la API y del worker
	docker compose logs -f api worker

pruebas: ## Pruebas unitarias con detector de condiciones de carrera
	$(DOCKER_GO) $(GO_IMG) go test -race ./...

pruebas-integracion: ## Pruebas unitarias y de integración contra un PostgreSQL temporal
	-docker network create red-pruebas >/dev/null 2>&1
	-docker rm -f pg-pruebas >/dev/null 2>&1
	docker run -d --name pg-pruebas --network red-pruebas -e POSTGRES_USER=pruebas -e POSTGRES_PASSWORD=pruebas -e POSTGRES_DB=pruebas postgres:16-bookworm
	sleep 5
	$(DOCKER_GO) --network red-pruebas -e DATABASE_URL_PRUEBAS=postgres://pruebas:pruebas@pg-pruebas:5432/pruebas?sslmode=disable \
		$(GO_IMG) go test -race -tags integracion -coverpkg=./internal/... -cover ./... ; status=$$?; docker rm -f pg-pruebas >/dev/null; exit $$status

lint: ## Formato y análisis estático (golangci-lint)
	$(DOCKER_GO) $(LINT_IMG) sh -c 'test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1); golangci-lint run ./...'

formato: ## Aplica gofmt a todo el código
	$(DOCKER_GO) $(GO_IMG) gofmt -w .
