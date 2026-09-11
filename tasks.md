# Claude Desktop Gateway — Implementation Tasks

> **Authoring model:** GPT-5.6 Luna  
> **Revision:** 2026-09-11 · Phase 9 fail-open sidecars (T140–T151) added by Cursor Grok 4.6  
> **Status:** Revised — review recommendations applied; sidecars appended  
> **Source requirements:** [requirements.md](requirements.md)  
> **Design:** [design.md](design.md)

## How to use this task list

Each task has one outcome, a bounded scope, an explicit dependency, and a
verification step. An agent must not expand a task into unrelated cleanup.
When implementation changes an architectural fact, decision, API, domain
behavior, deployment behavior, or development workflow, documentation must be
updated in the same task.

Every task completion note must include:

```text
Implementation:
Tests:
Documentation check:
- Architecture changed? [yes/no]
- Decision changed? [yes/no]
- API changed? [yes/no]
- Domain behavior changed? [yes/no]
- Deployment changed? [yes/no]
- Development workflow changed? [yes/no]
```

## Dependency legend

- `ROOT`: no implementation dependency.
- `F`: foundation task.
- `V`: vertical slice.
- `H`: hardening.
- `D`: documentation-only or documentation-support task.

## Phase 0 — Repository and documentation foundation

### T000 — Initialize Go module

- **Outcome:** A valid Go module exists with the selected module path.
- **Depends on:** ROOT.
- **Files:** `go.mod`, `go.sum`.
- **Steps:** Choose the supported Go version; initialize the module; record the
  choice in development documentation.
- **Tests:** Run `go test ./...` on an empty package tree.
- **Done when:** The module resolves and the documented Go version matches CI.
- **Docs:** Update `docs/DEVELOPMENT.md` and `docs/PROJECT.md`.

### T001 — Create command entrypoint

- **Outcome:** The main CLI binary starts and prints help.
- **Depends on:** T000.
- **Files:** `cmd/claude-gateway/main.go`.
- **Tests:** Execute the binary with no arguments and `--help`.
- **Done when:** Help exits zero and no business logic is in `main.go`.

### T002 — Add server entrypoint

- **Outcome:** A separate server binary starts in help mode.
- **Depends on:** T000.
- **Files:** `cmd/gateway-server/main.go`.
- **Tests:** Execute `gateway-server --help`.
- **Done when:** Server lifecycle is separate from CLI command parsing.
- **Docs:** Update `docs/DEPLOYMENT.md`.

### T003 — Add formatting and static checks

- **Outcome:** Repository commands exist for formatting, tests, vet, and static
  analysis.
- **Depends on:** T000.
- **Files:** `Makefile` or `Taskfile`, CI workflow, `tools.go`, `docs/DEVELOPMENT.md`.
- **Tests:** Run each command on the baseline repository.
- **Done when:** `staticcheck` (or `golangci-lint`) version is pinned in
  `tools.go` and the CI workflow uses the pinned version. Commands fail clearly
  when a check fails.
- **Docs:** Update `docs/DEVELOPMENT.md` with pinned tool versions.

### T004a — Add documentation lint scaffold

- **Outcome:** A lint command validates required headings in every `docs/` file.
- **Depends on:** T000.
- **Files:** `tools/docscheck/main.go`, CI workflow.
- **Tests:** Fixture file missing a required heading causes exit 1.
- **Done when:** Documentation validation runs in CI on every push.

### T004b — Add requirement and task ID cross-reference check

- **Outcome:** The lint command verifies that every requirement ID referenced in
  `tasks.md` exists in `requirements.md`, and every task ID referenced in
  `requirements.md` exists in `tasks.md`.
- **Depends on:** T004a.
- **Files:** `tools/docscheck/main.go`.
- **Tests:** A fixture with a dangling requirement reference causes exit 1.
- **Done when:** Documentation cross-reference check is part of the normal CI
  feedback loop.

### T005 — Verify Claude Desktop custom endpoint mechanism ⚑ CRITICAL GATE

> **Completed 2026-09-07 (Composer):** ADR-011 Accepted — Claude Desktop on 3P
> gateway with Anthropic Messages API inbound. See `docs/DECISIONS.md`.

- **Outcome:** A tested compatibility record in `docs/DECISIONS.md` (ADR-011)
  states exactly which Claude Desktop configuration mechanism — if any — allows
  directing model inference traffic to a user-controlled endpoint, the Claude
  Desktop version and OS tested, and the test method used.
- **Depends on:** ROOT (no implementation dependency).
- **Files:** `docs/DECISIONS.md` (ADR-011), `docs/API.md` (compatibility
  matrix entry), `testdata/clientintegration/`.
- **Steps:**
  1. Review official Claude Desktop documentation for any `base_url`,
     `apiEndpoint`, or equivalent field in `claude_desktop_config.json`.
  2. Explicitly distinguish MCP tool-server integration (JSON-RPC 2.0, tools
     and context) from model endpoint routing (HTTP REST, inference).
  3. Capture network traffic from Claude Desktop to confirm the endpoint it
     actually calls under the tested configuration.
  4. Record: supported mechanism or "none", Claude Desktop version, OS, date.
  5. Write ADR-011 with status "Accepted" and the compatibility matrix entry.
- **Tests:** ADR-011 exists in `docs/DECISIONS.md` with status "Accepted" and a
  dated entry. Documentation lint verifies the ADR exists before any file under
  `internal/protocol/` is created.
- **Done when:** ADR-011 is recorded. If the result is "none", FR-PROXY-002 is
  blocked and Phase 3 proxy scope must be revised before any proxy coding starts.
- **Docs:** Write ADR-011 in `docs/DECISIONS.md`; add compatibility matrix row
  in `docs/API.md`.

> **⚑ Phase 3 (proxy coding) and Phase 4 (client integration) are blocked until
> this task is complete and ADR-011 status is "Accepted".**

### T006 — Verify 9router API contract ⚑ GATE FOR T039

> **Completed 2026-09-07 (Composer):** OpenAI-compatible at
> `http://localhost:20128/v1`. Contract in `testdata/providers/9router/contract.json`.

- **Outcome:** The actual 9router request paths, authentication scheme,
  streaming format, error codes, and any deviations from the OpenAI-compatible
  protocol are recorded from verified documentation or controlled fixture tests.
- **Depends on:** ROOT.
- **Files:** `docs/API.md` (9router section), `testdata/providers/9router/`.
- **Steps:**
  1. Obtain 9router API documentation or access.
  2. For each field that differs from the OpenAI `/v1/chat/completions` contract,
     document the deviation and its required adapter behavior.
  3. Record any deviations as fixture files in `testdata/providers/9router/`.
- **Tests:** `testdata/providers/9router/contract.json` exists and is referenced
  in `docs/API.md`. Documentation lint verifies the contract file exists.
- **Done when:** The 9router contract is either (a) confirmed OpenAI-compatible
  (T039 proceeds as planned) or (b) documented with deviations (T039 becomes a
  custom adapter task). If the protocol is incompatible and undocumented, T039
  is cancelled and 9router is deferred.
- **Docs:** Update `docs/API.md` 9router section.

### T007 — Pin Go version and specify CI toolchain

- **Outcome:** Go version, `staticcheck` version, and `golangci-lint`
  configuration are committed and enforced in CI.
- **Depends on:** T000.
- **Files:** `tools.go`, `.github/workflows/ci.yml` (or equivalent),
  `docs/DEVELOPMENT.md`.
- **Tests:** CI workflow installs pinned tools; passes on a clean repository
  clone. A mismatched tool version causes a CI failure.
- **Done when:** Another agent can reproduce the build without guessing tool
  versions.
- **Docs:** Update `docs/DEVELOPMENT.md` with Go version and tool version table.

## Phase 1 — Configuration vertical slice

### T010 — Define configuration domain types

- **Outcome:** Go types represent providers, models, profiles, routing, and
  history settings without secrets as printable strings.
- **Depends on:** T000.
- **Files:** `internal/config/model.go`.
- **Tests:** Construct valid and invalid values.
- **Done when:** Domain types have no CLI or TOML parsing dependencies.

### T011 — Parse minimal TOML

- **Outcome:** A TOML file loads into configuration types.
- **Depends on:** T010.
- **Files:** `internal/config/load.go`, `go.mod`.
- **Tests:** Valid file, missing file, malformed TOML.
- **Done when:** Parser errors include path and field context.

### T012 — Implement configuration discovery

- **Outcome:** Explicit path and documented default paths resolve deterministically.
- **Depends on:** T011.
- **Files:** `internal/config/discovery.go`.
- **Tests:** Temporary directories for explicit, environment, and default paths.
- **Done when:** Discovery order is covered by tests on supported platforms.

