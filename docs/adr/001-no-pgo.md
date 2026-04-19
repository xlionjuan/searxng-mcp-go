# ADR-001: Do Not Use PGO (Profile-Guided Optimization)

## Status

Accepted

## Background

During the code modernization review on 2026-04-16, it was suggested to add `-pgo=auto` to the build for performance improvement.

## Decision

**Do not use PGO**, and explicitly prohibit its reintroduction in the future without first obtaining representative profiling data.

## Reasons

### 1. Tool Characteristics Are Not Suitable for PGO

searxng-mcp-go is an **MCP server / CLI tool**:
- Each execution is short-lived (request → response → exit)
- Cannot continuously collect profiling data like a long-running HTTP server
- Profiling data is written to disk after each process exit, making it impossible to accumulate meaningful hot-path information across processes

### 2. No Representative Profiling Data

The core prerequisite of PGO is: **profiling data must represent real workloads**.

| Data Source | Problem |
|-------------|---------|
| A few manual executions | Insufficient samples, one-sided path coverage |
| Benchmark script | Artificially constructed workloads cannot represent real usage scenarios |
| Accumulated personal usage | Low query diversity, cannot cover most code paths |

PGO without representative data **may actually degrade performance** (the compiler optimizes for incorrect hot paths).

### 3. Bottleneck Is Not Local Compute

For the SearXNG MCP server:
- **The real bottleneck is network latency** (time spent calling the external SearXNG API)
- Local CPU computation accounts for a very small proportion
- Even if PGO delivers a 5–15% local compute improvement, the overall end-to-end latency improvement would be negligible

### 4. Cost-Benefit Ratio Is Too Low

Implementing PGO requires:
- Researching how to collect profiling data
- Designing representative benchmarks
- Maintaining `.pgo` files and keeping them updated

The time investment far outweighs the actual benefit.

## Alternative Optimization Directions

If performance improvements are needed in the future, the following directions are more effective:

1. **Result caching** — return the previous result directly for identical queries (requires a separate mechanism; not recommended for now)
2. **Reduce network latency** — use a closer SearXNG instance
3. **Concurrent queries** — query multiple engines simultaneously without waiting for all results
4. **Response compression** — reduce transmitted data volume

## Conditions for Reconsideration

If any of the following occur in the future, PGO can be re-evaluated:

- searxng-mcp-go evolves into a long-running MCP server deployment
- Sufficient real production profiling data has been accumulated
- There is a clear benchmark proving PGO delivers quantifiable improvements

## Effective Date

2026-04-16
