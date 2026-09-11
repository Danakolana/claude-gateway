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
| Ensure prompt cache markers | `routing.ensure_prompt_cache` | Inject `cache_control` on system + tools **and** the last stable history block (conversation prefix) if the client omitted them |
| Max tokens cap | `routing.max_tokens_cap` / `models.*.max_tokens_cap` | Clamp Desktop `max_tokens`; model cap wins when set; 0 = unlimited |
| OpenRouter provider sort | `providers.*.sort = "price"` | Prefer cheaper backends for the same model ID |
| Sticky provider | `providers.*.sticky = true` | Pin the last successful OpenRouter backend per model so prompt-cache reads survive `sort=price` hops |
| Cache-aware cost estimate | automatic in usage line + `/debug/usage` | Estimates discount cache reads; warns on likely misses |
| Session spend + cache-miss nags | in-memory sidecar | This-process totals on the local guide; never rejects |
| Background catalog refresh | every 15m | Keeps last-known prices if OpenRouter `/models` fails |
| Context-growth nag | `/debug/usage` notes | Warns near `context_limit`; never truncates |
| Optional spend webhook | `spend_alert_usd` + `spend_alert_url` | One-shot, 1s timeout, dropped on error |
| Fail-open circuit breaker | `circuit_failures` (default 0 = off) | Skip a model after N transient errors **only if a fallback exists** |
| History secret redaction | `history.redact_secrets` | Imperfect; on failure the write is skipped, chat continues |
| Forward client cache markers | always | Preserves Desktop/Anthropic `cache_control` through encode |

### Recommended defaults (`config.toml`)

```toml
[providers.openrouter]
sort = "price"
require_parameters = true   # skip endpoints that drop tools/cache params
sticky = true               # reuse last backend so cache can hit

[profiles.cheap.routing]
prefer_cheapest = true
ensure_prompt_cache = true  # system + tools + conversation prefix
max_tokens_cap = 32768      # 0 = unlimited

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

- Session / daily cost rollup **from history DB** (in-memory session is T142)
- Per-model cache price multipliers from OpenRouter catalog when published
- Optional max input-token soft reject before upstream call (hard cap; opt-in)
- Mid-stream failover and hard spend budgets (explicitly out of sidecar scope; ADR-014)
