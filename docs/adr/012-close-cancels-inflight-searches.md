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

## Amendment (2026-07-20): Context-Based Close Lifecycle

### Background

During implementation, the `done` channel design was replaced with a context-based
approach that the codebase now uses:

- `SearXNGSearcher` owns a lifecycle `context.Context` and `context.CancelFunc`
  created by `NewSearXNGSearcher` (`searcher.go:38-39,132`).
- `Close()` calls `searcherCancel` exactly once via `sync.Once` (`searcher.go:185`).
- Each `Search()` derives a per-call context and uses `context.AfterFunc` to
  propagate searcher closure to in-flight searches without a dedicated watcher
  goroutine (`searcher.go:292-303`).
- Retry backoff uses `retryWait` which respects context cancellation, so close
  during backoff returns immediately.

### Why the change supersedes the original decision

1. **No per-search goroutine overhead.** `context.AfterFunc` registers a callback
   that runs in a goroutine only when the lifecycle context is actually canceled —
   zero cost in the happy path.
2. **Familiar Go idiom.** Context cancellation is the standard way to signal
   shutdown, making the design more recognizable to contributors.
3. **Simpler ownership.** The lifecycle context is created once at construction
   and canceled once at close; there is no channel to initialize, select on, or
   guard against double-close outside of `sync.Once`.

### Containedctx suppression

The `SearXNGSearcher` struct carries `//nolint:containedctx` because the lifecycle
context represents object lifecycle, not request-scoped data. The stored
`searcherCtx` is analogous to the `ctx` field in `exec.Command` or `sql.DB` — an
architectural invariant initialized at construction and unchanged for the life of
the value. The narrow suppression is intentional and acceptable.

### Constructor invariant

Production and integration code must construct `SearXNGSearcher` through
`NewSearXNGSearcher`, which initializes the lifecycle context. Tests that exercise
`Search` or `Close` also use the canonical constructor with a custom HTTP client;
internal white-box tests may adjust a narrow retry-control seam after construction
to keep timing deterministic. Direct struct literals in internal tests are
permitted only when they do not call lifecycle-dependent methods, or when a
defensive nil guard deliberately exercises that case (see `searchContext`).

### External contract preserved

- `Close` remains idempotent (backed by `sync.Once`).
- `Close` cancels in-flight searches and retry backoff waits.
- `Close` still releases idle HTTP connections only for owned transports.
- No behavior change was introduced to update this ADR.
