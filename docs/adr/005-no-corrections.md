# ADR-005: Do Not Expose `corrections`

Date: 2026-04-19
Status: Accepted

## Context

SearXNG's JSON response includes a `corrections` field containing spelling correction suggestions. During code review, it was identified that `SearchResponse` does not model this field, while the backend does return it.

## Decision

**Do not expose `corrections` in `SearchResponse` or any output format.**

## Rationale

1. **Almost never populated**: Most SearXNG engines do not implement correction extraction. In practice, the field is empty for the vast majority of queries.
2. **Low value for AI agents**: AI agents typically reformulate queries autonomously rather than relying on server-side spelling suggestions.
3. **Maintenance cost**: Adding a field that is almost always empty adds serialization overhead and documentation burden for negligible benefit.

## Consequences

- `corrections` data returned by SearXNG is silently discarded during response parsing.
- If a future use case requires corrections, this decision can be revisited.