### T013 — Implement environment overrides

- **Outcome:** Declared environment variables override allowed fields.
- **Depends on:** T011.
- **Files:** `internal/config/env.go`.
- **Tests:** Valid override, invalid type, undeclared path.
- **Done when:** Arbitrary environment mutation is rejected.

### T014 — Implement secret references

- **Outcome:** Environment-based secret handles resolve only at operation time.
- **Depends on:** T010.
- **Files:** `internal/secrets`.
- **Tests:** Resolution success, missing secret, redacted stringification.
- **Done when:** Debug output cannot reveal resolved values.

### T015 — Implement profile selection

- **Outcome:** Active profile selection produces an effective configuration.
- **Depends on:** T011, T014.
- **Files:** `internal/config/resolve.go`.
- **Tests:** Explicit profile, active profile, missing profile.
- **Done when:** Effective values retain source metadata.

### T016 — Implement validation errors

- **Outcome:** Validation reports all actionable errors in one invocation,
  including SSRF-unsafe provider URLs.
- **Depends on:** T015.
- **Files:** `internal/config/validate.go`, `internal/security/urlpolicy.go`.
- **Tests:** Missing URL, invalid scheme, duplicate model alias, route cycle.
  SSRF tests: loopback `127.0.0.1`, IPv6 loopback `::1`, link-local
  `169.254.x.x`, RFC1918 ranges (`10.x`, `172.16–31.x`, `192.168.x`), and
  scheme-less URLs are all rejected. Valid HTTPS external URL passes.
- **Done when:** Errors contain field, reason, and remediation. Provider
  `base_url` values are validated against the SSRF policy before the
  configuration is accepted. No provider call is possible with an unsafe URL.

### T017 — Add `config validate`

- **Outcome:** Users can validate a configuration from the CLI.
- **Depends on:** T016, T001.
- **Files:** `internal/cli/config_validate.go`.
- **Tests:** CLI success and failure exit codes.
- **Done when:** Invalid configuration cannot reach mutation code.

### T018 — Add `config explain`

- **Outcome:** Users can inspect effective redacted configuration.
- **Depends on:** T015, T001.
- **Files:** `internal/cli/config_explain.go`.
- **Tests:** Assert secrets and authorization headers never appear.
- **Done when:** Output identifies value origins and selected profile.

### T019 — Add named profile commands

- **Outcome:** Users can list, create, select, and remove profiles.
- **Depends on:** T017.
- **Files:** `internal/cli/profile.go`.
- **Tests:** Each mutation preserves unrelated profiles.
- **Done when:** Profile operations are idempotent where appropriate.

### T020 — Configuration slice checkpoint

- **Outcome:** A user can create a profile, validate it, and explain it.
  SSRF-unsafe provider URLs are rejected at `config validate`. Secret values
  never appear in `config explain` output.
- **Depends on:** T019.
- **Tests:** End-to-end CLI test using a temporary config and environment key.
  Confirm that a profile with a loopback provider URL fails validation with a
  clear SSRF error message.
- **Done when:** The vertical slice works without a provider network call.
- **Docs:** Update `docs/DOMAIN.md`, `docs/API.md`, and `docs/DEVELOPMENT.md`.

## Phase 2 — Provider and routing vertical slice

### T030a — Define canonical message and request types

- **Outcome:** Provider-independent message, content block, system prompt, and
  request metadata types exist without streaming or tool-specific types.
- **Depends on:** T010.
- **Files:** `pkg/api/request.go`.
- **Tests:** Serialization round-trip, required-field validation, nil-safety.
- **Done when:** Types have no CLI, TOML, or provider-protocol dependencies.
- **Docs:** Update `docs/API.md` canonical protocol section.

### T030b — Define canonical streaming event types

- **Outcome:** Streaming event types (text delta, reasoning delta, tool-call
  delta, usage, finish, error) exist as a closed sum type.
- **Depends on:** T030a.
- **Files:** `pkg/api/events.go`.
- **Tests:** Each event type serializes and deserializes correctly. Unknown event
  types produce an explicit error, not a silent zero value.
- **Done when:** Exactly one terminal event type exists (finish or error).

### T030c — Define canonical tool types

- **Outcome:** Tool definition, tool call, and tool result types exist.
- **Depends on:** T030a.
- **Files:** `pkg/api/tools.go`.
- **Tests:** Round-trip serialization. Multi-tool call ordering is preserved.
  Tool result content is typed as untrusted string, not parsed JSON.
- **Done when:** Tool types are independent of the Anthropic or OpenAI wire
  format.

### T030d — Define canonical error types

- **Outcome:** Every error category from the taxonomy in `docs/API.md` has a
  typed Go constant and a safe-message + retryability pair.
- **Depends on:** T030a.
- **Files:** `pkg/api/errors.go`.
- **Tests:** Each category serializes to a stable machine-readable code. Test
  that error stringification never reveals a secret handle value.
- **Done when:** Error types have no dependency on HTTP status codes (those are
  mapped at the adapter layer).
- **Docs:** Update `docs/API.md` error contract section.

### T031 — Define capability types

- **Outcome:** Capabilities and requirements have explicit tri-state semantics:
  supported, unsupported, and unknown.
- **Depends on:** T030a, T030c.
- **Files:** `pkg/api/capabilities.go`.
- **Tests:** Compatibility matrix tests. Unknown is never equal to supported in
  any comparison function.
- **Done when:** Unknown is never equal to supported.

### T032 — Define adapter interface

- **Outcome:** A provider adapter interface covers health, send, stream,
  capabilities, and normalized errors.
- **Depends on:** T030a, T030b, T030c, T030d, T031.
- **Files:** `internal/provider/provider.go`.
- **Tests:** Compile a fake adapter implementing the interface.
- **Done when:** Interface does not depend on a concrete provider.

### T033 — Implement fake provider

- **Outcome:** Deterministic provider behavior is available for tests.
- **Depends on:** T032.
- **Files:** `internal/provider/fake`.
- **Tests:** Text, streaming, tool call, timeout, error, and mid-stream
  disconnect fixtures.
- **Done when:** No network is needed for proxy tests.

### T048 — Inbound protocol specification document ⚑ DEPENDS ON T005/ADR-011

- **Outcome:** `docs/API.md` inbound section documents: every supported
  Anthropic API request field, every unsupported field, the streaming event
  sequence, the error shape, and the field-by-field mapping to canonical types
  defined in T030a–T030d.
- **Depends on:** T005 (ADR-011 accepted), T030a, T030b, T030c, T030d.
- **Files:** `docs/API.md`, `testdata/protocol/inbound/`.
- **Tests:** Documentation lint verifies every canonical request field maps to
  a stated inbound source field or is explicitly marked as gateway-internal.
  At least three golden request fixtures exist: text-only, streaming with tools,
  and unsupported reasoning field (must produce `unsupported_capability` error).
- **Done when:** A developer can implement T053 by reading this document without
  guessing any field mapping.
- **Docs:** Update `docs/API.md`.

### T049 — Outbound protocol specification document

- **Outcome:** `docs/API.md` outbound section documents the OpenAI-compatible
  request shape, the streaming SSE event sequence, the tool call format, the
  stop-reason mapping table, the error-code mapping table, and per-field
  translation rules including fields that are dropped and fields stored in the
  extension map.
- **Depends on:** T030a, T030b, T030c, T030d.
- **Files:** `docs/API.md`, `testdata/protocol/outbound/`.
- **Tests:** Documentation lint verifies field-level coverage for all canonical
  types. At least three golden outbound fixtures exist: text, streaming, tool
  call. Stop-reason mapping table is present and complete.
- **Done when:** T034–T036 implementors can write golden fixtures directly from
  this document without guessing OpenAI field names.
- **Docs:** Update `docs/API.md`.

### T034 — Implement OpenAI-compatible request encoder

- **Outcome:** Canonical requests encode to the selected OpenAI-compatible
  request shape per the field-mapping document in T049.
- **Depends on:** T030a, T030b, T030c, T032, T049.
- **Files:** `internal/provider/openai/encode.go`.
- **Tests:** Golden JSON fixtures for text, tools, streaming (per T049
  fixtures). Reasoning/thinking fields produce `unsupported_capability` error.
  Structured-output fields produce `unsupported_capability` error. Unknown
  fields are stored in the extension map, not silently dropped.
- **Done when:** Every field in the canonical request is either mapped,
  rejected, or extension-stored per the policy in T049.

### T035 — Implement OpenAI-compatible response decoder

- **Outcome:** Non-streaming provider responses decode to canonical responses
  per the stop-reason and usage mapping in T049.
