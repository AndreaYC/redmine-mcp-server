# Redmine MCP Server 設計文件

## 概述

建立一個 Redmine MCP Server，讓 AI 助手（Claude、ChatGPT、Cursor 等）可以操作 Redmine。

### 目標用戶
- 30 人團隊
- 每個用戶使用自己的 Redmine API Key

### 支援平台
| 平台 | 傳輸方式 |
|------|---------|
| Claude Desktop / Claude Code | MCP (stdio) |
| Cursor / Cline | MCP (SSE) |
| ChatGPT GPT Actions | REST API (OpenAPI) |

---

## 架構

```
┌─────────────────────────────────────────────────┐
│              redmine-mcp-server                 │
├─────────────────────────────────────────────────┤
│  核心層：Redmine API Client                      │
├──────────────────┬──────────────────────────────┤
│  MCP Server      │  REST API Server             │
│  (stdio / SSE)   │  (OpenAPI 3.0)               │
│                  │                              │
│  → Claude        │  → ChatGPT GPT Actions       │
│  → Cursor        │  → 任何 REST client          │
│  → Cline         │                              │
│  → Claude Code   │                              │
└──────────────────┴──────────────────────────────┘
```

### 執行模式

| 命令 | 傳輸方式 | API Key 來源 | 使用場景 |
|------|---------|-------------|---------|
| `./server mcp` | stdio | 環境變數 `REDMINE_API_KEY` | 本地 Claude Desktop |
| `./server mcp --sse --port 8080` | SSE over HTTP | Header `X-Redmine-API-Key` | Docker 部署 |
| `./server api --port 8080` | REST HTTP | Header `X-Redmine-API-Key` | ChatGPT GPT Actions |

### 認證設計

**共享部署模式（推薦）：**
- 一個 Docker container 服務所有用戶
- 每個 request 帶自己的 Redmine API Key（Header: `X-Redmine-API-Key`）
- Server 完全 stateless

**本地模式：**
- 用戶自己執行 binary
- API Key 從環境變數讀取

---

## 專案結構

```
redmine-mcp-server/
├── cmd/
│   └── server/
│       └── main.go              # CLI 入口
├── internal/
│   ├── redmine/
│   │   └── client.go            # Redmine REST API Client
│   ├── mcp/
│   │   ├── server.go            # MCP Server (stdio + SSE)
│   │   └── tools.go             # MCP Tools 定義
│   └── api/
│       ├── server.go            # REST API Server
│       ├── handlers.go          # REST endpoints
│       └── docs.go              # Swagger 註解
├── docs/
│   ├── plans/                   # 設計文件
│   └── openapi.yaml             # OpenAPI 3.0 spec
├── .gitea/
│   └── workflows/
│       └── ci.yaml              # Gitea Actions CI/CD
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── go.mod
└── README.md
```

---

## MCP Tools（14 個）

### 設計原則

1. **參數支援 ID 或名稱** - Server 自動解析
2. **名稱重複時回傳錯誤** - 列出候選項讓用戶選擇
3. **Custom Fields 用名稱** - 不需要記 ID

### Issues（8 個）

#### issues.search
搜尋 issues。

| 參數 | 類型 | 必填 | 說明 |
|------|------|------|------|
| project | string | 否 | 專案名稱或 ID |
| tracker | string | 否 | Tracker 名稱或 ID |
| status | string | 否 | 狀態：open/closed/all 或具體狀態名稱 |
| assigned_to | string | 否 | 指派人名稱、email 或 ID，"me" 表示自己 |
| limit | int | 否 | 回傳筆數，預設 25 |

#### issues.getById
取得單一 issue 詳情，包含描述、備註、附件、關聯。

| 參數 | 類型 | 必填 | 說明 |
|------|------|------|------|
| issue_id | int | 是 | Issue ID |

#### issues.update
更新 issue。

| 參數 | 類型 | 必填 | 說明 |
|------|------|------|------|
| issue_id | int | 是 | Issue ID |
| status | string | 否 | 新狀態名稱或 ID |
| assigned_to | string | 否 | 新指派人 |
| notes | string | 否 | 備註內容 |
| custom_fields | object | 否 | 自訂欄位 key-value |

#### issues.create
建立 issue。

| 參數 | 類型 | 必填 | 說明 |
|------|------|------|------|
| project | string | 是 | 專案名稱或 ID |
| tracker | string | 是 | Tracker 名稱或 ID |
| subject | string | 是 | 標題 |
| description | string | 否 | 描述 |
| assigned_to | string | 否 | 指派人 |
| parent_issue_id | int | 否 | 父任務 ID |
| start_date | string | 否 | 開始日期 YYYY-MM-DD |
| due_date | string | 否 | 到期日期 YYYY-MM-DD |
| custom_fields | object | 否 | 自訂欄位 key-value |

