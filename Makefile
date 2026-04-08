# Amimica — Go Code Clone Detection Tool
# ========================================

# Build configuration
VERSION    ?= 0.1.0-dev
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GOBIN      := bin
BINARY     := $(GOBIN)/amimica
MODULE     := github.com/user/amimica
LDFLAGS    := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

# Tools
GOLANGCI_LINT := golangci-lint
GO            := go

# Colors (when stdout is a terminal)
BOLD   := $(shell tput bold 2>/dev/null || true)
RESET  := $(shell tput sgr0 2>/dev/null || true)
GREEN  := $(shell tput setaf 2 2>/dev/null || true)
YELLOW := $(shell tput setaf 3 2>/dev/null || true)
CYAN   := $(shell tput setaf 6 2>/dev/null || true)

# ─── Default target ──────────────────────────────────────────────
.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help message
	@echo ""
	@echo "$(BOLD)Amimica$(RESET) — Go Code Clone Detection Tool"
	@echo ""
	@echo "$(BOLD)Usage:$(RESET)"
	@echo "  make $(CYAN)<target>$(RESET)"
	@echo ""
	@echo "$(BOLD)Targets:$(RESET)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  $(CYAN)%-16s$(RESET) %s\n", $$1, $$2}'
	@echo ""

# ─── Build ───────────────────────────────────────────────────────
.PHONY: build
build: ## Build the amimica binary to bin/amimica
	@mkdir -p $(GOBIN)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/amimica
	@echo "$(GREEN)Built:$(RESET) $(BINARY) ($(VERSION), $(COMMIT))"

.PHONY: install
install: ## Install amimica to GOPATH/bin
	$(GO) install -ldflags "$(LDFLAGS)" ./cmd/amimica
	@echo "$(GREEN)Installed:$(RESET) amimica"

# ─── Run ─────────────────────────────────────────────────────────
.PHONY: run
run: build ## Build and run (pass ARGS="scan ." to set args)
	./$(BINARY) $(ARGS)

.PHONY: version
version: build ## Print version info
	./$(BINARY) version

# ─── Test ────────────────────────────────────────────────────────
.PHONY: test
test: ## Run all tests with race detection
	$(GO) test ./... -race -count=1

.PHONY: test-v
test-v: ## Run all tests verbose
	$(GO) test ./... -race -count=1 -v

.PHONY: test-cover
test-cover: ## Run tests with coverage report
	@mkdir -p $(GOBIN)
	$(GO) test ./... -race -count=1 -coverprofile=$(GOBIN)/coverage.out
	$(GO) tool cover -func=$(GOBIN)/coverage.out
	@echo ""
	@echo "$(CYAN)HTML report:$(RESET) make test-cover-html"

.PHONY: test-cover-html
test-cover-html: test-cover ## Open coverage report in browser
	$(GO) tool cover -html=$(GOBIN)/coverage.out -o $(GOBIN)/coverage.html
	@echo "$(GREEN)Coverage report:$(RESET) $(GOBIN)/coverage.html"
	@open $(GOBIN)/coverage.html 2>/dev/null || xdg-open $(GOBIN)/coverage.html 2>/dev/null || true

.PHONY: bench
bench: ## Run all benchmarks
	$(GO) test -bench=. -benchmem ./...

# ─── Code Quality ────────────────────────────────────────────────
.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: lint
lint: ## Run golangci-lint
	$(GOLANGCI_LINT) run ./...

.PHONY: fmt
fmt: ## Format all Go source files
	$(GO) fmt ./...
	@echo "$(GREEN)Formatted$(RESET)"

.PHONY: check
check: fmt vet test ## Run fmt, vet, and test (lint requires golangci-lint)

# ─── Dependencies ────────────────────────────────────────────────
.PHONY: tidy
tidy: ## Tidy and verify module dependencies
	$(GO) mod tidy
	$(GO) mod verify
	@echo "$(GREEN)Modules tidy and verified$(RESET)"

# ─── Doctor ──────────────────────────────────────────────────────
.PHONY: doctor
doctor: ## Check dev environment and tool versions
	@echo ""
	@echo "$(BOLD)Amimica Doctor$(RESET)"
	@echo "──────────────────────────────────────"
	@printf "  %-20s " "Go:" ; $(GO) version 2>/dev/null || echo "$(YELLOW)NOT FOUND$(RESET)"
	@printf "  %-20s " "golangci-lint:" ; $(GOLANGCI_LINT) version --short 2>/dev/null || echo "$(YELLOW)NOT FOUND — https://golangci-lint.run/usage/install/$(RESET)"
	@printf "  %-20s " "Module:" ; head -1 go.mod | awk '{print $$2}'
	@printf "  %-20s " "Git:" ; git --version 2>/dev/null || echo "$(YELLOW)NOT FOUND$(RESET)"
	@printf "  %-20s " "Branch:" ; git branch --show-current 2>/dev/null || echo "$(YELLOW)not in repo$(RESET)"
	@printf "  %-20s " "Working tree:" ; git diff --quiet 2>/dev/null && echo "clean" || echo "$(YELLOW)dirty$(RESET)"
	@echo "──────────────────────────────────────"
	@printf "  %-20s " "Build:" ; $(GO) build ./... 2>/dev/null && echo "$(GREEN)OK$(RESET)" || echo "$(YELLOW)FAIL$(RESET)"
	@printf "  %-20s " "Vet:" ; $(GO) vet ./... 2>/dev/null && echo "$(GREEN)OK$(RESET)" || echo "$(YELLOW)FAIL$(RESET)"
	@printf "  %-20s " "Tests:" ; $(GO) test ./... -count=1 > /dev/null 2>&1 && echo "$(GREEN)OK$(RESET)" || echo "$(YELLOW)FAIL$(RESET)"
	@echo "──────────────────────────────────────"
	@echo ""

# ─── Clean ───────────────────────────────────────────────────────
.PHONY: clean
clean: ## Remove build artifacts and caches
	rm -rf $(GOBIN)
	$(GO) clean -cache -testcache
	@echo "$(GREEN)Cleaned$(RESET)"

# ─── CI ──────────────────────────────────────────────────────────
.PHONY: ci
ci: fmt vet lint test build ## Full CI: fmt, vet, lint, test, build
	@echo "$(GREEN)CI passed$(RESET)"

# ─── All ─────────────────────────────────────────────────────────
.PHONY: all
all: clean check build ## Clean, check, and build everything