- **Depends on:** T030a, T030c, T030d, T034, T049.
- **Files:** `internal/provider/openai/decode.go`.
- **Tests:** Golden JSON for success, tool call, usage, and errors. Each
  OpenAI `finish_reason` maps to the correct canonical stop reason. Usage
  fields map correctly; cache token fields are preserved in the extension map.
- **Done when:** Malformed responses produce classified `provider_protocol`
  errors. Every stop reason in the mapping table is covered by a test.

### T036 — Implement OpenAI-compatible stream decoder

- **Outcome:** SSE streaming chunks become canonical streaming events per T049.
- **Depends on:** T030b, T030c, T030d, T034, T049.
- **Files:** `internal/provider/openai/stream.go`.
- **Tests:** Complete stream, fragmented SSE lines, terminal error event,
  mid-stream TCP disconnect (must emit exactly one terminal error event).
  Verify that no event is lost or duplicated when lines arrive in arbitrary
  chunk boundaries.
- **Done when:** Exactly one terminal event is emitted per stream, regardless
  of how the connection terminates.

### T037 — Implement OpenRouter profile

- **Outcome:** OpenRouter settings produce a validated OpenAI-compatible
  provider with TLS enforcement.
- **Depends on:** T032, T034, T035, T036.
- **Files:** `internal/provider/openrouter`.
- **Tests:** Header redaction (no authorization header in logs), base URL
  override, model passthrough. TLS 1.2+ enforced; `tls_verify = false` emits
  a WARN log entry. Optional site metadata headers (`X-Title`, `HTTP-Referer`)
  are configurable and redacted in logs.
- **Done when:** No host is hard-coded into routing. TLS minimum version is
  enforced on the HTTP client.
- **Docs:** Update `docs/API.md` and `docs/DECISIONS.md`.

### T038 — 9router contract verification (retasked → see T006)

> **This task has been moved to Phase 0 as T006.** T006 must complete before
> this entry. When T006 is done, replace this task with the implementation
> outcome: either T039 proceeds as described, or T039 is redesigned as a
> custom adapter based on the T006 findings.

### T039 — Implement 9router profile ⚑ CONDITIONAL ON T006

- **Outcome:** 9router can be selected as a configured provider, with its
  adapter behavior consistent with the verified contract recorded in T006.
- **Depends on:** T006 (outcome recorded in `docs/API.md`), T034, T035, T036.
- **Files:** `internal/provider/nine`.
- **Tests:** 9router request, stream, health, and error fixtures derived
  directly from the T006 contract record. Every deviation from OpenAI-compatible
  behavior is covered by a contract test.
- **Done when:** Provider-specific differences are isolated in the adapter.
  T039 is **cancelled** and replaced with a custom adapter task if T006 records
  an incompatible protocol. If T006 records "no documentation available", T039
  is deferred until documentation exists.
- **Docs:** Update `docs/API.md` 9router section.

### T040 — Implement custom gateway profile

- **Outcome:** Arbitrary OpenAI-compatible base URL and auth scheme work through
  configuration, with TLS enforcement and SSRF protection active from the first
  request.
- **Depends on:** T037.
- **Files:** `internal/provider/custom`.
- **Tests:** Local HTTP fixture with bearer, API-key header, and custom header.
  `tls_verify = false` emits WARN log. HTTP client enforces TLS 1.2 minimum.
  A provider URL that resolves to a private IP at request time is rejected by
  the runtime URL check (defense-in-depth beyond config-time validation in T016).
- **Done when:** Insecure TLS requires explicit opt-in and is warned. TLS
  minimum version is enforced.

### T041 — Implement model registry

- **Outcome:** Model IDs, aliases, display names, tiers, and capabilities can
  be loaded and queried.
- **Depends on:** T015, T031.
- **Files:** `internal/routing/registry.go`.
- **Tests:** Duplicate alias, disabled model, unknown capability.
- **Done when:** Registry queries are deterministic.

### T042 — Implement exact replacement routing

- **Outcome:** A source model maps to one target model.
- **Depends on:** T041.
- **Files:** `internal/routing/resolve.go`.
- **Tests:** Match, no match, disabled target, missing target.
- **Done when:** Routing result includes explainable metadata.

### T043 — Implement tier routing

- **Outcome:** A source tier maps to a target model or tier.
- **Depends on:** T042.
- **Files:** `internal/routing/resolve.go`.
- **Tests:** Precedence between exact and tier rules.
- **Done when:** Precedence is documented and tested.

### T044 — Implement capability filtering

- **Outcome:** Router rejects or filters targets that cannot satisfy required
  capabilities according to policy, including context-window overflow.
- **Depends on:** T031, T042.
- **Files:** `internal/routing/capability.go`.
- **Tests:** Required tool/stream/vision/reasoning cases. Context overflow:
  routing selects a model with declared context 4096 tokens; a request
  annotated with 8000 tokens is either rejected with `unsupported_capability`
  or redirected to a fallback with a larger context. Silent truncation must
  never occur.
- **Done when:** Unknown capability follows explicit policy. Context overflow
  is an explicit error, not a silent pass-through.

### T045 — Implement fallback selection

- **Outcome:** A failed eligible target selects the next configured target.
- **Depends on:** T044.
- **Files:** `internal/routing/fallback.go`.
- **Tests:** Transient failure, unsupported capability, exhausted fallbacks.
- **Done when:** Required capability is never silently dropped.

### T046 — Implement retry policy

- **Outcome:** Retry counts, backoff, rate-limit hints, and cancellation are
  enforced.
- **Depends on:** T045.
- **Files:** `internal/routing/retry.go`.
- **Tests:** Retryable/non-retryable/status/rate-limit/cancellation cases.
- **Done when:** Streaming retries are handled according to documented policy.

### T047 — Provider/routing slice checkpoint

- **Outcome:** A canonical request can be routed to a fake or fixture provider
  with explainable model selection, context overflow handling, and capability
  rejection.
- **Depends on:** T046, T049.
- **Tests:** End-to-end in-memory route with streaming and tool call. Confirm
  routing metadata (source model, target model, fallback attempt, capability
  decision) is present in the response. Confirm context overflow route
  produces `unsupported_capability` error, not a provider call.
- **Done when:** No CLI or proxy layer contains routing logic.
- **Docs:** Update `docs/ARCHITECTURE.md`, `docs/API.md`, and
  `docs/DECISIONS.md`.

## Phase 3 — Local proxy vertical slice

> **⚑ Phase 3 is blocked until T005 (ADR-011) is complete.** The inbound
> protocol codec (T053) implements the mechanism identified in ADR-011.

### T050 — Add loopback HTTP lifecycle

- **Outcome:** Proxy starts, reports its address, and shuts down cleanly.
- **Depends on:** T047, T005.
- **Files:** `internal/proxy/server.go`.
- **Tests:** Start/stop, loopback binding, graceful shutdown. A non-loopback
  bind address emits a `WARN: proxy binding to non-loopback address <addr>;
  ensure firewall rules restrict access` log entry at WARN level, and the
  test asserts this entry is present.
- **Done when:** Non-loopback binding requires explicit configuration and
  always produces a visible warning.

### T051 — Add request size and timeout middleware

- **Outcome:** Oversized requests and expired deadlines fail safely.
- **Depends on:** T050.
- **Files:** `internal/proxy/middleware.go`.
- **Tests:** Size limit, deadline, malformed content type.
- **Done when:** Limits are configurable and documented.

### T052 — Add correlation IDs and safe logging

- **Outcome:** Every request has a correlation ID and redacted structured logs.
  Tool-result content is stored as an opaque string in structured log fields,
  never interpolated as JSON or parsed further.
- **Depends on:** T050.
- **Files:** `internal/observability`, `internal/proxy`.
- **Tests:** Assert API keys, bearer tokens, cookies, prompts, and tool
  payloads are absent from log output. Assert tool-result content containing
  JSON special characters (`"`, `\n`, `{`) does not cause structured log
  output to be malformed or re-parseable as control sequences. Assert
  correlation ID is present in every error response.
- **Done when:** Errors include correlation IDs without secrets. Tool results
  are never parsed or interpreted by the log layer.

### T053 — Decode inbound messages request ⚑ PROTOCOL DETERMINED BY ADR-011

- **Outcome:** Supported inbound request JSON (per the mechanism in ADR-011)
  becomes a canonical request.
- **Depends on:** T030a, T030b, T030c, T030d, T050, T048.
- **Files:** `internal/protocol/inbound/<mechanism>/decode.go`
  (sub-package name resolved from ADR-011: e.g., `anthropic` or `mcp`).
