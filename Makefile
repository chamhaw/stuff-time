# Makefile for stuff-time
# A screenshot-based time tracking agent

# Variables
BINARY_NAME=stuff-time
MAIN_PACKAGE=./cmd/stuff-time
BIN_DIR=./bin
BUILD_DIR=./build
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Go build flags
GO_BUILD_FLAGS=-trimpath
GO_TEST_FLAGS=-v -race -coverprofile=coverage.out

# Default target
.DEFAULT_GOAL := build

# Build the binary (development build)
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BIN_DIR)
	@go build $(GO_BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)
	@echo "Build complete: $(BIN_DIR)/$(BINARY_NAME)"

# Build with optimizations (release build)
.PHONY: build-release
build-release:
	@echo "Building $(BINARY_NAME) (release)..."
	@mkdir -p $(BIN_DIR)
	@go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME) $(MAIN_PACKAGE)
	@echo "Release build complete: $(BIN_DIR)/$(BINARY_NAME)"

# Build for macOS (current platform)
.PHONY: build-darwin
build-darwin:
	@echo "Building $(BINARY_NAME) for macOS..."
	@GOOS=darwin GOARCH=arm64 go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PACKAGE)
	@GOOS=darwin GOARCH=amd64 go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PACKAGE)
	@echo "macOS builds complete in $(BUILD_DIR)/"

# Build for Linux
.PHONY: build-linux
build-linux:
	@echo "Building $(BINARY_NAME) for Linux..."
	@GOOS=linux GOARCH=amd64 go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PACKAGE)
	@GOOS=linux GOARCH=arm64 go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PACKAGE)
	@echo "Linux builds complete in $(BUILD_DIR)/"

# Build for Windows
.PHONY: build-windows
build-windows:
	@echo "Building $(BINARY_NAME) for Windows..."
	@GOOS=windows GOARCH=amd64 go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PACKAGE)
	@GOOS=windows GOARCH=arm64 go build $(GO_BUILD_FLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-arm64.exe $(MAIN_PACKAGE)
	@echo "Windows builds complete in $(BUILD_DIR)/"

# Build for all platforms
.PHONY: build-all
build-all: build-darwin build-linux build-windows
	@echo "All platform builds complete in $(BUILD_DIR)/"

# Install to system (requires sudo for /usr/local/bin)
.PHONY: install
install: build-release
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	@sudo cp $(BIN_DIR)/$(BINARY_NAME) /usr/local/bin/
	@sudo chmod +x /usr/local/bin/$(BINARY_NAME)
	@echo "Installation complete. Run '$(BINARY_NAME) --help' to verify."

# Install to user directory (no sudo required)
.PHONY: install-user
install-user: build-release
	@echo "Installing $(BINARY_NAME) to ~/bin..."
	@mkdir -p ~/bin
	@cp $(BIN_DIR)/$(BINARY_NAME) ~/bin/
	@chmod +x ~/bin/$(BINARY_NAME)
	@echo "Installation complete. Add ~/bin to your PATH if not already done."
	@echo "Run '$(BINARY_NAME) --help' to verify."

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	@go test $(GO_TEST_FLAGS) ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage: test
	@echo "Generating coverage report..."
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Run linter (requires golangci-lint)
.PHONY: lint
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Download dependencies
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "Dependencies updated."

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BIN_DIR)
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "Clean complete."

# Clean all (including generated files)
.PHONY: clean-all
clean-all: clean
	@echo "Cleaning all generated files..."
	@rm -rf screenshots/* reports/* *.db *.log
	@echo "All clean."

# Run the application (development)
.PHONY: run
run: build
	@echo "Running $(BINARY_NAME)..."
	@$(BIN_DIR)/$(BINARY_NAME) $(ARGS)

# Show help
.PHONY: help
help:
	@echo "Stuff-time Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  build          - Build binary (development)"
	@echo "  build-release  - Build binary with optimizations (release)"
	@echo "  build-darwin   - Build for macOS (arm64 and amd64)"
	@echo "  build-linux    - Build for Linux (amd64 and arm64)"
	@echo "  build-windows  - Build for Windows (amd64 and arm64)"
	@echo "  build-all      - Build for all platforms"
	@echo "  install        - Install to /usr/local/bin (requires sudo)"
	@echo "  install-user   - Install to ~/bin (no sudo)"
	@echo "  test           - Run tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  fmt            - Format code"
	@echo "  lint           - Run linter (requires golangci-lint)"
	@echo "  deps           - Download and update dependencies"
	@echo "  clean          - Clean build artifacts"
	@echo "  clean-all      - Clean all (including data files)"
	@echo "  run            - Build and run (use ARGS='...' for arguments)"
	@echo "  help           - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make build                    # Build binary"
	@echo "  make build-release           # Build optimized binary"
	@echo "  make install-user            # Install to ~/bin"
	@echo "  make run ARGS='status'       # Run with arguments"
	@echo "  make test-coverage           # Run tests and generate coverage"


