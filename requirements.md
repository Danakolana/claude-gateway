# Claude Desktop Gateway — Requirements

> **Authoring model:** GPT-5.6 Luna  
> **Revision:** 2026-09-07 · Reviewed and revised by Claude Sonnet 4.6, 2026-09-07  
> **Status:** Revised — review recommendations applied  
> **Project:** `claude-desktop-gateway`

## 1. Purpose

Claude Desktop Gateway is a Go application that lets an organization use
Claude Desktop's productive local harness, where supported by the client, with
user-selected models delivered through OpenRouter, 9router, or another
OpenAI-compatible gateway. It provides configuration generation, an optional
local protocol proxy, portable conversation storage, and an optional
self-hosted synchronization server.

The product optimizes provider choice and operational control. It does not
patch vendor binaries, impersonate accounts, defeat authentication, evade
regional controls, or depend on undocumented access-control bypasses.

## 2. Product goals

### G-001 — Reuse the client harness

Users should be able to keep using Claude Desktop's interface, tool workflow,
workspace context, and other supported harness behavior while selecting an
upstream model independently.

### G-002 — Reduce model cost

Users and organizations should be able to replace an expensive model with an
alternative model by exact model ID, friendly alias, or capability tier.

### G-003 — Support gateway choice

The same local installation must support OpenRouter, 9router, and arbitrary
OpenAI-compatible gateways without code changes.

### G-004 — Avoid mandatory vendor account coupling

The application must not require the gateway's own user account for local
configuration or history operation. Any client endpoint behavior must use a
documented and permitted client configuration mode. Provider authentication
remains the user's/provider's responsibility.

### G-005 — Keep data portable

Conversation data must be exportable, importable, inspectable, and movable
between devices or self-hosted servers without requiring a vendor account.

### G-006 — Be operable by a company

The system must support named profiles, predictable configuration, backups,
diagnostics, secret-safe logs, and a server mode suitable for controlled
self-hosting.

## 3. Users and usage scenarios

### U-001 — Individual developer

The developer selects a provider, enters a key through an environment variable
or secret store, maps a client model to a cheaper model, validates the setup,
and launches the supported client.

### U-002 — Company administrator

The administrator publishes a standard profile containing a gateway endpoint,
routing rules, capability policy, and safe defaults. Individual API keys are
provided separately.

### U-003 — Multi-device developer

The developer exports a conversation or authenticates to a self-hosted server,
then imports or synchronizes conversations on another machine.

### U-004 — Gateway operator

The operator runs the local proxy and optionally the history server, observes
health and operational metrics, rotates credentials, and backs up data.

### U-005 — Recovery operator

The operator restores the previous client configuration, recovers a local
SQLite database from backup, or imports a portable archive after a failed
upgrade.

## 4. Scope and release boundaries

### 4.1 Initial scope

The first implementation is a CLI-first Go monorepo targeting Windows amd64
and Linux amd64. It includes:

1. Configuration loading, validation, profiles, and secret references.
2. Automatic backup and restore of supported Claude Desktop configuration.
3. Local proxy process with an explicitly defined compatibility surface
   (protocol determined by T005 / ADR-011 before proxy coding begins).
4. OpenAI-compatible upstream adapter contract.
5. OpenRouter profile implemented through the adapter contract. 9router profile
   conditional on T006 contract verification (deferred if unverified).
6. Custom OpenAI-compatible gateway configuration.
7. Model aliases, replacement rules, capability policy, fallback, retries,
   context-overflow handling, and safe diagnostics.
8. Local SQLite history with portable export/import.
9. Documentation, tests, reproducible builds, and release archives.

> **Note:** Remote history server and synchronization are deferred to Later
> Scope. The local schema is designed from the start to accommodate sync
> (stub columns, tombstones, versioned IDs) so no disruptive migration is
> needed when the sync phase is implemented.

### 4.2 Later scope

- Remote history server and authenticated synchronization (deferred from
  initial scope; reduces MVP complexity while the core value is validated).
- 9router profile if T006 contract verification confirms compatibility.
- Claude Code integration after Claude Desktop integration is validated.
- A GUI built on stable application APIs.
- Organization SSO and centralized policy management.
- Additional provider protocols.
- Object storage backends beyond local filesystem storage.

