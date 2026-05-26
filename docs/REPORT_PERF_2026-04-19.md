# Performance Profiling Review

NOTE: This report refers to the pre-refactoring codebase. Since then, core types and HTTP logic have moved to `internal/searxng/`. See CONTEXT.md for the current module structure.

Date: 2026-04-19
Project: `searxng-mcp-go`
Scope reviewed: `bench_test.go`, `format.go`, `date.go`, `search.go`, prior `REPORT_PERF.md`

## Executive Summary

The current profiling report is mostly directionally accurate:

- `formatResults` is a real allocation hotspot.
- `parseRelativeDate` does spend meaningful CPU in regex backtracking.
- content truncation does extra work on long strings.
- `deduplicateAnswers` has a measurable lowercase/string-search cost.

But it is not fully complete, and one TODO is not framed precisely enough:

- The biggest formatting issue is not only `fmt.Sprintf`; it is also the current two-stage formatting design where `formatResults` calls `formatAnswers` and `formatInfoboxes`, both of which build separate temporary strings and then copy them into the outer builder.
- The proposed `parseRelativeDate` fast path should not just be “add a few `strings.Contains` checks on an already lowercased string” without clarifying that this still pays the `strings.ToLower` allocation on every call. That is still useful for regex CPU, but it does not address all avoidable cost.
- The current `deduplicateAnswers` TODO says “avoid redundant `strings.ToLower`”, but there is no obviously redundant second lowercase on the answer path today. The actual cost comes from lowercasing every infobox content and every answer candidate, plus repeated substring scans.
- The current benchmarks are good microbenchmarks for parser/formatter hot paths, but they do not isolate some important cases: long-content truncation, entity-heavy formatting, no-date dominant workloads, and larger answer/infobox cardinalities.

## Review Of Existing Findings

### 1. Replace `fmt.Sprintf` with `strings.Builder` in `formatResults`

Verdict: Accurate, but incomplete.

What is correct:

- `fmt.Sprintf` in tight loops is allocation-heavy.
- `formatResults` is on a hot path and is a significant source of allocations.

What is missing:

- `formatAnswers` and `formatInfoboxes` also use `fmt.Sprintf` heavily.
- `formatResults` currently allocates three large strings in the common case:
  - one for answers
  - one for infoboxes
  - one for the final output
- Even after removing `fmt.Sprintf`, keeping those helper functions as `string`-returning builders still leaves avoidable copying/allocation.

Conclusion:

- This should be implemented as a broader formatting rewrite around one shared `strings.Builder`, not a narrow line-by-line `Sprintf` replacement.

### 2. Add fast-path keyword check before regex in `parseRelativeDate`

Verdict: Accurate, but the exact approach matters.

What is correct:

- Regex backtracking can dominate this function when most inputs do not contain a relative date.
- A cheap prefilter before `FindStringSubmatch` is worthwhile.

What is incomplete:

- The current report’s sample approach lowercases the whole string before the fast path. That still allocates and scans the full string before returning `nil`.
- That approach still improves regex CPU cost, but it does not create the best possible no-date fast path.

Conclusion:

- The fast path should first avoid regex work.
- Ideally it should also avoid allocating a lowercased copy when no candidate keyword exists.

### 3. Optimize content truncation single-pass

Verdict: Accurate.

What is correct:

- `utf8.RuneCountInString(content)` plus `[]rune(content)` does two full passes and may allocate a full rune slice for long content.
- This is unnecessary because the formatter only needs “at most N runes”.

Conclusion:

- A single-pass truncation helper is the right implementation.

### 4. Avoid redundant `strings.ToLower` in `deduplicateAnswers`

Verdict: Partially accurate, but the TODO text is misleading.

What is correct:

- `strings.ToLower` is visible in the profile for this path.
- The function does repeated case-normalization and repeated substring scans.

What is inaccurate:

