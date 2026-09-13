.PHONY: fmt test vet staticcheck docscheck check build sync-default-config

VERSION ?= 0.1.0
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%d)
LDFLAGS = -X github.com/danakolana/claude-gateway/internal/diagnose.GitCommit=$(GIT_COMMIT) \
	-X github.com/danakolana/claude-gateway/internal/diagnose.BuildDate=$(BUILD_DATE)

fmt:
	gofmt -w .

test:
	go test ./...

vet:
	go vet ./...

staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@v0.6.1 ./...

docscheck:
	go run ./tools/docscheck .

check: fmt vet test staticcheck docscheck

# Keep the embedded default identical to the repo-root config.toml.
sync-default-config:
	cp config.toml internal/config/default.toml

build: sync-default-config
	mkdir -p dist
	go build -ldflags "$(LDFLAGS)" -o dist/claude-gateway ./cmd/claude-gateway
	go build -ldflags "$(LDFLAGS)" -o dist/gateway-server ./cmd/gateway-server
