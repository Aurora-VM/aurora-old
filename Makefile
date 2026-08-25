.PHONY: all build build-server build-agent build-web test test-go test-web proto lint dev-up dev-down clean

VERSION ?= 0.1.0-dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags="-w -s -X 'github.com/aurora-vm/aurora/pkg/version.Version=$(VERSION)' -X 'github.com/aurora-vm/aurora/pkg/version.GitCommit=$(COMMIT)' -X 'github.com/aurora-vm/aurora/pkg/version.BuildDate=$(BUILD_DATE)'"

all: proto build test

## Protobuf Generation
proto:
	@echo "==> Generating Protobuf bindings..."
	@export PATH="$$PATH:$$(go env GOPATH)/bin"; \
	protoc \
		--proto_path=proto \
		--go_out=gen/go \
		--go_opt=module=github.com/aurora-vm/aurora/gen/go \
		--go-grpc_out=gen/go \
		--go-grpc_opt=module=github.com/aurora-vm/aurora/gen/go \
		proto/aurora/v1/common.proto proto/aurora/v1/health.proto proto/aurora/v1/node.proto
	@echo "==> Protobuf generation complete."

## Build Binaries
build: build-server build-agent build-web

build-server:
	@echo "==> Building Aurora Control Plane..."
	@mkdir -p bin
	go build $(LDFLAGS) -o bin/aurora-server ./cmd/aurora-server

build-agent:
	@echo "==> Building Aurora Node Agent..."
	@mkdir -p bin
	go build $(LDFLAGS) -o bin/aurora-agent ./cmd/aurora-agent

build-web:
	@echo "==> Building Web Frontend..."
	cd web && npm run build

## Testing
test: test-go test-web

test-go:
	@echo "==> Running Go unit tests..."
	go test -v -race ./...

test-web:
	@echo "==> Running Web frontend tests..."
	cd web && npm run test

## Development Environment
dev-up:
	@echo "==> Starting local PostgreSQL & Valkey containers..."
	docker compose -f deployments/docker-compose.dev.yml up -d

dev-down:
	@echo "==> Stopping local containers..."
	docker compose -f deployments/docker-compose.dev.yml down

clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf bin/ web/dist/
