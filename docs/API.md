# API and Protocol Contracts

> **Authoring model:** GPT-5.6 Luna  
> **Revision:** 2026-09-11 · `/debug/usage` sidecar (T141) added by Cursor Grok 4.6  
> **Status:** Canonical baseline — revised

## Contract status

The endpoints and types in this document are design contracts, not claims that
the implementation already exists. Each contract becomes supported only after
fixtures and integration tests pass.

> **Inbound protocol contract is conditional on ADR-011.** The inbound proxy
> protocol surface (Anthropic Messages API, MCP tool-server, or deferred) is
> determined by T005 (Claude Desktop endpoint verification). ADR-011 in
> `docs/DECISIONS.md` records the outcome. No code under
> `internal/protocol/inbound/` is written until ADR-011 status is "Accepted".
> The field-level translation mapping (see § "Field-level translation mapping"
> below) is populated by T048 and T049 before codec implementation begins.

> **Important — MCP vs. Model Endpoint.** Claude Desktop's documented MCP
> server support (JSON-RPC 2.0 over stdio or SSE) provides tool and context
> data to the model. It is NOT a mechanism for routing model inference to a
> different LLM provider. This document's local proxy contract covers model
> inference routing only. If T005 records MCP-only integration, this section
> will be revised to describe the MCP inbound surface instead.

## CLI contract

Command groups:

```text
config validate|explain
profile list|create|select|delete
provider list|health
model list|explain
proxy start|health
client discover|diff|apply|restore|launch
history list|show|export|import|backup
sync status|push|pull|conflicts
server start|health
doctor
```

Read-only commands should support `--output json`. Mutating commands support
`--dry-run` where meaningful and return documented stable exit codes.

## Canonical protocol

The internal protocol represents:

- request ID and source metadata,
- source and target model,
- ordered text/reasoning/tool/attachment blocks,
- tool definitions, calls, and results,
- streaming preference,
- required capabilities,
- cancellation/deadline,
- normalized usage and finish reason.

Provider and client codecs must preserve semantic content or return an explicit
unsupported-capability error.

## Local proxy contract

**The inbound surface is determined by ADR-011 (see T005).** Until ADR-011
is accepted, this section describes the design-level intent only. The outbound
surface is OpenAI-compatible provider traffic (documented in T049).

If ADR-011 records `custom-model-endpoint`, the inbound surface is an
Anthropic-compatible messages-style API. If ADR-011 records `mcp-only`, the
inbound surface is an MCP tool-server implementation. If ADR-011 records
`none`, the proxy is a standalone adapter (useful for Claude Code and direct
API consumers) with no automatic Claude Desktop integration.

Supported methods, paths, headers, event sequences, limits, and error bodies
must be captured in golden fixture tests (T048 for inbound, T049 for outbound)
before codec implementation begins (T053–T055).

The proxy binds to loopback by default, emits correlation IDs, logs a WARN
for non-loopback binding, and never returns provider authorization headers.

## Claude Desktop version compatibility matrix

> **Populated by T069.** At least one tested entry with version number, OS,
> test method, and date must exist before `client apply` is shipped. The
> documentation lint check verifies this.

| Claude Desktop version | OS | Tested mechanism | Test method | Test date | Notes |
|---|---|---|---|---|---|
| Claude Desktop on 3P (docs) | Linux/Windows (docs) | `custom-model-endpoint` via `enterpriseConfig.inferenceProvider=gateway` | Official Anthropic docs review: https://claude.com/docs/third-party/claude-desktop/gateway | 2026-09-07 | Requires Anthropic Messages API `POST /v1/messages`. |
| Claude Desktop on 3P (docs, path layouts) | Windows/macOS/Linux | Local `configLibrary/` + `claude_desktop_config.json`; Windows LocalAppData + MSIX | Official data-storage + configuration review: https://claude.com/docs/third-party/claude-desktop/data-storage | 2026-09-11 | Current Windows 3P dir is `%LOCALAPPDATA%\Claude-3p\`; earlier builds used `%APPDATA%\Claude-3p\` (migrated on upgrade). MSIX virtualizes under `Packages\Claude_*\LocalCache\`. Connection profile JSON is flat (no `enterpriseConfig` wrapper). |
| Claude Desktop on 3P (live) | Linux | Gateway `POST /v1/messages` text + streaming tool_use via OpenRouter | Live curl through local proxy; Desktop config apply + Models list | 2026-09-07 | Verified Anthropic SSE tool framing (`input_json_delta`, `stop_reason=tool_use`). Restart proxy after upgrades. |
| Any (experimental) | any | `env.ANTHROPIC_BASE_URL` in desktop config | Interactive prompt on start / `client apply` (choice 2 + confirm) | 2026-09-11 | Merges `env` into consumer `Claude/claude_desktop_config.json`. Not primary; picker stays Anthropic’s. |

