SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

.PHONY: help doctor bootstrap format format-check lint contracts migrations-check unit integration database-test authentication-test event-test adapters-test infrastructure-test full-stack-test e2e test build smoke smoke-backend smoke-frontend generate generate-check migrate-up migrate-down migrate-version infra-up infra-down infra-reset dev down check ci

help: ## Show the stable repository command surface.
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

doctor: ## Validate required local toolchains and print actionable failures.
	@./scripts/doctor.sh

bootstrap: doctor ## Install locked Go and JavaScript dependencies.
	@cd backend && go mod download
	@pnpm install --frozen-lockfile

format: ## Format all authored backend and frontend files.
	@cd backend && gofmt -w $$(find . -type f -name '*.go' -not -path './vendor/*')
	@pnpm --dir frontend format
	@pnpm --dir frontend exec prettier --write ../README.md ../docs ../contracts ../redocly.yaml ../package.json ../pnpm-workspace.yaml '../.github/**/*.yml' '../scripts/*.mjs'

format-check: ## Check formatting without modifying files.
	@./scripts/check-go-format.sh
	@pnpm --dir frontend format:check
	@pnpm --dir frontend exec prettier --check ../README.md ../docs ../contracts ../redocly.yaml ../package.json ../pnpm-workspace.yaml '../.github/**/*.yml' '../scripts/*.mjs'

lint: ## Run backend and frontend static analysis.
	@cd backend && go vet ./...
	@pnpm --dir frontend lint
	@pnpm --dir frontend typecheck

contracts: ## Validate OpenAPI and JSON Schema contracts.
	@pnpm contracts:lint

migrations-check: ## Validate migration naming and up/down pairing.
	@./scripts/validate-migrations.sh

unit: ## Run deterministic unit tests.
	@cd backend && go test ./...
	@pnpm --dir frontend test

integration: ## Run integration tests against isolated dependencies.
	@./scripts/test-database.sh
	@./scripts/test-authentication.sh
	@./scripts/test-events.sh
	@./scripts/test-adapters.sh
	@./scripts/test-infrastructure.sh
	@./scripts/test-full-stack.sh

database-test: ## Verify migrations, PostgreSQL pooling, and transaction behavior.
	@./scripts/test-database.sh

authentication-test: ## Verify Kratos sessions and application identity mapping.
	@./scripts/test-authentication.sh

event-test: ## Verify transactional outbox and JetStream delivery behavior.
	@./scripts/test-events.sh

adapters-test: ## Verify isolated Redis and SeaweedFS adapter behavior.
	@./scripts/test-adapters.sh

infrastructure-test: ## Start an isolated empty-volume stack and verify every dependency.
	@./scripts/test-infrastructure.sh

full-stack-test: ## Build the complete stack and prove correlated transaction-to-event delivery.
	@./scripts/test-full-stack.sh

e2e: ## Run critical browser journeys against built applications.
	@pnpm --dir frontend test:e2e

test: unit integration ## Run unit and integration tests.

build: ## Build production backend and frontend artifacts.
	@mkdir -p bin
	@cd backend && go build -trimpath -o ../bin/credar-api ./cmd/api
	@pnpm --dir frontend build

smoke: smoke-backend smoke-frontend ## Start each built application and probe its public surface.

smoke-backend: ## Start the backend process and probe its health endpoint.
	@./scripts/smoke-backend.sh

smoke-frontend: ## Start the frontend process and probe its shell.
	@./scripts/smoke-frontend.sh

generate: ## Regenerate contract-derived and database code.
	@cd backend && go generate ./...
	@pnpm --dir frontend generate

generate-check: generate ## Fail when committed generated files are stale.
	@git diff --exit-code -- contracts backend frontend

migrate-up: ## Apply all pending local database migrations.
	@docker compose -f deploy/compose.yaml run --rm migrate up

migrate-down: ## Revert one local database migration.
	@docker compose -f deploy/compose.yaml run --rm migrate down 1

migrate-version: ## Print the current local database migration version.
	@docker compose -f deploy/compose.yaml run --rm migrate version

infra-up: ## Start the local infrastructure and wait for dependency readiness.
	@docker compose -f deploy/compose.yaml up --detach --wait postgres nats redis seaweedfs kratos

infra-down: ## Stop local infrastructure while preserving named volumes.
	@docker compose -f deploy/compose.yaml down --remove-orphans

infra-reset: ## Stop local infrastructure and permanently remove its named volumes.
	@docker compose -f deploy/compose.yaml down --volumes --remove-orphans

dev: ## Build and start the complete local stack.
	@docker compose -f deploy/compose.yaml up --detach --build --wait

down: ## Stop the local stack without deleting persisted volumes.
	@docker compose -f deploy/compose.yaml down

check: format-check lint contracts migrations-check unit build ## Run fast local quality checks.

ci: check integration generate-check ## Run the non-browser continuous-integration gate.
