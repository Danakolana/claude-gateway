# API and Protocol Contracts

> **Authoring model:** GPT-5.6 Luna  
> **Revision:** 2026-09-07 · Reviewed and revised by Claude Sonnet 4.6, 2026-09-07  
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
| _(pending T005)_ | _(pending T005)_ | _(pending T005)_ | _(pending T005)_ | _(pending T005)_ | ADR-011 not yet accepted |

## Field-level translation mapping

> **Populated by T048 (inbound) and T049 (outbound).** These tables must exist
> before T053–T055 are implemented. Until then they contain the design intent.

### Inbound → Canonical (determined by ADR-011)

| Inbound field | Canonical field | Notes |
|---|---|---|
| _(pending T005 / T048)_ | | |

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
| `Thinking` | _(no standard equivalent)_ | Rejected with `unsupported_capability` unless provider declares thinking support | **No — MVP exclude** |
| `ResponseFormat` | `response_format` | No Anthropic equivalent; rejected with `unsupported_capability` | **No — MVP exclude** |
| `Usage.InputTokens` | `usage.prompt_tokens` | Direct | Yes |
| `Usage.OutputTokens` | `usage.completion_tokens` | Direct | Yes |
| `Usage.CacheReadTokens` | _(extension map)_ | Stored as `x_cache_read_tokens`; no OpenAI equivalent | Yes (stored) |
| `FinishReason.EndTurn` | `finish_reason: stop` | | Yes |
| `FinishReason.MaxTokens` | `finish_reason: length` | | Yes |
| `FinishReason.ToolUse` | `finish_reason: tool_calls` | | Yes |
| `FinishReason.StopSequence` | `finish_reason: stop` | | Yes |

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
