# Gateway Serve Subcommand & API Docs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use subagent-driven-development (recommended) to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `serve` subcommand to the Go gateway that enables an interactive `/docs` API testing page (Swagger UI), plus register additional database driver types.

**Architecture:** `db-gateway serve --port 8080 --auth-token x` replaces `--daemon` for long-running mode. The `/docs` page loads Swagger UI from CDN and reads the API spec from `/openapi.json`. New drivers (SQLite, ClickHouse, SQL Server) add Go module dependencies and registry entries.

**Tech Stack:** Go 1.25 (`embed`, `flag.NewFlagSet`, `net/http`), Swagger UI (CDN), `mattn/go-sqlite3`, `clickhouse/go-clickhouse`, `microsoft/go-mssqldb`

## Global Constraints

- Zero new Go dependencies for the serve/docs feature (uses stdlib `embed`)
- New database drivers require `go mod tidy` in `db-gateway/`
- `serve` subcommand always implies daemon mode + service pool config
- `/docs` and `/openapi.json` are only served in `serve` mode
- Auth token still applies (/docs page includes auth header in fetch calls)
- Python `_spawn_gateway` uses `serve` subcommand when `YAMLQ_GATEWAY_DAEMON=1`

---

## Files

| File | Action | Responsibility |
|------|--------|----------------|
| `db-gateway/main.go` | Modify | Add `serve` subcommand via `flag.NewFlagSet` |
| `db-gateway/server/server.go` | Modify | Add `SetServeMode()` method, `/docs` and `/openapi.json` routes |
| `db-gateway/server/openapi.json` | Create | OpenAPI 3.0 spec (embedded via `//go:embed`) |
| `db-gateway/server/swagger.html` | Create | Swagger UI loader page (embedded via `//go:embed`) |
| `db-gateway/server/server_test.go` | Modify | Add tests for `/docs` and `/openapi.json` |
| `db-gateway/driver/registry.go` | Modify | Register SQLite, ClickHouse, SQL Server |
| `db-gateway/go.mod` | Modify | Add `mattn/go-sqlite3`, `clickhouse-go`, `microsoft/go-mssqldb` |
| `db-gateway/dialect/pagination.go` | Modify | Add new driver pagination cases |
| `cli/yamlq/gateway.py` | Modify | Use `serve` subcommand instead of `--daemon` |
| `scripts/yamlq-gateway.sh` | Modify | Use `serve` subcommand |
| `docs/ARCHITECTURE.md` | Modify | Document serve mode + new drivers |

---

### Task 1: Add `serve` subcommand to Go gateway

**Files:**
- Modify: `db-gateway/main.go`

**Interfaces:**
- Consumes: current flag parsing with `flag.CommandLine`
- Produces: `db-gateway serve --port 8080 --auth-token x` subcommand; `Server.SetServeMode(bool)` called from main

- [ ] **Step 1: Read current `db-gateway/main.go`**

Read the full file to understand current flag setup.

- [ ] **Step 2: Refactor with subcommand support**

Add `serve` subcommand using `flag.NewFlagSet`. The existing `flag.CommandLine` handles the default (no subcommand) path. When `os.Args[1] == "serve"`, parse with a new FlagSet:

