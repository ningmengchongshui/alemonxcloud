.PHONY: help go-check dev build test format lint frontend-dev frontend-build agent-build agent-test agent-config-check agent-install agent-restart agent-verify agent-deploy docker-build docker-run integration-up test-integration integration-down gateway-build gateway-config-check gateway-install gateway-restart gateway-verify gateway-deploy control-build control-config-check control-install control-restart control-verify control-deploy

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

agent-config-check: ## Validate Agent configuration without printing or changing secrets
	@test -s /etc/xcloud-agent.env || { echo "Agent configuration is missing or empty: /etc/xcloud-agent.env"; exit 2; }
	@grep -qE '^[[:space:]]*AGENT_ADDR=.+$$' /etc/xcloud-agent.env || { echo "Agent configuration lacks AGENT_ADDR"; exit 2; }
	@grep -qE '^[[:space:]]*XCLOUD_AGENT_TOKEN=.+$$' /etc/xcloud-agent.env || { echo "Agent configuration lacks XCLOUD_AGENT_TOKEN"; exit 2; }
	@grep -qE '^[[:space:]]*XCLOUD_INSTANCE_DATA_ROOT=.+$$' /etc/xcloud-agent.env || { echo "Agent configuration lacks XCLOUD_INSTANCE_DATA_ROOT"; exit 2; }
	@test -d /opt/xcloud-agent || { echo "Agent deployment directory is missing: /opt/xcloud-agent"; exit 2; }

agent-install: ## Replace only Agent binary and systemd unit after config validation
	install -m 0755 agent/xcloud-agent $(AGENT_BIN)
	install -m 0644 deploy/xcloud-agent.service /etc/systemd/system/xcloud-agent.service
	$(SYSTEMCTL) daemon-reload

agent-restart: ## Restart platform Agent without touching its configuration
	$(SYSTEMCTL) restart xcloud-agent

agent-verify: ## Verify platform Agent is active and returns its status document
	@$(SYSTEMCTL) is-active --quiet xcloud-agent && echo "service: active"
	@$(SYSTEMCTL) show xcloud-agent -p MainPID -p ActiveEnterTimestamp
	@echo "Run the authenticated /container/status request from docs/04-Agent节点.md to verify capabilities."

agent-deploy: ## Build, validate config, replace Agent binary and unit, then restart
	@$(MAKE) --no-print-directory agent-build
	@$(MAKE) --no-print-directory agent-config-check
	@$(MAKE) --no-print-directory agent-install
	@$(MAKE) --no-print-directory agent-restart
	@$(MAKE) --no-print-directory agent-verify

# Deployment targets intentionally never read, create, copy, replace or
# overwrite files. They only restart an already installed systemd service and
# inspect its state. Binary and unit-file delivery belong to the release flow.
gateway-build: go-check ## Build Gateway with the current Git revision
	cd gateway && $(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o xcloud-tunnel-gateway .
	./gateway/xcloud-tunnel-gateway version

gateway-config-check: ## Validate Gateway configuration without printing or changing secrets
	@test -s /etc/xcloud-tunnel/gateway.env || { echo "Gateway configuration is missing or empty: /etc/xcloud-tunnel/gateway.env"; exit 2; }
	@grep -qE '^[[:space:]]*MYSQL_DSN=.+$$' /etc/xcloud-tunnel/gateway.env || { echo "Gateway configuration lacks MYSQL_DSN"; exit 2; }
	@grep -qE '^[[:space:]]*SESSION_REDIS_URL=.+$$' /etc/xcloud-tunnel/gateway.env || { echo "Gateway configuration lacks SESSION_REDIS_URL"; exit 2; }
	@grep -qE '^[[:space:]]*GATEWAY_COMMAND_TOKEN=.+$$' /etc/xcloud-tunnel/gateway.env || { echo "Gateway configuration lacks GATEWAY_COMMAND_TOKEN"; exit 2; }
	@test -d /opt/xcloud-tunnel-gateway || { echo "Gateway deployment directory is missing: /opt/xcloud-tunnel-gateway"; exit 2; }

gateway-install: ## Replace only Gateway binary and systemd unit after config validation
	install -m 0755 gateway/xcloud-tunnel-gateway $(GATEWAY_BIN)
	install -m 0644 deploy/xcloud-tunnel-gateway.service /etc/systemd/system/xcloud-tunnel-gateway.service
	$(SYSTEMCTL) daemon-reload

gateway-restart: ## Restart Gateway without touching its configuration
	$(SYSTEMCTL) restart xcloud-tunnel-gateway

gateway-verify: ## Verify the installed Gateway revision, restart time and readiness
	@echo "installed: $$($(GATEWAY_BIN) version)"
	@$(SYSTEMCTL) is-active --quiet xcloud-tunnel-gateway && echo "service: active"
	@$(SYSTEMCTL) show xcloud-tunnel-gateway -p MainPID -p ActiveEnterTimestamp

gateway-deploy: ## Build, validate config, replace Gateway binary and unit, then restart
	@$(MAKE) --no-print-directory gateway-build
	@$(MAKE) --no-print-directory gateway-config-check
	@$(MAKE) --no-print-directory gateway-install
	@$(MAKE) --no-print-directory gateway-restart
	@$(MAKE) --no-print-directory gateway-verify

control-build: go-check ## Build xcloud-control with the current Git revision
	cd control && $(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o xcloud-control .
	./control/xcloud-control version

control-config-check: ## Validate Control configuration without printing or changing credentials
	@test -s /etc/xcloud-control/config.json || { echo "Control configuration is missing or empty: /etc/xcloud-control/config.json"; exit 2; }
	@grep -qE '"tunnelURL"[[:space:]]*:[[:space:]]*".+"' /etc/xcloud-control/config.json || { echo "Control configuration lacks tunnelURL"; exit 2; }
	@grep -qE '"enrollToken"[[:space:]]*:[[:space:]]*".+"|"credential"[[:space:]]*:[[:space:]]*".+' /etc/xcloud-control/config.json || { echo "Control configuration lacks enrollment token or registered credential"; exit 2; }
	@test -d /usr/local/bin || { echo "Control deployment directory is missing: /usr/local/bin"; exit 2; }

control-install: ## Atomically replace only Control binary and systemd unit after config validation
	./control/xcloud-control install --target $(CONTROL_BIN) --service /etc/systemd/system/xcloud-control.service
	$(SYSTEMCTL) daemon-reload

control-restart: ## Restart Control without touching its configuration
	$(SYSTEMCTL) restart xcloud-control

control-verify: ## Verify Control revision, restart time and latest readiness delivery
	@echo "installed: $$($(CONTROL_BIN) version)"
	@$(SYSTEMCTL) is-active --quiet xcloud-control && echo "service: active"
	@$(SYSTEMCTL) show xcloud-control -p MainPID -p ActiveEnterTimestamp
	@journalctl -u xcloud-control -n 80 --no-pager -l | grep -E 'runtime readiness (delivered|report delivery failed)|连接已断开' || true

control-deploy: ## Build, validate config, replace Control binary and unit, then restart
	@$(MAKE) --no-print-directory control-build
	@$(MAKE) --no-print-directory control-config-check
	@$(MAKE) --no-print-directory control-install
	@$(MAKE) --no-print-directory control-restart
	@$(MAKE) --no-print-directory control-verify

docker-build: ## Run the container image locally
	docker compose up -d --build
