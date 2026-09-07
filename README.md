# Claude Desktop Gateway

Go CLI that configures Claude Desktop (3P gateway mode) and runs a local
Anthropic Messages API proxy that routes to OpenRouter, 9router, or any
OpenAI-compatible gateway.

## Build

```bash
make build
# binaries: dist/claude-gateway  dist/gateway-server
make check   # tests + staticcheck + docscheck
```

## Quick start

```bash
export OPENROUTER_API_KEY=sk-or-...

# 1) Validate example config
./dist/claude-gateway config validate --config examples/config.toml
./dist/claude-gateway config explain --config examples/config.toml

# 2) Start local proxy (Anthropic POST /v1/messages on loopback)
./dist/claude-gateway proxy start --config examples/config.toml --addr 127.0.0.1:8080

# Optional: fake provider (no network)
./dist/claude-gateway proxy start --config examples/config.toml --addr 127.0.0.1:8080 --fake

# 3) Preview / apply Claude Desktop on 3P config (points at local proxy)
./dist/claude-gateway client diff --config examples/config.toml --proxy-url http://127.0.0.1:8080
./dist/claude-gateway client apply --config examples/config.toml --proxy-url http://127.0.0.1:8080 --dry-run
# then without --dry-run when ready

# 4) Diagnostics
./dist/claude-gateway doctor --config examples/config.toml
```

## Optional history sync server

```bash
export CLAUDE_GATEWAY_SYNC_TOKEN=long-random-token
./dist/gateway-server --addr 127.0.0.1:8090 --data ~/.local/share/claude-gateway/server
```

## Safety

- Does not patch Claude Desktop binaries or bypass Anthropic auth.
- Uses documented Claude Desktop **on 3P** gateway settings (ADR-011).
- Proxy binds to loopback by default; secrets stay in env vars, not TOML.