### 4.3 Non-goals

- Patching, reverse engineering, or modifying Claude Desktop or Claude Code
  binaries.
- Bypassing login, subscription, entitlement, geographic restrictions, or
  service access controls.
- Claiming compatibility with a client endpoint that has not been verified.
- Reimplementing the Claude Desktop or Claude Code harness.
- Automatically scraping or importing vendor-hosted history without a supported
  export or documented API.
- Silent loss or destructive merging of conversation data.
- Storing provider keys in plaintext in normal configuration or logs.

## 5. Terminology

| Term | Meaning |
|---|---|
| Client | Claude Desktop in the first release; Claude Code later. |
| Gateway | Upstream HTTP service that accepts model requests. |
| Provider | A configured upstream gateway profile. |
| Local proxy | Optional process translating client requests to a provider. |
| Profile | Named, validated set of provider, routing, proxy, and storage settings. |
| Model ID | Provider-facing identifier, such as `deepseek/...`. |
| Display name | Human-readable name shown in CLI output. |
| Tier alias | Stable logical name such as `balanced`, `fast`, or `premium`. |
| Conversation | Ordered user, assistant, tool, and metadata records. |
| Session | A client/workspace execution context containing one or more conversations. |
| Sync object | Versioned object exchanged between local and remote stores. |
| Capability | A provider/model feature such as streaming or tool calling. |

## 6. Functional requirements

### Configuration and profiles

#### FR-CONFIG-001 — Canonical configuration

The application MUST support TOML as the canonical configuration format.
`.env` MUST be supported for secret values and environment overrides, but must
not be the only representation for multi-profile routing.

#### FR-CONFIG-002 — Configuration discovery

The CLI MUST document and implement deterministic discovery order:

1. Explicit `--config` path.
2. `CLAUDE_GATEWAY_CONFIG`.
3. User configuration directory.
4. Project-local configuration when explicitly enabled.

The exact platform paths MUST be documented and tested on Windows and Linux.
When a configuration source is found at position N in the order, sources at
positions N+1 through 4 MUST be silently ignored. Environment overrides
(FR-CONFIG-007) are applied on top of the selected base configuration
regardless of discovery position. The `config explain` output (FR-CONFIG-010)
MUST identify which discovery path was selected and which were found but
ignored.

Acceptance criterion: a config file is present at both `CLAUDE_GATEWAY_CONFIG`
and the default path; only the `CLAUDE_GATEWAY_CONFIG` file's values are used.
`config explain` identifies the selected path.

#### FR-CONFIG-003 — Profiles

Users MUST be able to create, select, list, validate, export, and delete named
profiles without deleting unrelated profiles.

#### FR-CONFIG-004 — Provider fields

A provider configuration MUST support:

- `base_url`
- `api_key` secret reference, never an inline secret by default
- `auth_scheme`
- optional custom headers
- timeout
- TLS verification policy (disabled only as explicit opt-in, with a logged WARNING)
- minimum TLS version (default: TLS 1.2; must be enforced on the HTTP client)
- upstream protocol
- health-check path or method
- retry policy

#### FR-CONFIG-005 — Model records

A model record MUST support:

- provider-facing model ID
- display name
- tier alias
- declared capabilities
- context limit when known
- input and output pricing metadata when known
- enabled/disabled state

#### FR-CONFIG-006 — Replacement rules

Users MUST be able to map a client-visible source model or tier alias to a
provider model ID. Rules MUST have deterministic precedence and validation for
cycles, missing targets, and disabled models.

#### FR-CONFIG-007 — Environment overrides

Environment overrides MUST be documented, validated, and limited to declared
configuration paths. Arbitrary environment interpolation MUST be opt-in.

#### FR-CONFIG-008 — Secrets

The configuration loader MUST support environment references and an abstraction
for OS/external secret stores. It MUST reject or warn on plaintext secrets
according to a configurable policy. The sync server token MUST use the same
secret-reference pattern as provider API keys. An inline sync token value in
TOML MUST trigger the same plaintext-secret warning as an inline API key.
Configuration files containing secrets MUST be checked for world-readable
permissions at load time (warning on Linux/Windows if world-readable).

