# Cost controls

Goal: keep **total user spend** below direct Anthropic account usage when
routing through intermediary APIs (OpenRouter / mirrors / 9router), without
gateway-side markups beyond the chosen model prices.

## What the gateway can control (implemented / in progress)

| Control | Config | Effect |
|---|---|---|
| Model remap | `[models.*]` + routing rules | Primary savings: cheaper upstream IDs |
| Prefer cheapest eligible | `routing.prefer_cheapest` | Among capable candidates, pick lowest list price |
| Thinking policy | `models.*.thinking_policy` | Stop / cap reasoning tokens (billed as output) |
| Ensure prompt cache markers | `routing.ensure_prompt_cache` | Inject `cache_control` on system + tools if client omitted them |
| OpenRouter provider sort | `providers.*.sort = "price"` | Prefer cheaper backends for the same model ID |
| Cache-aware cost estimate | automatic in usage line | Estimates discount cache reads; warns on likely misses |
| Forward client cache markers | always | Preserves Desktop/Anthropic `cache_control` through encode |

### Recommended defaults (`config.toml`)

```toml
[providers.openrouter]
sort = "price"
require_parameters = true   # skip endpoints that drop tools/cache params

[profiles.cheap.routing]
prefer_cheapest = true
ensure_prompt_cache = true

[models.deepseek_flash]
reasoning = false
thinking_policy = "force_off"   # also implied when reasoning = false
```

### Thinking policy values

| Value | Behavior |
|---|---|
| _(empty)_ | If `reasoning = false` → force off; else passthrough |
| `passthrough` | Forward client thinking as-is |
| `force_off` | Strip / disable thinking on the upstream request |
| `cap` | Allow thinking but clamp `budget_tokens` to `thinking_budget_max` |

## What still burns tokens (not fully controllable here)

Documented for operators — these still dominate bills even with perfect gateway
settings. See also the README “Cost & token burn” section.

1. **Upstream ignores Anthropic-style prompt cache** — many non-Claude models
   never return `cached_tokens`; long agent contexts pay full input every turn.
2. **Tokenizer differences** — same chat text can be more/fewer tokens on
   DeepSeek/Kimi/GLM than on Claude.
3. **OpenRouter / mirror list prices & markups** — not set by this repo; re-check
   with `models status`.
4. **Conversation growth** — Desktop/agent history + tool schemas grow without
   bound; gateway cannot truncate client context safely.
5. **Vision / image inputs** — large image token bursts when the user attaches media.
6. **Client aborts & reconnects** — tokens already generated are usually billed;
   streaming path does not auto-retry (by design).
7. **Desktop enabling thinking** — if policy is `passthrough` and the client
   turns thinking on, output cost spikes regardless of model list price.

## Backlog (next)

- Session / daily cost rollup from history DB
- Live alert when `cache_read == 0` for N consecutive large prompts
- Per-model cache price multipliers from OpenRouter catalog when published
- Optional max input-token soft reject before upstream call