```go
func main() {
    if len(os.Args) > 1 && os.Args[1] == "serve" {
        serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
        port := serveCmd.Int("port", 0, "listen port (default: random)")
        authToken := serveCmd.String("auth-token", "", "optional auth token for API access")
        serveCmd.Parse(os.Args[2:])
        runServe(*port, *authToken)
        return
    }
    // existing flag parsing for backward compat
    showVersion := flag.Bool("version", false, "...")
    mode := flag.String("mode", "cli", "...")
    authToken := flag.String("auth-token", "", "...")
    daemonMode := flag.Bool("daemon", false, "...")
    portFlag := flag.Int("port", 0, "...")
    flag.Parse()
    // ... existing logic
}

func runServe(port int, authToken string) {
    cfg := config.Service
    mgr := conn.NewManager(cfg)
    srv := server.New(mgr)
    srv.SetServeMode(true)
    if authToken != "" {
        srv.SetAuthToken(authToken)
    }
    addr := fmt.Sprintf("127.0.0.1:%d", port)
    ln, err := net.Listen("tcp", addr)
    if err != nil {
        log.Fatalf("failed to listen: %v", err)
    }
    actualPort := ln.Addr().(*net.TCPAddr).Port
    fmt.Printf("listening on port %d\n", actualPort)
    writeEnvFile(actualPort, authToken, true)
    httpServer := &http.Server{Handler: srv.Handler()}
    go func() {
        if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
            log.Fatalf("server error: %v", err)
        }
    }()
    srv.SetShutdownFunc(func() {
        cleanupEnvFile()
        mgr.CloseAll()
        httpServer.Shutdown(context.Background())
        os.Exit(0)
    })
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh
    cleanupEnvFile()
    mgr.CloseAll()
    httpServer.Shutdown(context.Background())
}
```

- [ ] **Step 3: Verify it compiles and works**

Run: `cd db-gateway && go build ./...`
Then: `./db-gateway serve --port 9999` in background, `curl http://127.0.0.1:9999/ping`
Expected: `go build` passes, `/ping` returns `{"status":"ok"}`

- [ ] **Step 4: Add `--version` to serve subcommand**

In `runServe`, if `os.Args[2] == "--version"`:
```go
fmt.Printf("yamlq/db-gateway %s (commit %s)\n", version, commit)
os.Exit(0)
```

Or simpler: add `version` as a `serveCmd.Bool` flag and check it.

- [ ] **Step 5: Commit**

```bash
git add db-gateway/main.go
git commit -m "feat: add serve subcommand to gateway"
```

---

### Task 2: Add `/openapi.json` and `/docs` endpoints

**Files:**
- Create: `db-gateway/server/openapi.json`
- Create: `db-gateway/server/swagger.html`
- Modify: `db-gateway/server/server.go`

**Interfaces:**
- Consumes: `Server.SetServeMode(bool)` called from main; `go:embed` directive
- Produces: `GET /openapi.json` returns OpenAPI 3.0 spec; `GET /docs` returns Swagger UI HTML

- [ ] **Step 1: Create OpenAPI 3.0 spec**

Create `db-gateway/server/openapi.json`:

```json
{
  "openapi": "3.0.3",
  "info": {
    "title": "yamlQ Gateway API",
    "description": "HTTP gateway for multi-database queries. Spawned by yamlQ CLI or run standalone via `db-gateway serve`.",
    "version": "0.1.0"
  },
  "servers": [
    { "url": "http://127.0.0.1:{port}", "variables": { "port": { "default": "0" } } }
  ],
  "paths": {
    "/connect": {
      "post": {
        "summary": "建立数据库连接",
        "operationId": "connect",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["driver", "dsn"],
                "properties": {
                  "driver": {"type": "string", "enum": ["mysql","postgres","oracle","opengauss","ob-mysql","ob-oracle","gd-mysql","gd-oracle","sqlite","clickhouse","sqlserver"]},
                  "dsn": {"type": "string"},
                  "max_open_conns": {"type": "integer", "default": 5},
                  "max_idle_conns": {"type": "integer", "default": 2},
                  "conn_max_lifetime": {"type": "integer", "default": 300}
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "连接成功/失败",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "conn_id": {"type": "string"},
                    "error": {"type": "string"}
                  }
                }
              }
            }
          }
        }
      }
    },
    "/query": {
      "post": {
        "summary": "执行 SQL 查询",
        "operationId": "query",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["conn_id", "sql"],
                "properties": {
                  "conn_id": {"type": "string"},
                  "sql": {"type": "string"},
                  "params": {"type": "array", "items": {"type": "string"}},
                  "page": {"type": "integer", "default": 0},
                  "page_size": {"type": "integer", "default": 0},
                  "timeout": {"type": "integer", "default": 30}
                }
              }
            }
          }
        },
        "responses": {
          "200": {
            "description": "查询结果",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "columns": {"type": "array", "items": {"type": "string"}},
                    "rows": {"type": "array", "items": {"type": "array"}},
                    "truncated": {"type": "boolean"},
                    "error": {"type": "object"}
                  }
                }
              }
            }
          },
          "503": {"description": "队列满"}
        }
      }
    },
    "/close": {
      "post": {
        "summary": "关闭数据库连接",
        "operationId": "close",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["conn_id"],
                "properties": {"conn_id": {"type": "string"}}
              }
            }
          }
        },
        "responses": {"200": {"description": "关闭结果"}}
      }
    },
    "/ping": {
      "get": {
        "summary": "连接健康检查",
        "operationId": "ping",
        "parameters": [
          {"name": "conn_id", "in": "query", "schema": {"type": "string"}}
        ],
        "responses": {"200": {"description": "连接状态"}}
      }
    },
    "/shutdown": {
      "post": {
        "summary": "关闭网关",
        "operationId": "shutdown",
        "responses": {"200": {"description": "网关即将退出"}}
      }
    }
  }
}
```