- There is no clear redundant second lowercase on `lowerAnswer` in the current function. It is lowercased once, then trimmed, then sliced.
- The bigger issue is:
  - lowercasing all infobox contents on every call
  - lowercasing every answer string on every call
  - O(answers x infoboxes) repeated `strings.Contains`

Conclusion:

- The implementation task should be reframed as “reduce normalization and substring-search work in `deduplicateAnswers`”, with a minimal first step that avoids unnecessary lowercasing when exact-case match already succeeds.

## Exact Implementation Plan For Each TODO

These steps are written so a subagent can implement them directly.

### TODO 1: Replace `fmt.Sprintf` with `strings.Builder` in formatting

Priority: High
Files: `format.go`

Implement exactly this:

1. Remove the `fmt` import from `format.go`.
2. Add `strconv` to `format.go` imports.
3. Keep `unescapeIfNeeded` unchanged for this change.
4. Introduce one helper for truncation:

```go
func truncateRunes(s string, limit int) string {
	if limit <= 0 || s == "" {
		return ""
	}
	runeCount := 0
	for i := range s {
		if runeCount == limit {
			return s[:i]
		}
		runeCount++
	}
	return s
}
```

5. Change `formatAnswers` from `func formatAnswers([]Answer) string` to `func writeAnswers(b *strings.Builder, answers []Answer)`.
6. Change `formatInfoboxes` from `func formatInfoboxes([]Infobox) string` to `func writeInfoboxes(b *strings.Builder, infoboxes []Infobox)`.
7. In both new writer helpers, replace every `fmt.Sprintf` call with direct writes:
   - use `b.WriteByte('[')` / `b.WriteByte(']')`
   - use `b.WriteString(strconv.Itoa(i + 1))`
   - use `b.WriteString("\n")` or `b.WriteByte('\n')`
8. In `formatResults`, stop capturing helper output as temporary strings. Call the writers directly:

```go
writeAnswers(&b, resp.Answers)
writeInfoboxes(&b, resp.Infoboxes)
```

9. In the results loop, replace all `fmt.Sprintf` uses with direct builder writes.
10. For result counts and numbering, use `strconv.Itoa`.
11. Replace the current truncation logic in both result summaries and infobox content with `truncateRunes(content, MaxContentRunes)`.
12. Preserve output byte-for-byte except for any existing truncation behavior bugs. Do not change headings, spacing, or line breaks.
13. Add or update tests only if output changes unexpectedly; otherwise keep behavior identical.
14. Re-run:
   - `go test ./...`
   - `go test -bench='FormatResults|FormatResultsLarge|FormatResultsInfoboxes' -benchmem`

Expected outcome:

- materially fewer allocations in formatting benchmarks
- fewer large temporary strings
- lower full-pipeline allocations

Minimal code shape to prefer:

- Do not create helper wrappers like `writeLinef` or mini formatting DSLs.
- Keep the implementation as direct builder writes in the existing functions.

### TODO 2: Add fast-path keyword check before regex in `parseRelativeDate`

Priority: Medium
Files: `date.go`

Implement exactly this:

1. Keep the package-level regexes as-is for now.
2. Add a small helper near `parseRelativeDate`:

```go
func mayContainRelativeDate(content string) bool {
	if content == "" {
		return false
	}
	return strings.Contains(content, "ago") ||
		strings.Contains(content, "Ago") ||
		strings.Contains(content, "AGO") ||
		strings.Contains(content, "yesterday") ||
		strings.Contains(content, "Yesterday") ||
		strings.Contains(content, "last week") ||
		strings.Contains(content, "Last week") ||
		strings.Contains(content, "Last Week") ||
		strings.Contains(content, "vor") ||
		strings.Contains(content, "Vor") ||
		strings.Contains(content, "vorgestern") ||
		strings.Contains(content, "Vorgestern") ||
		strings.Contains(content, "stunde") ||
		strings.Contains(content, "stunden") ||
		strings.Contains(content, "tag") ||
		strings.Contains(content, "tagen")
}
```

3. At the top of `parseRelativeDate`, after the empty-string check, add:

