.PHONY: help go-check dev build test format lint frontend-dev frontend-build agent-build agent-test agent-install agent-install-artifact agent-enable agent-restart agent-verify agent-deploy agent-deploy-artifact release-linux-amd64 release-linux-arm64 docker-build docker-run integration-up test-integration integration-down gateway-build gateway-install gateway-install-artifact gateway-enable gateway-restart gateway-verify gateway-deploy gateway-deploy-artifact control-build control-install control-install-artifact control-enable control-restart control-verify control-deploy control-deploy-artifact

VERSION ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo dev)
SYSTEMCTL ?= systemctl
GATEWAY_BIN ?= /opt/xcloud-tunnel-gateway/xcloud-tunnel-gateway
CONTROL_BIN ?= /usr/local/bin/xcloud-control
AGENT_BIN ?= /usr/local/bin/xcloud-agent
GATEWAY_ARTIFACT ?= dist/linux-amd64/xcloud-tunnel-gateway
CONTROL_ARTIFACT ?= dist/linux-amd64/xcloud-control
AGENT_ARTIFACT ?= dist/linux-amd64/xcloud-agent

.DEFAULT_GOAL := help

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

go-check: ## Verify that the Go compiler needed for source builds is installed
	@command -v go >/dev/null 2>&1 || { echo "Go is not installed. Build release artifacts on a build machine with 'make release-linux-amd64', copy dist/linux-amd64/ to this host, then run the matching *-deploy-artifact target."; exit 2; }

dev: ## Start the Go API server
	go run .

build: ## Build the production binary
	go build -o app .

test: ## Run Go tests
	go test ./...

format: ## Format Go files
	go fmt ./... 
	yarn --cwd frontend format

lint: ## Run Go vet
	go vet ./...

dev-fe: ## Start the Vite development server
	cd frontend && yarn dev

build-fe: ## Build the frontend into dist/
	cd frontend && yarn build

agent-build: go-check ## Build the bare-metal systemd agent
	cd agent && go build -ldflags "-X main.Version=$(VERSION)" -o xcloud-agent .

agent-run: ## Build the bare-metal systemd agent
	./agent/xcloud-agent --serve

agent-test: ## Run the bare-metal agent tests
	cd agent && go test ./...

agent-install: agent-build ## Replace platform Agent binary and unit, never its environment file
	install -d -m 0750 /var/lib/xcloud/instances
	install -m 0755 agent/xcloud-agent $(AGENT_BIN)
	install -m 0644 deploy/xcloud-agent.service /etc/systemd/system/xcloud-agent.service
	$(SYSTEMCTL) daemon-reload

agent-install-artifact: ## Install a prebuilt Linux Agent artifact, never its environment file
	@test -x "$(AGENT_ARTIFACT)" || { echo "Missing executable Agent artifact: $(AGENT_ARTIFACT)"; exit 2; }
	install -d -m 0750 /var/lib/xcloud/instances
	install -m 0755 "$(AGENT_ARTIFACT)" $(AGENT_BIN)
	install -m 0644 deploy/xcloud-agent.service /etc/systemd/system/xcloud-agent.service
	$(SYSTEMCTL) daemon-reload

agent-enable: ## Enable platform Agent after its environment file has been configured once
	$(SYSTEMCTL) enable xcloud-agent

agent-restart: ## Restart platform Agent without touching its configuration
	$(SYSTEMCTL) restart xcloud-agent

agent-verify: ## Verify platform Agent is active and returns its status document
	@$(SYSTEMCTL) is-active --quiet xcloud-agent && echo "service: active"
	@$(SYSTEMCTL) show xcloud-agent -p MainPID -p ActiveEnterTimestamp
	@echo "Run the authenticated /container/status request from docs/04-Agent节点.md to verify capabilities."

agent-deploy: agent-install agent-restart agent-verify ## Build, install, restart and verify platform Agent without changing config

agent-deploy-artifact: agent-install-artifact agent-restart agent-verify ## Install, restart and verify a prebuilt Agent without changing config

# The following deployment targets never read, create or overwrite .env files,
# /etc/xcloud-tunnel/gateway.env, /etc/xcloud-control/config.json, or tokens.
# Run them as root (for example: sudo make gateway-deploy) only after the
# corresponding service has been configured for the first time.
gateway-build: go-check ## Build Gateway with the current Git revision
	cd gateway && go build -trimpath -ldflags "-X main.version=$(VERSION)" -o xcloud-tunnel-gateway .
	./gateway/xcloud-tunnel-gateway version

gateway-install: gateway-build ## Replace Gateway binary and systemd unit, never its environment file
	install -d -m 0750 /opt/xcloud-tunnel-gateway /etc/xcloud-tunnel
	install -m 0755 gateway/xcloud-tunnel-gateway $(GATEWAY_BIN)
	install -m 0644 deploy/xcloud-tunnel-gateway.service /etc/systemd/system/xcloud-tunnel-gateway.service
	$(SYSTEMCTL) daemon-reload

