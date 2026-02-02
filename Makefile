.PHONY: build test clean run dev docker lint fmt help

# Variables
BINARY_NAME=temporal-profiler
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-w -s -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'

## build: Build the binary
build:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) ./cmd/temporal-profiler

## build-linux: Build for Linux (for Docker)
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) ./cmd/temporal-profiler

## test: Run tests
test:
	$(GOTEST) -v -race -cover ./...

## test-coverage: Run tests with coverage report
test-coverage:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## clean: Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html

## run: Build and run the profiler
run: build
	./$(BINARY_NAME) start

## dev: Run with hot reload (requires air: go install github.com/cosmtrek/air@latest)
dev:
	air -c .air.toml || $(GOCMD) run ./cmd/temporal-profiler start

## docker-build: Build Docker image
docker-build:
	docker build -t $(BINARY_NAME):$(VERSION) -f deploy/docker/Dockerfile .
	docker tag $(BINARY_NAME):$(VERSION) $(BINARY_NAME):latest

## docker-run: Run in Docker
docker-run:
	docker run -p 7234:7234 -p 8080:8080 $(BINARY_NAME):latest

## docker-compose-up: Start full stack with docker-compose
docker-compose-up:
	cd deploy/docker && docker-compose up -d

## docker-compose-down: Stop docker-compose stack
docker-compose-down:
	cd deploy/docker && docker-compose down

## docker-compose-logs: View docker-compose logs
docker-compose-logs:
	cd deploy/docker && docker-compose logs -f

## lint: Run linters (requires golangci-lint)
lint:
	golangci-lint run ./...

## fmt: Format code
fmt:
	$(GOCMD) fmt ./...
	gofumpt -w .

## tidy: Tidy go modules
tidy:
	$(GOMOD) tidy

## deps: Download dependencies
deps:
	$(GOMOD) download

## generate: Run go generate
generate:
	$(GOCMD) generate ./...

## config: Generate default config file
config:
	./$(BINARY_NAME) config > temporal-profiler.yaml

## install: Install binary to GOPATH/bin
install:
	$(GOCMD) install $(LDFLAGS) ./cmd/temporal-profiler

## release: Create release builds for multiple platforms
release: clean
	mkdir -p dist
	# Linux amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-amd64 ./cmd/temporal-profiler
	# Linux arm64
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-arm64 ./cmd/temporal-profiler
	# Darwin amd64
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-amd64 ./cmd/temporal-profiler
	# Darwin arm64
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-arm64 ./cmd/temporal-profiler
	# Windows amd64
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o dist/$(BINARY_NAME)-windows-amd64.exe ./cmd/temporal-profiler

## all: Build, test, and lint
all: deps fmt lint test build