```go
if !mayContainRelativeDate(content) {
	return nil
}
```

4. Keep the existing `lower := strings.ToLower(content)` after that fast path.
5. Keep the existing semantic checks and year guards unchanged.
6. Do not broaden the accepted language/date grammar in this change. This is a performance-only change.
7. Re-run:
   - `go test ./...`
   - `go test -bench='ParseRelativeDate|InferDates|FullPipeline' -benchmem`

Why this exact approach:

- It is intentionally minimal and low risk.
- It avoids regex work entirely for obvious no-date content.
- It still preserves all current case-insensitive behavior by keeping the existing lowercase+regex logic once a candidate keyword is detected.

Follow-up optimization, only if needed after measuring:

- Replace `mayContainRelativeDate` with a tighter ASCII fold helper to reduce repeated `strings.Contains` scans.
- Or replace regex entirely with token scanning for `number + unit + ago/vor` patterns.

### TODO 3: Optimize content truncation single-pass

Priority: Low
Files: `format.go`

Implement exactly this:

1. Add `truncateRunes` exactly once in `format.go`.
2. Use it in both places that currently do:
   - `utf8.RuneCountInString(content) > MaxContentRunes`
   - `[]rune(content)` slicing
3. Remove the `unicode/utf8` import once no longer needed.
4. Do not append ellipsis in this change, because the current output does not do that.
5. Keep returned content unchanged when the string is already within the limit.
6. Re-run:
   - `go test ./...`
   - a new truncation-specific benchmark described below

Notes for the implementing agent:

- The helper must slice on a valid rune boundary.
- The `for i := range s` pattern is the simplest low-risk way to get rune boundaries without allocating a `[]rune`.

### TODO 4: Avoid redundant lowercase/search work in `deduplicateAnswers`

Priority: Low
Files: `search.go`

Reframe the task before implementing: the problem is not “redundant `strings.ToLower` on `lowerAnswer`” alone. The exact minimal optimization should be:

1. Keep function behavior identical.
2. Add a fast exact-case check before lowercasing anything:

```go
prefix := a.Answer
prefix = strings.TrimSuffix(prefix, " More at Wikipedia")
if len(prefix) > prefixLen {
	prefix = prefix[:prefixLen]
}

duplicated := false
for _, ib := range infoboxes {
	if ib.Content != "" && strings.Contains(ib.Content, prefix) {
		duplicated = true
		break
	}
}
if duplicated {
	continue
}
```

3. Only if the exact-case pass fails, fall back to the current lowercase-based comparison.
4. For the lowercase fallback:
   - build `infoboxTexts` lazily only if at least one answer reaches fallback
   - lowercase infobox content once when building that fallback slice
5. Keep the existing `" more at wikipedia"` lowercase suffix trimming in the fallback path so current behavior stays intact.
6. Do not introduce more complex indexing structures for this pass.
7. Re-run:
   - `go test ./...`
   - `go test -bench='DeduplicateAnswers' -benchmem`

Why this exact approach:

- It is the smallest safe change.
- It skips lowercasing/allocation when exact content already matches, which is common for duplicated Wikipedia-style summaries.
- It avoids overengineering a path that is currently a small share of full-pipeline time.

## Other Performance Concerns Missed By The Current Report

### 1. Temporary string construction between formatting helpers

This is the most important missed issue.

Current shape:

- `formatAnswers` builds a string.
- `formatInfoboxes` builds a string.
- `formatResults` appends both into another builder.

This creates avoidable intermediate allocations and copies independent of `fmt.Sprintf`.

Recommendation:

- Fold formatting into one builder via `writeAnswers` and `writeInfoboxes` as described above.

### 2. Missing `Builder.Grow` estimate in formatting

After the builder rewrite, the next likely improvement is pre-growing the output buffer.

Recommendation:

- After verifying the simpler builder rewrite, optionally add a cheap size estimate in `formatResults` and call `b.Grow(estimate)`.
- Keep this as a second pass only if the first rewrite still leaves noticeable allocation churn.