gateway-install-artifact: ## Install a prebuilt Linux Gateway artifact, never its environment file
	@test -x "$(GATEWAY_ARTIFACT)" || { echo "Missing executable Gateway artifact: $(GATEWAY_ARTIFACT)"; exit 2; }
	install -d -m 0750 /opt/xcloud-tunnel-gateway /etc/xcloud-tunnel
	install -m 0755 "$(GATEWAY_ARTIFACT)" $(GATEWAY_BIN)
	install -m 0644 deploy/xcloud-tunnel-gateway.service /etc/systemd/system/xcloud-tunnel-gateway.service
	$(SYSTEMCTL) daemon-reload

gateway-enable: ## Enable Gateway after its environment file has been configured once
	$(SYSTEMCTL) enable xcloud-tunnel-gateway

gateway-restart: ## Restart Gateway without touching its configuration
	$(SYSTEMCTL) restart xcloud-tunnel-gateway

gateway-verify: ## Verify the installed Gateway revision, restart time and readiness
	@echo "installed: $$($(GATEWAY_BIN) version)"
	@$(SYSTEMCTL) is-active --quiet xcloud-tunnel-gateway && echo "service: active"
	@$(SYSTEMCTL) show xcloud-tunnel-gateway -p MainPID -p ActiveEnterTimestamp
	@curl -fsS http://127.0.0.1:13072/healthz >/dev/null && echo "health: ready"

gateway-deploy: gateway-install gateway-restart gateway-verify ## Build, install, restart and verify Gateway without changing config

gateway-deploy-artifact: gateway-install-artifact gateway-restart gateway-verify ## Install, restart and verify a prebuilt Gateway without changing config

control-build: go-check ## Build xcloud-control with the current Git revision
	cd control && go build -trimpath -ldflags "-X main.version=$(VERSION)" -o xcloud-control .
	./control/xcloud-control version

control-install: control-build ## Atomically replace Control binary and unit, never its config or environment file
	install -d -m 0700 /etc/xcloud-control /var/lib/xcloud-control/instances
	./control/xcloud-control install --target $(CONTROL_BIN) --service /etc/systemd/system/xcloud-control.service
	$(SYSTEMCTL) daemon-reload

control-install-artifact: ## Atomically install a prebuilt Linux Control artifact, never its config or environment file
	@test -x "$(CONTROL_ARTIFACT)" || { echo "Missing executable Control artifact: $(CONTROL_ARTIFACT)"; exit 2; }
	install -d -m 0700 /etc/xcloud-control /var/lib/xcloud-control/instances
	"$(CONTROL_ARTIFACT)" install --target $(CONTROL_BIN) --service /etc/systemd/system/xcloud-control.service
	$(SYSTEMCTL) daemon-reload

control-enable: ## Enable Control after config.json has been configured once
	$(SYSTEMCTL) enable xcloud-control

control-restart: ## Restart Control without touching its configuration
	$(SYSTEMCTL) restart xcloud-control

control-verify: ## Verify Control revision, restart time and latest readiness delivery
	@echo "installed: $$($(CONTROL_BIN) version)"
	@$(SYSTEMCTL) is-active --quiet xcloud-control && echo "service: active"
	@$(SYSTEMCTL) show xcloud-control -p MainPID -p ActiveEnterTimestamp
	@journalctl -u xcloud-control -n 80 --no-pager -l | grep -E 'runtime readiness (delivered|report delivery failed)|连接已断开' || true

control-deploy: control-install control-restart control-verify ## Build, install, restart and verify Control without changing config

control-deploy-artifact: control-install-artifact control-restart control-verify ## Install, restart and verify a prebuilt Control without changing config

release-linux-amd64: go-check ## Build Linux amd64 artifacts to dist/linux-amd64 for hosts without Go
	mkdir -p dist/linux-amd64
	cd gateway && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-X main.version=$(VERSION)" -o ../dist/linux-amd64/xcloud-tunnel-gateway .
	cd control && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-X main.version=$(VERSION)" -o ../dist/linux-amd64/xcloud-control .
	cd agent && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-X main.Version=$(VERSION)" -o ../dist/linux-amd64/xcloud-agent .
	@echo "Built Linux amd64 artifacts in dist/linux-amd64/ with revision $(VERSION)"

release-linux-arm64: go-check ## Build Linux arm64 artifacts to dist/linux-arm64 for hosts without Go
	mkdir -p dist/linux-arm64
	cd gateway && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-X main.version=$(VERSION)" -o ../dist/linux-arm64/xcloud-tunnel-gateway .
	cd control && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-X main.version=$(VERSION)" -o ../dist/linux-arm64/xcloud-control .
	cd agent && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-X main.Version=$(VERSION)" -o ../dist/linux-arm64/xcloud-agent .
	@echo "Built Linux arm64 artifacts in dist/linux-arm64/ with revision $(VERSION)"

docker-build: ## Run the container image locally
	docker compose up -d --build