## Known MVP limitations

| Item | Behavior |
|---|---|
| Consumer Desktop custom model list | Not supported (3P Connection UI only); experimental env override only |
| Unknown / beta content block types | Skipped (request continues) |
| Structured output (`response_format`) | Rejected with `unsupported_capability` |
| Response `model` field | Echoes **client** `desktop_id` / source model; upstream ID is in `X-Routed-Model` |
| Sync server Phase 6 | Implemented optionally; not required for Desktop proxy MVP |
| Mid-stream provider failover | Not automatic; stream errors surface to client |
| Golden fixtures | Under `testdata/protocol/` (text, tools round-trip, OpenAI tool SSE) |
| Sidecar history / usage / guide | Fail-open (ADR-014); chat continues |

Local proxy loopback extras (not the Anthropic Messages contract):

| Path | Behavior |
|---|---|
| `GET /` | Bilingual operator guide (static) |
| `GET /health` | Liveness; may list sidecar URLs |
| `GET /debug/harness` | Opt-in prompt/tool snapshots (`inspect_prompts`) |
| `GET /debug/usage` | In-memory last request + session spend; **no prompts**. Empty object on sidecar failure, never blocks `/v1/messages` |

Forwarded (local proxy → OpenRouter):

| Item | Behavior |
|---|---|
| `cache_control` (top-level + content/tool/system blocks) | Passed through on OpenRouter chat completions |
| `thinking` / `redacted_thinking` history blocks | Mapped to OpenRouter `reasoning` / `reasoning_details` |
| Request `thinking: {type,budget_tokens}` | Mapped to OpenRouter `reasoning.enabled` / `max_tokens` |
| Reasoning stream deltas | Mapped to Anthropic `thinking_delta` SSE |
| Tool streaming (`input_json_delta`) | Supported (OpenAI tool_calls chunks → Anthropic tool_use) |

## Field-level translation mapping

> **Populated by T048 (inbound) and T049 (outbound).** These tables must exist
> before T053–T055 are implemented. Until then they contain the design intent.

### Inbound → Canonical (Anthropic Messages API — ADR-011 Accepted)

> Full golden fixtures land in T048. Baseline mapping:

| Inbound field | Canonical field | Notes |
|---|---|---|
| `model` | `SourceModel` | Client-visible model / tier |
| `system` | `SystemPrompt` | string or text blocks concatenated |
| `messages` | `Messages` | roles user/assistant |
| `messages[].content[].type=text` | text block | |
| `messages[].content[].type=image` | image block | base64 or url |
| `messages[].content[].type=tool_use` | tool call | |
| `messages[].content[].type=tool_result` | tool result | untrusted |
| `messages[].content[].type=thinking` | thinking block | Forwarded as OpenRouter `reasoning` / `reasoning_details` |
| `messages[].content[].type=redacted_thinking` | thinking block (redacted) | Forwarded as `reasoning.encrypted` detail |
| `tools` | `Tools` | `input_schema` → schema; optional `cache_control` |
| `tool_choice` | `ToolChoice` | mapped to OpenAI `tool_choice` |
| `temperature` / `top_p` | `Temperature` / `TopP` | forwarded when present |
| `stream` | `Stream` | |
| `max_tokens` | `MaxTokens` | required inbound |
| `stop_sequences` | `StopSequences` | |
| `cache_control` | `CacheControl` (+ per-block) | forwarded upstream |
| `thinking` | `Thinking` | mapped to OpenRouter `reasoning` |

### Canonical → Outbound (OpenAI-compatible)

