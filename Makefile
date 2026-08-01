TAILWIND_VERSION ?= v3.4.17
TEMPL_VERSION    ?= v0.3.898
GOLANGCI_VERSION ?= v2.1.6
TAILWIND         ?= ./bin/tailwindcss

# 5432 and 5433 are taken by other projects on this machine, so the dev database
# is published on 5434. The compose stack still publishes no database port.
DEV_DB_NAME      ?= qzq-dev-db
DEV_DB_PORT      ?= 5434

.PHONY: help tools generate css build test lint vet vuln gate run migrate seed db-dev db-dev-stop up down logs backup set-webhook

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "%-12s %s\n", $$1, $$2}'

tools: ## install the pinned code generators and linters
	go install github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION)
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_VERSION)
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/pressly/goose/v3/cmd/goose@latest

generate: ## regenerate sqlc queries and templ components
	sqlc generate
	templ generate

css: ## build the Tailwind bundle
	$(TAILWIND) -c tailwind.config.js -i web/static/css/input.css -o web/static/css/app.css --minify

build: generate css ## build the binary
	go build -trimpath -o bin/app ./cmd/app

vet:
	go vet ./...

lint:
	golangci-lint config verify
	golangci-lint run

vuln:
	govulncheck ./...

test:
	go test ./... -race

gate: ## the same checks the pull request gate runs
	@test -z "$$(gofmt -l . | grep -v '_templ.go')" || (echo "gofmt:"; gofmt -l . | grep -v '_templ.go'; exit 1)
	templ generate
	@git diff --exit-code -- '*_templ.go' || (echo "templ output is stale, commit it"; exit 1)
	go vet ./...
	golangci-lint config verify
	golangci-lint run
	govulncheck ./...
	go test ./... -race

# The binary reads plain environment variables, so the local targets load .env
# themselves. Inside docker it is compose that supplies them.
LOCAL_ENV = set -a; . ./.env; set +a;

run: ## run the server against the local environment
	$(LOCAL_ENV) go run ./cmd/app serve

migrate: ## apply migrations
	$(LOCAL_ENV) go run ./cmd/app migrate

seed: ## reset the demo dataset
	$(LOCAL_ENV) go run ./cmd/app seed

db-dev: ## start the throwaway postgres the local binary talks to
	docker run -d --rm --name $(DEV_DB_NAME) \
		-e POSTGRES_USER=app -e POSTGRES_PASSWORD=app -e POSTGRES_DB=shop \
		-p $(DEV_DB_PORT):5432 postgres:16-alpine
	@echo "waiting for postgres on $(DEV_DB_PORT)"
	@until docker exec $(DEV_DB_NAME) pg_isready -U app -d shop >/dev/null 2>&1; do sleep 1; done
	@echo "ready: postgres://app:app@localhost:$(DEV_DB_PORT)/shop"

db-dev-stop: ## drop the throwaway dev database
	-docker rm -f $(DEV_DB_NAME)

# --env-file makes compose interpolate SITE_ADDRESS, APP_BASE_URL and
# POSTGRES_PASSWORD from the same .env the app itself reads, instead of from a
# second file next to docker/compose.yml.
COMPOSE = docker compose --env-file .env -f docker/compose.yml

up: ## start app, postgres and caddy
	$(COMPOSE) up -d --build

down:
	$(COMPOSE) down

logs: ## follow the application log
	$(COMPOSE) logs -f app

backup: ## take a database dump right now, outside the cron schedule
	./scripts/backup-db.sh

set-webhook: ## point the telegram bot at APP_BASE_URL (production only)
	$(LOCAL_ENV) go run ./cmd/app set-webhook
