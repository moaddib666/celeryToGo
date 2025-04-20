# Makefile for CeleryToGo

# Variables
BINARY_NAME=celeryToGo
GO=go
GOFLAGS=-v
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

# Directories
CMD_DIR=./cmd/worker
INTERNAL_DIR=./internal
PKG_DIR=./pkg
SCRIPTS_DIR=./scripts
EXAMPLES_DIR=./examples
TEST_DIR=./test
BIN_DIR=./bin

# Build targets
.PHONY: all build clean

all: clean build

build: build-linux build-darwin
	@echo "All builds completed!"

build-linux:
	@echo "Building $(BINARY_NAME) for Linux..."
	mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-amd64-linux $(CMD_DIR)
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-arm64-linux $(CMD_DIR)

build-darwin:
	@echo "Building $(BINARY_NAME) for macOS..."
	mkdir -p $(BIN_DIR)
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-amd64-darwin $(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-arm64-darwin $(CMD_DIR)

build-windows:
	@echo "Building $(BINARY_NAME) for Windows..."
	mkdir -p $(BIN_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-amd64-windows.exe $(CMD_DIR)

clean:
	@echo "Cleaning..."
	rm -rf $(BIN_DIR)
	mkdir -p $(BIN_DIR)

# Test targets
.PHONY: test test-unit test-integration test-coverage

test: test-unit test-integration

test-unit:
	@echo "Running unit tests..."
	$(GO) test -v ./internal/...

test-integration:
	@echo "Running integration tests..."
	$(GO) test -v ./test/integration/...

test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Lint targets
.PHONY: lint lint-fix

lint:
	@echo "Running linters..."
	golangci-lint run

lint-fix:
	@echo "Running linters and fixing issues..."
	golangci-lint run --fix

# Run targets
.PHONY: run run-local

run:
	@echo "Running $(BINARY_NAME)..."
	$(GO) run $(CMD_DIR) $(ARGS)

run-local:
	@echo "Running $(BINARY_NAME) with local SQS..."
	# Determine the appropriate binary based on current OS and architecture
	$(eval CURRENT_OS := $(shell go env GOOS))
	$(eval CURRENT_ARCH := $(shell go env GOARCH))
	$(eval BINARY_EXT := $(if $(filter windows,$(CURRENT_OS)),.exe,))
	./$(BIN_DIR)/$(BINARY_NAME)-$(CURRENT_ARCH)-$(CURRENT_OS)$(BINARY_EXT) --queue-url=http://localhost:9324/queue/test.fifo --endpoint=http://localhost:9324 $(ARGS)

# Docker targets
.PHONY: docker-build docker-run

docker-build:
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):$(VERSION) .

docker-run:
	@echo "Running Docker container..."
	docker run --rm -it $(BINARY_NAME):$(VERSION) $(ARGS)

# Development helpers
.PHONY: deps deps-update tidy

deps:
	@echo "Installing dependencies..."
	$(GO) mod download

deps-update:
	@echo "Updating dependencies..."
	$(GO) get -u ./...

tidy:
	@echo "Tidying dependencies..."
	$(GO) mod tidy

# Setup targets
.PHONY: setup setup-dev

setup: build
	@echo "Setting up directories..."
	mkdir -p $(EXAMPLES_DIR)
	mkdir -p $(TEST_DIR)/integration
	mkdir -p $(TEST_DIR)/e2e

setup-dev: setup
	@echo "Setting up development environment..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Help
.PHONY: help

help:
	@echo "CeleryToGo Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@echo "  build              Build for all platforms (Linux, macOS, Windows)"
	@echo "  build-linux        Build for Linux (amd64, arm64)"
	@echo "  build-darwin       Build for macOS (amd64, arm64)"
	@echo "  build-windows      Build for Windows (amd64)"
	@echo "  clean              Remove build artifacts"
	@echo "  test               Run all tests"
	@echo "  test-unit          Run unit tests"
	@echo "  test-integration   Run integration tests"
	@echo "  test-coverage      Run tests with coverage"
	@echo "  lint               Run linters"
	@echo "  lint-fix           Run linters and fix issues"
	@echo "  run                Run the application"
	@echo "  run-local          Run with local SQS"
	@echo "  docker-build       Build Docker image"
	@echo "  docker-run         Run Docker container"
	@echo "  deps               Install dependencies"
	@echo "  deps-update        Update dependencies"
	@echo "  tidy               Tidy dependencies"
	@echo "  setup              Set up directories"
	@echo "  setup-dev          Set up development environment"
	@echo "  help               Show this help"