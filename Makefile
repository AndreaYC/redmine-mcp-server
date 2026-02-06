.PHONY: build build-macos run-mcp run-sse run-api test lint clean docker-build docker-build-multiarch docker-run docker-push docker-release release

# Variables
BINARY_NAME=server
DOCKER_IMAGE=harbor.sw.ciot.work/mcp/redmine
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Build
build:
	go build -ldflags="-w -s -X main.version=$(VERSION)" -o bin/$(BINARY_NAME) ./cmd/server

# Run locally
run-mcp:
	REDMINE_URL=http://advrm.advantech.com:3002 go run ./cmd/server mcp

run-sse:
	REDMINE_URL=http://advrm.advantech.com:3002 go run ./cmd/server mcp --sse --port 8080

run-api:
	REDMINE_URL=http://advrm.advantech.com:3002 go run ./cmd/server api --port 8080

# Test
test:
	go test -v -race ./...

test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Lint
lint:
	golangci-lint run

fmt:
	go fmt ./...
	goimports -w .

# Clean
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Dependencies
deps:
	go mod download
	go mod tidy

# Docker (single platform, loads into local daemon)
docker-build:
	docker buildx build --platform linux/amd64 -t $(DOCKER_IMAGE):latest -t $(DOCKER_IMAGE):$(VERSION) --load .

# Docker multi-arch (linux/amd64 + linux/arm64, push directly)
docker-build-multiarch:
	docker buildx build --platform linux/amd64,linux/arm64 -t $(DOCKER_IMAGE):latest -t $(DOCKER_IMAGE):$(VERSION) --push .

docker-run:
	docker run -p 8080:8080 -e REDMINE_URL=http://advrm.advantech.com:3002 $(DOCKER_IMAGE):latest

docker-run-sse:
	docker run -p 8080:8080 -e REDMINE_URL=http://advrm.advantech.com:3002 $(DOCKER_IMAGE):latest mcp --sse

docker-push:
	docker push $(DOCKER_IMAGE):latest
	docker push $(DOCKER_IMAGE):$(VERSION)

# Docker release (lint, test, build, push)
docker-release: lint test docker-build docker-push
	@echo "Docker release complete: $(DOCKER_IMAGE):$(VERSION)"

# Build native macOS binaries (amd64 + arm64)
build-macos:
	@mkdir -p bin
	GOOS=darwin GOARCH=amd64 go build -ldflags="-w -s -X main.version=$(VERSION)" -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/server
	GOOS=darwin GOARCH=arm64 go build -ldflags="-w -s -X main.version=$(VERSION)" -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/server

# Multi-platform release
release:
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 go build -ldflags="-w -s -X main.version=$(VERSION)" -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/server
	GOOS=linux GOARCH=arm64 go build -ldflags="-w -s -X main.version=$(VERSION)" -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/server
	GOOS=darwin GOARCH=amd64 go build -ldflags="-w -s -X main.version=$(VERSION)" -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/server
	GOOS=darwin GOARCH=arm64 go build -ldflags="-w -s -X main.version=$(VERSION)" -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/server
	GOOS=windows GOARCH=amd64 go build -ldflags="-w -s -X main.version=$(VERSION)" -o bin/$(BINARY_NAME)-windows-amd64.exe ./cmd/server

# Help
help:
	@echo "Available targets:"
	@echo "  build       - Build the binary"
	@echo "  run-mcp     - Run MCP server in stdio mode"
	@echo "  run-sse     - Run MCP server in SSE mode"
	@echo "  run-api     - Run REST API server"
	@echo "  test        - Run tests"
	@echo "  lint        - Run linter"
	@echo "  clean       - Clean build artifacts"
	@echo "  build-macos    - Build native macOS binaries (amd64 + arm64)"
	@echo "  docker-build   - Build Docker image (linux/amd64)"
	@echo "  docker-build-multiarch - Build multi-arch Docker image (amd64 + arm64) and push"
	@echo "  docker-run     - Run Docker container (API mode)"
	@echo "  docker-push    - Push Docker image to Harbor"
	@echo "  docker-release - Lint, test, build and push Docker image"
	@echo "  release        - Build for all platforms (linux, darwin, windows)"
