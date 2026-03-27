# SlashStage Makefile
# Usage: make [target]

# Variables
BINARY    := slashstage
MODULE    := github.com/kuk1song/slashstage
CMD_DIR   := ./cmd/slashstage
BUILD_DIR := ./dist
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS   := -ldflags "-s -w -X main.version=$(VERSION)"
GOTEST    := go test
GOFLAGS   := -race

.PHONY: all build test lint run clean ci fmt vet help

## help: Show this help message
help:
	@echo "SlashStage 🎸 — Build Commands"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | column -t -s ':' | sed 's/^/  /'

## build: Compile the binary
build:
	go build $(LDFLAGS) -o $(BINARY) $(CMD_DIR)

## run: Build and run the server
run: build
	./$(BINARY)

## test: Run all tests
test:
	$(GOTEST) $(GOFLAGS) ./...

## test-v: Run all tests with verbose output
test-v:
	$(GOTEST) $(GOFLAGS) -v ./...

## test-cover: Run tests with coverage report
test-cover:
	$(GOTEST) $(GOFLAGS) -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@echo ""
	@echo "To view HTML report: go tool cover -html=coverage.out"

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## fmt: Format all Go files
fmt:
	gofmt -s -w .
	goimports -w .

## vet: Run go vet
vet:
	go vet ./...

## ci: Full CI pipeline (fmt check + vet + lint + test + build)
ci: vet lint test build
	@echo ""
	@echo "✅ CI passed"

## clean: Remove build artifacts
clean:
	rm -f $(BINARY) coverage.out
	rm -rf $(BUILD_DIR)

## deps: Download and verify dependencies
deps:
	go mod download
	go mod verify

## tidy: Tidy go.mod
tidy:
	go mod tidy

## dist: Build for all platforms (via goreleaser)
dist:
	goreleaser release --snapshot --clean

all: ci
