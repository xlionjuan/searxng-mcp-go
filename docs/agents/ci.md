# CI Workflow Rules

- GitHub Actions `uses:` entries must pin to SHA with a `# vX.Y.Z` version
  comment.
- CI `go-version` must use a fixed version, not `stable`; step/job names must
  not contain version numbers.
- MCP stdin mode does not accept CLI args; use env vars only. See
  `docs/adr/004-mcp-stdin-env-only.md`.