**custom_fields 範例：**
```json
{
  "custom_fields": {
    "Severity": "Major",
    "HW Version": "L11 SKYRack Cabinet",
    "FW Version": "v0.01",
    "Issue Finder": "SWQA",
    "Solution(Root cause)": "Done / Fixed",
    "Error Type": "Missing implementation/coding",
    "RD_Function_Team": "N/A"
  }
}
```

#### issues.createSubtask
在指定 issue 下建立子任務。

| 參數 | 類型 | 必填 | 說明 |
|------|------|------|------|
| parent_issue_id | int | 是 | 父 issue ID |
| subject | string | 是 | 子任務標題 |
| tracker | string | 否 | Tracker，預設繼承父任務 |
| description | string | 否 | 描述 |
| assigned_to | string | 否 | 指派人 |
| custom_fields | object | 否 | 自訂欄位 |

#### issues.addWatcher
將用戶加入 issue 觀察者。

| 參數 | 類型 | 必填 | 說明 |
|------|------|------|------|
| issue_id | int | 是 | Issue ID |
| user | string | 是 | 用戶名稱、email 或 ID |

#### issues.addRelation
建立兩個 issue 間的關聯。

| 參數 | 類型 | 必填 | 說明 |
|------|------|------|------|
| issue_id | int | 是 | 來源 issue ID |
| issue_to_id | int | 是 | 目標 issue ID |
| relation_type | string | 是 | relates/duplicates/blocks/precedes/copied_to |

### Projects（2 個）

#### projects.list
列出所有專案。

| 參數 | 類型 | 必填 | 說明 |
|------|------|------|------|
| limit | int | 否 | 回傳筆數，預設 100 |

#### projects.create
建立新專案。

| 參數 | 類型 | 必填 | 說明 |
|------|------|------|------|
| name | string | 是 | 專案名稱 |
| identifier | string | 是 | 專案識別碼（URL 用）|
| description | string | 否 | 專案描述 |
| parent | string | 否 | 父專案名稱或 ID |

### Time Entries（1 個）

#### timeEntries.create
記錄工時。

| 參數 | 類型 | 必填 | 說明 |
|------|------|------|------|
| issue_id | int | 是 | Issue ID |
| hours | float | 是 | 工時（小時）|
| activity | string | 否 | 活動類型名稱或 ID |
| comments | string | 否 | 備註 |

### Account（1 個）

#### me
取得目前用戶資訊。

回傳：user_id、姓名、email、登入帳號

### Reference（2 個）

#### trackers.list
列出所有 tracker。

回傳：tracker ID 與名稱列表

#### customFields.list
列出自訂欄位，可依 tracker 過濾。

| 參數 | 類型 | 必填 | 說明 |
|------|------|------|------|
| tracker | string | 否 | Tracker 名稱或 ID |

回傳：欄位名稱、類型、是否必填、選項列表

---

## REST API Endpoints

### Base URL
```
https://<server>/api/v1
```

### 認證
```
Header: X-Redmine-API-Key: <your_redmine_api_key>
```

### Endpoints

| Method | Path | 對應 MCP Tool |
|--------|------|--------------|
| GET | `/me` | me |
| GET | `/projects` | projects.list |
| POST | `/projects` | projects.create |
| GET | `/issues` | issues.search |
| GET | `/issues/:id` | issues.getById |
| POST | `/issues` | issues.create |
| PATCH | `/issues/:id` | issues.update |
| POST | `/issues/:id/subtasks` | issues.createSubtask |
| POST | `/issues/:id/watchers` | issues.addWatcher |
| POST | `/issues/:id/relations` | issues.addRelation |
| POST | `/time_entries` | timeEntries.create |
| GET | `/trackers` | trackers.list |
| GET | `/custom_fields` | customFields.list |

### API 文件

| URL | 說明 |
|-----|------|
| `GET /docs` | Swagger UI 互動式文件 |
| `GET /openapi.yaml` | OpenAPI 3.0 spec（給 ChatGPT 匯入）|

---

## 錯誤處理

### 名稱解析錯誤

當名稱找不到或有多個符合時：

```json
{
  "error": "找到多個符合「小明」的用戶",
  "matches": [
    { "id": 123, "name": "王小明", "email": "ming.wang@company.com" },
    { "id": 456, "name": "李小明", "email": "ming.lee@company.com" }
  ],
  "hint": "請用 email 或 ID 指定"
}
```

### API 錯誤

```json
{
  "error": "Redmine API 錯誤",
  "status": 403,
  "message": "You are not authorized to access this resource"
}
```

---

## 環境變數

| 變數 | 說明 | 預設值 |
|------|------|--------|
| `REDMINE_URL` | Redmine 伺服器位址 | (必填) |
| `REDMINE_API_KEY` | API Key（stdio 模式用）| - |
| `PORT` | HTTP 服務 port | 8080 |
| `LOG_LEVEL` | 日誌等級 debug/info/warn/error | info |

---

## Docker

### Dockerfile

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/server /usr/local/bin/
EXPOSE 8080
ENTRYPOINT ["server"]
CMD ["api"]
```

### docker-compose.yml

```yaml
version: '3.8'

