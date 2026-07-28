# ADR-001: YAML 视图与多数据源模型

**状态**: 已采纳 · 2026-07-27

## 背景

终端多数据库查询工具需要一种配置格式来描述业务视图。用户按业务域组织查询，一个业务视图可能涉及多个异构数据库（MySQL + Oracle + PG 等），需要在一个 YAML 文件中声明。

## 决策

### YAML 顶层结构

采用 **Map 结构**，顶级 key 为视图标识符，value 为视图配置：

```yaml
daily_active_users:
  view_name: "查询每日活跃用户数"
  description: "查询每日活跃用户数"
  db_type: "mysql"
  dsn: "user:pwd@tcp(host:3306)/db"
  sql: |
    SELECT date, COUNT(user_id) as dau
    FROM user_stats
    WHERE date >= {{start_date}} AND date <= {{end_date}}
    GROUP BY date ORDER BY date DESC
  params:
    - name: start_date
      prompt: "开始日期"
      default: "2023-10-01"
    - name: end_date
      prompt: "结束日期"
  view:
    enable: true
    enable_paging: true
    page_size: 20
    columns:
      - field: date
        header: "日期"
        align: "center"
        style: dim
        overflow: fold
        converter: date_to_iso
      - field: dau
        header: "日活"
weekly_report:
  view_name: "周报统计"
  description: "每周数据汇总"
  db_type: "oracle"
  dsn: "user:pwd@host:1521/service"
  sql: |
    SELECT week, SUM(amount) FROM reports GROUP BY week
  params: []
  view:
    enable: true
    enable_paging: false
    columns:
      - field: week
        header: "周次"
      - field: amount
        header: "金额"
```

### 关键规则

| 规则 | 说明 |
|---|---|
| 一个 view = 一个数据源 + 一条 SQL | 视图和数据源是 1:1 关系 |
| 一个视图一个 tab | TUI 中 tab 与 view 一一对应 |
| `view_name` 为 tab 标签 | 优先使用，不存在则用顶级 key |
| `enable: false` 跳过 | 不查询、不创建 tab |
| 多文件支持 | 用户按业务域拆分多个 YAML，可传目录加载 |

### 文件加载

```
-c 指向文件 → 加载单个 YAML
-c 指向目录 → 递归加载所有 *.yaml / *.yml 合并
```

### 参数与默认值

- params 中 `default` 为静默默认值
- 无 default 的参数需用户传入（CLI `--param` 或 TUI 弹窗）
- `yaml-view check -c config.yaml` 校验哪些 view 缺少 default

## 后果

- 简化 Python 侧处理：不需要跨数据源 join，每个 view 独立查询展示
- 多数据源业务通过定义多个 view 实现，视图之间完全解耦
- 文件可拆分可合并，灵活度好
