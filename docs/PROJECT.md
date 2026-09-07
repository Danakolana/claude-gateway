# Project Knowledge Index

> **Authoring model:** GPT-5.6 Luna  
> **Revision:** 2026-09-07 · Reviewed and revised by Claude Sonnet 4.6, 2026-09-07  
> **Status:** Canonical baseline — revised

## Start here

- Product requirements: [../requirements.md](../requirements.md)
- Technical design: [../design.md](../design.md)
- Implementation tasks: [../tasks.md](../tasks.md)

## Canonical knowledge

- [Architecture](ARCHITECTURE.md)
- [Domain](DOMAIN.md)
- [Architectural decisions](DECISIONS.md)
- [Development](DEVELOPMENT.md)
- [API](API.md)
- [Deployment](DEPLOYMENT.md)

## Product summary

Claude Desktop Gateway is a Go CLI-first application for configuring and,
where supported, proxying Claude Desktop traffic to user-selected
OpenRouter, 9router, or custom OpenAI-compatible gateways. It also provides
portable local history and an optional self-hosted synchronization server.

The project uses only supported/documented client integration surfaces. It
does not patch vendor binaries, bypass authentication, impersonate accounts,
or evade access controls.

## Current baseline

- Platforms: Windows amd64 and Linux amd64.
- First client: Claude Desktop.
- Future client: Claude Code.
- Interface: CLI first, GUI later.
- Config: TOML with `.env` secret references.
- Local history: SQLite plus portable export/import.
- Remote history: **planned for Phase 6 (deferred from initial scope)**.
  The local schema includes sync stubs so no migration is needed when
  Phase 6 is implemented.
- Providers: OpenRouter (initial); 9router conditional on T006; custom
  OpenAI-compatible gateway.

> **Implementation gate status:** ADR-011 **Accepted** (2026-09-07). Inbound
> protocol is Anthropic Messages API via Claude Desktop on 3P gateway config.
> Phase 3 may proceed. See `docs/DECISIONS.md` ADR-011.

## Documentation update rule

Implementation and documentation are one change. At the end of every task,
check the applicable canonical document:

- Architecture → `ARCHITECTURE.md`
- Decision → `DECISIONS.md`
- API → `API.md`
- Domain behavior → `DOMAIN.md`
- Deployment → `DEPLOYMENT.md`
- Development workflow → `DEVELOPMENT.md`

## Model attribution

The current planning baseline was authored by `GPT-5.6 Luna`. Future agents
must add authoring model and revision date to every substantive update.