services:
  redmine-mcp:
    image: harbor.sw.ciot.work/mcp/redmine:latest
    ports:
      - "8080:8080"
    environment:
      - REDMINE_URL=http://advrm.advantech.com:3002
    restart: unless-stopped
```

### 使用方式

```bash
# REST API 模式
docker run -p 8080:8080 \
  -e REDMINE_URL=http://advrm.advantech.com:3002 \
  harbor.sw.ciot.work/mcp/redmine:latest api

# MCP SSE 模式
docker run -p 8080:8080 \
  -e REDMINE_URL=http://advrm.advantech.com:3002 \
  harbor.sw.ciot.work/mcp/redmine:latest mcp --sse
```

---

## Makefile

```makefile
.PHONY: build run-mcp run-sse run-api test lint swagger docker-build docker-run release

# 編譯
build:
	go build -o bin/server ./cmd/server

# 本地執行
run-mcp:
	go run ./cmd/server mcp

run-sse:
	go run ./cmd/server mcp --sse --port 8080

run-api:
	go run ./cmd/server api --port 8080

# 測試
test:
	go test -v -race ./...

# Lint
lint:
	golangci-lint run

# Swagger 文件產生
swagger:
	swag init -g cmd/server/main.go -o docs

# Docker
docker-build:
	docker build -t harbor.sw.ciot.work/mcp/redmine:latest .

docker-run:
	docker run -p 8080:8080 -e REDMINE_URL=http://advrm.advantech.com:3002 harbor.sw.ciot.work/mcp/redmine:latest

docker-push:
	docker push harbor.sw.ciot.work/mcp/redmine:latest

# 多平台發布
release:
	GOOS=linux GOARCH=amd64 go build -o bin/server-linux-amd64 ./cmd/server
	GOOS=darwin GOARCH=amd64 go build -o bin/server-darwin-amd64 ./cmd/server
	GOOS=darwin GOARCH=arm64 go build -o bin/server-darwin-arm64 ./cmd/server
	GOOS=windows GOARCH=amd64 go build -o bin/server-windows-amd64.exe ./cmd/server
```

---

## Gitea Actions CI/CD

```yaml
# .gitea/workflows/ci.yaml
name: CI/CD

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v4

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Run tests
        run: go test -v -race ./...

  build:
    runs-on: ubuntu-latest
    needs: [lint, test]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Build
        run: go build -o server ./cmd/server

  docker:
    runs-on: ubuntu-latest
    needs: [build]
    if: github.ref == 'refs/heads/main'
    steps:
      - uses: actions/checkout@v4
      - name: Login to Harbor
        uses: docker/login-action@v3
        with:
          registry: harbor.sw.ciot.work
          username: ${{ secrets.HARBOR_USERNAME }}
          password: ${{ secrets.HARBOR_PASSWORD }}
      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          push: true
          tags: |
            harbor.sw.ciot.work/mcp/redmine:latest
            harbor.sw.ciot.work/mcp/redmine:${{ github.sha }}
```

---

## 用戶端設定範例

### Claude Desktop（stdio 模式）

`~/.claude/claude_desktop_config.json`
```json
{
  "mcpServers": {
    "redmine": {
      "command": "/path/to/server",
      "args": ["mcp"],
      "env": {
        "REDMINE_URL": "http://advrm.advantech.com:3002",
        "REDMINE_API_KEY": "your-api-key"
      }
    }
  }
}
```

### Cursor / Cline（SSE 模式）

```json
{
  "mcpServers": {
    "redmine": {
      "url": "http://localhost:8080/sse",
      "headers": {
        "X-Redmine-API-Key": "your-api-key"
      }
    }
  }
}
```

### ChatGPT GPT Actions

1. 前往 GPT 編輯頁面
2. 新增 Action
3. 匯入 `http://<server>/openapi.yaml`
4. 設定 Authentication: API Key (Header: X-Redmine-API-Key)

---

## 技術選擇

| 項目 | 選擇 | 原因 |
|------|------|------|
| 語言 | Go 1.22 | 效能好、單一 binary 易部署 |
| MCP SDK | mark3labs/mcp-go | 目前最成熟的 Go MCP SDK |
| HTTP Router | chi 或 gin | 輕量、效能好 |
| Swagger | swaggo/swag | 從註解產生 OpenAPI spec |
| 日誌 | slog (標準庫) | Go 1.21+ 內建，夠用 |

---

## 待確認項目

實作時需要測試確認：

1. **Redmine 版本** - 確認 API 相容性
2. **Custom Fields API** - 確認回傳格式
3. **Tracker 對應的 Custom Fields** - 哪些欄位對應哪些 tracker
4. **必填欄位驗證** - 哪些 custom fields 是必填

---

## 下一步

1. 設定 Git 專案基礎檔案（go.mod, .gitignore）
2. 實作 Redmine API Client
3. 實作 MCP Server（先做 stdio 模式）
4. 實作 REST API Server
5. 加入 Swagger 文件
6. Docker 化
7. 設定 CI/CD
