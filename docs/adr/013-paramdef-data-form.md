# ADR-013: ParamDef as Data Form with Per-Consumer Renderers

**Status:** Accepted (revised 2026-06-12)

## Context

Search parameters (`ParamDef`) were defined as a data struct with fields for
type, description, bounds, and constraints. Three separate consumers — CLI help
text, MCP JSON Schema generation, and flag registration — each read `ParamDef`
fields and reconstructed their own representation. Adding a new parameter meant
touching all three consumers, and the JSON Schema generation in
`buildSearchSchema()` was a 30-line conditional ladder that duplicated the same
field-to-schema mapping logic.

The initial attempt (ADR-013 v1) embedded a pre-computed `Schema map[string]any`
field in `ParamDef`, populated at `init()` time. While this simplified
`buildSearchSchema()`, it left the other two consumers unchanged and required
a `//nolint:gochecknoinits` suppression. The drift tests in
`params_validation_drift_test.go` remained ~500 lines.

## Decision

Add three methods on `ParamDef`, each owning the rendering contract for one
consumer:

- `ParamDef.JSONSchema() map[string]any` — replaces `buildParamSchema()`.
- `ParamDef.FlagDefault() (any, error)` — returns the parsed default value
  for use with Go's `flag` package.
- `ParamDef.CLIHelpLine() string` — returns the full formatted CLI help line
  including indentation, flag expression, and help text.

The `Schema` field, `buildParamSchema()` function, and package `init()` are
removed. Each consumer calls the appropriate method instead of interpreting
`ParamDef` fields directly:

```go
// mcp.go — buildSearchSchema
for _, p := range searxng.SearchParams {
    props[p.Name] = p.JSONSchema()
}

// main.go — registerFlags
for _, p := range searxng.SearchParams {
    defaultVal, err := p.FlagDefault()
    switch v := defaultVal.(type) {
    case string: r.searchFlags[p.Name] = fs.String(p.Name, v, p.Description)
    case int:    r.searchFlags[p.Name] = fs.Int(p.Name, v, p.Description)
    }
}

// cli.go — printCLIHelp
for _, p := range searxng.SearchParams {
    fmt.Println(p.CLIHelpLine())
}
```

## Consequences

- **No init() needed.** Schema computation is lazy and per-call; no
  `//nolint:gochecknoinits` required.
- **Drift resistance.** Each consumer calls a method whose contract is tested
  by a single test in `params_validation_drift_internal_test.go`
  (`TestParamDefJSONSchema`). The ~500-line drift test file shrinks because
  the per-consumer mappings are now methods, not copied logic.
- **No exported Schema field.** `ParamDef` no longer exposes a pre-computed
  map, removing the mutation-hazard concern.
- **No change to ParamDef fields.** The existing `CLIHelp`, `CLIType`,
  `MCPType`, `Minimum`, `Maximum`, `Enum`, `Examples`, and `Nullable` fields
  remain. They are the raw material from which the renderer methods build
  their output.
- **CLIHelpLine layout is fixed.** The method hardcodes 18-character padding
  for the flag-expression column. If a future consumer needs different
  padding, it can format manually from the raw fields.

Supersedes the pre-computed Schema approach from ADR-013 v1.
