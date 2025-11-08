.PHONY: build test clean run build-release test-coverage lint fmt deps help

# Build del progetto
build:
	@echo "Building proxmox-backup..."
	go build -o build/proxmox-backup ./cmd/proxmox-backup

# Build ottimizzato per release
build-release:
	@echo "Building release..."
	go build -ldflags="-s -w" -o build/proxmox-backup ./cmd/proxmox-backup

# Test
test:
	go test -v ./...

# Test con coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

# Lint
lint:
	go vet ./...
	@command -v golint >/dev/null 2>&1 && golint ./... || echo "golint not installed"

# Format code
fmt:
	go fmt ./...

# Clean build artifacts
clean:
	rm -rf build/
	rm -f coverage.out

# Run in development
run:
	go run ./cmd/proxmox-backup

# Install/update dependencies
deps:
	go mod download
	go mod tidy

# Help
help:
	@echo "Available targets:"
	@echo "  build         - Build the project"
	@echo "  build-release - Build optimized release binary"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  lint          - Run linters"
	@echo "  fmt           - Format Go code"
	@echo "  clean         - Remove build artifacts"
	@echo "  run           - Run in development mode"
	@echo "  deps          - Download and tidy dependencies"
