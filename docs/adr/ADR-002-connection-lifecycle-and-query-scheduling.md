# ADR-002: 连接生命周期与查询调度

**状态**: 已采纳 · 2026-07-27

## 背景

Python 侧按 YAML view 执行查询时，如何处理多个 view 共享同一数据库的连接复用？Go 侧如何处理同连接多 SQL 的并发问题？

## 决策

### 连接生命周期

```
Python 侧流程:
1. 收集所有 enabled view, 按 (db_type, dsn) 去重分组
2. 每组调用 /connect 建立一条连接
3. 将同一组内的所有 SQL 按序投递到该 conn_id 的队列
4. 等待所有结果返回 → 调用 /close
5. 超时 → 调用 /close + 记录日志
```

### Go 侧队列调度

```
同 conn_id 串行:    [SQL-A] → [SQL-B] → [SQL-C]  (FIFO channel)
不同 conn_id 并发:   conn_1 ── goroutine_A ──┐
                    conn_2 ── goroutine_B ──┼── 全局 MaxConcurrentQuery 控制
                    conn_3 ── goroutine_C ──┘
```

### 连接复用策略

| 场景 | 行为 |
|---|---|
| 多个 view 使用相同 dsn | Python 去重，只建一条连接，串行执行所有 SQL |
| 一个会话中多次出现同一 dsn | 复用已有 conn_id（缓存映射到 Python 侧） |
| 空闲回收 | Go 侧 30 分钟无活跃自动关闭 |

### 队列满拒绝

当某 conn_id 的 channel 已满时，/query 立即返回 503：
```json
{ "status": "rejected", "error": { "code": "QUEUE_FULL", "message": "..." } }
```

## 后果

- 避免同连接事务/锁冲突
- 减少不必要的连接数（复用）
- 全局信号量防止资源耗尽