- [ ] **Step 2: Create Swagger UI loader page**

Create `db-gateway/server/swagger.html`:

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <title>yamlQ Gateway API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({
      url: "/openapi.json",
      dom_id: "#swagger-ui",
      authAction: {
        auth_token: {
          name: "auth_token",
          schema: {type: "apiKey", in: "header", name: "X-Auth-Token"},
          value: ""
        }
      },
      presets: [
        SwaggerUIBundle.presets.apis,
        SwaggerUIBundle.SwaggerUIStandalonePreset
      ],
      defaultModelExpandDepth: 3
    });
  </script>
</body>
</html>
```

- [ ] **Step 3: Embed files and add routes to server.go**

Add embed directives at the top of `server.go`:

```go
package server

import (
    "embed"
    "encoding/json"
    "errors"
    "net/http"
    "time"

    "yamlq/db-gateway/conn"
    "yamlq/db-gateway/errs"
)

//go:embed openapi.json swagger.html
var staticFiles embed.FS
```

Add `serveMode` field to `Server` struct and `SetServeMode` method:

```go
type Server struct {
    mgr          *conn.Manager
    authToken    string
    shutdownFunc func()
    serveMode    bool
}

func (s *Server) SetServeMode(v bool) {
    s.serveMode = v
}
```

Modify `Handler()` to add docs routes when in serve mode:

```go
func (s *Server) Handler() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("POST /connect", s.handleConnect)
    mux.HandleFunc("POST /query", s.handleQuery)
    mux.HandleFunc("POST /close", s.handleClose)
    mux.HandleFunc("GET /ping", s.handlePing)
    mux.HandleFunc("POST /shutdown", s.handleShutdown)
    if s.serveMode {
        mux.HandleFunc("GET /openapi.json", s.handleOpenAPI)
        mux.HandleFunc("GET /docs", s.handleDocs)
    }
    if s.authToken != "" {
        return s.authMiddleware(mux)
    }
    return mux
}
```

Add handlers:

```go
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
    data, _ := staticFiles.ReadFile("openapi.json")
    w.Header().Set("Content-Type", "application/json")
    w.Write(data)
}

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
    data, _ := staticFiles.ReadFile("swagger.html")
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.Write(data)
}
```

- [ ] **Step 4: Verify it compiles**

Run: `cd db-gateway && go build ./...`
Expected: no errors (embed works, no new deps)

- [ ] **Step 5: Manual test**

Run: `./db-gateway serve --port 9999` in background.
Verify: `curl http://127.0.0.1:9999/openapi.json` returns JSON spec.
Verify: `curl http://127.0.0.1:9999/docs` returns HTML.
Verify: Open `http://127.0.0.1:9999/docs` in browser — Swagger UI renders.

- [ ] **Step 6: Write unit tests**

Add to `db-gateway/server/server_test.go`:

