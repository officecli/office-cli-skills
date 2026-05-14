APP := officecli
PREFIX ?= /usr/local
BIN_DIR ?= $(PREFIX)/bin
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
DIST_DIR ?= dist
CLI_EMBEDDED_PUBLISH_BASE_URL ?=
CLI_EMBEDDED_PUBLISH_AUTH_KEY_ID ?=
CLI_EMBEDDED_PUBLISH_AUTH_KEY ?=
LDFLAGS := -X github.com/officecli/officecli-internal/internal/cli.Version=$(VERSION) -X github.com/officecli/officecli-internal/internal/cli.Commit=$(COMMIT) -X github.com/officecli/officecli-internal/internal/cli.BuildDate=$(BUILD_DATE) -X github.com/officecli/officecli-internal/internal/providers/publish.EmbeddedPublishBaseURL=$(CLI_EMBEDDED_PUBLISH_BASE_URL) -X github.com/officecli/officecli-internal/internal/providers/publish.EmbeddedPublishAuthKeyID=$(CLI_EMBEDDED_PUBLISH_AUTH_KEY_ID) -X github.com/officecli/officecli-internal/internal/providers/publish.EmbeddedPublishAuthKey=$(CLI_EMBEDDED_PUBLISH_AUTH_KEY)

.PHONY: help build test test-fast test-full test-smoke test-local fmt install uninstall run-help demo release release-darwin-amd64 release-darwin-arm64 release-linux-amd64 release-linux-arm64 demo-ppt demo-docx demo-xlsx usage-limits-smoke

help:
	@echo "Available targets:"
	@echo "  make build     - Build the officecli binary"
	@echo "  make test      - Run all Go tests"
	@echo "  make test-fast - Run the fast local regression flow"
	@echo "  make test-full - Run the full local regression flow"
	@echo "  make test-smoke - Run the local smoke flow against a running platform"
	@echo "  make test-local - Run fast, full, then smoke local flows"
	@echo "  make fmt       - Format Go sources"
	@echo "  make init-config - Generate the first-use config template"
	@echo "  make install   - Install the binary into $(BIN_DIR)"
	@echo "  make uninstall - Remove the installed binary from $(BIN_DIR)"
	@echo "  make run-help  - Show CLI help"
	@echo "  make demo      - Run the local demo script"
	@echo "  make usage-limits-smoke - Run usage limits smoke checks against local platform"
	@echo "  make release   - Build local release artifacts into $(DIST_DIR)"
	@echo "  make demo-ppt  - Generate a PPTX using examples/prompt.txt"
	@echo "  make demo-docx - Generate a DOCX using examples/docx-prompt.txt"
	@echo "  make demo-xlsx - Generate a XLSX using examples/xlsx-prompt.txt"

build:
	go build -ldflags "$(LDFLAGS)" -o $(APP) ./cmd/officecli

test:
	go test ./...

test-fast:
	bash ./scripts/run-local-test-flow.sh fast

test-full:
	bash ./scripts/run-local-test-flow.sh full

test-smoke:
	bash ./scripts/run-local-test-flow.sh smoke

test-local:
	bash ./scripts/run-local-test-flow.sh local

fmt:
	gofmt -w cmd/officecli/main.go internal/cli/*.go internal/providers/llm/*.go internal/providers/publish/*.go internal/runtime/*.go

init-config: build
	./$(APP) init

install: build
	mkdir -p $(BIN_DIR)
	cp ./$(APP) $(BIN_DIR)/$(APP)
	chmod +x $(BIN_DIR)/$(APP)
	@echo "Installed to $(BIN_DIR)/$(APP)"

uninstall:
	rm -f $(BIN_DIR)/$(APP)
	@echo "Removed $(BIN_DIR)/$(APP)"

run-help: build
	./$(APP) --help

demo: build
	bash ./scripts/demo.sh ./$(APP)

release: release-darwin-amd64 release-darwin-arm64 release-linux-amd64 release-linux-arm64

release-darwin-amd64:
	mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)_$(VERSION)_darwin_amd64 ./cmd/officecli

release-darwin-arm64:
	mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)_$(VERSION)_darwin_arm64 ./cmd/officecli

release-linux-amd64:
	mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)_$(VERSION)_linux_amd64 ./cmd/officecli

release-linux-arm64:
	mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)_$(VERSION)_linux_arm64 ./cmd/officecli

demo-ppt: build
	./$(APP) new pptx "Enterprise Collaboration Platform Overview" --prompt-file ./examples/prompt.txt

demo-docx: build
	./$(APP) new docx "Quarterly Retrospective" --prompt-file ./examples/docx-prompt.txt

demo-xlsx: build
	./$(APP) new xlsx "Sales Analysis Workbook" --prompt-file ./examples/xlsx-prompt.txt

usage-limits-smoke:
	bash ./scripts/usage-limits-smoke.sh
