# GoFileBeam Makefile

.PHONY: all build clean test run deploy docker-build docker-run help

# Variables
BINARY_NAME=gofilebeam
VERSION=$(shell git describe --tags 2>/dev/null || echo "v0.1.0")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GO_VERSION=$(shell go version | awk '{print $$3}')
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.goVersion=$(GO_VERSION)"

# Default target
all: build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/gofilebeam
	@echo "Build complete: ./$(BINARY_NAME)"

# Build for production (stripped, optimized)
build-prod:
	@echo "Building production binary..."
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-s -w" -o $(BINARY_NAME) ./cmd/gofilebeam
	@echo "Production build complete: ./$(BINARY_NAME)"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -rf dist/
	rm -rf uploads/
	rm -f test_file.txt downloaded_* 2>/dev/null || true
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	go test ./... -v

# Run with race detector
test-race:
	@echo "Running tests with race detector..."
	go test ./... -race -v

# Run the application
run: build
	@echo "Starting $(BINARY_NAME)..."
	./$(BINARY_NAME)

# Run with development settings
run-dev:
	@echo "Starting in development mode..."
	GOFILEBEAM_PORT=3000 GOFILEBEAM_DEBUG=true go run ./cmd/gofilebeam

# Build and install as system service (requires root)
install: build-prod
	@echo "Installing as system service..."
	sudo ./deploy.sh install

# Uninstall system service (requires root)
uninstall:
	@echo "Uninstalling system service..."
	sudo ./deploy.sh uninstall

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t gofilebeam:latest .

# Run Docker container
docker-run: docker-build
	@echo "Running Docker container..."
	docker run -p 8080:8080 -v ./uploads:/uploads gofilebeam:latest

# Run integration tests
test-integration: build
	@echo "Running integration tests..."
	./test.sh

# Generate coverage report
coverage:
	@echo "Generating coverage report..."
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Vet code
vet:
	@echo "Checking code with vet..."
	go vet ./...

# Lint code
lint:
	@echo "Linting code..."
	@if command -v golangci-lint >/dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Check dependencies
deps:
	@echo "Checking dependencies..."
	go mod tidy
	go mod verify

# Show help
help:
	@echo "GoFileBeam Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  build          - Build the binary"
	@echo "  build-prod     - Build optimized production binary"
	@echo "  clean          - Clean build artifacts"
	@echo "  test           - Run tests"
	@echo "  test-race      - Run tests with race detector"
	@echo "  run            - Build and run"
	@echo "  run-dev        - Run with development settings"
	@echo "  install        - Install as system service (requires root)"
	@echo "  uninstall      - Uninstall system service (requires root)"
	@echo "  docker-build   - Build Docker image"
	@echo "  docker-run     - Build and run Docker container"
	@echo "  test-integration - Run integration tests"
	@echo "  coverage       - Generate coverage report"
	@echo "  fmt            - Format code"
	@echo "  vet            - Check code with vet"
	@echo "  lint           - Lint code (requires golangci-lint)"
	@echo "  deps           - Check and tidy dependencies"
	@echo "  help           - Show this help message"
	@echo ""
	@echo "Environment variables can be set in .env file or directly"
	@echo "Example: GOFILEBEAM_PORT=3000 make run"