```go
func TestServeModeServesOpenAPI(t *testing.T) {
    srv := New(nil)
    srv.SetServeMode(true)
    ts := httptest.NewServer(srv.Handler())
    defer ts.Close()

    resp, err := http.Get(ts.URL + "/openapi.json")
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("expected 200, got %d", resp.StatusCode)
    }
    var spec map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&spec)
    if spec["openapi"] != "3.0.3" {
        t.Fatalf("expected openapi 3.0.3, got %v", spec["openapi"])
    }
}

func TestServeModeServesDocs(t *testing.T) {
    srv := New(nil)
    srv.SetServeMode(true)
    ts := httptest.NewServer(srv.Handler())
    defer ts.Close()

    resp, err := http.Get(ts.URL + "/docs")
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("expected 200, got %d", resp.StatusCode)
    }
    body, _ := io.ReadAll(resp.Body)
    if !strings.Contains(string(body), "swagger-ui") {
        t.Fatal("expected swagger-ui in response")
    }
}

func TestNonServeModeNoDocs(t *testing.T) {
    srv := New(nil)
    ts := httptest.NewServer(srv.Handler())
    defer ts.Close()

    resp, _ := http.Get(ts.URL + "/docs")
    if resp.StatusCode != http.StatusMethodNotAllowed {
        t.Fatalf("expected 405 without serve mode, got %d", resp.StatusCode)
    }
}
```

Need to add imports: `"io"`, `"strings"`, `"encoding/json"`.

- [ ] **Step 7: Run tests**