#### FR-CONFIG-009 — Validation

`config validate` MUST report actionable errors including file, field, reason,
and remediation. A failed validation MUST not modify client configuration.

#### FR-CONFIG-010 — Explain mode

`config explain` MUST show the effective non-secret configuration, including
selected profile, routing decision, and source of each value.

### Client integration

#### FR-CLIENT-001 — Claude Desktop discovery

The application MUST discover the supported Claude Desktop configuration path
or accept an explicit path. Discovery MUST be platform-specific and tested.

#### FR-CLIENT-002 — Backup before mutation

Every mutation MUST create an integrity-checked timestamped backup before
writing. The backup MUST identify the source path, profile, tool version, and
checksum.

#### FR-CLIENT-003 — Atomic writes

Configuration writes MUST use a temporary file, flush/close, permission
handling, and atomic replacement where the platform permits it.

#### FR-CLIENT-004 — Restore

`client restore` MUST list compatible backups and restore a selected backup
only after validation. Restore MUST itself create a rollback backup.

#### FR-CLIENT-005 — Dry run

All mutating client commands MUST support `--dry-run` and show a redacted
diff or planned operation without writing.

#### FR-CLIENT-006 — Supported integration boundary

The implementation MUST distinguish verified Claude Desktop configuration
surfaces from experimental or unsupported endpoint behavior. Unsupported
behavior MUST produce a clear error rather than silently claiming success.

#### FR-CLIENT-007 — Launch

`client launch` MUST launch the configured client only when the selected
integration mode has passed validation. It MUST show the active profile and
proxy endpoint without exposing credentials.

### Local proxy

#### FR-PROXY-001 — Explicit protocol surface

The proxy MUST document its inbound and outbound protocol versions, supported
methods, streaming behavior, error mapping, and unsupported fields.

#### FR-PROXY-002 — Inbound proxy protocol surface (conditional on T005)

The inbound proxy protocol surface is **not specified** until T005 (Claude
Desktop endpoint verification) records a supported configuration mechanism in
ADR-011. Depending on the T005 outcome:

- If T005 records a verified custom model inference endpoint: the inbound
  surface is Anthropic-compatible messages API.
- If T005 records MCP-only integration: the inbound surface is MCP
  tool-server protocol. Note that MCP (JSON-RPC 2.0, tool and context
  traffic) and the Anthropic Messages API (HTTP REST, model inference) are
  distinct mechanisms; this requirement covers model inference routing only.
- If T005 records no viable endpoint: the inbound proxy surface is deferred
  to a later release.

No proxy coding under `internal/protocol/` may begin until ADR-011 status
is "Accepted". FR-PROXY-002 is satisfied when the inbound codec is
implemented for the mechanism confirmed by ADR-011 and passing golden
fixtures exist.

#### FR-PROXY-003 — OpenAI-compatible upstream

The proxy MUST support an adapter contract for OpenAI-compatible chat or
responses endpoints and MUST isolate provider-specific behavior behind that
contract.

#### FR-PROXY-004 — Streaming

The proxy MUST preserve streaming when both sides support it. It MUST emit
well-formed terminal events and translate upstream disconnects into a
documented error.

#### FR-PROXY-005 — Tool calls

The proxy MUST translate tool schemas, tool calls, and tool results where the
upstream capability declaration permits it. Unsupported translation MUST be
reported explicitly.

#### FR-PROXY-006 — Request identity

Each request MUST receive a correlation ID. The ID MUST be propagated to
provider calls and safe logs, but prompts and secrets MUST not be logged by
default.

#### FR-PROXY-007 — Timeouts and cancellation

Client cancellation, request deadlines, and process shutdown MUST cancel
upstream requests and release resources.

#### FR-PROXY-008 — Retries

Retries MUST be limited to configured transient failures and MUST respect
idempotency, streaming state, rate-limit hints, and a maximum attempt count.

#### FR-PROXY-009 — Fallbacks

Fallback selection MUST be deterministic, capability-aware, and visible in
diagnostic metadata. A fallback MUST not silently downgrade a required
capability.

