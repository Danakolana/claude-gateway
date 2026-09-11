.PHONY: fmt test vet staticcheck docscheck check build sync-default-config

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
	go build -o dist/claude-gateway ./cmd/claude-gateway
	go build -o dist/gateway-server ./cmd/gateway-server
