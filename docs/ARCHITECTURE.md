# 架构设计

## 1. 系统架构

```
┌──────────┐  YAML  ┌──────────┐  HTTP   ┌──────────┐
│  Python   │ ──────→ │  Go      │ ──────→ │  MySQL   │
│  CLI/TUI  │ ←────── │ Gateway  │ ←────── │  PG      │
│  Rich +   │  JSON   │ 127.0.0.1│         │  Oracle  │
│  Textual  │         │ :随机端口 │         │  ...     │
└──────────┘         └──────────┘         └──────────┘
```

- Python 启动时 `subprocess.Popen` 拉起 Go 二进制
- Go 监听 `127.0.0.1:0`，端口号写入 stdout，Python 读取
- 可选 `--auth-token`，Python 在 `X-Auth-Token` Header 中携带
- Python 退出时 `POST /shutdown`，异常崩溃依赖 Go 侧 30min 空闲回收

## 2. 数据库驱动

| db_type | Go 驱动 | 备注 |
|---|---|---|
| `mysql` | `go-sql-driver/mysql` | |
| `ob-mysql` | `go-sql-driver/mysql` | OceanBase MySQL 租户，MySQL 协议 |
| `ob-oracle` | `go-sql-driver/mysql` | OceanBase Oracle 租户，MySQL 协议，Oracle SQL 方言 |
| `gd-mysql` | `go-sql-driver/mysql` | GoldenDB MySQL 模式，MySQL 协议 |
| `gd-oracle` | `go-sql-driver/mysql` | GoldenDB Oracle 模式，MySQL 协议，Oracle SQL 方言 |
| `postgres` | `jackc/pgx/v5/stdlib` | |
| `oracle` | `sijms/go-ora/v2` | 纯 Go，免 Instant Client |
| `opengauss` | `jackc/pgx/v5/stdlib` | PG 协议兼容 |
| `sqlite` | `mattn/go-sqlite3` | 本地文件数据库 |
| `clickhouse` | `clickhouse-go/v2` | 列式分析数据库 |
| `sqlserver` | `denisenkom/go-mssqldb` | Microsoft SQL Server |

> **OB Oracle DSN 说明：**
> - 非集群 OBProxy / 直连: `user@tenant:password@tcp(host:port)/db`
> - 集群 OBProxy: `user@tenant#cluster:password@tcp(host:port)/db`
> - `go-sql-driver/mysql` 按最后一个 `@` 分割，用户名内的 `@` 和 `#` 均属合法字符

扩展无原生 Go 驱动的库（达梦、Kingbase）：参考 GoNavi driver-agent 模式，通过子进程桥接，主进程 HTTP 接口不变。

## 3. HTTP API 合约

**Base URL**: `http://127.0.0.1:{port}`

### POST /connect

```json
// Request
{ "driver": "mysql", "dsn": "root:pass@tcp(127.0.0.1:3306)/test",
  "max_open_conns": 5, "max_idle_conns": 2, "conn_max_lifetime": 300 }

// 200 Success
{ "conn_id": "conn_1743123456_1", "error": "" }

// 200 Fail
{ "conn_id": "", "error": "global connection limit reached" }
```

### POST /query

```json
// Request
{ "conn_id": "conn_xxx", "sql": "SELECT * FROM users WHERE id > ?",
  "params": [100], "page": 1, "page_size": 20, "timeout": 30 }

// 200 Success
{ "columns": ["id", "name"], "rows": [[1, "Alice"]],
  "truncated": false, "error": null }

// 200 Timeout (partial)
{ "columns": ["id"], "rows": [[1]], "truncated": true,
  "error": { "code": "QUERY_TIMEOUT", "message": "...", "hint": "..." } }

// 503 Queue Full
{ "status": "rejected",
  "error": { "code": "QUEUE_FULL", "message": "...", "hint": "..." } }
```

### POST /close

```json
{ "conn_id": "conn_xxx" }
→ { "error": "" }
```

### GET /ping

```
GET /ping?conn_id=conn_xxx
→ { "status": "ok" }  或  { "status": "lost", "error": "..." }
```

### POST /shutdown

```json
{} → { "error": "" }   // Go 关闭所有连接后退出
```

## 4. Go 内部结构

```
db-gateway/
├── main.go              # 入口: :0 端口, --mode, --auth-token, 信号处理
├── server/
│   ├── server.go        # HTTP handler 注册, auth middleware
│   └── server_test.go   # 集成测试 (真实 MySQL)
├── conn/
│   ├── manager.go       # ConnManager: 连接生命周期, FIFO 队列, 信号量
│   └── query.go         # executeOne: 超时, 截断, []byte→string
├── driver/
│   └── registry.go      # map[string]OpenFunc 驱动注册表
├── dialect/
│   ├── pagination.go    # 方言分页: LIMIT OFFSET / OFFSET FETCH
│   └── pagination_test.go
├── config/
│   └── pool.go          # CLI / Service 水位线预设
└── errs/
    └── errors.go        # 四级错误分类 + QueueFullError
```

### 核心类型

