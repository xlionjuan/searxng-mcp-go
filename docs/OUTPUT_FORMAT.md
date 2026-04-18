# Output Format

SearXNG MCP Server 提供兩種輸出模式：**CLI 文字模式**（預設）和 **JSON 模式**（`--json`）。

```bash
# CLI 文字模式（預設）
./searxng-mcp-go "query"

# JSON 模式
./searxng-mcp-go "query" --json
```

---

## 範例一：完整輸出（所有欄位皆有值）

查詢：`apple inc`，此查詢同時觸發 Answers、Infoboxes、Results 與 Suggestions。

### CLI 文字模式

```
$ ./searxng-mcp-go "apple inc"

=== Answers ===

[1] Apple Inc. is an American multinational technology company headquartered in Cupertino, California, in Silicon Valley, best known for its consumer electronics, software and online services. Founded in 1976 as Apple Computer Company by Steve Jobs, Steve Wozniak and Ronald Wayne, the company was incorporated by Jobs and Wozniak as Apple Computer, Inc. the following year. It was renamed to its current name in 2007 as the company had expanded its focus from computers to consumer electronics. More at Wikipedia
    Engine: duckduckgo

=== Infoboxes ===

[1] Apple Inc.
    Apple Inc. is an American multinational technology company headquartered in Cupertino, California, in Silicon Valley, best known for its consumer electronics, software and online services. ...
    Attributes:
      - Formerly called: Apple Computer Company (1976–1977), Apple Computer, Inc. (1977–2007)
      - Type: Public
      - Traded as: NASDAQ: AAPL, Nasdaq-100 component, DJIA component, S&P 100 component, S&P 500 component
      - Industry: Consumer electronics, Software services, Online services
      - Founded: April 01, 1976, in Los Altos, California, US
      - Founders: Steve Jobs, Steve Wozniak, Ronald Wayne
      - Key people: Arthur Levinson (chairman), Tim Cook (CEO)
      - Products: AirPods, AirTag, Apple TV, Apple Vision Pro, Apple Watch, HomePod, iPad, iPhone, Mac
      ...
    URLs:
      - Wikipedia: https://en.wikipedia.org/wiki/Apple_Inc.
      - Official site: https://www.apple.com/
      ...

Found 16 results for 'apple inc':

1. Apple Inc.
   URL: https://www.apple.com/
   Summary: Discover the innovative world of Apple and shop everything iPhone, iPad, Apple Watch, Mac, and Apple TV, plus explore accessories, entertainment, and expert device support.
   Engine: ddg definitions

2. Apple Inc. (AAPL) Stock Price, News, Quote & History - Yahoo Finance
   URL: https://finance.yahoo.com/quote/AAPL/
   Summary: Find the latest Apple Inc. (AAPL) stock quote, history, news and other vital information to help you with your stock trading and investing.
   Engine: google

...

Suggestions:
  - Apple app
  - Retail companies of the United States
  - Apple Store locations
  - Apple Inc stock
  - Steve Jobs
  - ...
```

### JSON 模式

```json
{
  "query": "apple inc",
  "number_of_results": 16,
  "answers": [
    {
      "answer": "Apple Inc. is an American multinational technology company...",
      "engine": "duckduckgo",
      "template": "answer/legacy.html"
    }
  ],
  "infoboxes": [
    {
      "infobox": "Apple Inc.",
      "content": "Apple Inc. is an American multinational technology company...",
      "attributes": [
        { "label": "Formerly called", "value": "Apple Computer Company (1976–1977)..." },
        { "label": "Type", "value": "Public" },
        { "label": "Industry", "value": "Consumer electronics, Software services, Online services" }
      ],
      "urls": [
        { "title": "Wikipedia", "url": "https://en.wikipedia.org/wiki/Apple_Inc." },
        { "title": "Official site", "url": "https://www.apple.com/" }
      ]
    }
  ],
  "results": [
    {
      "title": "Apple Inc.",
      "url": "https://www.apple.com/",
      "content": "Discover the innovative world of Apple and shop everything iPhone, iPad, Apple Watch, Mac, and Apple TV...",
      "engine": "ddg definitions"
    },
    {
      "title": "Apple Inc. (AAPL) Stock Price, News, Quote & History - Yahoo Finance",
      "url": "https://finance.yahoo.com/quote/AAPL/",
      "content": "Find the latest Apple Inc. (AAPL) stock quote, history, news...",
      "engine": "google"
    }
  ],
  "suggestions": [
    "Apple app",
    "Retail companies of the United States",
    "Apple Store locations",
    "Apple Inc stock"
  ]
}
```

---

## 範例二：簡單輸出（僅有 Results 和 Suggestions）

查詢：`golang tutorial`，此查詢無 Answers、無 Infoboxes。

### CLI 文字模式

```
$ ./searxng-mcp-go "golang tutorial"

Found 17 results for 'golang tutorial':

1. Tutorials - The Go Programming Language
   URL: https://go.dev/doc/tutorial/
   Summary: Learn Go with tutorials on various topics, such as modules, databases, APIs, generics, fuzzing, and vulnerabilities.
   Engine: google

2. Go Tutorial - GeeksforGeeks
   URL: https://www.geeksforgeeks.org/go-language/go/
   Summary: Go (or Golang) is a modern programming language developed by Google, designed for building fast and reliable applications...
   Engine: google

3. Go by Example
   URL: https://gobyexample.com/
   Summary: Learn Go by doing with annotated example programs. Go by Example covers topics such as variables, functions, channels...
   Engine: google

...

Suggestions:
  - Best Golang tutorial
  - Golang tutorial interactive
  - Golang tutorial w3schools
  - Golang tutorial youtube
  - Golang tutorial for beginners
  - Golang tutorial free
```

