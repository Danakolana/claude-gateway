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

## Default models (Desktop dropdown)

`examples/config.toml` includes gateway budget models **and** official Claude
models via OpenRouter. Claude Desktop only accepts Anthropic-looking IDs —
the gateway maps `desktop_id` → real `model_id`.

| Desktop ID | Upstream | Notes |
|---|---|---|
| `claude-haiku-4` | `deepseek/deepseek-v4-flash-0731` | DeepSeek V4 Flash (default haiku) |
| `claude-haiku-4-1` | `z-ai/glm-5.3-flash` | GLM 5.3 Flash |
| `claude-haiku-4-2` | `inception/mercury-2.5-preview` | Mercury 2.5 |
| `claude-sonnet-4` | `moonshotai/kimi-k2.5` | Kimi K2.5 (default sonnet) |
| `anthropic/claude-haiku-4.5` | same | Official Claude Haiku 4.5 |
| `anthropic/claude-3-haiku` | same | Official Claude Haiku 3 |
| `anthropic/claude-sonnet-4.5` | same | Official Claude Sonnet 4.5 |
| `anthropic/claude-sonnet-4.6` | same | Official Claude Sonnet 4.6 |
| `anthropic/claude-opus-4.6` | same | Official Claude Opus 4.6 |

**Claude Haiku 3.5** is not available on OpenRouter (only Haiku 3 and Haiku 4.5).

### Live price / context / coding metrics

```bash
./dist/claude-gateway models status --config examples/config.toml
./dist/claude-gateway models status --config examples/config.toml --json
./dist/claude-gateway models status --config examples/config.toml --watch 60
```

Shows live OpenRouter input/output $/MTok, context length, Artificial Analysis
coding/agentic indexes, and design-arena coding rank when present.

Add more models in config (any OpenRouter / compatible ID), then re-run `client apply`:

```toml
[models.my_pick]
model_id = "moonshotai/kimi-k2"
display_name = "Kimi K2"
desktop_id = "claude-sonnet-4-7"          # must look like claude-* or anthropic/claude-*
desktop_label = "Kimi K2 (gateway)"
desktop_tier = "sonnet"
streaming = true
tool_calls = true
enabled = true
context_limit = 128000
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