#### FR-PROXY-010 — Health

The proxy MUST expose a local health/readiness command or endpoint that does
not reveal provider secrets.

#### FR-PROXY-011 — Context overflow handling

When the incoming request exceeds the selected model's declared context limit,
the router MUST either reject the request with `unsupported_capability` and a
message indicating which limit was exceeded, or route to a fallback model with
a larger context limit if one is configured. Silent truncation MUST never
occur. When context limit metadata is unavailable or stale, the proxy MUST
pass the request to the provider and classify any resulting context error as
`provider_protocol`.

Acceptance criterion: unit test routes a request annotated with 8000 tokens
to a model declared at 4096 tokens; the request is rejected or redirected.
Integration test: a context error from the provider is classified as
`provider_protocol`, not `internal`.

#### FR-PROXY-012 — Client-side rate budget (optional)

The proxy SHOULD support optional per-profile request and token budgets that
reject or queue requests after the configured limit, protecting the user's
provider account from runaway consumption. Budgets are advisory and default to
unlimited. When a budget is exceeded, the proxy returns a
`provider_rate_limit`-compatible error with the budget source identified as
local.

Acceptance criterion: unit test — proxy with a request-count budget of 5
rejects request 6 with the correct error category and budget source field.

### Providers and model routing

#### FR-PROVIDER-001 — Adapter interface

The adapter interface MUST cover request translation, response translation,
streaming, health checks, capability reporting, and safe error normalization.

#### FR-PROVIDER-002 — OpenRouter

OpenRouter MUST be configurable through base URL, API key reference, model ID,
optional site metadata headers, timeout, and retry settings.

#### FR-PROVIDER-003 — 9router (conditional on T006)

9router MUST be configured only after T006 records verified protocol behavior
in `docs/API.md`. Until T006 completes, 9router is a planned provider, not a
required MVP provider. FR-PROVIDER-003 is satisfied when: (a) the 9router
contract is documented in `docs/API.md`, (b) a contract test suite passes
against a fixture that represents the verified behavior, and (c) any deviation
from OpenAI-compatible protocol is isolated in `internal/provider/nine`. If
T006 records that the 9router protocol is incompatible and undocumented, this
requirement is deferred to Later Scope.

#### FR-PROVIDER-004 — Custom gateway

Users MUST be able to configure arbitrary OpenAI-compatible base URLs and
authentication schemes without recompiling the binary.

#### FR-PROVIDER-005 — Capability negotiation

Routing MUST consider declared support for streaming, tools, vision,
reasoning/thinking, context size, and structured output. Unknown capability
MUST not be treated as guaranteed support.

#### FR-PROVIDER-006 — Model replacement

Users MUST be free to select an original model or an alternative model for
each source model/tier, subject to capability policy and provider validation.

#### FR-PROVIDER-007 — Cost metadata

The application SHOULD calculate an estimate from configured pricing metadata,
clearly labeling estimates as non-authoritative.

### Local history

#### FR-HISTORY-001 — SQLite store

Local history MUST use SQLite with schema versioning and migration tests. The
local store MUST support a configurable retention policy. Conversations older
than the retention window MUST be tombstoned and optionally purged via a
documented `history cleanup` command. The store MUST operate in WAL mode with
foreign-key enforcement enabled on every connection.

#### FR-HISTORY-002 — Record types

The schema MUST support user prompts, assistant responses, tool calls, tool
results, attachments, workspace/project metadata, provider/model metadata,
timestamps, request correlation IDs, and conversation branches.

#### FR-HISTORY-003 — Append safety

Conversation writes MUST be transactional and recoverable after interruption.

#### FR-HISTORY-004 — Export

Users MUST be able to export selected conversations or all local data into a
portable archive containing schema version, metadata, records, attachments,
checksums, and redaction/encryption metadata.

#### FR-HISTORY-005 — Import

Import MUST validate archive structure and checksums, use migrations where
supported, avoid overwriting by default, and report skipped/conflicting items.

#### FR-HISTORY-006 — Copyability