Run: `cd db-gateway && go test ./server/ -run TestServe -v`
Expected: 3 tests pass (these tests don't need MySQL, they use `srv := New(nil)`)

- [ ] **Step 8: Commit**

```bash
git add db-gateway/server/
git commit -m "feat: add /openapi.json and /docs endpoints"
```

---

### Task 3: Update Python CLI to use `serve` subcommand

**Files:**
- Modify: `cli/yamlq/gateway.py`

- [ ] **Step 1: Read current `_spawn_gateway` method**

Read `cli/yamlq/gateway.py` around line 93.

- [ ] **Step 2: Replace `--daemon` with `serve` subcommand**

```python
def _spawn_gateway(self) -> None:
    binary = _find_binary()
    if os.environ.get("YAMLQ_GATEWAY_DAEMON") == "1":
        cmd = [binary, "serve"]
        if self._auth_token:
            cmd.append(f"--auth-token={self._auth_token}")
        self._owned = False
    else:
        cmd = [binary, f"--mode={self._mode}"]
        if self._auth_token:
            cmd.append(f"--auth-token={self._auth_token}")
    # ... rest of spawn logic unchanged
```

- [ ] **Step 3: Run tests**

Run: `cd cli && .venv/bin/python -m pytest tests/test_gateway.py -v`
Expected: All 17 pass (TestDaemonMode tests should still pass after updating the assertion for `serve` subcommand)

- [ ] **Step 4: Commit**

```bash
git add cli/yamlq/gateway.py
git commit -m "feat: use serve subcommand in daemon mode"
```

---

### Task 4: Register additional database drivers

**Files:**
- Modify: `db-gateway/driver/registry.go`
- Modify: `db-gateway/go.mod`
- Modify: `db-gateway/dialect/pagination.go`

- [ ] **Step 1: Add Go module dependencies**

Run from `db-gateway/`:
```bash
go get github.com/mattn/go-sqlite3
go get github.com/ClickHouse/clickhouse-go/v2
go get github.com/denisenkom/go-mssqldb
go mod tidy
```

- [ ] **Step 2: Register new drivers in `registry.go`**

Add blank imports and registry entries:

```go
import (
    "database/sql"
    "fmt"

    _ "github.com/go-sql-driver/mysql"
    _ "github.com/jackc/pgx/v5/stdlib"
    _ "github.com/sijms/go-ora/v2"
    _ "github.com/mattn/go-sqlite3"
    _ "github.com/ClickHouse/clickhouse-go/v2"
    _ "github.com/denisenkom/go-mssqldb"
)

var registry = map[string]OpenFunc{
    // ... existing entries ...
    "sqlite": func(dsn string) (*sql.DB, error) {
        return sql.Open("sqlite3", dsn)
    },
    "clickhouse": func(dsn string) (*sql.DB, error) {
        return sql.Open("clickhouse", dsn)
    },
    "sqlserver": func(dsn string) (*sql.DB, error) {
        return sql.Open("sqlserver", dsn)
    },
}
```

- [ ] **Step 3: Add pagination cases**

In `db-gateway/dialect/pagination.go`, add `"sqlite"` to the MySQL/LIMIT group:

```go
case "mysql", "opengauss", "postgres", "ob-mysql", "gd-mysql", "sqlite", "clickhouse":
    return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, pageSize, offset)
```

SQL Server uses `OFFSET ... FETCH NEXT`:
```go
case "oracle", "ob-oracle", "gd-oracle", "sqlserver":
    return fmt.Sprintf("%s OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", sql, offset, pageSize)
```

- [ ] **Step 4: Update driver tests**

Add new drivers to `TestSupported` and `TestOpenReturnsDB` in `db-gateway/driver/registry_test.go`.

- [ ] **Step 5: Verify build and tests**

Run: `cd db-gateway && go build ./... && go test ./driver/ ./dialect/ -v`
Expected: build passes, all tests pass with new driver entries

- [ ] **Step 6: Commit**

```bash
git add db-gateway/driver/ db-gateway/dialect/ db-gateway/go.mod db-gateway/go.sum
git commit -m "feat: add sqlite, clickhouse, sqlserver drivers"
```

---

### Task 5: Update bash script and documentation

**Files:**
- Modify: `scripts/yamlq-gateway.sh`
- Modify: `docs/ARCHITECTURE.md`

- [ ] **Step 1: Update bash script to use `serve` subcommand**

In `scripts/yamlq-gateway.sh`, change the start command from:
```bash
args=("--mode" "service" "--daemon")
```
to:
```bash
args=("serve")
```

- [ ] **Step 2: Update architecture docs**

In `docs/ARCHITECTURE.md`:

1. Update driver table to include new drivers:
```markdown
| `sqlite` | `mattn/go-sqlite3` | 本地文件数据库 |
| `clickhouse` | `clickhouse-go/v2` | 列式分析数据库 |
| `sqlserver` | `denisenkom/go-mssqldb` | Microsoft SQL Server |
```

2. Update the daemon mode section to mention `serve`:
```
| 独立启动 | `./db-gateway serve --port 9999 --auth-token x` | serve 子命令，自带 /docs 调测页面 |
```

- [ ] **Step 3: Commit**

```bash
git add scripts/yamlq-gateway.sh docs/ARCHITECTURE.md
git commit -m "feat: update scripts/docs for serve subcommand and new drivers"
```

---

### How to test

**Unit tests:**
```bash
cd db-gateway && go test ./... -short -timeout 30s
```
Expected: driver tests (12+ drivers), dialect tests (9+ cases), server tests (3 new serve mode tests). Server tests that need real MySQL will be skipped/expected-fail.

**Manual E2E:**
```bash
# 1. Start serve mode
./db-gateway serve --port 9999 --auth-token test123

# 2. Open browser
open http://127.0.0.1:9999/docs
# → Swagger UI renders, can test all endpoints

# 3. Test via curl
curl -H "X-Auth-Token: test123" http://127.0.0.1:9999/ping

# 4. Python CLI daemon mode uses serve subcommand
YAMLQ_GATEWAY_DAEMON=1 yamlq testdata/e2e_basic.yaml
# → spawns `db-gateway serve --auth-token ...` instead of `--mode cli --daemon`

# 5. Script
scripts/yamlq-gateway.sh start --port 9999 --auth-token x
scripts/yamlq-gateway.sh status
scripts/yamlq-gateway.sh stop
```
