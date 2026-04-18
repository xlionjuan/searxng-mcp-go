# SearXNG MCP AI UX Testing Guide

> This document establishes an "UX testing" workflow conducted by AI — unlike unit tests that verify functional correctness, UX testing evaluates whether the tools feel good to use from the **user (AI agent) perspective**, and quantifies feedback to drive improvements.

---

## Testing Objectives

Evaluate the user experience of the SearXNG MCP server from an AI agent perspective:

1. **Schema clarity** — Can the AI infer correct usage from the schema alone?
2. **Response format quality** — Are results easy to understand and further process?
3. **Stability and error handling** — Do various inputs cause unexpected behavior?
4. **Overall convenience** — Is the workflow smooth, or are there pain points?

---

## Prerequisites

### Required Environment

- **Project location**: `.`
- **Binary**: `./searxng-mcp-go`
- **SearXNG instance**: `https://search-4.xlion.dev/`
- **Available toolsets**: `terminal`, `file`, `web`, `skills`, `session_search`

### MCP Server Startup

The MCP server runs in stdio mode, meaning all communication happens via JSON-RPC over stdin/stdout.

**Two testing approaches:**

1. **Integration testing** (via Hermes Agent MCP tool):
   ```bash
   # Directly call the mcp_searxng_search tool
   # Suitable for end-to-end workflow testing
   ```

2. **Isolated testing** (direct JSON-RPC):
   ```bash
   # Initialize
   echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./searxng-mcp-go
   
   # Call tools/list
   echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | ./searxng-mcp-go
   
   # Call tools/call (search)
   echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search","arguments":{"query":"test","language":"en","safesearch":null,"time_range":"","categories":"","engines":"","pageno":null}}}' | ./searxng-mcp-go
   ```

---

## Test Checklist

### 1. Schema Declaration Evaluation

**Method:** Call `tools/list` to inspect `inputSchema`. Do not look at the source code — infer usage purely from the schema text.

**Evaluation Dimensions:**

| Dimension | Question | Score (1-5) |
|-----------|----------|-------------|
| Parameter name intuitiveness | Are the names self-explanatory? | |
| Type declaration clarity | Does seeing `type: ["null","integer"]` make it clear what to pass? | |
| Default value documentation | Does the schema explain default values? | |
| Enum completeness | Are all valid values listed for fields like `safesearch` and `time_range`? | |
| Required/optional distinction | Can the AI determine which parameters are optional from the schema? | |

**Must-try scenarios:**

```bash
# Pass only query (test required field correctness)
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search","arguments":{"query":"Golang MCP server"}}}' | ./searxng-mcp-go

# Expected: success (only query is required)
# Pitfall: if required is incorrectly declared, a validation error will be returned
```

### 2. Response Format Evaluation

**Method:** Run several different queries and observe the returned text format.

**Evaluation Dimensions:**

| Dimension | Question | Score (1-5) |
|-----------|----------|-------------|
| Result count indication | Is it clear how many results were found? | |
| Field completeness | Does each record include title/URL/summary/source? | |
| Summary quality | Does the summary contain actual content rather than being empty? | |
| Format consistency | Is the format uniform across multiple results? | |
| Structured output | Is it plain text or JSON? Can the AI parse it programmatically? | |

**Must-try queries:**

```bash
# Basic English
query="MCP protocol specification", language="en"

# Chinese
query="測試", language="zh-tw"

# Empty result expected
query="xyzzyqqq1234567890xyz", language="en"

# Long query
query="What are the best practices for building MCP servers in Go in 2024 and 2025", language="en"
```

### 3. Parameter Combination Testing

**Must-try combinations:**

| Parameter Combination | Expected Behavior | Observation |
|-----------------------|-------------------|-------------|
| `query="news", categories="news"` | News only | Are results filtered? |
| `query="AI", engines="google,bing"` | Only these two engines | Are results diverse? |
| `query="test", pageno=2` | Second page | Is the content different from the first page? |
| `query="test", time_range="day"` | Last 24 hours | Is the time filter applied? |
| `query="test", safesearch=2` | Strict filtering | Are there fewer results? |

### 4. Error Handling and Stability

**Must-try scenarios:**

```bash
# Empty query
{"query": ""} → Should return "query is required" error

# Invalid time_range
{"query": "test", "time_range": "invalid"} → Should explain valid values

# Invalid safesearch
{"query": "test", "safesearch": 5} → Should explain valid range 0-2

# Invalid engine
{"query": "test", "engines": "nonexistent_engine_xyz"} → How does SearXNG handle this?

# Network errors (if simulable)
# Disconnect, target unreachable, etc.
```

**Observation points:**
- Is the error message human-readable (not a JSON parse error)?
- Does it panic or output non-JSON garbage?
- Does the server continue working normally after an error (or does it die after one use)?

### 5. Isolation Testing

**Objective:** Confirm that requests are cleanly isolated from each other.

```bash
# Request 1: Set a parameter
{"query": "apple", "language": "en"}

# Request 2: No parameters at all (test for leftover state)
{"query": "banana"}

# Observation: Is the second request affected by the first?
```

---

## Output Format

After completing the tests, produce a report in the following format:

```markdown
## 🔴 Critical Issues (Blocking)

## ⚠️ Moderate Issues (Affecting UX)

## 💡 Improvement Suggestions (Optional)

## Summary

| Item | Score (1-5) | Notes |
|------|-------------|-------|
| Schema clarity | | |
| Response format satisfaction | | |
| Stability | | |
| Error handling quality | | |
| Parameter validation completeness | | |
| Overall convenience | | |
```

---

## Delight and Frustration Log

In addition to quantitative scores, please also note the following:

**Impressive moments:**
- Which design decisions made the AI experience especially smooth?
- Were any error messages surprisingly precise?

**Frustrating moments:**
- Why did something simple require a roundabout approach?
- Which error wasted the most debugging time?

**"If I were the developer" thoughts:**
- What missing information made it hard to optimize usage?
- Which schema descriptions created false expectations?

---

## Appendix: Common JSON-RPC Interaction Examples

### Full Conversation Flow

```bash
# 1. Initialize
→ {"jsonrpc":"2.0","id":0,"method":"initialize","params":{...}}
← {"jsonrpc":"2.0","id":0,"result":{"protocolVersion":"...","capabilities":{...}}}

# 2. List tools
→ {"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}
← {"jsonrpc":"2.0","id":1,"result":{"tools":[...]}}

# 3. Execute search
→ {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{...}}
← {"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"..."}]}}
```

### Quick Verification Commands

```bash
# Verify required fields bug (if passing only query fails = bug)
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"query":"test"}}}' | ./searxng-mcp-go

# List all tools and format output
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | ./searxng-mcp-go | jq '.result.tools[0].inputSchema'
```
