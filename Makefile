.PHONY: fmt test vet staticcheck docscheck check build

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

build:
	mkdir -p dist
	go build -o dist/claude-gateway ./cmd/claude-gateway
	go build -o dist/gateway-server ./cmd/gateway-server
