# greg - Unified Media Streaming TUI

# Binary name
binary := "greg"

# Version information
version := `git describe --tags --always --dirty 2>/dev/null || echo "dev"`
commit := `git rev-parse --short HEAD 2>/dev/null || echo "none"`
date := `date -u +"%Y-%m-%dT%H:%M:%SZ"`

# Build flags
ldflags := "-s -w -X main.version=" + version + " -X main.commit=" + commit + " -X main.date=" + date

# Directories
dist_dir := "dist"

# Default recipe (list all recipes)
default:
    @just --list

# Build development binary
build:
    @echo "Building {{binary}} (dev)..."
    go build -o {{binary}} cmd/greg/main.go
    @echo "Built: ./{{binary}}"

# Build optimized release binary
build-release:
    @echo "Building {{binary}} {{version}}..."
    go build -ldflags="{{ldflags}}" -o {{binary}} cmd/greg/main.go
    @echo "Built: ./{{binary}}"
    @./{{binary}} version

# Build Windows executable
build-windows:
    @echo "Building {{binary}} for Windows..."
    GOOS=windows GOARCH=amd64 go build -ldflags="{{ldflags}}" -o {{binary}}.exe cmd/greg/main.go
    @ls -lh {{binary}}.exe
    @echo "Built: ./{{binary}}.exe"

# Build Windows executable and copy to Windows Downloads (this is just for personal usage, bc i was lazy)
build-windows-copy: build-windows
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -d "/mnt/c/Users" ]; then
        USER_DIR=$(ls -1 /mnt/c/Users | grep -v -e "^Public" -e "^Default" -e "^All" -e "desktop.ini" | grep -v "User" | head -1)
        if [ -z "$USER_DIR" ]; then
            echo "Could not detect Windows user directory"
            echo "Executable available at: ./{{binary}}.exe"
            exit 1
        fi
        cp {{binary}}.exe "/mnt/c/Users/$USER_DIR/Downloads/{{binary}}.exe"
        echo "✓ Copied to C:\\Users\\$USER_DIR\\Downloads\\{{binary}}.exe"
        echo ""
        echo "Test in PowerShell:"
        echo "  cd C:\\Users\\$USER_DIR\\Downloads"
        echo "  .\\{{binary}}.exe version"
    else
        echo "Not running in WSL, executable available at: ./{{binary}}.exe"
    fi

# Build for all platforms
build-all:
    @./build-all.sh

# Run unit tests
test:
    @echo "Running tests..."
    go test -v ./...

# Run integration tests
test-integration:
    @echo "Running integration tests..."
    go test -v -tags=integration ./...

# Run tests with coverage
test-coverage:
    @echo "Running tests with coverage..."
    go test -cover -coverprofile=coverage.out ./...
    go tool cover -func=coverage.out
    @echo ""
    @echo "HTML coverage report: coverage.html"
    go tool cover -html=coverage.out -o coverage.html

# Run tests with race detector
test-race:
    @echo "Running tests with race detector..."
    go test -race ./...

# Run benchmarks
bench:
    @echo "Running benchmarks..."
    go test -bench=. -benchmem ./...

# Run linters
lint:
    @echo "Running go vet..."
    go vet ./...
    @echo "Checking formatting..."
    @test -z "$(gofmt -l .)" || (echo "Code not formatted, run 'just fmt'" && exit 1)
    @echo "Running golangci-lint..."
    @golangci-lint run 2>&1 || echo "Note: golangci-lint found issues (run 'just lint' to see details)"

# Format code
fmt:
    @echo "Formatting code..."
    gofmt -w .
    @goimports -w . 2>/dev/null || echo "goimports not installed, run: go install golang.org/x/tools/cmd/goimports@latest"

# Clean build artifacts
clean:
    @echo "Cleaning..."
    rm -f {{binary}}
    rm -rf {{dist_dir}}
    rm -f coverage.out coverage.html
    rm -f *.test
    go clean

# Install binary to $GOPATH/bin
install:
    @echo "Installing {{binary}}..."
    GOBIN="$(go env GOPATH)/bin" go build -ldflags="{{ldflags}}" -o "$(go env GOPATH)/bin/{{binary}}" cmd/greg/main.go
    @echo "Installed: $(go env GOPATH)/bin/{{binary}}"

# Uninstall binary from $GOPATH/bin
uninstall:
    @echo "Uninstalling {{binary}}..."
    rm -f "$(go env GOPATH)/bin/{{binary}}"
    @echo "Uninstalled from $(go env GOPATH)/bin/{{binary}}"

