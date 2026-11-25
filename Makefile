# Encomendo monorepo tasks. Each app also has its own tooling:
#   backend/Makefile, web/package.json scripts, mobile/gradlew.

COMPOSE := docker compose --env-file .env -f deploy/docker-compose.yml

.PHONY: help setup dev-up dev-down dev-logs test test-integration lint migrate api-types coverage

help:
	@echo "setup             create .env from .env.example and install web deps"
	@echo "dev-up            build and start postgres, redis, migrations, api and web"
	@echo "dev-down          stop the local stack"
	@echo "test              unit tests: backend, web, mobile"
	@echo "test-integration  backend integration tests (needs Docker)"
	@echo "lint              all linters"
	@echo "migrate           apply migrations to the local database"
	@echo "api-types         regenerate web API types from docs/api/openapi.yaml"

setup:
	@test -f .env || cp .env.example .env
	cd web && pnpm install

dev-up:
	@test -f .env || cp .env.example .env
	$(COMPOSE) up --build -d
	@echo "admin: http://admin.localhost:$${WEB_PORT:-8080}  store: http://altarpm.localhost:$${WEB_PORT:-8080}"

dev-down:
	$(COMPOSE) down

dev-logs:
	$(COMPOSE) logs -f api

test:
	$(MAKE) -C backend test
	cd web && pnpm -r test
	cd mobile && ./gradlew check

test-integration:
	$(MAKE) -C backend test-integration

coverage:
	$(MAKE) -C backend coverage

lint:
	$(MAKE) -C backend lint
	cd web && pnpm lint && pnpm -r typecheck
	cd mobile && ./gradlew check

migrate:
	$(COMPOSE) run --rm migrate up

api-types:
	cd web && pnpm api:types
