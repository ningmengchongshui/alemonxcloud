.PHONY: help go-check dev build test format lint frontend-dev frontend-build agent-build agent-test agent-preflight agent-restart agent-verify agent-deploy docker-build docker-run integration-up test-integration integration-down gateway-build gateway-preflight gateway-restart gateway-verify gateway-deploy control-build control-preflight control-restart control-verify control-deploy

VERSION ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo dev)
GO ?= go
SYSTEMCTL ?= systemctl
GATEWAY_BIN ?= /opt/xcloud-tunnel-gateway/xcloud-tunnel-gateway
CONTROL_BIN ?= /usr/local/bin/xcloud-control
AGENT_BIN ?= /opt/xcloud-agent/xcloud-agent

.DEFAULT_GOAL := help

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

go-check: ## Verify that the Go compiler needed for source builds is installed
	@command -v "$(GO)" >/dev/null 2>&1 || { echo "Go compiler '$(GO)' was not found in this shell. When already logged in as root, run 'make gateway-deploy' directly instead of 'sudo make gateway-deploy'."; exit 2; }

dev: ## Start the Go API server
	go run .

build: ## Build the production binary
	$(GO) build -o app .

test: ## Run Go tests
	$(GO) test ./...

format: ## Format Go files
	$(GO) fmt ./...
	yarn --cwd frontend format

lint: ## Run Go vet
	$(GO) vet ./...

dev-fe: ## Start the Vite development server
	cd frontend && yarn dev

build-fe: ## Build the frontend into dist/
	cd frontend && yarn build

agent-build: go-check ## Build the bare-metal systemd agent
	cd agent && $(GO) build -ldflags "-X main.Version=$(VERSION)" -o xcloud-agent .

agent-run: ## Build the bare-metal systemd agent
	./agent/xcloud-agent --serve

agent-test: ## Run the bare-metal agent tests
	cd agent && $(GO) test ./...

agent-preflight: agent-build ## Refuse Agent restart unless deployed files exactly match this build
	@test -r /etc/xcloud-agent.env || { echo "Agent configuration is missing: /etc/xcloud-agent.env"; exit 2; }
	@test -x "$(AGENT_BIN)" || { echo "Deployed Agent binary is missing: $(AGENT_BIN)"; exit 2; }
	@cmp -s agent/xcloud-agent "$(AGENT_BIN)" || { echo "Deployed Agent binary differs from this build; release it using your external file-delivery process, then retry. Nothing was restarted."; exit 2; }
	@test -r /etc/systemd/system/xcloud-agent.service || { echo "Agent systemd unit is missing"; exit 2; }
	@cmp -s deploy/xcloud-agent.service /etc/systemd/system/xcloud-agent.service || { echo "Agent systemd unit differs from the reviewed deployment unit; reconcile it manually, then retry. Nothing was restarted."; exit 2; }

agent-restart: ## Restart platform Agent without touching its configuration
	$(SYSTEMCTL) restart xcloud-agent

agent-verify: ## Verify platform Agent is active and returns its status document
	@$(SYSTEMCTL) is-active --quiet xcloud-agent && echo "service: active"
	@$(SYSTEMCTL) show xcloud-agent -p MainPID -p ActiveEnterTimestamp
	@echo "Run the authenticated /container/status request from docs/04-Agent节点.md to verify capabilities."

agent-deploy: agent-preflight agent-restart agent-verify ## Build, compare deployed files, then restart Agent only when they match

# Deployment targets intentionally never read, create, copy, replace or
# overwrite files. They only restart an already installed systemd service and
# inspect its state. Binary and unit-file delivery belong to the release flow.
gateway-build: go-check ## Build Gateway with the current Git revision
	cd gateway && $(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o xcloud-tunnel-gateway .
	./gateway/xcloud-tunnel-gateway version

gateway-preflight: gateway-build ## Refuse Gateway restart unless deployed files exactly match this build
	@test -r /etc/xcloud-tunnel/gateway.env || { echo "Gateway configuration is missing: /etc/xcloud-tunnel/gateway.env"; exit 2; }
	@test -x "$(GATEWAY_BIN)" || { echo "Deployed Gateway binary is missing: $(GATEWAY_BIN)"; exit 2; }
	@cmp -s gateway/xcloud-tunnel-gateway "$(GATEWAY_BIN)" || { echo "Deployed Gateway binary differs from this build; release it using your external file-delivery process, then retry. Nothing was restarted."; exit 2; }
	@test -r /etc/systemd/system/xcloud-tunnel-gateway.service || { echo "Gateway systemd unit is missing"; exit 2; }
	@cmp -s deploy/xcloud-tunnel-gateway.service /etc/systemd/system/xcloud-tunnel-gateway.service || { echo "Gateway systemd unit differs from the reviewed deployment unit; reconcile it manually, then retry. Nothing was restarted."; exit 2; }

gateway-restart: ## Restart Gateway without touching its configuration
	$(SYSTEMCTL) restart xcloud-tunnel-gateway

gateway-verify: ## Verify the installed Gateway revision, restart time and readiness
	@echo "installed: $$($(GATEWAY_BIN) version)"
	@$(SYSTEMCTL) is-active --quiet xcloud-tunnel-gateway && echo "service: active"
	@$(SYSTEMCTL) show xcloud-tunnel-gateway -p MainPID -p ActiveEnterTimestamp

gateway-deploy: gateway-preflight gateway-restart gateway-verify ## Build, compare deployed files, then restart Gateway only when they match

control-build: go-check ## Build xcloud-control with the current Git revision
	cd control && $(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o xcloud-control .
	./control/xcloud-control version

control-preflight: control-build ## Refuse Control restart unless deployed files exactly match this build
	@test -r /etc/xcloud-control/config.json || { echo "Control configuration is missing: /etc/xcloud-control/config.json"; exit 2; }
	@test -x "$(CONTROL_BIN)" || { echo "Deployed Control binary is missing: $(CONTROL_BIN)"; exit 2; }
	@cmp -s control/xcloud-control "$(CONTROL_BIN)" || { echo "Deployed Control binary differs from this build; release it using your external file-delivery process, then retry. Nothing was restarted."; exit 2; }
	@test -r /etc/systemd/system/xcloud-control.service || { echo "Control systemd unit is missing"; exit 2; }
	@cmp -s deploy/xcloud-control.service /etc/systemd/system/xcloud-control.service || { echo "Control systemd unit differs from the reviewed deployment unit; reconcile it manually, then retry. Nothing was restarted."; exit 2; }

control-restart: ## Restart Control without touching its configuration
	$(SYSTEMCTL) restart xcloud-control

control-verify: ## Verify Control revision, restart time and latest readiness delivery
	@echo "installed: $$($(CONTROL_BIN) version)"
	@$(SYSTEMCTL) is-active --quiet xcloud-control && echo "service: active"
	@$(SYSTEMCTL) show xcloud-control -p MainPID -p ActiveEnterTimestamp
	@journalctl -u xcloud-control -n 80 --no-pager -l | grep -E 'runtime readiness (delivered|report delivery failed)|连接已断开' || true

control-deploy: control-preflight control-restart control-verify ## Build, compare deployed files, then restart Control only when they match

docker-build: ## Run the container image locally
	docker compose up -d --build
