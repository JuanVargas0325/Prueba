# Comandos habituales del proyecto. Usan Docker, así que no requieren Go instalado.
# En Windows puede usarlos desde Git Bash o WSL, o copiar el comando de cada objetivo.

.PHONY: ayuda arriba abajo logs

ayuda: ## Muestra esta ayuda
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "} {printf "  %-22s %s\n", $$1, $$2}'

arriba: ## Construye y levanta todo el sistema (requiere .env)
	docker compose up --build -d --wait

abajo: ## Detiene el sistema (conserva los datos)
	docker compose down

logs: ## Muestra los logs de la API
	docker compose logs -f api
