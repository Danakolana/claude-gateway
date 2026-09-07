# Development

> **Authoring model:** GPT-5.6 Luna  
> **Revision:** 2026-09-07 · Reviewed and revised by Claude Sonnet 4.6, 2026-09-07  
> **Status:** Canonical baseline — revised

## Starting point

Module: `github.com/danakolana/claude-gateway`  
Go version: as declared in `go.mod` (developed on Go 1.26).  
Pinned analyzer: `honnef.co/go/tools/cmd/staticcheck@v0.6.1` via `tools.go`.

This is a greenfield Go project. Initialize the module before implementing
application packages. Keep the CLI, proxy, and server independently runnable.

## Feedback loop

For each small task:

1. Read the relevant requirements, design section, and decision.
2. Make one bounded change.
3. Add or update executable tests.
4. Run formatting, unit tests, vet/static analysis, and focused integration
   tests.
5. Run the documentation check.
6. Record the result and model attribution.

## Required checks

```text
gofmt
go test ./...
go test -race ./...
go vet ./...
staticcheck ./...          # version pinned in tools.go (T007)
documentation lint         # tools/docscheck (T004a + T004b)
cross-platform build checks
```

Tool versions are pinned in `tools.go` and the CI workflow uses the pinned
versions (T007). Run `go generate ./tools/` to re-download pinned tools after
a clean checkout.

## Task discipline

- One task has one outcome.
- Prefer vertical slices over disconnected infrastructure.
- Do not improve unrelated code.
- Do not hide unsupported behavior behind permissive fallbacks.
- Use fakes and fixture servers instead of real provider calls in tests.
- Never commit credentials, real prompts, private attachments, or local history.
- ADR-011 is **Accepted** (Anthropic inbound). Implement
  `internal/protocol/inbound/anthropic/` for Phase 3. Experimental
  `ANTHROPIC_BASE_URL` env overrides require `--allow-experimental`.

## Documentation-as-code rule

Architecture, decisions, APIs, domain behavior, deployment, and workflow
documentation are part of the implementation. A task changing one of these
must update the relevant canonical file before completion.

## Test categories

- Unit tests for pure parsing, validation, routing, codecs, and redaction.
- Golden tests for protocol payloads and CLI JSON output.
- Fixture-server tests for provider adapters and synchronization.
- SQLite migration and transaction tests (including WAL mode, foreign-key
  enforcement, and concurrent read/write correctness).
- Cross-platform path/process tests.
- End-to-end vertical-slice tests using local fakes.
- Security regression tests for secrets, SSRF, quotas, and authorization.
- Context overflow tests: request exceeds declared context limit → rejected
  or redirected; never silently truncated.
- Archive safety tests: import rejects archive bombs before extraction.
- Path traversal tests: attachment and server-side checksum fields reject
  non-hex values before any filesystem operation.
- Concurrency/cancellation tests: client disconnect cancels upstream request;
  streaming goroutine terminates within deadline.
- Benchmark tests: streaming proxy first-chunk latency < 50 ms on loopback;
  history write < 20 ms on SSD (NFR-002).

## Git discipline

Use small commits aligned with vertical slices. Do not commit generated
credentials, build artifacts, local databases, or copied provider source.
