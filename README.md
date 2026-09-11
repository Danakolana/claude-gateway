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
make build

# One command: load TOML, apply Desktop config, start proxy
./dist/claude-gateway
```

From the repo root this auto-discovers `./examples/config.toml`. Listen address
and Desktop apply come from:

```toml
[proxy]
mode = "local"          # or "direct" = Desktop → OpenRouter (no local proxy)
listen = "127.0.0.1:8080"
apply_desktop = true
```

**Compare without local proxy** (OpenRouter’s Anthropic-compatible API):

```toml
[proxy]
mode = "direct"
```

Then run `./dist/claude-gateway` — it writes Desktop **Connection** settings
(`configLibrary` + `claude_desktop_config.json`) to `https://openrouter.ai/api`
for **Anthropic-looking OpenRouter IDs only** (`anthropic/claude-*`), turns
discovery off, and exits. DeepSeek/GLM/Kimi remaps need `mode = "local"`
(Desktop rejects non-Anthropic route names and there is no remapper in direct).
Quit/reopen Desktop. Switch back with `mode = "local"` and re-run.

Then open Claude Desktop and click **Apply Changes** if prompted.

Optional overrides: `--config PATH`, `--listen HOST:PORT`, `--no-apply`, `--fake`.

### Which Claude Desktop? (interactive)

On start (when applying), the CLI asks:

1. **3P (recommended)** — Connection Gateway UI + custom model labels  
2. **Consumer (experimental)** — regular Desktop via `env.ANTHROPIC_BASE_URL` only  

Non-interactive runs (no TTY) default to **3P**. Consumer requires a typed `y`
confirm after selecting option 2.

Consumer limits:

- Model picker stays Anthropic’s — no custom DeepSeek/GLM labels
- Remaps only when Desktop sends a model ID matching your routing rules
- Still typically needs Anthropic login
- May break across Desktop builds

### OpenRouter mirror / reverse proxy

Point the provider `base_url` at official OpenRouter **or** your regional mirror
(same OpenAI-compatible `/v1` API). Same API key usually works if the mirror
forwards auth.

```toml
[providers.openrouter]
base_url = "https://your-mirror.example.com/api/v1"
# allow_private_network = true  # only for private LAN mirrors
```

Or without editing TOML:

```bash
export OPENROUTER_BASE_URL=https://your-mirror.example.com/api/v1
```

For `[proxy] mode = "direct"`, Desktop needs the Anthropic base (no `/v1`):

```toml
[proxy]
mode = "direct"
direct_base_url = "https://your-mirror.example.com/api"
```

If `direct_base_url` is unset, it is derived by stripping `/v1` from `base_url`.

Advanced commands still exist (`proxy start`, `client apply`, `models status`, …).

## Default models (Desktop dropdown)

`examples/config.toml` includes gateway budget models **and** official Claude
models via OpenRouter. Claude Desktop only accepts Anthropic-looking IDs —
the gateway maps `desktop_id` → real `model_id`.

Approximate OpenRouter prices (USD per 1M tokens). Snapshot dated **2026-09-11**
from OpenRouter `/models` — not a billing guarantee; re-check with
`./dist/claude-gateway models status`.

| Desktop ID | Upstream | In $/M | Out $/M | Notes |
|---|---|---:|---:|---|
| `claude-haiku-4` | `deepseek/deepseek-v4-flash-0731` | 0.065 | 0.18 | DeepSeek V4 Flash (default haiku) |
| `claude-haiku-4-1` | `z-ai/glm-5.3-flash` | 0.15 | 0.50 | GLM 5.3 Flash |
| `claude-haiku-4-2` | `inception/mercury-2.5-preview` | 0.04* | 0.15* | Mercury 2.5 (*config.toml; not in live catalog) |
| `claude-sonnet-4` | `moonshotai/kimi-k2.5` | 0.45 | 2.25 | Kimi K2.5 (default sonnet) |
| `claude-sonnet-4-1` | `moonshotai/kimi-k2.6` | 0.95 | 4.00 | Kimi K2.6 |
| `claude-sonnet-4-2` | `moonshotai/kimi-k2.7-code` | 0.71 | 3.50 | Kimi K2.7 Code |
| `anthropic/claude-haiku-4.5` | same | 1.00 | 5.00 | Official Claude Haiku 4.5 |
| `anthropic/claude-3-haiku` | same | 0.25 | 1.25 | Official Claude Haiku 3 |
| `anthropic/claude-sonnet-4.5` | same | 3.00 | 15.00 | Official Claude Sonnet 4.5 |
| `anthropic/claude-sonnet-4.6` | same | 3.00 | 15.00 | Official Claude Sonnet 4.6 |
| `anthropic/claude-sonnet-5` | same | 2.00 | 10.00 | Official Claude Sonnet 5 |
| `anthropic/claude-opus-4.6` | same | 5.00 | 25.00 | Official Claude Opus 4.6 |

**Claude Haiku 3.5** is not available on OpenRouter (only Haiku 3 and Haiku 4.5).

### Live price / context / coding metrics

From the repo root (auto-discovers `./examples/config.toml`), or with an explicit path:

```bash
./dist/claude-gateway models list
./dist/claude-gateway models status
./dist/claude-gateway models status --json
./dist/claude-gateway models status --watch 60
# or permanently:
# export CLAUDE_GATEWAY_CONFIG=~/.config/claude-gateway/config.toml
```

`models list` shows the Desktop picker mapping (no network). `models status`
shows live OpenRouter input/output $/MTok, context length, Artificial Analysis
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

## Cost & token burn

Full plan and knobs: [`docs/COST.md`](docs/COST.md).

**What this gateway can improve (actionable):**

- Remap Desktop models → cheaper OpenRouter IDs (main savings).
- `routing.prefer_cheapest` — among capable candidates, pick lowest list price.
- `routing.ensure_prompt_cache` — inject `cache_control` on system + tools when the client omitted markers.
- `models.*.thinking_policy` (`force_off` / `cap` / `passthrough`) — stop or clamp reasoning tokens (billed as output).
- `providers.*.sort = "price"` (+ optional `ignore_providers` / `require_parameters`) — OpenRouter provider routing toward cheaper backends.
- Usage line: cache-aware cost estimate + warning when a large prompt reports **no** cache read.

**What still affects cost significantly (not fully controllable here):**

| Factor | Why it still matters |
|---|---|
| Upstream has no Anthropic-style prompt cache | Long agent turns pay full input every request even if we forward/`inject` markers |
| Tokenizer differences | Same text ≠ same token counts across Claude vs DeepSeek/Kimi/GLM |
| OpenRouter / mirror list price & markup | Outside this repo; verify with `models status` |
| Growing Desktop/agent context | History + tools grow; we do not truncate client context |
| Vision / images | Large input bursts when media is attached |
| Abort / reconnect mid-stream | Already-generated tokens are usually billed; stream path does not auto-retry |
| Client turns thinking on | With `passthrough`, reasoning tokens still inflate output cost |

Model list-price differences vs direct Anthropic are expected and intentional. The
controls above aim to avoid **extra** burn beyond that (cache misses, thinking,
expensive backends).

## Optional history sync server

```bash
export CLAUDE_GATEWAY_SYNC_TOKEN=long-random-token
./dist/gateway-server --addr 127.0.0.1:8090 --data ~/.local/share/claude-gateway/server
```

## Safety

- Does not patch Claude Desktop binaries or bypass Anthropic auth.
- Uses documented Claude Desktop **on 3P** gateway settings (ADR-011).
- Proxy binds to loopback by default; secrets stay in env vars, not TOML.