Suggested estimate formula:

- start with a small constant for headings
- add lengths of query/title/url/content/engine strings already present in the response
- clamp to avoid integer overflow paranoia if desired

This is lower priority than removing `fmt.Sprintf` and intermediate helper strings.

### 3. `parseRelativeDate` still lowercases the whole string on all keyword hits

Even after the proposed fast path, any candidate string still allocates a lowercased copy.

That is acceptable for the first optimization pass, but it is still a cost center.

If more optimization is needed later:

- replace regex matching plus full lowercase with manual parsing over ASCII-lowercased bytes for the specific supported phrases
- or extract numbers from the original string and match units with case-folded token checks

This should only be attempted after re-benchmarking because it increases code complexity.

### 4. `deduplicateAnswers` remains O(answers x infoboxes x substring-search)`

This is not urgent based on the current full-pipeline profile, but the benchmark with 10 infoboxes already shows scaling.

If this becomes important later:

- restrict infobox match scope to an initial content prefix rather than the entire infobox body, if behavior remains acceptable
- or precompute a smaller normalized search window per infobox

Do not implement this yet without benchmark evidence.

### 5. Current benchmarks do not isolate network/search-path costs

Search execution does several string-heavy operations not covered by the current benchmark set:

- URL parameter encoding
- POST body construction
- debug preview slicing/string conversion
- content-type checks and body handling

These are likely not dominant versus network latency, but there is no benchmark coverage today.

If the project cares about local server throughput or synthetic tests, add dedicated request-construction and response-parse benchmarks.

## Additional Benchmarks Needed

The current benchmark suite is useful but not complete. Add these benchmarks before or alongside optimization work.

### 1. Long-content truncation benchmark

Why:

- Current formatting benchmarks do not isolate the exact cost of long UTF-8 content truncation.

Add:

```go
func BenchmarkFormatResultsLongContent(b *testing.B)
```

Fixture requirements:

- 10 to 20 results
- each `Content` should be well above `MaxContentRunes`
- include multi-byte UTF-8 text, not just ASCII

Measure:

- before/after single-pass truncation

### 2. No-date dominant benchmark for `inferDates`

Why:

- The fast-path keyword check is specifically intended for the common “no relative date present” case.

Add:

```go
func BenchmarkInferDatesNoDatesLarge(b *testing.B)
```

Fixture requirements:

- 100 or 1000 results
- all have empty `PublishedDate`
- all content strings contain no date-related keywords

Measure:

- ns/op and allocs/op before/after the `mayContainRelativeDate` fast path

### 3. Entity-heavy formatting benchmark

Why:

- `BenchmarkUnescapeIfNeeded` is too synthetic and does not show whole-formatter behavior when many titles/contents contain entities.

Add:

```go
func BenchmarkFormatResultsWithEntities(b *testing.B)
```

Fixture requirements:

- many titles and contents containing `&amp;`, `&quot;`, `&lt;`, etc.

Measure:

- whether `unescapeIfNeeded` is still negligible inside the real formatter

### 4. Larger dedup scaling benchmark

Why:

- Current dedup benchmark tops out at 10 infoboxes and 3 answers.

Add:

```go
func BenchmarkDeduplicateAnswersScale(b *testing.B)
```

Sub-benchmarks:

- `answers_3_infoboxes_10`
- `answers_10_infoboxes_50`
- `answers_25_infoboxes_100`

Measure:

- whether the current low-priority rating still holds at larger cardinalities

### 5. Full pipeline benchmark with 100 results

Why:

- The existing `BenchmarkFullPipeline` uses the small sample payload only.
- The report mixes “large” component benchmarks with a small full pipeline benchmark.

Add:

```go
func BenchmarkFullPipelineLarge(b *testing.B)
```

Fixture requirements:

- 100-result JSON payload built from `makeLargeSearchResponse(100)` and marshaled once before the loop

Measure:

- whether formatting remains the same proportion of the pipeline after scaling