### JSON 模式

```json
{
  "query": "golang tutorial",
  "number_of_results": 17,
  "results": [
    {
      "title": "Tutorials - The Go Programming Language",
      "url": "https://go.dev/doc/tutorial/",
      "content": "Learn Go with tutorials on various topics, such as modules, databases, APIs, generics, fuzzing, and vulnerabilities.",
      "engine": "google"
    },
    {
      "title": "Go Tutorial - GeeksforGeeks",
      "url": "https://www.geeksforgeeks.org/go-language/go/",
      "content": "Go (or Golang) is a modern programming language developed by Google...",
      "engine": "google"
    }
  ],
  "suggestions": [
    "Best Golang tutorial",
    "Golang tutorial interactive",
    "Golang tutorial w3schools",
    "Golang tutorial for beginners",
    "Golang tutorial free"
  ]
}
```

---

## 空欄位處理

當查詢結果中某個欄位沒有值時，處理方式如下：

| 模式 | 行為 |
|------|------|
| **CLI 文字模式** | 整個區塊被省略，不輸出任何內容。例如沒有 Answers 時，`=== Answers ===` 標題也不會出現。 |
| **JSON 模式** | 使用 Go `json:"...,omitempty"` 標籤，空欄位的 key 會從 JSON 輸出中完全省略。 |

### 具體規則

**CLI 文字模式：**
- `answers` 為空 → 省略 `=== Answers ===` 整個區塊
- `infoboxes` 為空 → 省略 `=== Infoboxes ===` 整個區塊
- `infobox.attributes` 為空 → 省略該 infobox 的 `Attributes:` 子區塊
- `infobox.urls` 為空 → 省略該 infobox 的 `URLs:` 子區塊
- `result.content` 為空 → 省略該結果的 `Summary:` 行
- `result.publishedDate` 為空 → 省略該結果的 `Date:` 行
- `suggestions` 為空 → 省略 `Suggestions:` 整個區塊

**JSON 模式：**
- `answers` 為空 → JSON 中無 `answers` key
- `infoboxes` 為空 → JSON 中無 `infoboxes` key
- `result.publishedDate` 為空 → 該 result 物件中無 `publishedDate` key
- `result.dateSource` 為空 → 該 result 物件中無 `dateSource` key
- `infobox.attributes` 為空 → 該 infobox 物件中無 `attributes` key
- `infobox.urls` 為空 → 該 infobox 物件中無 `urls` key
- `answer.template` 為空 → 該 answer 物件中無 `template` key

---

## 欄位順序

### CLI 文字模式輸出順序

1. **`answers`** — 直接答案（如 IP、Hash、時區等），以 `=== Answers ===` 標題開始
2. **`infoboxes`** — 知識面板，以 `=== Infoboxes ===` 標題開始
3. **`results`** — 搜尋結果列表，以 `Found N results for 'query':` 標題開始
4. **`suggestions`** — 相關搜尋建議，以 `Suggestions:` 標題開始

每個區塊僅在有值時才出現，區塊之間以空行分隔。

### JSON 模式欄位順序

```json
{
  "query": "string",
  "number_of_results": "int",
  "answers": [],
  "infoboxes": [],
  "results": [],
  "suggestions": []
}
```

注：JSON 標準本身不保證欄位順序，但上述為 Go 結構體定義順序，實際序列化時通常會遵循此順序。

---

## 欄位觸發條件

各欄位何時會有值，取決於 SearXNG 後端引擎的回傳結果：

| 欄位 | 觸發條件 |
|------|----------|
| **`answers`** | SearXNG 的 answerer 模組被觸發（如 DuckDuckGo Instant Answer、計算器、IP 查詢、Hash 查詢、時區轉換等）。實體查詢（如公司名、名人名）容易觸發。 |
| **`infoboxes`** | 查詢目標為知名實體（人物、公司、地點、概念），且 SearXNG 引擎（如 Wikipedia、Wikidata）有對應知識面板資料。 |
| **`results`** | 幾乎所有查詢都會回傳結果。數量取決於 SearXNG 配置的引擎數量及該引擎的回應。 |
| **`suggestions`** | SearXNG 引擎回傳了相關搜尋建議。大部分查詢會有建議，但並非 100%。 |

### Result 子欄位觸發條件

| 子欄位 | 觸發條件 |
|--------|----------|
| `title` | 永遠有值（必填欄位） |
| `url` | 永遠有值（必填欄位） |
| `content` | 引擎回傳了摘要/描述文字，部分結果可能無此欄位 |
| `engine` | 永遠有值，標示此結果來自哪個搜尋引擎 |
| `publishedDate` | 引擎提供了發布日期，或程式從內容推斷出日期 |
| `dateSource` | 僅在 `publishedDate` 有值時出現，標示日期來源（`provided` / `inferred`） |

---

## 快速參考

```bash
# CLI 文字模式（預設）
./searxng-mcp-go "apple inc"

# JSON 模式
./searxng-mcp-go "apple inc" --json

# 搭配其他參數
./searxng-mcp-go "apple inc" --json --language=en --safesearch=1 --time_range=month

# 指定自訂 SearXNG 伺服器
./searxng-mcp-go "query" --searxng-url=https://your-searxng.example.com

# Debug 模式
./searxng-mcp-go "query" --debug
```