| Canonical field | OpenAI field | Translation notes | MVP? |
|---|---|---|---|
| `RequestID` | `—` (not forwarded) | Gateway-internal | Yes |
| `Messages[].Role` | `messages[].role` | Direct map: `user`→`user`, `assistant`→`assistant` | Yes |
| `Messages[].Content[].Text` | `messages[].content` (string) | Direct | Yes |
| `Messages[].Content[].Image` | `messages[].content[].image_url` | Base64 wrapped in `data:` URL | Yes |
| `SystemPrompt` | `messages[0]` with `role: system` | Prepended to messages array | Yes |
| `Tools[]` | `tools[]` with `function` type | `input_schema` → `parameters` | Yes |
| `ToolCalls[]` | `tool_calls[]` in assistant message | `tool_use` id → `tool_call_id` | Yes |
| `ToolResults[]` | messages with `role: tool` | `tool_result` → `tool` role with `tool_call_id` | Yes |
| `Stream` | `stream: true` | Direct | Yes |
| `MaxTokens` | `max_tokens` | Direct | Yes |
| `StopSequences` | `stop` | Direct | Yes |
| `Thinking` | `reasoning` / `reasoning_details` | History thinking blocks + request `thinking` → OpenRouter reasoning | Yes |
| `CacheControl` | `cache_control` | Top-level and per-block / tool | Yes |
| `ResponseFormat` | `response_format` | No Anthropic equivalent; rejected with `unsupported_capability` | **No** |
| `ToolChoice` | `tool_choice` | Anthropic auto/any/tool → OpenAI auto/required/function | Yes |
| `Temperature` / `TopP` | `temperature` / `top_p` | Forwarded when present | Yes |
| `Usage.InputTokens` | `usage.prompt_tokens` | Direct | Yes |
| `Usage.OutputTokens` | `usage.completion_tokens` | Direct | Yes |
| `Usage.CacheReadTokens` | _(extension map)_ | Stored as `x_cache_read_tokens`; no OpenAI equivalent | Yes (stored) |
| `FinishReason.EndTurn` | `finish_reason: stop` | | Yes |
| `FinishReason.MaxTokens` | `finish_reason: length` | | Yes |
| `FinishReason.ToolUse` | `finish_reason: tool_calls` | | Yes |
| `FinishReason.StopSequence` | `finish_reason: stop` | | Yes |


## 9router contract (T006)

> Verified from public 9Router documentation and GitHub README (decolua/9router)
> on 2026-09-07. Fixture file: `testdata/providers/9router/contract.json`.

| Item | Value |
|---|---|
| Compatibility | OpenAI-compatible |
| Default base URL | `http://localhost:20128/v1` |
| Chat endpoint | `POST /v1/chat/completions` |
| Models endpoint | `GET /v1/models` |
| Auth | `Authorization: Bearer <dashboard-api-key>` |
| Streaming | SSE (`stream: true`) |
| Model IDs | Provider-prefixed (e.g. `kr/claude-sonnet-4.5`, `cc/claude-opus-4-7`) |
| Extra | Also exposes Claude-shaped `POST /v1/messages`; **outbound adapter uses OpenAI chat completions only** |

Deviations from stock OpenAI: model ID prefixes and local default host. No
hard-coded host in routing — users configure `base_url`. T039 may proceed as
an OpenAI-compatible profile with documented defaults.

## Sync server contract

> **Phase 6 — Deferred from initial scope.** These endpoints are design
> contracts only until T092 passes. The local schema includes sync stub
> columns (T093) so that activation does not require DDL changes.

Design-level endpoints:

```text
GET  /v1/health
GET  /v1/sync/changes?cursor=<cursor>&limit=<limit>
POST /v1/sync/push
POST /v1/sync/ack
GET  /v1/attachments/{sha256}
PUT  /v1/attachments/{sha256}
```

All data endpoints require authenticated, authorized requests. Push operations
must be idempotent. Change pages must be cursor-based and deterministic.
Conflict responses must identify the stale parent and preserved versions.

## Error contract

Machine-readable categories:

```text
invalid_config
unsupported_client_integration
unsupported_capability
provider_auth
provider_rate_limit
provider_transient
provider_protocol
request_cancelled
history_conflict
sync_auth
sync_checksum
storage
internal
```

Every error includes a safe message, category, retryability, and correlation
ID. Provider payloads and secrets are excluded unless a redacted diagnostic
fixture explicitly proves safety.

## Compatibility policy

An API is supported only when:

1. request and response fixtures exist,
2. streaming behavior is tested where applicable,
3. error and cancellation behavior is tested,
4. limits and authentication are documented,
5. the relevant architecture and decision documents are updated.
