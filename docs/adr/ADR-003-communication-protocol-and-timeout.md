# ADR-003: Python-Go 通信协议与超时模型

**状态**: 已采纳 · 2026-07-27

## 背景

Python 和 Go 之间通过 HTTP 通信，需要明确端口分配、超时模型、错误分类。

## 决策

### 端口分配

Go 监听 `:0`，系统分配随机端口，将端口号写入 stdout。Python 读取 stdout 获取端口。

```
Go 启动: listening on port 54781  (stdout)
Python:  subprocess.Popen → 读取 stdout → 提取端口号
Python 退出: POST /shutdown
Python 崩溃: Go 侧 30 分钟空闲回收
```

### 双层超时模型

```
┌─────────────────────────────────────────────┐
│  会话超时 (Session Timeout)  默认 120s       │
│  ┌───────────────────────────────────────┐   │
│  │ 单查询超时 (Query Timeout)  默认 30s  │   │
│  └───────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
```

| 层级 | 默认值 | 控制方 | 超时行为 |
|---|---|---|---|
| 单查询超时 | 30s | Go 侧 context.WithTimeout | 返回已获取部分行 + QUERY_TIMEOUT |
| 会话超时 | 120s | Python 侧 asyncio/timeout | 未完成查询标记失败，全部 /close |

### 错误分类

| Code | HTTP Status | 触发场景 |
|---|---|---|
| QUEUE_FULL | 503 | 连接队列满 |
| QUERY_TIMEOUT | 200 (partial) | context 超时，返回部分行 |
| CONNECTION_LOST | 200 | driver.ErrBadConn |
| DB_ERROR | 200 | SQL 语法错误等 |

### 部分失败策略

多 view 并行查询时，各自独立。一个失败不影响其他 view。

```json
// TUI 展示: 成功的 tab 正常显示表格
//           失败的 tab 显示错误标记，hover/点击展开错误详情
```

### API 端点

| 端点 | 方向 | 说明 |
|---|---|---|
| `POST /connect` | Python → Go | 建立连接 |
| `POST /query` | Python → Go | 同步查询，阻塞等待 |
| `POST /close` | Python → Go | 关闭连接 |
| `GET /ping` | Python → Go | 探活 |
| `POST /shutdown` | Python → Go | 优雅退出 |

## 后果

- 单查询超时不丢已获取数据（返回部分行）
- 会话超时防止 Python 侧卡死
- 错误分类清晰，Python 侧可针对不同错误码做不同的 UI 提示