- **Tests:** Golden fixtures from T048: text, tools, streaming, malformed,
  unknown field. Unsupported fields produce `unsupported_capability` errors per
  the T048 specification. Reasoning/thinking field produces
  `unsupported_capability` error.
- **Done when:** Protocol validation errors are stable and match the T048
  specification exactly.

### T054 — Encode outbound messages response

- **Outcome:** Canonical non-streaming responses become client-compatible JSON
  in the protocol format determined by ADR-011.
- **Depends on:** T030a, T030c, T030d, T053.
- **Files:** `internal/protocol/inbound/<mechanism>/encode.go`.
- **Tests:** Golden fixtures: text, usage, tool call, terminal error. Each
  canonical stop reason maps to the correct client-protocol value. Usage fields
  round-trip without loss.
- **Done when:** Golden fixtures document the supported surface. Every field
  mapping is traceable to T048.

### T055 — Encode outbound message stream

- **Outcome:** Canonical streaming events become valid client-compatible
  streaming events in the protocol format determined by ADR-011.
- **Depends on:** T030b, T053.
- **Files:** `internal/protocol/inbound/<mechanism>/stream.go`.
- **Tests:** Event order, fragmented delivery, client disconnect mid-stream,
  terminal event. Test proves no event is lost or duplicated across arbitrary
  chunk boundaries.
- **Done when:** Exactly one terminal event is emitted per stream. Client
  disconnect cancels the upstream request within the request context deadline.

### T056 — Connect proxy to router

- **Outcome:** An inbound request reaches the selected adapter and returns a
  response or stream.
- **Depends on:** T047, T054, T055.
- **Files:** `internal/proxy/handler.go`.
- **Tests:** Fake provider end-to-end HTTP tests.
- **Done when:** Cancellation propagates through all layers.

### T057 — Add proxy health endpoint

- **Outcome:** Local health/readiness reports configuration and provider state
  without secrets.
- **Depends on:** T050, T016.
- **Files:** `internal/proxy/health.go`.
- **Tests:** Healthy, invalid config, provider unavailable.
- **Done when:** Health output is safe for local diagnostics.

### T058 — Add proxy CLI commands

- **Outcome:** Users can start, stop/check, and inspect the proxy.
- **Depends on:** T056, T057.
- **Files:** `internal/cli/proxy.go`.
- **Tests:** Help, invalid profile, startup, shutdown.
- **Done when:** Foreground mode is reliable before background mode is added.

### T059 — Proxy slice checkpoint

- **Outcome:** A local client-compatible request can stream through the proxy to
  a fake provider. Cancellation, tool calls, and context overflow all behave
  correctly.
- **Depends on:** T058.
- **Tests:** Full golden HTTP test covering: streaming text, tool call
  round-trip, client disconnect (upstream is cancelled, history records
  `incomplete`), context overflow rejection. Non-loopback binding warning is
  asserted.
- **Done when:** Protocol limitations are documented in `docs/API.md`.
- **Docs:** Update `docs/API.md`, `docs/ARCHITECTURE.md`, and `docs/DOMAIN.md`.

## Phase 4 — Claude Desktop configuration integration

> **⚑ This phase implements the integration mechanism confirmed by T005
> (ADR-011).** The render step (T062) uses the supported configuration
> surface, not an assumed one.

### T060 — Implement platform path abstraction

- **Outcome:** Windows and Linux Claude Desktop path resolution is isolated.
- **Depends on:** T002.
- **Files:** `internal/platform`, `internal/clientintegration`.
- **Tests:** Injected home/config paths; no host-specific assumptions.
- **Done when:** Tests run on both target platforms.

### T061 — Implement configuration snapshot

- **Outcome:** Existing client configuration can be read, checksummed, and
  stored as a backup record.
- **Depends on:** T060.
- **Files:** `internal/clientintegration/backup.go`.
- **Tests:** Missing file, valid file, checksum mismatch.
- **Done when:** Snapshot never contains unredacted secret output in logs.

### T062 — Implement candidate renderer

- **Outcome:** A selected profile renders a candidate client configuration or
  override artifact without writing it.
- **Depends on:** T015, T061.
- **Files:** `internal/clientintegration/render.go`.
- **Tests:** Golden output, unsupported endpoint mode, redaction.
- **Done when:** Rendering is deterministic.

### T063 — Implement redacted diff

- **Outcome:** Users can review planned configuration changes.
- **Depends on:** T062.
- **Files:** `internal/clientintegration/diff.go`.
- **Tests:** Added/changed/removed keys and secret placeholders.
- **Done when:** Diff never prints secret values.

### T064 — Implement atomic apply

- **Outcome:** Candidate configuration is backed up and atomically applied.
- **Depends on:** T061, T063.
- **Files:** `internal/clientintegration/apply.go`.
- **Tests:** Failure before rename, permission failure, successful write.
- **Done when:** Failed writes leave the original intact.

### T065 — Implement restore

- **Outcome:** A selected backup can be restored with a new rollback backup.
- **Depends on:** T064.
- **Files:** `internal/clientintegration/restore.go`.
- **Tests:** Restore success, incompatible backup, rollback on failure.
- **Done when:** Restore is explicit and never automatic-destructive.

### T066 — Add client dry-run/apply/restore commands

- **Outcome:** CLI exposes the client integration workflow.
- **Depends on:** T062, T063, T064, T065.
- **Files:** `internal/cli/client.go`.
- **Tests:** End-to-end temporary-file tests.
- **Done when:** `--dry-run` performs no writes.

### T067 — Claude Desktop endpoint verification (retasked → see T005)

> **This task has been moved to Phase 0 as T005.** T005 must be complete
> before Phase 3 or Phase 4 coding begins. This entry is retained for
> traceability. When T005 is done, record its result here as a cross-reference.

### T069 — Claude Desktop version compatibility matrix

- **Outcome:** A versioned table in `docs/API.md` records, for each tested
  Claude Desktop version and OS, which configuration mechanism is supported
  (none, MCP tool-server, custom model endpoint), the test method, and the
  test date.
- **Depends on:** T005.
- **Files:** `docs/API.md`.
- **Tests:** Documentation lint verifies the matrix has at least one entry
  with version number, OS, test method, and date.
- **Done when:** Every integration claim made by `client apply` or `client
  launch` is attributable to a row in this matrix. Claims without a matrix
  row cause a documentation lint failure.
- **Docs:** Update `docs/API.md`.

### T068 — Client integration slice checkpoint

- **Outcome:** A user can back up, preview, apply, launch/check, and restore a
  supported client configuration. The integration mechanism matches ADR-011.
- **Depends on:** T069.
- **Tests:** Clean-machine procedure. Confirm `--dry-run` performs no writes.
  Confirm recovery procedure succeeds after a simulated bad write. Confirm
  that an unsupported integration mode produces a clear error rather than
  silently claiming success.
- **Done when:** Recovery procedure succeeds after a simulated bad change. ADR-011
  mechanism is the only integration surface that `client apply` will attempt.
- **Docs:** Update `docs/DEPLOYMENT.md`, `docs/ARCHITECTURE.md`, and
  `docs/DECISIONS.md`.

## Phase 5 — Local history and portability

### T070a — Define core conversation entity schema

- **Outcome:** Versioned SQLite schema covers `workspace`, `session`,
  `conversation`, `conversation_branch`, `message`, and `content_block`
  entities with stable UUIDs, UTC timestamps, and explicit nullable fields.
- **Depends on:** T030a.
- **Files:** `migrations/001_initial.sql`, `docs/DOMAIN.md`.
- **Tests:** Schema applies to a fresh database. Foreign key constraints are
  verified with `PRAGMA foreign_keys = ON`. All expected indexes exist.
- **Done when:** IDs, timestamps, status values (including `incomplete`), and
  indexes are explicit. No JSON columns for fields with integrity constraints.

### T070b — Define model invocation and provider metadata schema

- **Outcome:** `model_invocation` entity captures provider, source model,
  target model, usage, outcome, and routing metadata.
- **Depends on:** T070a.
- **Files:** `migrations/001_initial.sql`.
- **Tests:** A model invocation record can be inserted and queried with all
  required fields.
- **Done when:** Usage fields accommodate both Anthropic and OpenAI field
  shapes (extension map for cache tokens).

### T070c — Define attachment metadata and sync stub schema

- **Outcome:** `attachment` entity covers content hash (validated hex),
  size, MIME type, path/object key, and encryption metadata. `sync_object`
  stub columns (ID, version, checksum, tombstone, sync state) are added to
  conversation and message tables to enable future migration-free sync
  addition.
- **Depends on:** T070a.
- **Files:** `migrations/001_initial.sql`.
- **Tests:** Attachment row with valid and invalid hex checksum values (invalid
  rejected by schema constraint or application-level check). Sync stub columns
  exist with correct nullable types.