```go
type ManagedConn struct {
    DB         *sql.DB
    Driver     string
    QueryChan  chan *QueryTask   // FIFO 队列
    done       chan struct{}     // 关闭信号
    cancel     context.CancelFunc
}

type QueryTask struct {
    SQL        string
    Params     []interface{}
    Page, PageSize int
    Timeout    time.Duration
    ResultChan chan *QueryResult
}
```

### 队列调度

```
同 conn_id 串行:    [SQL-A] → [SQL-B] → [SQL-C]  (FIFO channel)
不同 conn_id 并发:   conn_1 ── goroutine ──┐
                    conn_2 ── goroutine ──┼── querySem 全局限制
                    conn_3 ── goroutine ──┘
```

### 水位线

| 参数 | CLI | Service |
|---|---|---|
| GlobalMaxOpen | 30 | 1000 |
| PerConnMaxOpen | 5 | 50 |
| MaxConcurrentQuery | 10 | 200 |
| QueueDepth | 20 | 100 |
| MaxRowsPerQuery | 10000 | 100000 |

### 错误分类

| Code | 触发 | 行为 |
|---|---|---|
| `QUEUE_FULL` | channel 满 | 503 立即拒绝 |
| `QUERY_TIMEOUT` | context 超时 | 返回已获取部分行 |
| `CONNECTION_LOST` | driver.ErrBadConn | 提示重连 |
| `DB_ERROR` | SQL 语法等 | 透传驱动错误 |

## 5. Python 内部结构

```
cli/yamlq/
├── __main__.py          # 入口
├── cli.py               # argparse, 视图调度, --tui 分发
├── parser.py            # YAML Map 解析, {{param}}→?, check 校验
├── gateway.py           # Go 进程守护, HTTP client, atexit
├── renderer.py          # Rich 表格 + converter + 截断提示
├── converters.py        # 内置 converter 注册表
└── tui/
    └── app.py           # Textual: Tab, 翻页, 异步加载, 错误展示
```

### 查询调度流程

```
1. load_views(-c) → 解析 YAML, 过滤 enable + -v
2. 按 (db_type, dsn) 去重分组
3. ThreadPoolExecutor 并行:
   每组 → /connect → 串行投递 SQL → 收集结果 → /close
4. 按 YAML 声明顺序渲染结果
```

### 双层超时

| 层级 | 默认 | 控制方 | 超时行为 |
|---|---|---|---|
| 单查询 | 30s | Go context.WithTimeout | 返回部分行 + QUERY_TIMEOUT |
| 会话 | 120s | Python ThreadPoolExecutor | 取消剩余, 全部 /close |

## 6. Gateway 常驻模式

### 环境变量 / 文件发现

| 机制 | 路径 | 说明 |
|---|---|---|
| `YAMLQ_GATEWAY_URL` | 环境变量 | 直接指定已运行 gateway 地址 |
| `.yamlq-gateway.env` | 当前目录 | gateway 启动时自动写入 |

`.yamlq-gateway.env` 文件内容:
```
YAMLQ_GATEWAY_URL=http://127.0.0.1:54321
YAMLQ_GATEWAY_AUTH=my-token
YAMLQ_GATEWAY_DAEMON=1
```

### 启动模式

| 模式 | 命令 | 行为 |
|---|---|---|
| 默认（一次性） | `yamlq config.yaml` | Python 拉起 gateway，退出时关停 |
| 常驻（daemon） | `YAMLQ_GATEWAY_DAEMON=1 yamlq config.yaml` | Python 拉起 gateway 并常驻，退出不影响 gateway |
| 独立启动 | `./db-gateway serve --port 9999 --auth-token x` | serve 子命令，自带 /docs 调测页面 |
| 直接复用 | `YAMLQ_GATEWAY_URL=http://127.0.0.1:54321 yamlq config.yaml` | 跳过发现，直连指定 gateway |

## 7. 术语表

| 术语 | 定义 |
|---|---|
| View | 一个业务视图 = 一个数据源 + 一条 SQL，对应一个 Tab |
| view_name | Tab 标签名，缺省用 YAML 顶级 key |
| conn_id | Go 侧为每个数据库连接分配的唯一标识 |
| FIFO Channel | 每连接的带缓冲 channel，保证同连接串行 |
| 水位线 | GlobalMaxOpen / PerConnMaxOpen / MaxConcurrentQuery / QueueDepth |
| Converter | Python 侧字段转换函数（datetime_to_iso, status_to_cn 等） |
| 双层超时 | 单查询超时 + 会话超时 |

## 8. 关键决策

| 决策 | 理由 |
|---|---|
| Go 标准库 net/http | 零外部依赖，部署简单 |
| 同 dsn 复用连接 | 减少连接数，避免同库锁冲突 |
| FIFO 串行 | 避免同连接事务/隔离问题 |
| 同步查询（方案A） | 终端场景 90% 是等结果→看表格→退出 |
| Python 侧 Converter | 展示逻辑与数据层解耦 |
| 部分失败不阻塞 | 多库场景一个挂不影响其他 |

详细 ADR 见 [adr/](adr/)。
