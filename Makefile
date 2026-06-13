.PHONY: all build clean test run dev

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Binary parameters
BINARY_NAME=kubepilot
BINARY_PATH=./bin/$(BINARY_NAME)
MAIN_PATH=./cmd/server/main.go

# Build
all: test build

build:
	$(GOBUILD) -o $(BINARY_PATH) -v $(MAIN_PATH)

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_PATH) -v $(MAIN_PATH)

# Test
test:
	$(GOTEST) -v ./...

test-coverage:
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out

# Clean
clean:
	$(GOCLEAN)
	rm -rf ./bin

# Run
run: build
	$(BINARY_PATH)

dev:
	$(GOCMD) run $(MAIN_PATH)

# Dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Format
fmt:
	$(GOCMD) fmt ./...

# Lint
lint:
	golangci-lint run

# Docker
docker-build:
	docker build -t kubepilot:latest -f deploy/docker/Dockerfile .

docker-run:
	docker-compose -f deploy/docker/docker-compose.yml up -d

# Database
db-migrate:
	$(GOCMD) run ./scripts/migrate.go

# Help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all          - Run tests and build"
	@echo "  build        - Build the binary"
	@echo "  build-linux  - Build for Linux"
	@echo "  test         - Run tests"
	@echo "  test-coverage- Run tests with coverage"
	@echo "  clean        - Clean build artifacts"
	@echo "  run          - Build and run"
	@echo "  dev          - Run in development mode"
	@echo "  deps         - Download dependencies"
	@echo "  fmt          - Format code"
	@echo "  lint         - Lint code"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run   - Run with Docker Compose"
	@echo "  db-migrate   - Run database migrations"
	@echo "  help         - Show this help"
