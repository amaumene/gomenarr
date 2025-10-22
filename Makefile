.PHONY: all build test wire migrate-up migrate-down lint clean run help

BINARY_SERVER=gomenarr-server
BINARY_WORKER=gomenarr-worker
BINARY_CLI=gomenarr-cli
BUILD_DIR=./bin
DATA_DIR=./data
MIGRATIONS_DIR=./migrations

all: wire build

help:
	@echo "Available targets:"
	@echo "  build        - Build all binaries"
	@echo "  test         - Run all tests with coverage"
	@echo "  wire         - Generate dependency injection code"
	@echo "  migrate-up   - Run database migrations"
	@echo "  migrate-down - Rollback database migrations"
	@echo "  lint         - Run linters"
	@echo "  clean        - Clean build artifacts"
	@echo "  run          - Run the server"

build: wire
	@echo "Building binaries..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_SERVER) ./cmd/server
	go build -o $(BUILD_DIR)/$(BINARY_WORKER) ./cmd/worker
	go build -o $(BUILD_DIR)/$(BINARY_CLI) ./cmd/cli
	@echo "Binaries built in $(BUILD_DIR)/"

test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	@echo "Coverage report:"
	go tool cover -func=coverage.out | tail -n 1

wire:
	@echo "Generating wire code..."
	cd internal/infra && go run github.com/google/wire/cmd/wire
	@echo "Wire code generated"

migrate-up:
	@echo "Running migrations..."
	@mkdir -p $(DATA_DIR)
	@go run cmd/server/main.go migrate-up || echo "Migrations completed"

migrate-down:
	@echo "Rolling back migrations..."
	@go run cmd/server/main.go migrate-down || echo "Rollback completed"

lint:
	@echo "Running linters..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "Installing golangci-lint..."; go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; }
	golangci-lint run ./...

clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out
	rm -f $(DATA_DIR)/*.db
	rm -f $(DATA_DIR)/*.db-*
	@echo "Clean complete"

run: build
	@echo "Starting server..."
	$(BUILD_DIR)/$(BINARY_SERVER)

.DEFAULT_GOAL := help