- **Done when:** Attachment hash is a TEXT column with a CHECK constraint
  matching `^[0-9a-f]{64}$`. Sync stub columns are nullable and zero-cost to
  populate as stubs.

### T070d — Define audit event schema and update DOMAIN.md

- **Outcome:** `audit_event` entity captures action, timestamp, profile,
  result, correlation ID, and excludes secret values and prompt contents.
- **Depends on:** T070a.
- **Files:** `migrations/001_initial.sql`, `docs/DOMAIN.md`.
- **Tests:** Audit event insert and query. Assert that no audit event record
  contains a field named `key`, `token`, `secret`, `password`, or `prompt`.
- **Done when:** `docs/DOMAIN.md` documents the audit event entity and its
  privacy invariants.
- **Docs:** Update `docs/DOMAIN.md`.

### T071 — Add migration runner

- **Outcome:** Database migrations apply transactionally and record versions.
  Every connection opens with WAL mode and foreign-key enforcement enabled.
- **Depends on:** T070a, T070b, T070c, T070d.
- **Files:** `internal/history/migrate.go`.
- **Tests:** Fresh database, repeated migration (idempotent), interrupted
  migration (leaves schema at prior version), unknown future version (startup
  refuses with clear error). Confirm `PRAGMA journal_mode = WAL` and
  `PRAGMA foreign_keys = ON` are active after connection open.
- **Done when:** Startup refuses incompatible future schemas safely. All DDL
  within a migration is wrapped in a single transaction.

### T079 — SQLite WAL mode and connection configuration

- **Outcome:** Every database connection is opened with the correct PRAGMAs,
  and concurrent readers + one writer do not produce lock errors.
- **Depends on:** T071.
- **Files:** `internal/history/db.go`.
- **Tests:** Concurrent goroutine test: one writer appending streaming records
  and two readers querying conversation history simultaneously; no errors
  after 1000 iterations. Verify `PRAGMA journal_mode` returns `wal` after
  open.
- **Done when:** WAL mode is confirmed active. A note in `docs/DEPLOYMENT.md`
  warns that WAL mode is unsafe on some network filesystems.

### T072 — Implement conversation append

- **Outcome:** A conversation and ordered message blocks can be appended in one
  transaction. A streaming conversation interrupted by a client disconnect is
  persisted with `incomplete` status and reason `client_cancelled`.
- **Depends on:** T071, T079.
- **Files:** `internal/history/write.go`.
- **Tests:** Commit success, rollback (simulated crash mid-transaction leaves
  no partial record), message ordering preserved, incomplete stream with
  explicit status and reason.
- **Done when:** Partial responses have explicit status. No dangling partial
  records after a simulated crash.

### T073 — Implement history query

- **Outcome:** Users can list and retrieve conversations with filters.
- **Depends on:** T072.
- **Files:** `internal/history/read.go`.
- **Tests:** Workspace, date, profile, provider, and branch filters.
- **Done when:** Queries never expose secret fields accidentally.

### T074 — Implement content-addressed attachments

- **Outcome:** Attachments are stored by checksum with metadata references.
  Path traversal via checksum field is impossible.
- **Depends on:** T071, T070c.
- **Files:** `internal/attachments`.
- **Tests:** Deduplication, checksum mismatch, size limit, missing blob.
  Path traversal: checksum values containing `/`, `..`, null bytes, or non-hex
  characters are rejected before any file-system operation. Atomic write test:
  blob written to temp path, metadata row inserted in transaction, rename
  completes; crash simulation after blob write leaves no committed metadata row.
- **Done when:** Writes are atomic and incomplete blobs are not referenced.
  Path traversal via checksum is impossible and tested.

### T075 — Implement export manifest

- **Outcome:** Selected history exports with a versioned manifest and checksums.
- **Depends on:** T073, T074.
- **Files:** `internal/history/export.go`.
- **Tests:** Empty, single conversation, attachments, redaction.
- **Done when:** Archive is self-describing.

### T076 — Implement import staging

- **Outcome:** Imports validate into staging before database commit. Archive
  bombs and path traversal in archive entries are rejected before extraction.
- **Depends on:** T075.
- **Files:** `internal/history/import.go`.
- **Tests:** Corrupt archive, duplicate IDs (rejected unless `--overwrite`),
  valid round trip, unsupported version. Archive bomb: uncompressed:compressed
  ratio > configured threshold (default 100:1) is rejected before any entry is
  extracted. Entry count limit enforced. ZIP-in-ZIP rejected. Staging blobs are
  cleaned up on import failure (no orphaned files). Failed import leaves
  existing database data unchanged.
- **Done when:** Failed import leaves existing data and attachment directory
  unchanged. Archive safety checks run before extraction begins.

### T077 — Add history CLI commands

- **Outcome:** Users can list, export, import, backup, and inspect local history.
- **Depends on:** T073, T075, T076.
- **Files:** `internal/cli/history.go`.
- **Tests:** CLI round trip with a temporary store.
- **Done when:** JSON output is stable enough for automation.

### T078 — Local history slice checkpoint

- **Outcome:** A proxy interaction is persisted and can be exported/imported.
  Incomplete streams, orphaned blobs, and duplicate imports all behave
  correctly.
- **Depends on:** T059, T077.
- **Tests:** End-to-end stream plus round-trip archive comparison. Simulate a
  mid-stream crash: confirm `incomplete` status. Re-import the same archive:
  confirm duplicate is rejected without `--overwrite`. Archive bomb fixture:
  confirm import rejects before extraction.
- **Done when:** Records, metadata, and attachments survive the round trip.
  Privacy: export manifest includes `contains_prompts = true` flag.
- **Docs:** Update `docs/DOMAIN.md`, `docs/API.md`, and `docs/DEPLOYMENT.md`.

### T093 — Sync cursor and tombstone schema stubs

- **Outcome:** The local SQLite schema includes sync cursor and tombstone
  columns on conversation, message, and attachment tables. These columns are
  stub-populated (zero/null) and not used by any application logic yet. Their
  presence allows Phase 7 sync tasks to activate sync behavior without DDL
  changes to production tables.
- **Depends on:** T070c, T071.
- **Files:** `migrations/` (new migration or part of 001_initial), `internal/history/db.go`.
- **Tests:** Migration applies cleanly. Columns exist with correct nullable
  types. Sync logic (Phase 7) can be enabled without a schema migration that
  alters existing tables.
- **Done when:** Stub columns are present, typed correctly, and never populated
  by history write logic before Phase 7.

## Phase 6 — Remote history server and synchronization

> **Phase 6 is deferred from initial scope.** The local schema stubs (T093)
> enable Phase 6 tasks to activate sync without DDL changes to production
> tables. See `docs/DECISIONS.md` ADR-006 for the deferral rationale.

### T080 — Define sync object envelope

- **Outcome:** Sync objects have IDs, versions, checksums, parent versions,
  tombstones, and tenant/user scope.
- **Depends on:** T070.
- **Files:** `pkg/api/sync.go`.
- **Tests:** Encode/decode and checksum stability.
- **Done when:** Envelope is independent of HTTP.

### T081 — Implement server health endpoint

- **Outcome:** Server reports liveness and readiness without data disclosure.
- **Depends on:** T002.
- **Files:** `internal/server/health.go`.
- **Tests:** Startup, dependency unavailable, safe output.
- **Done when:** Health endpoint has documented semantics.

### T082 — Implement token authentication

- **Outcome:** Server authenticates bearer tokens and supports revocation.
  Inline sync tokens in TOML configuration trigger the same plaintext-secret
  warning as inline provider API keys.
- **Depends on:** T081.
- **Files:** `internal/server/auth.go`.
- **Tests:** Missing token, malformed token, revoked token, expired token,
  valid token. Inline `token = "..."` value in TOML produces a validation
  warning equivalent to the plaintext API-key warning (covered by T014 update).
  Token value never appears in any log output.
- **Done when:** Token values never appear in logs. Sync token requires the
  same secret-reference pattern as provider API keys.

### T083 — Implement tenant authorization

- **Outcome:** Authenticated users cannot read or write another tenant's data.
- **Depends on:** T082.
- **Files:** `internal/server/authorization.go`.
- **Tests:** Cross-tenant read/write denial.
- **Done when:** Authorization is enforced before object access.

### T082c — Auth and authorization integration test

- **Outcome:** T082 and T083 are tested together through a test server
  exercising all combined failure modes.
- **Depends on:** T082, T083.
- **Files:** `internal/server/auth_authz_integration_test.go`.
- **Tests:** Cross-tenant read denial, cross-tenant write denial, expired
  token denial, revoked token denial, successful authorized read and write.