# Build and run
run: build
    ./{{binary}}

# Run with arguments
run-args *args: build
    ./{{binary}} {{args}}

# Run with hot reload (requires air)
dev:
    @which air > /dev/null || (echo "air not installed, run: go install github.com/air-verse/air@latest" && exit 1)
    air

# Download dependencies
deps:
    @echo "Downloading dependencies..."
    go mod download
    go mod verify

# Tidy dependencies
tidy:
    @echo "Tidying dependencies..."
    go mod tidy

# Update dependencies
update-deps:
    @echo "Updating dependencies..."
    go get -u ./...
    go mod tidy

# Reset database (WARNING: deletes all data)
db-reset:
    @echo "Resetting database..."
    rm -f ~/.local/share/greg/greg.db*
    @echo "Database reset. Will be recreated on next run."

# Generate default config file
config-init:
    @echo "Generating config..."
    mkdir -p ~/.config/greg
    @echo "Config directory: ~/.config/greg"
    @echo "Run 'greg config init' to generate default config"

# Install development tools
tools:
    #!/usr/bin/env bash
    set -euo pipefail
    echo "Installing development tools..."
    # golangci-lint (use official installer)
    if ! command -v golangci-lint &> /dev/null; then
        curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b "$(go env GOPATH)/bin" v2.8.0
    fi
    # goimports
    go install golang.org/x/tools/cmd/goimports@latest
    # air
    go install github.com/air-verse/air@latest
    # govulncheck
    go install golang.org/x/vuln/cmd/govulncheck@latest
    # watchexec
    if ! command -v watchexec &> /dev/null; then
        if command -v cargo &> /dev/null; then
            echo "Installing watchexec via cargo..."
            cargo install watchexec-cli
        else
            echo "Warning: cargo not found, cannot install watchexec automatically."
        fi
    fi
    echo "Tools installed!"

# Check development environment
doctor:
    @echo "Checking development environment..."
    @echo ""
    @echo "Go version:"
    @go version
    @echo ""
    @echo "Dependencies:"
    @which mpv > /dev/null && mpv --version | head -1 || echo "⚠️  mpv not found"
    @which ffmpeg > /dev/null && ffmpeg -version | head -1 || echo "⚠️  ffmpeg not found"
    @which golangci-lint > /dev/null && echo "✓ golangci-lint installed" || echo "⚠️  golangci-lint not installed"
    @which air > /dev/null && echo "✓ air installed" || echo "⚠️  air not installed"
    @echo ""
    @echo "Project status:"
    @go mod verify && echo "✓ go.mod verified" || echo "⚠️  go.mod issues"
    @test -f {{binary}} && echo "✓ Binary exists" || echo "ℹ️  Binary not built (run 'just build')"

# Watch and run tests on file changes
watch-test:
    @echo "Watching for changes..."
    @which watchexec > /dev/null || (echo "watchexec not installed, install from: https://github.com/watchexec/watchexec" && exit 1)
    watchexec -e go --clear -- just test

# Run specific package tests
test-pkg package:
    @echo "Testing {{package}}..."
    go test -v ./internal/{{package}}/...

# Check for security vulnerabilities
security:
    @echo "Checking for vulnerabilities..."
    @which govulncheck > /dev/null || (echo "govulncheck not installed, run: go install golang.org/x/vuln/cmd/govulncheck@latest" && exit 1)
    govulncheck ./...

# Generate release archives
release: build-all
    @echo "Creating release archives..."
    cd {{dist_dir}} && tar czf {{binary}}-linux-amd64.tar.gz {{binary}}-linux-amd64
    cd {{dist_dir}} && tar czf {{binary}}-linux-arm64.tar.gz {{binary}}-linux-arm64
    cd {{dist_dir}} && tar czf {{binary}}-darwin-amd64.tar.gz {{binary}}-darwin-amd64
    cd {{dist_dir}} && tar czf {{binary}}-darwin-arm64.tar.gz {{binary}}-darwin-arm64
    cd {{dist_dir}} && zip {{binary}}-windows-amd64.zip {{binary}}-windows-amd64.exe
    @echo "Release archives created in {{dist_dir}}/"
    @ls -lh {{dist_dir}}/*.{tar.gz,zip}

# Quick check before commit
pre-commit: fmt lint test
    @echo "✓ Pre-commit checks passed!"
