# Comandos habituales del proyecto. Usan Docker, así que no requieren Go instalado.
# En Windows puede usarlos desde Git Bash o WSL, o copiar el comando de cada objetivo.

GO_IMG      := golang:1.26
LINT_IMG    := golangci/golangci-lint:latest
NEWMAN_IMG  := postman/newman:alpine
DOCKER_GO   := docker run --rm -v "$(CURDIR)/backend:/src" -v gomod:/go/pkg/mod -v gocache:/root/.cache/go-build -w /src
ADMIN_PASSWORD ?= $(shell grep '^ADMIN_PASSWORD=' .env 2>/dev/null | cut -d= -f2)

.PHONY: ayuda arriba abajo logs demo pruebas pruebas-integracion lint formato e2e

ayuda: ## Muestra esta ayuda
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "} {printf "  %-22s %s\n", $$1, $$2}'

arriba: ## Construye y levanta todo el sistema (requiere .env)
	docker compose up --build -d --wait

abajo: ## Detiene el sistema (conserva los datos)
	docker compose down

logs: ## Muestra los logs de la API y del worker
	docker compose logs -f api worker

demo: ## Carga los datos de demostración
	docker compose exec -T postgres psql -U $$(grep '^POSTGRES_USER=' .env | cut -d= -f2) -d $$(grep '^POSTGRES_DB=' .env | cut -d= -f2) < scripts/datos_demo.sql

pruebas: ## Pruebas unitarias con detector de condiciones de carrera
	$(DOCKER_GO) $(GO_IMG) go test -race ./...

pruebas-integracion: ## Pruebas unitarias y de integración contra un PostgreSQL temporal
	-docker network create red-pruebas >/dev/null 2>&1
	-docker rm -f pg-pruebas >/dev/null 2>&1
	docker run -d --name pg-pruebas --network red-pruebas -e POSTGRES_USER=pruebas -e POSTGRES_PASSWORD=pruebas -e POSTGRES_DB=pruebas postgres:16-alpine
	sleep 5
	$(DOCKER_GO) --network red-pruebas -e DATABASE_URL_PRUEBAS=postgres://pruebas:pruebas@pg-pruebas:5432/pruebas?sslmode=disable \
		$(GO_IMG) go test -race -tags integracion -cover ./... ; status=$$?; docker rm -f pg-pruebas >/dev/null; exit $$status

lint: ## Formato y análisis estático (golangci-lint)
	$(DOCKER_GO) $(LINT_IMG) sh -c 'test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1); golangci-lint run ./...'

formato: ## Aplica gofmt a todo el código
	$(DOCKER_GO) $(GO_IMG) gofmt -w .

e2e: ## Ejecuta la colección de Postman contra el sistema levantado
	docker run --rm -v "$(CURDIR)/postman:/etc/newman" $(NEWMAN_IMG) run seguimiento.postman_collection.json \
		--env-var "baseUrl=http://host.docker.internal:8080" --env-var "adminPassword=$(ADMIN_PASSWORD)"
