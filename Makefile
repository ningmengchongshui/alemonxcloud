.PHONY: help dev build test format lint frontend-dev frontend-build agent-build agent-test docker-build docker-run

.DEFAULT_GOAL := help

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

dev: ## Start the Go API server
	go run ./src

build: ## Build the production binary
	go build -o app ./src

test: ## Run Go tests
	go test ./src

format: ## Format Go files
	go fmt ./...

lint: ## Run Go vet
	go vet ./...

frontend-dev: ## Start the Vite development server
	cd frontend && yarn dev

frontend-build: ## Build the frontend into dist/
	cd frontend && yarn build

agent-build: ## Build the bare-metal systemd agent
	cd agent && go build -o xcloud-agent .

agent-test: ## Run the bare-metal agent tests
	cd agent && go test ./...

docker-build: ## Build the container image
	docker build -t go-react-template:latest .

docker-run: ## Run the container image locally
	docker run --rm -p 8082:8082 go-react-template:latest
