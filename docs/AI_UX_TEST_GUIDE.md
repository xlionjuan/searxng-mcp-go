# SearXNG MCP AI 體感測試指南

> 本文件旨在建立一套由 AI 進行的「體感測試」流程——不同於單元測試驗證功能是否正常，體感測試從**使用者（AI agent）視角**感受工具是否好用，並量化回饋以驅動改進。

---

## 測試目標

以 AI agent 視角，全面評估 SearXNG MCP server 的使用體驗：

1. **Schema 宣告是否足夠清楚** — AI 能否從 schema 推斷出正確用法
2. **回傳格式是否符合需求** — 結果是否容易理解和進一步處理
3. **穩定性與錯誤處理** — 各種輸入會不會造成非預期行為
4. **整體便利性** — 使用流程是否順暢，有沒有讓人想翻白眼的地方

---

## 前置準備

### 必要環境

- **專案位置**: `~/git/searxng-mcp-go`
- **Binary**: `./searxng-mcp-go`
- **SearXNG 實例**: `https://search-4.xlion.dev/`
- **可用 toolsets**: `terminal`, `file`, `web`, `skills`, `session_search`

### MCP Server 啟動方式

MCP server 使用 stdio 模式運行，意思是所有溝通都透過 stdin/stdout 的 JSON-RPC 進行。

**兩種測試切入點：**

1. **整合測試**（透過 Hermes Agent MCP tool）:
   ```bash
   # 直接 call mcp_searxng_search tool
   # 適合測試 end-to-end workflow
   ```

2. **隔離測試**（直接送 JSON-RPC）:
   ```bash
   # 初始化
   echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./searxng-mcp-go
   
   # 呼叫 tools/list
   echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | ./searxng-mcp-go
   
   # 呼叫 tools/call（search）
   echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search","arguments":{"query":"test","language":"en","safesearch":null,"time_range":"","categories":"","engines":"","pageno":null}}}' | ./searxng-mcp-go
   ```

---

## 測試項目清單

### 1. Schema 宣告評估

**測試方法:** 呼叫 `tools/list` 查看 `inputSchema`，不要看 source code，純就 schema 文字推斷用法。

**評估維度:**

| 維度 | 問題 | 評分（1-5）|
|------|------|-----------|
| 參數名稱直覺性 | 名稱是否能見名知意？ | |
| 型別宣告清晰度 | 看到 `type: ["null","integer"]` 知道要傳什麼？ | |
| 預設值說明 | 宣告中是否有說明預設值？ | |
| 列舉值完整性 | 如 `safesearch`、`time_range` 是否列出所有有效值？ | |
| 必填/選填區分 | AI 能否從 schema 判斷哪些參數可省略？ | |

**必試場景:**

```bash
# 只傳 query（測試 required 是否正確）
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search","arguments":{"query":"Golang MCP server"}}}' | ./searxng-mcp-go

# 預期：成功（只有 query 必填）
# 陷阱：若 required 宣告錯誤，會回傳驗證錯誤
```

### 2. 回傳格式評估

**測試方法:** 執行幾個不同 query，觀察回傳文字格式。

**評估維度:**

| 維度 | 問題 | 評分（1-5）|
|------|------|-----------|
| 結果數量標示 | 是否清楚說明找到多少結果？ | |
| 欄位完整性 | 每筆記錄是否包含標題/URL/摘要/來源？ | |
| 摘要品質 | summary 是否真的有內容而非空白？ | |
| 格式一致性 | 多筆結果格式是否統一？ | |
| 結構化程度 | 純文字還是有 JSON？AI 能否程式化解析？ | |

**必試 query:**

```bash
# 基本英文
query="MCP protocol specification", language="en"

# 中文
query="測試", language="zh-tw"

# 空結果預期
query="xyzzyqqq1234567890xyz", language="en"

# 長 query
query="What are the best practices for building MCP servers in Go in 2024 and 2025", language="en"
```

### 3. 參數組合測試

**必試組合:**

| 參數組合 | 預期行為 | 觀察 |
|---------|---------|------|
| `query="news", categories="news"` | 只搜新聞 | 結果有過濾嗎？|
| `query="AI", engines="google,bing"` | 只用這兩個引擎 | 結果多樣嗎？|
| `query="test", pageno=2` | 第二頁 | 內容與第一頁不同嗎？|
| `query="test", time_range="day"` | 最近一天 | 時間有過濾嗎？|
| `query="test", safesearch=2` | 嚴格過濾 | 結果變少了嗎？|

### 4. 錯誤處理與穩定性

**必試場景:**

```bash
# 空 query
{"query": ""} → 應回傳「query is required」錯誤

# 無效 time_range
{"query": "test", "time_range": "invalid"} → 應說明有效值

# 無效 safesearch
{"query": "test", "safesearch": 5} → 應說明有效範圍 0-2

# 無效 engine
{"query": "test", "engines": "nonexistent_engine_xyz"} → SearXNG 如何處理？

# 網路錯誤（若可模擬）
# 斷網、target unreachable 等
```

**觀察重點:**
- 錯誤訊息是否 human-readable（非 JSON parse 錯誤）
- 會不會 panic / 吐出非 JSON 的垃圾
- 錯誤後 server 是否繼續正常運作（還是用一次就死了）

### 5. 隔離性測試

**測試目的:** 確認多次請求之間是否乾淨隔離。

```bash
# 請求 1：設定某個參數
{"query": "apple", "language": "en"}

# 請求 2：完全不帶參數（測試殘留）
{"query": "banana"}

# 觀察：第二次是否受到第一次影響？
```

---

## 輸出格式

完成測試後，請產出以下格式的報告：

```markdown
## 🔴 嚴重問題（阻擋使用）

## ⚠️ 中等問題（影響體驗）

## 💡 改進建議（可選）

## 總評

| 項目 | 分數（1-5）| 備註 |
|------|-----------|------|
| Schema 清晰度 | | |
| 回傳格式滿意度 | | |
| 穩定性 | | |
| 錯誤處理品質 | | |
| 參數驗證完整性 | | |
| 整體便利性 | | |
```

---

## 測試時的爽點與痛點記錄

除了量化評分，也請記錄這些：

**讓人眼睛一亮的地方：**
- 什麼設計決定讓 AI 用起來特別順？
- 錯誤訊息是否精準到讓人驚訝？

**讓人血壓上升的地方：**
- 明明很簡單的事為什麼要繞路？
- 哪個錯誤讓你浪費了最多時間 debug？

**「如果我是開發者」的心聲：**
- 哪些資訊缺失讓你難以最佳化使用方式？
- 哪些 schema 描述讓你產生錯誤預期？

---

## 附錄：常見 JSON-RPC 互動範例

### 完整對話流程

```bash
# 1. 初始化
→ {"jsonrpc":"2.0","id":0,"method":"initialize","params":{...}}
← {"jsonrpc":"2.0","id":0,"result":{"protocolVersion":"...","capabilities":{...}}}

# 2. 列出 tools
→ {"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}
← {"jsonrpc":"2.0","id":1,"result":{"tools":[...]}}

# 3. 執行 search
→ {"jsonrpc":"2.0","id":2,"method":"tools/call","params":{...}}
← {"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"..."}]}}
```

### 快速驗證指令

```bash
# 驗證 required bug（若只傳 query 失敗 = 有 bug）
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"query":"test"}}}' | ./searxng-mcp-go

# 列出所有 tools 並格式化
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | ./searxng-mcp-go | jq '.result.tools[0].inputSchema'
```
