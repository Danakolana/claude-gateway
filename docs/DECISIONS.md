# Architectural Decisions

> **Authoring model:** GPT-5.6 Luna  
> **Revision:** 2026-09-07 · Reviewed and revised by Claude Sonnet 4.6, 2026-09-07  
> **Status:** Canonical baseline — revised

## ADR-001 — Go monorepo

- **Status:** Accepted
- **Context:** CLI, proxy, and server need shared contracts and release tooling.
- **Decision:** Use one Go repository with independently runnable binaries and
  package boundaries.
- **Consequences:** Shared code is simple; independent deployment remains
  possible; package boundaries must be enforced by review.

## ADR-002 — CLI first

- **Status:** Accepted
- **Context:** The product must be automatable and usable without a GUI.
- **Decision:** Build CLI and stable application services first; add a GUI later.
- **Consequences:** Every GUI operation needs a service/API boundary.

## ADR-003 — TOML plus secret references

- **Status:** Accepted
- **Context:** Profiles and routing exceed `.env`'s expressive scope.
- **Decision:** TOML is canonical; `.env` supplies secrets and declared
  overrides.
- **Consequences:** Parsing, validation, and redacted explanation are required.

## ADR-004 — Conditional client integration

- **Status:** Accepted
- **Context:** Claude Desktop endpoint capabilities may vary by version and
  documented mode. MCP tool-server integration (JSON-RPC 2.0, tool and context
  traffic) and model inference endpoint routing (HTTP REST) are distinct
  mechanisms; the project must not confuse them.
- **Decision:** Verify each integration surface and classify it as supported,
  experimental, or unavailable. Never claim an unsupported override works. The
  inbound proxy protocol package name (`internal/protocol/inbound/<mechanism>`)
  is determined by ADR-011 after T005 verification completes.
- **Consequences:** Client integration includes a compatibility matrix (T069)
  and explicit failure behavior. ADR-011 records the T005 verification outcome
  and gates Phase 3 coding.

## ADR-005 — Adapter boundary

- **Status:** Accepted
- **Context:** OpenRouter, 9router, and custom gateways can differ in paths,
  headers, streaming, and error shapes.
- **Decision:** Use provider-neutral canonical types and adapter interfaces.
- **Consequences:** Protocol fixtures and adapter contract tests are mandatory.

## ADR-006 — SQLite plus portable archives; sync server deferred

- **Status:** Accepted
- **Context:** Local history needs transactions, queries, and portability.
  Remote sync adds significant complexity (authentication, multi-tenant
  authorization, idempotency, conflict branches) that is not required to
  validate the core value proposition.
- **Decision:** SQLite is local source of truth; versioned export/import is the
  interchange mechanism. The sync server is deferred to Phase 6. The local
  schema includes sync-cursor and tombstone stub columns (T093) from the
  initial migration so that Phase 6 does not require DDL changes to production
  tables.
- **Consequences:** Migrations, checksums, staging imports, and backups are
  required. Sync stub columns are null/zero-populated by history write logic
  until Phase 6. Client-side encryption of conversation content before sync
  is a privacy enhancement deferred to Phase 6 design.
- **Open question:** Should attachments use client-side encryption before
  sync? To be decided at Phase 6 design time.

## ADR-007 — Content-addressed attachments

- **Status:** Accepted
- **Context:** Binary attachments should not inflate SQLite rows and must sync
  independently.
- **Decision:** Store blobs by checksum with database metadata references.
- **Consequences:** Atomic blob writes, quotas, checksums, and retention rules
  are required.

## ADR-008 — Preserve synchronization conflicts

- **Status:** Accepted
- **Context:** Devices can edit the same conversation offline.
- **Decision:** Preserve divergent versions as branches; never silently apply
  last-write-wins.
- **Consequences:** Branch entities, conflict CLI commands, and explicit
  resolution semantics are required.

## ADR-009 — Loopback and TLS defaults

- **Status:** Accepted
- **Context:** A local proxy handles credentials and private prompts.
- **Decision:** Bind locally to loopback by default; require TLS in production
  for remote server traffic. Minimum TLS version 1.2 is enforced on all
  provider connections. Disabling TLS verification (`tls_verify = false`)
  requires explicit opt-in and logs a WARN message identifying the provider.
