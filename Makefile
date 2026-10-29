# Local development helpers. Runs the BFF and PostgreSQL via docker compose so the API can be
# exercised from Postman (or curl) at http://localhost:8080/graphql.

COMPOSE   ?= docker compose
BFF_URL   ?= http://localhost:8080
READY_TRIES ?= 60

.DEFAULT_GOAL := help

.PHONY: help up down restart rebuild logs ps wait clean psql

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'

up: ## Build and start the stack in the background, then wait until the BFF is ready
	$(COMPOSE) up --build --detach
	@$(MAKE) --no-print-directory wait
	@echo "GraphQL:    $(BFF_URL)/graphql  (POST, Content-Type: application/json)"
	@echo "Playground: $(BFF_URL)/playground"

wait: ## Poll /readyz until the BFF reports ready (database reachable, migrations applied)
	@echo "Waiting for $(BFF_URL)/readyz ..."
	@i=0; until curl -fs -o /dev/null $(BFF_URL)/readyz; do \
		i=$$((i+1)); \
		if [ $$i -ge $(READY_TRIES) ]; then echo "BFF not ready after $(READY_TRIES)s"; $(COMPOSE) logs --tail=50 bff; exit 1; fi; \
		sleep 1; \
	done; echo "BFF is ready."

down: ## Stop the stack (keeps the database volume)
	$(COMPOSE) down

restart: down up ## Stop and start the stack again

rebuild: ## Rebuild and restart only the BFF container after code changes
	$(COMPOSE) up --build --detach --no-deps bff
	@$(MAKE) --no-print-directory wait

logs: ## Follow the BFF logs
	$(COMPOSE) logs --follow bff

ps: ## Show container status
	$(COMPOSE) ps

psql: ## Open a psql shell in the database container
	$(COMPOSE) exec postgres psql -U miszacsi -d miszacsi

clean: ## Stop the stack and delete the database volume
	$(COMPOSE) down --volumes