The local store MUST be copyable to another machine through documented export
and backup procedures. Direct copying of a live SQLite database MUST not be
presented as safe without a consistent snapshot.

#### FR-HISTORY-007 — Redaction

Users MUST be able to configure redaction of likely secrets from stored
diagnostics and optionally from conversation payloads, with an explicit warning
that automated redaction is not perfect.

### Remote history and synchronization

#### FR-SYNC-001 — Optional server

The history server MUST be independently runnable and optional for local-only
use.

#### FR-SYNC-002 — Authenticated transport

Remote synchronization MUST use TLS in production and authenticated requests.
The server MUST support token rotation and revocation.

#### FR-SYNC-003 — Versioned objects

Conversations, messages, attachments, and metadata MUST have stable IDs,
versions, timestamps, and checksums suitable for idempotent synchronization.

#### FR-SYNC-004 — Incremental sync

The client MUST be able to push and pull changes incrementally instead of
uploading the complete database on every operation.

#### FR-SYNC-005 — Conflict preservation

Conflicts MUST preserve both versions or create explicit branches. The system
MUST not silently discard one device's changes.

#### FR-SYNC-006 — Offline operation

Local operation MUST continue without network access. Pending synchronization
must be inspectable and retryable.

#### FR-SYNC-007 — Attachments

Attachments MUST be content-addressed, size-limited, checksum-verified, and
transferable separately from message metadata.

#### FR-SYNC-008 — Deletion

Deletion MUST define tombstone retention and server behavior. A client MUST not
recreate a deliberately deleted object during ordinary sync.

### CLI and operations

#### FR-CLI-001 — Command groups

The CLI MUST provide discoverable command groups for `config`, `provider`,
`model`, `proxy`, `client`, `history`, `sync`, `server`, and `doctor`.

#### FR-CLI-002 — Stable exit codes

Commands MUST document exit codes for invalid input, provider failure,
authentication failure, conflict, unavailable service, and internal failure.

#### FR-CLI-003 — Machine-readable output

Read-only and diagnostic commands SHOULD support JSON output for automation.

#### FR-CLI-004 — Doctor

`doctor` MUST check configuration validity, filesystem permissions, client
integration, proxy reachability, provider health, database migrations, and
server authentication without printing secrets.

#### FR-CLI-005 — Audit trail

Mutating operations MUST produce a local audit event containing action,
timestamp, profile, result, and correlation ID, excluding secret values and
prompt contents by default.

## 7. Non-functional requirements

### NFR-001 — Cross-platform

The first release MUST build reproducibly for Windows amd64 and Linux amd64.
Path, permissions, process, signal, and atomic-write behavior MUST be covered
by platform-specific tests where relevant.

### NFR-002 — Performance

The proxy MUST avoid buffering an entire streaming response before forwarding
it. Local history writes MUST be bounded by transaction completion rather than
network latency. Streaming first-chunk latency overhead introduced by the proxy
MUST be less than 50 ms on loopback under nominal load. History write time for
a single message record MUST be less than 20 ms on SSD storage. These bounds
MUST be verified by benchmark tests in CI.

### NFR-003 — Reliability

The process MUST fail closed on invalid configuration, preserve backups before
mutation, and recover pending sync work after restart.

### NFR-004 — Observability

Logs MUST be structured, correlation-aware, level-controlled, and secret-safe.
Metrics MUST be optional and disabled from external exposure by default.

### NFR-005 — Security

Local proxy binding MUST default to loopback. Remote server endpoints MUST use
TLS in production. Credentials MUST be redacted from logs, errors, and
diagnostic output.

### NFR-006 — Privacy

The application MUST not send history, prompts, tool results, or attachments to
the project maintainers. Data leaves the device only through the configured
provider or explicitly configured sync server.

### NFR-007 — Upgradeability

Configuration and database schemas MUST be versioned. Upgrade and downgrade
limitations MUST be documented before releases.

### NFR-008 — Accessibility of operation

Every GUI-future operation MUST have a CLI/API equivalent. Initial operation
must be possible without a GUI.

### NFR-009 — Testability

Provider adapters, protocol translation, configuration generation, migration,
export/import, and sync conflict behavior MUST have deterministic tests.