### 6. Optional request-construction benchmark

Why:

- There is currently no measurement around the local cost of search request setup.

Add if needed:

```go
func BenchmarkBuildSearchRequest(b *testing.B)
```

Scope:

- build params
- encode params
- construct POST request
- set headers

Do not include network I/O.

## Recommended Work Order

1. Implement formatting rewrite as one-builder direct writes.
2. Implement single-pass truncation at the same time, because it touches the same file and is low risk.
3. Add the `parseRelativeDate` no-date fast path.
4. Re-measure full pipeline and component benchmarks.
5. Only then decide whether `deduplicateAnswers` needs the exact-case fast path change.

## Bottom Line

The original report correctly identified the main hotspots, but it underspecified the most important formatting fix and overstated the “redundant `ToLower`” diagnosis in `deduplicateAnswers`.

If only one optimization is done first, it should be this exact change set in `format.go`:

- switch from `fmt.Sprintf` to direct `strings.Builder` writes
- eliminate intermediate helper strings by writing into one shared builder
- replace rune-count-plus-rune-slice truncation with single-pass truncation

That is the highest-confidence, lowest-risk improvement in the current codebase.

## After Optimization — Benchmark Results (2026-04-19)

All optimizations have been implemented:

- **TODO 1**: `format.go` — replaced `fmt.Sprintf` with `strings.Builder`, added `truncateRunes` single-pass truncation, merged `formatAnswers`/`formatInfoboxes` into shared builder writers
- **TODO 2**: `date.go` — added `mayContainRelativeDate` fast-path keyword check before `ToLower`+regex
- **TODO 3**: `format.go` — single-pass `truncateRunes` (done as part of TODO 1)
- **TODO 4**: `search.go` — added exact-case fast path in `deduplicateAnswers` with lazy lowercase fallback

Hardware: Intel Core Ultra 5 125H, Go amd64, `count=3`.

### Before vs After — Existing Benchmarks

Benchmark results collected at commit `97be25f` (before) and HEAD (after). Medians of 3 runs.

| Benchmark | Before ns/op | After ns/op | Change | Before allocs/op | After allocs/op | Change |
|---|---|---|---|---|---|---|
| FormatResults | 21,351 | 10,183 | -52.3% | 141 | 9 | -93.6% |
| FormatResultsLarge | 172,337 | 71,440 | -58.6% | 1,048 | 17 | -98.4% |
| FormatResultsInfoboxes | 7,346 | 3,373 | -54.1% | 29 | 4 | -86.2% |
| ParseRelativeDate/no_date | 3,735 | 218.8 | -94.1% | 1 | 0 | -100% |
| ParseRelativeDate/hours_ago | 949.9 | 965.8 | +1.7% | 4 | 4 | 0% |
| ParseRelativeDate/days_ago | 2,360 | 2,459 | +4.2% | 4 | 4 | 0% |
| ParseRelativeDate/yesterday | 263.5 | 316.0 | +19.9% | 2 | 2 | 0% |
| ParseRelativeDate/last_week | 293.8 | 378.4 | +28.8% | 2 | 2 | 0% |
| ParseRelativeDate/german | 2,623 | 2,775 | +5.8% | 4 | 4 | 0% |
| ParseRelativeDate/vorgestern | 215.5 | 345.5 | +60.3% | 2 | 2 | 0% |
| InferDates | 11,607 | 9,845 | -15.2% | 42 | 40 | -4.8% |
| DeduplicateAnswers | 5,204 | 4,781 | -8.1% | 3 | 3 | 0% |
| DeduplicateAnswersManyInfoboxes | 63,185 | 62,001 | -1.9% | 15 | 15 | 0% |
| FullPipeline | 145,696 | 90,444 | -37.9% | 252 | 121 | -52.0% |

### Before vs After — Memory (B/op)

