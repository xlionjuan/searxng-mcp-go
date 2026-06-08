# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`CONTEXT.md`** at the repo root
- **`docs/adr/`** — read ADRs that touch the area you're about to work in

If any of these files don't exist, **proceed silently**. Don't flag their absence; don't suggest creating them upfront.

## File structure

Single-context repo:

```
/
├── CONTEXT.md
├── docs/adr/
│   ├── 001-no-pgo.md
│   ├── 003-http-warning-for-non-private-hosts.md
│   ├── 004-mcp-stdin-env-only.md
│   ├── 005-no-corrections.md
│   ├── 006-unresponsive-engines-debug-only.md
│   ├── 007-no-dns-rebinding.md
│   ├── 008-same-hostname-redirect.md
│   ├── 009-disable-get-fallback-by-default.md
│   ├── 010-cli-terminal-control-sanitization.md
│   ├── 011-max-content-runes-cli-only.md
│   └── 012-close-cancels-inflight-searches.md
└── src/
```

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in `CONTEXT.md`. Don't drift to synonyms the glossary explicitly avoids.

If the concept you need isn't in the glossary yet, that's a signal — either you're inventing language the project doesn't use (reconsider) or there's a real gap (note it for later documentation).

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-00X (title) — but worth reopening because…_
