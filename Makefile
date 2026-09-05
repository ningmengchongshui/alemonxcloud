.PHONY: help dev build test format lint frontend-dev frontend-build agent-build agent-test docker-build docker-run integration-up test-integration integration-down

VERSION ?= dev

.DEFAULT_GOAL := help

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

dev: ## Start the Go API server
	go run .

build: ## Build the production binary
	go build -o app .

test: ## Run Go tests
	go test ./...

format: ## Format Go files
	go fmt ./...

lint: ## Run Go vet
	go vet ./...

dev-fe: ## Start the Vite development server
	cd frontend && yarn dev

build-fe: ## Build the frontend into dist/
	cd frontend && yarn build

agent-build: ## Build the bare-metal systemd agent
	cd agent && go build -ldflags "-X main.Version=$(VERSION)" -o xcloud-agent .

agent-run: ## Build the bare-metal systemd agent
	./agent/xcloud-agent --serve

agent-test: ## Run the bare-metal agent tests
	cd agent && go test ./...

docker-build: ## Build the container image
	@if [ -n "$$GITHUB_TOKEN" ]; then \
		docker build --secret id=github_token,env=GITHUB_TOKEN -t alemonxcloud:latest .; \
	else \
		docker build -t alemonxcloud:latest .; \
	fi

docker-run: ## Run the container image locally
	docker run --rm -p 8082:8082 alemonxcloud:latest

integration-up: ## Start the isolated MySQL, Redis, RabbitMQ and Agent test stack
	docker compose -f compose.integration.yml up -d --build --wait

test-integration: ## Run opt-in integration tests against the isolated stack
	XCLOUD_INTEGRATION_MYSQL_DSN='xcloud:xcloud-test-only@tcp(127.0.0.1:13306)/xcloud_integration?parseTime=true' go test -tags=integration ./src/...

integration-down: ## Stop and remove only the xcloud-integration test stack and volumes
	docker compose -f compose.integration.yml down --volumes --remove-orphans