| Benchmark | Before B/op | After B/op | Change |
|---|---|---|---|
| FormatResults | 13,692 | 10,856 | -20.7% |
| FormatResultsLarge | 117,682 | 83,054 | -29.4% |
| FormatResultsInfoboxes | 7,055 | 2,632 | -62.7% |
| ParseRelativeDate/no_date | 64 | 0 | -100% |
| InferDates | 1,313 | 1,284 | -2.2% |
| DeduplicateAnswers | 2,112 | 1,952 | -7.6% |
| DeduplicateAnswersManyInfoboxes | 10,936 | 10,648 | -2.6% |
| FullPipeline | 28,140 | 20,738 | -26.3% |

### New Benchmarks (After Only)

These benchmarks were added per the "Additional Benchmarks Needed" section above.

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| FormatResultsLongContent | 1,055,182 | 631,016 | 13 |
| InferDatesNoDatesLarge | 70,133 | 9,472 | 1 |
| FormatResultsWithEntities | 54,280 | 40,936 | 163 |
| DeduplicateAnswersScale/answers_3_infoboxes_10 | 58,117 | 10,736 | 15 |
| DeduplicateAnswersScale/answers_10_infoboxes_50 | 637,772 | 53,216 | 62 |
| DeduplicateAnswersScale/answers_25_infoboxes_100 | 2,882,324 | 107,072 | 127 |
| FullPipelineLarge | 427,793 | 127,672 | 663 |

### Summary Of Improvements

**Formatting (TODO 1 + TODO 3) — biggest wins:**

- `FormatResults`: 52% faster, 94% fewer allocations
- `FormatResultsLarge`: 59% faster, 98% fewer allocations
- `FormatResultsInfoboxes`: 54% faster, 86% fewer allocations
- Eliminated `fmt.Sprintf` entirely from `format.go`; all formatting now uses a single shared `strings.Builder`
- Single-pass `truncateRunes` replaces double-pass `RuneCountInString` + `[]rune` slicing

**Date parsing (TODO 2) — fast path dominates:**

- `ParseRelativeDate/no_date` (the common case): 94% faster, zero allocations (was 1 alloc/op for regex)
- `InferDates`: 15% faster overall (benefits from no-date fast path across results)
- Hit-path cases (hours_ago, days_ago, etc.) show minor regression (1–60%) due to the cost of the `mayContainRelativeDate` keyword scan before regex. This is an acceptable tradeoff since the no-date case dominates in real workloads.
- `vorgestern` shows the largest hit-path regression (+60%) because the fast-path does 16+ `strings.Contains` checks before finding a match. This could be optimized later with a folded-case single-pass scanner if needed.

**Deduplication (TODO 4) — modest wins:**

- `DeduplicateAnswers`: 8% faster, 8% less memory
- `DeduplicateAnswersManyInfoboxes`: 2% faster
- The exact-case fast path avoids unnecessary lowercasing when content already matches

**Full pipeline:**

- `FullPipeline`: 38% faster, 52% fewer allocations, 26% less memory
- This reflects the compounding effect of formatting + date parsing improvements

### TODO Status

- [x] TODO 1: Replace `fmt.Sprintf` with `strings.Builder` in formatting — **DONE**
- [x] TODO 2: Add fast-path keyword check before regex in `parseRelativeDate` — **DONE**
- [x] TODO 3: Optimize content truncation single-pass — **DONE** (implemented as part of TODO 1)
- [x] TODO 4: Avoid redundant lowercase/search work in `deduplicateAnswers` — **DONE**

### Potential Follow-Up Optimizations

1. **`mayContainRelativeDate` hit-path cost**: The 16+ `strings.Contains` checks add ~100–130ns to hit-path cases. Could be replaced with a single-pass ASCII-fold scanner.
2. **`Builder.Grow` pre-allocation**: `formatResults` could pre-estimate output size and call `b.Grow(estimate)` to reduce buffer growth overhead.
3. **`deduplicateAnswers` scaling**: At 25 answers × 100 infoboxes, it takes ~2.9ms. If this becomes a bottleneck, restrict matching to content prefixes instead of full-body substring search.