- **Done when:** All five cases pass. No test bypasses authorization by
  constructing internal objects directly.

### T084 — Implement sync push endpoint

- **Outcome:** Server accepts validated idempotent metadata batches.
  Idempotency keys expire after a configured TTL (default 24h).
- **Depends on:** T080, T082c.
- **Files:** `internal/server/sync_push.go`.
- **Tests:** Duplicate idempotency key (returns idempotent response within
  TTL), expired idempotency key (TTL elapsed → rejected or treated as new),
  invalid checksum, stale parent version, batch size limit exceeded.
- **Done when:** Responses identify accepted and conflicted objects.
  Idempotency key expiry is tested and the TTL is configurable.

### T085 — Implement sync changes endpoint

- **Outcome:** Server returns paginated changes after a cursor.
- **Depends on:** T084.
- **Files:** `internal/server/sync_pull.go`.
- **Tests:** Cursor, page size, tenant scope, ordering, expired cursor.
- **Done when:** Cursor advances only over committed changes.

### T086 — Implement attachment upload/download

- **Outcome:** Server stores and serves checksum-addressed attachments safely.
  Path traversal via checksum field is impossible.
- **Depends on:** T082c, T084.
- **Files:** `internal/server/attachments.go`.
- **Tests:** Missing attachment, checksum mismatch, range/chunk limit,
  authorization failure. Path traversal: checksum values containing `/`, `..`,
  null bytes, or non-hex characters are rejected before any file-system
  operation. Test matches the local attachment store tests in T074.
- **Done when:** Path traversal is impossible and tested. Authorization is
  checked before any file is opened.

### T087 — Implement local pending queue

- **Outcome:** Local database records pending pushes and retry metadata.
- **Depends on:** T071, T080.
- **Files:** `internal/sync/queue.go`.
- **Tests:** Enqueue, retry count, backoff, restart recovery.
- **Done when:** Queue survives process interruption.

### T088 — Implement sync push client

- **Outcome:** Client pushes pending metadata idempotently.
- **Depends on:** T084, T087.
- **Files:** `internal/sync/push.go`.
- **Tests:** Offline, retry, accepted, conflict, auth failure.
- **Done when:** Successful objects are acknowledged transactionally.

### T089 — Implement sync pull client

- **Outcome:** Client pulls changes and applies non-conflicting objects.
- **Depends on:** T085, T087.
- **Files:** `internal/sync/pull.go`.
- **Tests:** Cursor, replay, conflict, partial apply, restart.
- **Done when:** Cursor is not advanced after failed local application.

### T090 — Implement conflict branches

- **Outcome:** Stale writes become preserved local conflict branches.
- **Depends on:** T088, T089.
- **Files:** `internal/history/conflicts.go`.
- **Tests:** Two-device divergent edits and repeated sync.
- **Done when:** No version disappears silently.

### T091 — Add sync CLI commands

- **Outcome:** Users can configure, inspect, push, pull, and resolve/list
  conflicts.
- **Depends on:** T088, T089, T090.
- **Files:** `internal/cli/sync.go`.
- **Tests:** Two temporary stores and a test server.
- **Done when:** Offline mode remains fully usable.

### T092 — Remote history slice checkpoint

- **Outcome:** Two local installations synchronize a conversation and preserve a
  deliberate conflict.
- **Depends on:** T091.
- **Tests:** End-to-end server/client test with attachments.
- **Done when:** Both accepted and conflict paths are observable.
- **Docs:** Update `docs/API.md`, `docs/DOMAIN.md`, `docs/DEPLOYMENT.md`, and
  `docs/ARCHITECTURE.md`.

## Phase 7 — Security, operations, and packaging

### T100 — Add secret-redaction test suite

- **Outcome:** Central tests prove secrets are absent from logs, diffs, errors,
  JSON output, and audit events.
- **Depends on:** T018, T052, T082.
- **Files:** `internal/security/*_test.go`.
- **Tests:** API keys, bearer tokens, cookies, private-key patterns.
- **Done when:** Tests fail on accidental serialization.

### T101 — Add server-side SSRF policy layer

- **Outcome:** Server-side provider URL policy adds a second layer of SSRF
  protection for multi-tenant or company deployments, beyond the config-time
  validation in T016.
- **Depends on:** T040, T082c.
- **Files:** `internal/security/ssrf.go`.
- **Tests:** Loopback `127.0.0.1`, IPv6 loopback `::1`, link-local
  `169.254.0.0/16`, RFC1918 ranges, DNS rebinding simulation (mock resolver
  changes answer between validation and request). All blocked by default.
  Administrator allowlist allows specific private ranges when explicitly
  configured.
- **Done when:** Policy is explicit and administrator-configurable. Tests
  cover IPv6 and DNS rebinding, not just IPv4.
- **Docs:** Update `docs/DEPLOYMENT.md` and `docs/DECISIONS.md`.

### T102 — Add request and attachment quotas

- **Outcome:** Configurable limits prevent unbounded memory, disk, and network
  consumption.
- **Depends on:** T051, T074, T086.
- **Files:** `internal/security/limits.go`.
- **Tests:** Boundary and over-limit cases.
- **Done when:** Limits apply before expensive processing.

### T103 — Add audit events

- **Outcome:** Mutating operations create safe local/server audit records.
- **Depends on:** T072, T084.
- **Files:** `internal/audit`.
- **Tests:** Config apply, import, sync, restore, token revocation.
- **Done when:** Audit records are queryable without payload secrets.

### T104a — Implement partial doctor command

- **Outcome:** `doctor` checks configuration validity, filesystem permissions,
  client integration readiness, and proxy reachability. Available after Phase 4.
- **Depends on:** T020, T059, T068.
- **Files:** `internal/cli/doctor.go`.
- **Tests:** Healthy, invalid config, proxy not started, client path missing,
  backup directory unwritable. Each failure produces a remediation message and
  stable exit code.
- **Done when:** Output includes remediation and stable exit status for all
  tested failure states. No secrets appear in output.

### T104b — Extend doctor command with history and sync checks

- **Outcome:** `doctor` adds database migration status, provider health, and
  server authentication checks.
- **Depends on:** T104a, T078, T092.
- **Files:** `internal/cli/doctor.go`.
- **Tests:** Database incompatible schema, provider unreachable, sync auth
  failure. Each failure produces an actionable remediation.
- **Done when:** All checks run without printing secrets, tokens, or prompt
  content.

### T105 — Add signal/process handling

- **Outcome:** Foreground proxy and server shut down gracefully on supported
  platform signals.
- **Depends on:** T050, T081.
- **Files:** `internal/platform/process.go`.
- **Tests:** Cancellation, active stream shutdown, pending DB transaction.
- **Done when:** No corrupt database or orphaned temporary files remain.

### T106 — Add Windows amd64 build

- **Outcome:** Reproducible Windows amd64 binary is produced.
- **Depends on:** T003, T105.
- **Files:** CI workflow, release scripts.
- **Tests:** Build and run `--help` under a Windows runner.
- **Done when:** Checksums and version metadata are generated.

### T107 — Add Linux amd64 build

- **Outcome:** Reproducible Linux amd64 binary is produced.
- **Depends on:** T003, T105.
- **Files:** CI workflow, release scripts.
- **Tests:** Build and run `--help` under a Linux runner.
- **Done when:** Checksums and version metadata are generated.

### T108 — Add release archive

- **Outcome:** Each target archive contains binary, checksum, license,
  quickstart, and example config.
- **Depends on:** T106, T107.
- **Files:** Release scripts, `examples/`.
- **Tests:** Extract archive and run quickstart validation.
- **Done when:** Archives contain no secrets or local history.

### T109 — Add backup/restore operational runbook

- **Outcome:** Operators can back up and restore config, SQLite, attachments,
  server database, and server attachments.
- **Depends on:** T078, T092, T108.
- **Files:** `docs/DEPLOYMENT.md`.
- **Tests:** Follow runbook in a temporary environment.
- **Done when:** Recovery time and limitations are documented.

## Phase 8 — Future-facing API and GUI preparation

### T120 — Stabilize application service interfaces

- **Outcome:** CLI calls application services that a future GUI can reuse.
- **Depends on:** T104.
- **Files:** `internal/app`, `pkg/api`.
- **Tests:** CLI contract tests call services without shell parsing.
- **Done when:** GUI work does not require moving business logic out of CLI.

### T121 — Add JSON output contracts

- **Outcome:** Read-only commands expose versioned machine-readable output.
- **Depends on:** T120.
- **Files:** `docs/API.md`, CLI serializers.
- **Tests:** Golden JSON fixtures.
- **Done when:** Secret redaction and backward-compatibility rules are explicit.