### NFR-010 — Documentation integrity

Any implementation task that changes an architectural fact, decision, API, or
domain behavior MUST update the corresponding canonical documentation in the
same task.

## 8. Security and compliance constraints

1. Never log API keys, authorization headers, cookies, private keys, or full
   prompt payloads by default.
2. Never store inline provider secrets in generated client configuration unless
   the user explicitly chooses an insecure mode and receives a warning.
3. Validate remote URLs and reject unexpected schemes where appropriate.
4. Prevent SSRF in server-side provider configuration through allowlists or
   explicit administrator policy.
5. Protect local proxy from non-loopback access by default.
6. Require explicit configuration for insecure TLS.
7. Apply size limits to requests, responses, archives, attachments, and sync
   batches.
8. Keep security-sensitive behavior auditable and covered by tests.
9. Do not present a local endpoint as an official Anthropic service.
10. Clearly label experimental client integration and stop when the client does
    not support the requested configuration.

## 9. Acceptance strategy

The project is ready for implementation when:

- Requirements in this document have stable IDs.
- Every requirement is mapped to a design section and at least one task.
- The proxy contract has golden request/response and streaming fixtures.
- Client configuration backup, dry-run, write, and restore behavior is tested.
- SQLite export/import round trips preserve records and checksums.
- Sync tests cover offline queueing, retries, idempotency, conflicts, and
  deletion tombstones.
- Windows amd64 and Linux amd64 builds are reproducible in CI.
- A clean-machine quickstart can configure a provider without a vendor login,
  subject to the client integration actually supporting the selected endpoint.
- Security review has explicitly examined credentials, SSRF, prompt privacy,
  attachment handling, and authentication.

## 10. Requirement-to-documentation policy

At the end of every implementation task, the agent MUST answer:

- Architecture changed? Update `docs/ARCHITECTURE.md`.
- Decision changed? Update `docs/DECISIONS.md`.
- API changed? Update `docs/API.md`.
- Domain behavior changed? Update `docs/DOMAIN.md`.
- Deployment or operational behavior changed? Update `docs/DEPLOYMENT.md`.
- Development workflow changed? Update `docs/DEVELOPMENT.md`.

The task is incomplete until the applicable checks are recorded in the task's
completion note.

## 11. Traceability baseline

The following matrix is the initial requirement-to-design-to-task contract.
When a task is split or reordered, update this matrix rather than relying on
implicit knowledge.

| Requirement group | Design coverage | Primary tasks |
|---|---|---|
| G-001–G-006 | Sections 1–4, 9 | T020, T047, T059, T068, T078 |
| U-001–U-005 | Sections 3, 9–15 | T020, T059, T068, T078, T104a |
| FR-CONFIG-001–010 | Section 6 | T010–T020 |
| FR-CLIENT-001–007 | Section 9 | T060–T069 |
| FR-PROXY-001–010 | Sections 7–8 | T030a–T030d, T031–T059 |
| FR-PROXY-002 | Section 9 | T005 (ADR-011 gate), T048, T053 |
| FR-PROXY-011 (new) | Section 7–8 | T044 (context overflow) |
| FR-PROXY-012 (new) | Section 7–8 | T046 (rate budget, optional) |
| FR-PROVIDER-001–007 | Sections 7–8 | T030a–d, T032–T047, T049 |
| FR-PROVIDER-003 | Section 8 | T006 (gate), T039 (conditional) |
| FR-HISTORY-001–007 | Sections 10–11 | T070a–d, T071–T078, T079, T093 |
| FR-SYNC-001–008 | Section 12–13 | T080–T092 (Phase 6 — deferred) |
| FR-CLI-001–005 | Sections 5, 15–16 | T001–T004b, T017–T020, T058, T066, T077, T091, T104a |
| NFR-001–010 | Sections 13–16 | T003, T007, T050–T059, T100–T109, T120–T133 |
| Security constraints 1–10 | Sections 1, 7, 13, 16 | T014, T016, T040, T052, T082–T086, T100–T105 |

This matrix is a planning baseline, not proof that implementation is complete.
Completion requires the tests and acceptance criteria described in
`tasks.md`.
