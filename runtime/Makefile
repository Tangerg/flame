# Run from this directory so configuration resolves relative to ./config.
# The terminal owns the foreground server and sends Ctrl-C through its normal
# shutdown protocol. A port or pidfile is not proof of process/data ownership.

PORT ?= 17171
BIN := build/flame
PKG := ./cmd/flame
HEALTH := http://127.0.0.1:$(PORT)/v2/health/ready

FLAME_HOME ?= $(HOME)/.flame
export FLAME_HOME

.DEFAULT_GOAL := help
.PHONY: help build run health clean

help: ## List targets
	@awk -F':.*?## ' '/^[a-z-]+:.*## / {printf "  %-9s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the Runtime distribution and first-party packages
	node ../plugins/build.mjs --os "$$(go env GOOS)" --arch "$$(go env GOARCH)" --output "$(abspath build/plugins)" --replace
	go build -o "$(BIN)" $(PKG)

run: build ## Run in the foreground; Ctrl-C requests graceful shutdown
	./$(BIN)

health: ## Query the configured development port
	curl -fsS --max-time 2 "$(HEALTH)"

clean: ## Remove the compiled binary
	rm -f "$(BIN)"