### T122 — Reserve GUI integration boundary

- **Outcome:** Design documents define which stable services a future GUI may
  consume without implementing GUI code.
- **Depends on:** T121.
- **Files:** `docs/ARCHITECTURE.md`, `docs/API.md`.
- **Tests:** Documentation lint.
- **Done when:** GUI remains out of the CLI/proxy domain packages.

## Phase 9 — Fail-open sidecars (must not block chat)

Sidecars are optional features around the local proxy. The core path remains
decode → route → upstream → encode. A sidecar error is a WARN; `/v1/messages`
must still be served. Interactive user cancel of Desktop apply (`ExitUsage`)
is not a sidecar failure. `client apply` stays strict.

### T140 — Fail-open proxy start

- **Outcome:** Local proxy start continues when history cannot open, Desktop
  apply fails (except user cancel), or the browser guide cannot open. Catalog
  fetch is already best-effort. `doctor` reports history problems as WARN, not
  a failing exit, when config is valid.
- **Depends on:** T050, T058, T072, T104a.
- **Files:** `internal/cli/sidecar.go`, `internal/cli/root.go`,
  `internal/proxy/server.go`.
- **Steps:** Open history best-effort; wrap `OnRequest` with panic recovery;
  write history off the request goroutine; on local start, treat apply errors
  other than user cancel as WARN; keep `client apply` failing as today.
- **Tests:** Blocked history parent path → store is nil; panicking `OnRequest`
  still returns HTTP 200 for `/v1/messages`; `doctor` with an unwritable
  history path exits 0 with `history: WARN`.
- **Done when:** A locked or missing history DB does not prevent
  `POST /v1/messages`. Direct mode still fails if apply fails (apply is the
  product in that mode).
- **Docs:** `docs/ARCHITECTURE.md`, `docs/DECISIONS.md` ADR-014,
  `docs/DOMAIN.md`.

### T141 — In-memory last-request snapshot

- **Outcome:** The proxy keeps a ring of recent request metadata (models,
  tokens, cost, outcome, correlation ID) with **no prompt or tool payloads**.
  `GET /debug/usage` returns it. Failures render empty JSON, never 5xx on the
  messages path.
- **Depends on:** T140.
- **Files:** `internal/proxy/usage.go`, `internal/proxy/server.go`.
- **Tests:** After a fake `/v1/messages` call, `/debug/usage` includes the
  routed model and tokens and does not contain the user prompt text.
- **Done when:** Endpoint is loopback-only by virtue of the existing bind
  policy. Response is `Cache-Control: no-store`.
- **Docs:** `docs/API.md`.

### T142 — Session spend and cache-miss nags

- **Outcome:** The snapshot accumulates this-process request count, tokens,
  and estimated USD. Consecutive large prompts with `cache_read == 0` raise an
  advisory note. Nothing is rejected.
- **Depends on:** T141.
- **Files:** `internal/proxy/usage.go`, `internal/modelstatus/usage.go`.
- **Tests:** Two 5k-token prompts with no cache increment `cache_miss_streak`
  and attach a note; a later cached prompt resets the streak.
- **Done when:** Advisories are strings on the snapshot only. No
  `provider_rate_limit` or other reject is emitted.
- **Docs:** `docs/COST.md`.

### T143 — EN/FA live card on the local guide

- **Outcome:** The bilingual guide shows last request + session spend by
  polling `/debug/usage`. If the fetch fails, the card says usage is
  unavailable and the rest of the page still works.
- **Depends on:** T142.
- **Files:** `internal/guide/embed/index.html`, `internal/proxy/guide_test.go`.
- **Tests:** Guide HTML includes `#live` and `/debug/usage`; fetch-failure
  copy exists in both EN and FA strings.
- **Done when:** No prompt text is rendered on the live card. Polling is
  best-effort.
- **Docs:** `README.md`.

### T144 — Background catalog refresh

- **Outcome:** After start, OpenRouter `/models` prices refresh on a timer.
  A failed refresh keeps last-known + `config.toml` prices. Start never waits
  on a hung catalog.
- **Depends on:** T140.
- **Files:** `internal/modelstatus`, `internal/cli/root.go`.
- **Tests:** Fake catalog server down after first success; second refresh
  error leaves previous prices in place.
- **Done when:** Refresh errors are logged at WARN. Proxy requests never
  wait on the fetch.
- **Docs:** `docs/COST.md`.
- **Implementation:** `modelstatus.CatalogCache` keeps last-known prices; CLI
  refreshes every 15m with an 8s timeout and WARNs on error.
- **Tests:** `TestCatalogCacheKeepsLastOnRefreshError`.
- **Documentation check:** Architecture no / Decision no / API no / Domain no /
  Deployment no / Development workflow no. Cost yes.

### T145 — Advisory model health probes

- **Outcome:** A background probe records upstream reachability per configured
  model. `models status` and `/debug/usage` may show a stale/down hint. The
  user’s selected model is still attempted.
- **Depends on:** T144.
- **Files:** `internal/modelstatus`, `internal/cli`.
- **Tests:** Probe timeout records down; a subsequent `/v1/messages` to that
  model still reaches the adapter.
- **Done when:** Health is never a routing hard-gate unless the user later
  opts into that (out of scope here).
- **Docs:** `docs/API.md`.
- **Implementation:** `modelstatus.Probe` + background `Adapter.Health` every
  60s. Hints land on `/debug/usage`; routing still attempts the selected model.
- **Tests:** `TestProbeTimeoutIsDown`, `TestHealthDownDoesNotBlockMessages`,
  `TestHealthTimeoutDoesNotBlockMessages`.
- **Documentation check:** Architecture no / Decision no / API yes / Domain no /
  Deployment no / Development workflow no.

### T146 — Context-growth advisor (never truncate)

- **Outcome:** When input tokens (or a coarse char estimate) approach the
  selected model’s declared context limit, emit an advisory note. Do not drop
  or truncate messages. Router overflow reject (FR-PROXY-011) is unchanged.
- **Depends on:** T141, T044.
- **Files:** `internal/proxy/usage.go`.
- **Tests:** High input vs small `ContextLimit` adds a note; request body is
  unmodified.
- **Done when:** Silent truncation remains forbidden (`docs/DOMAIN.md`
  invariant 9).
- **Docs:** `docs/DOMAIN.md`, `docs/COST.md`.
- **Implementation:** `ContextNotes` at 80% of `context_limit`; never mutates
  the request body.
- **Tests:** `TestContextNotesNearLimit`, `TestContextGrowthNoteDoesNotMutateRequest`.
- **Documentation check:** Architecture no / Decision no / API no / Domain yes /
  Deployment no / Development workflow no. Cost yes.

### T147 — Desktop config drift watcher

- **Outcome:** After a successful apply, a watcher detects if
  `claude_desktop_config.json` / `configLibrary` no longer matches the last
  applied fingerprint. Log WARN + `client apply --dry-run` hint. Do not
  auto-rewrite.
- **Depends on:** T064, T140.
- **Files:** `internal/clientintegration`, `internal/cli`.
- **Tests:** Mutating the applied file after start produces a WARN in a test
  hook; apply is not invoked.
- **Done when:** Watcher errors (missing file, permission) are WARN and the
  proxy stays up.
- **Docs:** `docs/DEPLOYMENT.md`.

### T148 — Fire-and-forget spend alerts

- **Outcome:** Optional webhook/`ntfy` URL fires when session estimated USD
  crosses a threshold. Timeout ≤1s; errors dropped. Chat does not wait.
- **Depends on:** T142.
- **Files:** `internal/config/model.go`, sidecar notifier.
- **Tests:** Slow/unreachable webhook does not delay a fake `/v1/messages`
  beyond a small bound; threshold firing is unit-tested with a fake server.
- **Done when:** Default is off. Secrets are not placed in the alert body.
- **Docs:** `docs/COST.md`, `docs/API.md`.

### T149 — Fail-open upstream circuit breaker

- **Outcome:** Repeated upstream 5xx/timeouts can skip a backend for a short
  cooldown **only when a configured fallback exists**. If the breaker itself
  errors, send the request as usual. No extra latency on the happy path.
  Mid-stream failover remains out of scope.
- **Depends on:** T045, T140.
- **Files:** `internal/routing`, `internal/provider`.
- **Tests:** After N failures, the next request uses fallback; breaker
  storage panic/error still attempts the primary.
- **Done when:** First-token path is unchanged when the breaker is healthy.
  Disabled by default or empty-fallback = no skip.
