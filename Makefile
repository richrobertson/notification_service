SHELL := /bin/bash

APP_NAME := notification-platform
DB_URL := postgres://notification:notification@localhost:5432/notification_platform?sslmode=disable
MIGRATIONS_DIR := ./migrations

.PHONY: help dev-up dev-down dev-logs db-shell migrate-up migrate-reset run-api run-dispatcher run-webhook-worker run-email-worker test fmt lint docs-godoc

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
	@echo "  run-dispatcher     Run the dispatch router"
	@echo "  run-webhook-worker Run the webhook delivery worker"
	@echo "  run-email-worker   Run the email delivery worker"
	@echo "  test               Run Go tests"
	@echo "  fmt                Format Go files"
	@echo "  lint               Run go vet"
	@echo "  docs-godoc         Generate package API docs from Go doc comments"

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

test:
	go test ./...

fmt:
	@gofmt -w $$(find . -path ./vendor -prune -o -type f -name '*.go' -print)

lint:
	go vet ./...

docs-godoc:
	@mkdir -p docs/api
	@set -euo pipefail; \
	packages=$$(go list -f '{{if or .GoFiles .CgoFiles}}{{.ImportPath}}{{end}}' ./... | sed '/^$$/d' | sort); \
	for pkg in $$packages; do \
		name=$$(echo $$pkg | sed 's#github.com/richrobertson/notification-platform/##; s#^\./##; s#/#_#g'); \
		out="docs/api/$${name}.md"; \
		echo "Generating $$out"; \
		{ \
			echo "# $$pkg"; \
			echo ""; \
			echo "_Generated from Go doc comments. Run \`make docs-godoc\` to refresh._"; \
			echo ""; \
			echo '```text'; \
			go doc -all $$pkg; \
			echo '```'; \
		} > "$$out"; \
	done

run-webhook-worker:
	go run ./cmd/webhook_worker

run-email-worker:
	go run ./cmd/email_worker
