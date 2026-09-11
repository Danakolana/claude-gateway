# Deployment

> **Authoring model:** GPT-5.6 Luna  
> **Revision:** 2026-09-11 · Desktop config drift watcher (T147) added by Cursor Grok 4.6  
> **Status:** Canonical baseline — revised

## Supported targets

- Windows amd64
- Linux amd64

Release archives contain the binary, checksum, license, quickstart, and
example configuration. They must not contain credentials, local history, or
attachments.

## Local-only deployment

Run the CLI and local proxy on the same machine as Claude Desktop. SQLite and
attachments remain local. The proxy binds to loopback. This mode works without
the optional history server and should continue working offline.

## Self-hosted history deployment

Run `gateway-server` behind TLS with:

- authenticated access tokens,
- tenant/user authorization,
- a server database,
- an attachment directory or object store,
- request and storage quotas,
- scheduled backups,
- health/readiness monitoring.

Provider API keys and sync credentials are separate secrets. The server must
not treat a provider key as a history-server credential.

## Configuration backup

Before applying client changes, create a timestamped, checksummed backup.
Backups should be stored separately from the active client configuration and
must be included in the documented restore procedure.

After a successful local-proxy start apply, a sidecar watcher fingerprints
`claude_desktop_config.json`. If Desktop or another tool changes that file,
the proxy logs WARN and suggests `client apply --dry-run`. It does **not**
rewrite the file automatically. Watcher errors (missing file, permissions)
are also WARN; chat continues.

## Data backup

Back up:

1. SQLite database using the `sqlite3` `.backup` command or the Go SQLite
   backup API. **Do not copy a live SQLite WAL-mode file directly** — without
   a checkpoint the copy may be inconsistent. Before a filesystem-level copy,
   run `PRAGMA wal_checkpoint(FULL)` and verify the copy with
   `PRAGMA integrity_check`.
2. Attachment directory/object store (after the database backup, so attachment
   references are consistent with the snapshot).
3. Server database and attachment store independently (same guidance applies).
4. Configuration files without secrets, plus secret-management instructions.

Test restoration on a clean machine. Document the restoration time and any
limitations (attachments referenced in the database but absent from the
backup).

> **WAL mode note:** WAL mode is required for concurrent access and is enabled
> by default. WAL mode is unsafe on some network filesystems (NFS, CIFS/SMB).
> Do not store the SQLite database on a network filesystem without disabling
> WAL mode and documenting the limitation.

## Security defaults

- Loopback-only local proxy. Non-loopback binding requires explicit
  configuration and logs a WARN message with the bound address.
- Minimum TLS 1.2 enforced on all provider connections.
- TLS required for production remote synchronization.
- Disabling TLS verification (`tls_verify = false`) requires explicit
  configuration and logs a WARNING identifying the provider.
- Secret-safe logs and diagnostics.
- Explicit opt-in for insecure TLS.
- Explicit policy for non-loopback binding.
- SSRF protection for server-side provider URLs.
- Bounded request, archive, attachment, and sync batch sizes.

## Upgrade policy

Version configuration and database schemas. Run migrations before serving
traffic. Refuse unknown future schema versions safely. Keep a rollback backup
before upgrades and document downgrade limitations.

## Retention and storage management

Configure a conversation retention policy to prevent unbounded database growth:

```toml
[history]
retention_days = 90   # tombstone conversations older than this; 0 = unlimited
```

Use `history cleanup --before <date>` to remove tombstoned conversations and
their associated attachment blobs. Monitor SQLite file size and attachment
directory size. When the database exceeds a comfortable size for the
installation, export older conversations and purge them from the local store.

Attachment deduplication (content-addressed storage) reduces disk usage when
the same image or file appears in multiple conversations. Run
`history cleanup --orphans` to remove blobs that have no remaining metadata
references after a purge.

## Operational checks

Use `doctor`, proxy health, `/debug/status`, logs, and audit events
to diagnose operation. After start, the terminal prints READY / PARTIAL /
NOT READY. If the machine is not READY, copy the support prompt (or
`last-doctor.txt` in the gateway data dir) — it is already redacted.
Health output must not reveal prompts, attachments, authorization headers,
or provider keys.

Listen port conflicts: the local proxy tries the configured port, then the
next 20 ports on the same host, then rewrites Desktop to the bound URL.