- **Docs:** `docs/ARCHITECTURE.md`, `docs/API.md`.

### T150 — History redaction skip-on-fail

- **Outcome:** Optional secret redaction of stored history runs after the
  client already has the response. Redaction or DB errors skip that write.
- **Depends on:** T140, FR-HISTORY-007.
- **Files:** `internal/history`, `internal/cli`.
- **Tests:** Injected redaction failure still returns 200 from the proxy;
  a successful redaction omits a synthetic API-key-shaped string from SQLite.
- **Done when:** Redaction is clearly documented as imperfect. Default must
  not drop history silently without a WARN counter/log.
- **Docs:** `docs/DOMAIN.md`.

### T151 — Sidecar checkpoint

- **Outcome:** Phase 9 implemented tasks have tests, docs, and a short
  operator note in README: extras can fail; chat must not.
- **Depends on:** T140–T143 (MVP); T144–T150 may be partial with backlog
  pointers in `docs/COST.md`.
- **Files:** `README.md`, `docs/COST.md`, `docs/ARCHITECTURE.md`.
- **Tests:** `go test ./internal/cli ./internal/proxy ./internal/modelstatus`
  and `docscheck`.
- **Done when:** Traceability matrix lists FR-SIDECAR-* against T140–T151.
- **Docs:** `requirements.md` matrix, this file.

Implementation: T140–T150 are in tree. Sidecars recover/WARN; `/v1/messages`
is unchanged. Operator note is in README (EN + FA). Matrix: FR-SIDECAR-001–007
→ T140–T151.
Tests: `internal/cli` (sidecar, alerts, doctor history WARN), `internal/proxy`
(usage, health-down, OnRequest panic), `internal/modelstatus` (catalog keep-last,
context notes, probe), `internal/routing` (breaker), `internal/history` (redact),
`internal/clientintegration` (drift).
Documentation check:
- Architecture changed? [yes]
- Decision changed? [yes] (ADR-014 already Accepted)
- API changed? [yes] (`/debug/usage` extras)
- Domain behavior changed? [yes] (invariants 11–12)
- Deployment changed? [yes] (drift watcher)
- Development workflow changed? [no]

### T152 — Colleague-ready setup verdict and support prompt

- **Outcome:** Start and `doctor` tell a non-expert whether the machine is
  READY / PARTIAL / NOT READY, auto-heal common OS/Desktop/port differences,
  and emit a redacted copy-paste prompt for the person who distributed the
  binary.
- **Depends on:** T104a, T140.
- **Files:** `internal/diagnose`, `internal/cli`, `internal/platform`,
  `internal/clientintegration`, `internal/proxy`, `internal/guide/embed/index.html`.
- **Steps:**
  1. Collect config, API key presence, provider probe, Desktop layouts,
     applied gateway URL, proxy `/health`, history.
  2. Auto-detect 3P vs consumer; apply every existing layout of that product.
  3. If listen port is busy, bind the next ports and rewrite Desktop.
  4. Print a status box; if not READY, print the support prompt; persist
     `last-doctor.txt`; expose `GET /debug/status`.
- **Tests:** Missing key is FAIL; prompt redacts fixture keys; listen fallback;
  guide includes `/debug/status`.
- **Done when:** A colleague can run the binary and either chat or send one
  prompt. No secrets in doctor output.
- **Docs:** `README.md`, `docs/API.md`, `docs/ARCHITECTURE.md`,
  `docs/DEPLOYMENT.md`, `docs/DECISIONS.md`, `requirements.md`.

Implementation: diagnose package + start auto-heal (desktop detect, apply-all
layouts, port fallback, status box, support prompt, `/debug/status`).
Tests: `internal/diagnose`, `internal/platform` listen fallback, `internal/cli`
doctor --prompt redaction, `internal/proxy` status endpoint, guide HTML.
Documentation check:
- Architecture changed? [yes]
- Decision changed? [yes] (ADR-014 tasks T152)
- API changed? [yes] (`/debug/status`, doctor flags)
- Domain behavior changed? [no]
- Deployment changed? [yes]
- Development workflow changed? [no]

## Final review tasks

### T130 — Requirements traceability audit

- **Outcome:** Every FR/NFR/G/security requirement maps to a task and design
  section.
- **Depends on:** T122.
- **Files:** `requirements.md`, `tasks.md`.
- **Tests:** Automated ID cross-reference check.
- **Done when:** Unmapped requirements are fixed or explicitly deferred.

### T131 — Architecture consistency audit

- **Outcome:** C4 diagrams, package layout, deployment, and decisions agree.
- **Depends on:** T130.
- **Files:** `design.md`, `docs/ARCHITECTURE.md`, `docs/DECISIONS.md`.
- **Tests:** Documentation lint and human review.
- **Done when:** No contradictory source of truth remains.

### T132 — Security review preparation

- **Outcome:** A review bundle identifies trust boundaries, threats, controls,
  and open questions.
- **Depends on:** T131.
- **Files:** `docs/SECURITY.md`, if approved, or `docs/ARCHITECTURE.md`.
- **Tests:** Verify every high-risk area has a test or explicit acceptance.
- **Done when:** Authentication, SSRF, secrets, privacy, sync, and endpoint
  concerns are covered.

### T133 — Large-model review and revision

- **Outcome:** An independent large model critiques and proposes revisions to
  the planning documents before implementation proceeds beyond the slices.
- **Depends on:** T132.
- **Files:** All planning and canonical documents.
- **Tests:** Record review findings, decisions, and resolved task changes.
- **Done when:** Accepted changes include model attribution and revision notes.

> **Status: Satisfied at planning stage.** Claude Sonnet 4.6 reviewed all
> planning documents on 2026-09-07. Findings were applied to all documents in
> this revision. See the review record in `docs/DECISIONS.md` ADR-011 note and
> the attribution lines in each updated file.

## Documentation check template

Copy this into every task completion note:

- [ ] Architecture changed? → updated `docs/ARCHITECTURE.md`
- [ ] Decision changed? → updated `docs/DECISIONS.md`
- [ ] API changed? → updated `docs/API.md`
- [ ] Domain behavior changed? → updated `docs/DOMAIN.md`
- [ ] Deployment changed? → updated `docs/DEPLOYMENT.md`
- [ ] Development workflow changed? → updated `docs/DEVELOPMENT.md`
- [ ] Project scope changed? → updated `docs/PROJECT.md`

## Prompt for the larger-model review

```text
You are reviewing the Claude Desktop Gateway planning package before coding.

Project intent:
- Build a Go CLI-first tool for Windows amd64 and Linux amd64.
- Reuse Claude Desktop's supported harness/configuration mechanisms where
  available.
- Route model requests through OpenRouter, 9router, or custom
  OpenAI-compatible gateways.
- Provide an optional local protocol proxy.
- Provide local SQLite history, portable export/import, and an optional
  self-hosted synchronization server.

Files to review:
- requirements.md
- design.md
- tasks.md
- docs/PROJECT.md
- docs/ARCHITECTURE.md
- docs/DOMAIN.md
- docs/DECISIONS.md
- docs/DEVELOPMENT.md
- docs/API.md
- docs/DEPLOYMENT.md

Review objectives:
1. Identify requirements that are ambiguous, contradictory, infeasible, or
   dependent on undocumented Claude Desktop behavior.
2. Verify that the design separates client integration, local proxy,
   provider translation, local history, and remote synchronization.
3. Challenge the Anthropic-compatible inbound and OpenAI-compatible outbound
   protocol assumptions, especially streaming, tool calls, reasoning, usage,
   cancellation, and error mapping.
4. Review authentication, secret storage, TLS, SSRF, prompt privacy,
   attachment handling, multi-tenant authorization, replay, and deletion.
5. Check whether SQLite, export/import, content-addressed attachments, and
   conflict branches are sufficient for multi-device operation.
6. Check Windows amd64 and Linux amd64 feasibility.
7. Identify tasks that are too large, combine multiple outcomes, lack tests,
   lack dependencies, or change architecture without documentation work.
8. Find missing vertical-slice checkpoints and feedback loops.
9. Validate all requirement IDs, decision IDs, API claims, and task references.
10. Recommend the smallest safe MVP if the current scope is too large.

Required output:
- Executive verdict.
- Critical blockers.
- High-risk assumptions requiring verification.
- Security findings.
- Architecture changes.
- Requirements changes.
- Task decomposition changes.
- Documentation consistency findings.
- A prioritized revision list.
- Explicitly label facts, assumptions, and recommendations.

Do not propose binary patching, authentication bypass, regional restriction
evasion, or undocumented access-control circumvention. If a client integration
depends on unsupported behavior, mark it as an experiment or remove it from
the MVP.
```
