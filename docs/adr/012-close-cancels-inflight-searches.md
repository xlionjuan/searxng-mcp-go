# ADR-012: `SearXNGSearcher.Close` Cancels In-Flight Searches

**Status:** Accepted
**Date:** 2026-06-07

## Context

`SearXNGSearcher.Close()` previously only released idle HTTP connections owned
by the searcher. Active `Search()` calls could continue until their caller
context, HTTP timeout, retry loop, or upstream response completed.

That behavior made shutdown ambiguous: closing a searcher did not provide a
direct way to stop work already in progress, and retry backoff could continue
after the searcher had been closed. A first implementation used a stored
`context.Context` and `context.CancelFunc` on `SearXNGSearcher`, but that
introduced two problems:

1. The production struct stored a context, which violates the repository's
   `containedctx` lint policy.
2. Tests and other internal code sometimes construct `SearXNGSearcher`
   literals directly. Those literals could leave the stored context nil and
   make cancellation hooks fragile.

## Decision

`SearXNGSearcher` owns an internal `done chan struct{}` plus `sync.Once` for
shutdown signaling.

- `NewSearXNGSearcher` initializes `done`.
- `Close()` closes `done` once and still closes idle connections only for owned
  transports.
- `Search()` derives a per-call context from the caller context and cancels it
  when `done` closes.
- Search attempts, retry decisions, and retry backoff waits all use the derived
  search context.

## Rationale

1. **Close has an observable shutdown contract.** After `Close()` is called,
   in-flight searches should stop instead of waiting for upstream completion.
2. **Caller cancellation still wins normally.** Each `Search()` still starts
   from the caller-provided context; the searcher close signal is an additional
   cancellation source.
3. **No stored context fields.** A plain channel avoids the `containedctx`
   lint issue and keeps the shutdown state independent from request contexts.
4. **Idempotent close.** `sync.Once` keeps repeated `Close()` calls safe.

## Consequences

- `Close()` now cancels active searches. Callers may observe
  `context.Canceled` from a `Search()` that was in flight when the searcher was
  closed.
- Retry and empty-response backoff paths now stop promptly after `Close()`.
- Tests that construct `SearXNGSearcher` directly and call `Search()` must
  initialize `done`, or use `NewSearXNGSearcher` / shared test helpers.
- `Close()` remains safe for searchers using a shared default client because it
  still closes idle transport connections only when the searcher owns the
  transport.
