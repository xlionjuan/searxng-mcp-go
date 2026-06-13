# CI Workflow Rules

- GitHub Actions `uses:` entries must pin to SHA with a `# vX.Y.Z` version
  comment when the upstream action publishes version tags or releases.
- If the upstream action does not publish version tags or releases, pin to a
  SHA with a `# main` branch comment, or the equivalent upstream branch name,
  so Renovate can track the pinned digest without inventing a version.
- CI `go-version` must use a fixed version, not `stable`; step/job names must
  not contain version numbers.
- MCP stdin mode does not accept CLI args; use env vars only. See
  `docs/adr/004-mcp-stdin-env-only.md`.
