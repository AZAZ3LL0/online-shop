TAILWIND_VERSION ?= v3.4.17
TEMPL_VERSION    ?= v0.3.898
TAILWIND         ?= ./bin/tailwindcss

.PHONY: help tools generate css build test lint vet vuln gate run migrate seed up down

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "%-12s %s\n", $$1, $$2}'

tools: ## install the pinned code generators and linters
	go install github.com/a-h/templ/cmd/templ@$(TEMPL_VERSION)
	go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6
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
	golangci-lint run
	govulncheck ./...
	go test ./... -race

run: ## run the server against the local environment
	go run ./cmd/app serve

migrate: ## apply migrations
	go run ./cmd/app migrate

seed: ## reset the demo dataset
	go run ./cmd/app seed

up: ## start app, postgres and caddy
	docker compose -f docker/compose.yml up -d --build

down:
	docker compose -f docker/compose.yml down
