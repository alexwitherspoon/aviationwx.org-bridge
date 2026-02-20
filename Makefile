.PHONY: help build test fmt vet clean docker-build docker-up docker-down dev

# Get git commit SHA (short)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

export GIT_COMMIT

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the Go binary
	go build -o bin/bridge ./cmd/bridge

test: ## Run Go tests
	go test -v -race -coverprofile=coverage.out ./...

test-js: ## Run frontend JS tests
	node --test internal/web/static/js/form-utils.test.js

test-coverage: test ## Run tests with coverage report
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

fmt: ## Format code
	gofmt -s -w .

vet: ## Run go vet
	go vet ./...

lint: fmt vet ## Run all linters

clean: ## Clean build artifacts
	rm -rf bin/ coverage.out coverage.html

# Docker commands
docker-build: ## Build Docker image
	docker build -f docker/Dockerfile \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg VERSION=$(VERSION) \
		-t aviationwx-bridge:latest .

docker-up: ## Start Docker Compose
	docker compose -f docker/docker-compose.yml up -d

docker-down: ## Stop Docker Compose
	docker compose -f docker/docker-compose.yml down

docker-logs: ## Show Docker logs
	docker compose -f docker/docker-compose.yml logs -f

docker-restart: docker-down docker-up ## Restart Docker Compose

# Development
dev: ## Start local development environment with Docker
	@echo "🚀 Setting up AviationWX Bridge development environment..."
	@mkdir -p docker/data
	@if [ ! -f docker/data/config.json ]; then \
		echo '{"version":2,"timezone":"America/Chicago","cameras":[],"web_console":{"enabled":true,"port":1229,"password":"aviationwx"}}' > docker/data/config.json; \
		echo "✓ Created default config.json"; \
	fi
	@echo "📦 Building Docker image ($(GIT_COMMIT))..."
	docker build -f docker/Dockerfile \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg VERSION=$(VERSION) \
		-t aviationwx-bridge:latest .
	@echo "🔄 Starting container..."
	docker compose -f docker/docker-compose.yml up -d
	@echo ""
	@echo "╔══════════════════════════════════════════════════════════════╗"
	@echo "║           AviationWX Bridge - Development Mode               ║"
	@echo "╠══════════════════════════════════════════════════════════════╣"
	@echo "║                                                               ║"
	@echo "║  Web Console: http://localhost:1229                          ║"
	@echo "║  Password:    aviationwx                                      ║"
	@echo "║                                                               ║"
	@echo "║  Commands:                                                    ║"
	@echo "║    make docker-logs    - View logs                           ║"
	@echo "║    make docker-down    - Stop container                      ║"
	@echo "║    make dev            - Rebuild and restart                 ║"
	@echo "║                                                               ║"
	@echo "╚══════════════════════════════════════════════════════════════╝"

dev-clean: docker-down ## Stop and clean development environment
	rm -rf docker/data
	@echo "✓ Development environment cleaned"

check: fmt vet test test-js ## Run all checks (format, vet, Go tests, JS tests)
