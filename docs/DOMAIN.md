# Domain

> **Authoring model:** GPT-5.6 Luna  
> **Revision:** 2026-09-07 · Reviewed and revised by Claude Sonnet 4.6, 2026-09-07  
> **Status:** Canonical baseline — revised

## Core concepts

- **Provider:** Named upstream gateway with URL, protocol, authentication
  reference, timeout, health, and retry policy.
- **Model:** Provider-facing ID plus display name, tier alias, capabilities,
  context, pricing metadata, and enabled state.
- **Profile:** Named effective configuration selecting provider, routing,
  proxy, history, and sync behavior.
- **Conversation:** Ordered interaction with branches, messages, content
  blocks, model invocation metadata, and workspace context.
- **Attachment:** Content-addressed blob referenced by a content block.
- **Sync object:** Versioned, checksummed representation exchanged between
  local and remote stores.

## Invariants

1. A model route cannot target a missing or disabled model.
2. Unknown capability is not equivalent to supported capability.
3. History appends are transactional.
4. Import validates before modifying existing history.
5. Sync conflicts preserve both versions.
6. Deleted objects use tombstones until the documented retention window.
7. Secrets are not domain payloads and are never serialized into normal logs.
8. Tool calls are data; the gateway never executes them.
9. Context overflow is not a silent event. When the incoming request exceeds the
   selected model's declared context limit, the router must surface it as an
   explicit `unsupported_capability` error or route to a fallback with a larger
   context limit. Silent truncation is a data-integrity violation.
10. Attachment blobs are stored by cryptographically valid hex-encoded checksums
    only. Any value that is not a valid 64-character lowercase hex string is
    rejected before any filesystem operation. This invariant applies to both the
    local attachment store (T074) and the server-side attachment endpoint (T086).

## Conversation lifecycle

`open → streaming → completed` is the normal lifecycle. A canceled, timed-out,
or provider-failed interaction becomes `incomplete` with an explicit reason.
A later retry creates a new invocation rather than rewriting the old one.

A streaming conversation interrupted by a client disconnect becomes `incomplete`
with reason `client_cancelled`. The provider connection is cancelled within the
request context deadline. The history write uses a separate context with a
shutdown deadline so the `incomplete` record is committed even after the client
disconnects. No partial record is visible to queries until the transaction
commits.

## Routing lifecycle

`source model/tier → exact rule → tier rule → default → capability filter →
fallback candidates → selected target`.

The effective route records the source, target, provider, capability decision,
and fallback attempt without recording secrets.

## Sync lifecycle

`local → pending → pushed → acknowledged`, with side states `retryable`,
`rejected`, and `conflict`. A conflict creates a branch and remains visible
until the user resolves or archives it.

## Documentation rule

Changes to statuses, entities, invariants, lifecycle transitions, or conflict
semantics must update this file and include domain tests.
