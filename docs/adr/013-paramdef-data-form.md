# ADR-013: Embed JSON Schema Directly in ParamDef

**Status:** Accepted
**Date:** 2026-06-12

## Context

Search parameters (`ParamDef`) were defined as a data struct with fields for
type, description, bounds, and constraints. Three separate consumers — CLI help
text, MCP JSON Schema generation, and runtime validation — each read `ParamDef`
fields and reconstructed their own representation. Adding a new parameter meant
touching all three consumers, and the JSON Schema generation in
`buildSearchSchema()` was a 30-line conditional ladder that duplicated the same
field-to-schema mapping logic in a single place.

Issue #226 proposed a **mild** fix: add three methods on `ParamDef`
(`JSONSchema()`, `FlagSpec()`, `CLIHelpLine()`) so each consumer calls a
method instead of reimplementing the mapping.

## Decision

Adopt the **deep** version proposed in issue #239: embed the pre-computed JSON
Schema property directly in `ParamDef` as a `Schema map[string]any` field,
populated at package `init()` time by `buildParamSchema()`.

`buildSearchSchema()` in `mcp.go` no longer rebuilds each property schema from
individual fields. It iterates `searxng.SearchParams`, copies the pre-computed
`Schema` map into the properties object, and collects `required` names. The old
conditional ladder is removed entirely.

## Consequences

- **Simpler MCP consumer.** `buildSearchSchema()` drops from ~30 lines to a
    straight copy loop. No branching per parameter type.
- **Drift resistance.** The schema is now a *value* on each `ParamDef` rather
    than a function output. The 383-line `params_validation_drift_internal_test.go`
    still locks `ParamDef` fields against runtime validators; the new `Schema`
    field is a direct derivative and does not need separate drift coverage.
- **Public API addition.** `ParamDef` gains an exported `Schema` field.
    Consumers other than the root package may exist; this is documented and
    the field is read-only after init.
- **init() over lazy init.** Deterministic startup cost with no nil-check
    burden on every `buildSearchSchema` call. The `//nolint:gochecknoinits`
    suppression is justified by package-level correctness: `SearchParams` is a
    `var` with function-call initializers, so init ordering is well-defined.
- **Mutation hazard (advisory).** `Schema` is a shared `map[string]any` on a
    package-level slice. No current consumer mutates it. A future defensive
    accessor can be added if mutation becomes a concern.

Supersedes the mild approach in issue #226.
