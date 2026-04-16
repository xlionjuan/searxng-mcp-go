# ADR-001: 不使用 PGO（Profile-Guided Optimization）

## 狀態

已接受（Accepted）

## 背景

在 2026-04-16 的程式碼現代化審查中，有人建議為 build 加入 `-pgo=auto` 來提升效能。

## 決定

**不使用 PGO**，並明確禁止日後在未取得具代表性的 profiling data 前重新引入。

## 原因

### 1. 工具特性不適合 PGO

searxng-mcp-go 是 **MCP server / CLI 工具**：
- 每次執行時間短（請求 → 響應 → 退出）
- 無法像長期執行的 HTTP server 一樣持續收集 profiling data
- 每次 process 退出後 profiling data 就寫入磁碟，無法跨 process 累積有意義的熱路徑資訊

### 2. 沒有具代表性的 profiling data

PGO 的核心前提是：**profiling data 必須代表真實 workload**。

| 資料來源 | 問題 |
|----------|------|
| 手工少數幾次執行 | 樣本不足，路徑覆蓋片面 |
| Benchmark script | 人工構造的 workload 無法代表真實使用情境 |
| 個人使用累積 | 查詢多樣性低，無法涵蓋多數程式碼路徑 |

沒有 representative data 的 PGO **可能反而讓效能變差**（編譯器對錯誤的熱路徑做錯誤的優化）。

### 3. 瓶頸不在本地 compute

對 SearXNG MCP server 來說：
- **真正的瓶頸是網路延遲**（打外部 SearXNG API 的時間）
- 本地 CPU 計算所佔比例極小
- 即使 PGO 帶來 5~15% 的本地 compute 提升，整體 end-to-end 延遲改善也微乎其微

### 4. CP 值過低

折騰 PGO 需要：
- 研究如何收集 profiling data
- 設計 representative benchmark
- 維護 `.pgo` 檔案並持續更新

投入的時間成本遠高於實際收益。

## 替代優化方向

如果未來需要提升效能，以下方向更有效：

1. **結果快取（result cache）** — 針對完全相同的 query 直接回傳上次結果（需另開機制，不建議現在做）
2. **降低網路延遲** — 使用更近的 SearXNG instance
3. **並行查詢** — 同時打多個 engine 但不等待所有結果
4. **壓縮回應** — 減少傳輸資料量

## 重新考慮的條件

如果未來發生以下情況，可以重新評估加入 PGO：

- searxng-mcp-go 變成长期运行的 MCP server 部署
- 累积了足够多的真实 production profiling data
- 有明确的 benchmark 证明 PGO 带来可量化的改善

## 生效日期

2026-04-16
