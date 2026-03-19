SHELL := /bin/bash

APP_NAME := notification-platform
DB_URL := postgres://notification:notification@localhost:5432/notification_platform?sslmode=disable
MIGRATIONS_DIR := ./migrations

.PHONY: help dev-up dev-down dev-logs db-shell migrate-up migrate-reset run-api run-dispatcher run-worker-email run-worker-webhook test fmt lint

help:
	@echo "$(APP_NAME) developer workflow"
	@echo ""
	@echo "Available targets:"
	@echo "  help               Show available targets"
	@echo "  dev-up             Start local infrastructure with Docker Compose"
	@echo "  dev-down           Stop local infrastructure"
	@echo "  dev-logs           Follow Docker Compose logs"
	@echo "  db-shell           Open a psql shell against the local database"
	@echo "  migrate-up         Apply SQL migrations in sorted order"
	@echo "  migrate-reset      Recreate public schema and reapply migrations"
	@echo "  run-api            Run the API service"
	@echo "  run-dispatcher     Run the dispatcher service"
	@echo "  run-worker-email   Run the email worker"
	@echo "  run-worker-webhook Run the webhook worker"
	@echo "  test               Run Go tests"
	@echo "  fmt                Format Go files"
	@echo "  lint               Run go vet"

dev-up:
	docker compose -f deployments/docker-compose.yml up -d

dev-down:
	docker compose -f deployments/docker-compose.yml down

dev-logs:
	docker compose -f deployments/docker-compose.yml logs -f

db-shell:
	psql "$(DB_URL)"

migrate-up:
	@set -euo pipefail; \
	shopt -s nullglob; \
	files=($(MIGRATIONS_DIR)/*.sql); \
	if [ $${#files[@]} -eq 0 ]; then \
		echo "No migration files found in $(MIGRATIONS_DIR)"; \
		exit 0; \
	fi; \
	IFS=$$'\n' files=($$(printf '%s\n' "$${files[@]}" | sort)); \
	unset IFS; \
	for file in "$${files[@]}"; do \
		echo "Applying migration: $$file"; \
		psql "$(DB_URL)" -v ON_ERROR_STOP=1 -f "$$file"; \
	done

migrate-reset:
	psql "$(DB_URL)" -v ON_ERROR_STOP=1 -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
	$(MAKE) migrate-up

run-api:
	go run ./cmd/api

run-dispatcher:
	go run ./cmd/dispatcher

run-worker-email:
	go run ./cmd/worker-email

run-worker-webhook:
	go run ./cmd/worker-webhook

test:
	go test ./...

fmt:
	@gofmt -w $$(find . -path ./vendor -prune -o -type f -name '*.go' -print)

lint:
	go vet ./...