- **Consequences:** Non-loopback and insecure TLS are explicit opt-ins with
  warnings and tests. Non-loopback binding logs a WARN with the bound address.
  TLS minimum version enforcement is part of T037 and T040 acceptance criteria.

## ADR-010 — No access-control circumvention

- **Status:** Accepted
- **Context:** The product goal is gateway choice, not vendor access bypass.
- **Decision:** Do not patch binaries, impersonate accounts, defeat login,
  evade regional restrictions, or rely on undocumented access-control bypass.
- **Consequences:** Unsupported client behavior is reported and deferred.

## ADR-011 — Claude Desktop endpoint verification result

- **Status:** Accepted
- **Authoring model:** Composer
- **Date:** 2026-09-07
- **Context:** The local proxy design depends on Claude Desktop exposing a
  documented mechanism that routes model inference traffic to a
  user-controlled gateway. Official Anthropic documentation for
  "Claude Desktop on 3P" (third-party inference) was reviewed on 2026-09-07.
- **Verified facts (documentation review, not live binary capture):**
  - Claude Desktop on 3P supports `inferenceProvider: "gateway"` with
    `inferenceGatewayBaseUrl`, `inferenceGatewayApiKey`, and optional
    `inferenceGatewayAuthScheme`.
  - The gateway **must** implement the Anthropic Messages API:
    `POST /v1/messages` with streaming and tool use required;
    `GET /v1/models` optional.
  - Config locations (per Anthropic / third-party deployment docs):
    - Linux/macOS per-user 3P: `Claude-3p/claude_desktop_config.json`
      under the application support directory, with `enterpriseConfig`.
    - Windows per-user 3P: `%APPDATA%\Claude-3p\claude_desktop_config.json`.
  - MCP servers in `claude_desktop_config.json` are a separate mechanism
    (tools/context only) and are **not** the model inference route.
  - Community/developer `env.ANTHROPIC_BASE_URL` overrides are treated as
    **experimental** until confirmed against a specific Desktop build.
- **Decision:**
  - Supported mechanism: `custom-model-endpoint` via Claude Desktop on 3P
    gateway configuration.
  - Inbound proxy protocol: Anthropic-compatible Messages API
    (`internal/protocol/inbound/anthropic/`).
  - Primary apply target: 3P `enterpriseConfig` gateway fields.
  - Experimental secondary: `env.ANTHROPIC_BASE_URL` via interactive prompt
    (consumer choice + confirm) on start / `client apply`; labeled experimental
    in CLI output. Non-interactive sessions default to 3P.
- **Consequences:** Phase 3 may proceed with Anthropic inbound codecs.
  Client integration (Phase 4) renders 3P gateway config, not MCP tool
  stubs. Live version matrix rows are filled by T069 as builds are tested.
- **Affected requirements:** FR-PROXY-002, FR-CLIENT-006.
- **Affected tasks:** T005 (complete), T048, T053–T055, T068, T069.

## ADR-012 — Context overflow policy (no client tokenizer)

- **Status:** Accepted
- **Authoring model:** Composer
- **Date:** 2026-09-07
- **Context:** FR-PROXY-011 requires non-silent context overflow handling.
  Accurate token counting needs a model-specific tokenizer that drifts over
  time and differs across providers.
- **Decision:** Do not embed a tokenizer in MVP. When the selected model has
  a declared context limit and the request carries an explicit token estimate
  or oversized attachment metadata that exceeds it, reject or fallback with
  `unsupported_capability`. When no reliable estimate exists, forward to the
  provider and classify provider context errors as `provider_protocol`.
- **Consequences:** T044 implements metadata-based checks only. A future ADR
  may add optional tokenizer estimates.

## ADR-013 — Portable archive format is ZIP

- **Status:** Accepted
- **Authoring model:** Composer
- **Date:** 2026-09-07
- **Context:** Export/import needs a widely supported container on Windows
  and Linux.
- **Decision:** Use ZIP (ZIP64 when needed). Enforce entry count, total
  uncompressed size, and compression-ratio limits before extraction.
- **Consequences:** T075/T076 use `archive/zip`.

---

## Decision update protocol

When a decision changes, add a new ADR or amend the affected ADR with:

- authoring model,
- date,
- context,
- decision,
- alternatives,
- consequences,
- affected requirements and tasks.
