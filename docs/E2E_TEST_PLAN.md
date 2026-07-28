# E2E 测试用例设计 (M2–M4)

## 测试环境

| 数据库 | DSN | 来源 |
|---|---|---|
| MySQL | `root:root123456@tcp(127.0.0.1:3306)/default_db` | docker-compose |
| PostgreSQL | `postgres:postgres123@127.0.0.1:5432/postgres_db` | docker-compose |
| OpenGauss | `gaussdb:OpenGauss@123@127.0.0.1:5433/postgres` | docker-compose |
| Oracle | `appuser:App123!@127.0.0.1:1521/XEPDB1` | docker-compose |

### 测试数据准备

每个数据库在测试前执行 seed SQL：

```sql
-- MySQL / PG / OpenGauss
CREATE TABLE IF NOT EXISTS e2e_users (
  id INT PRIMARY KEY AUTO_INCREMENT,  -- PG/OpenGauss: SERIAL
  name VARCHAR(50),
  email VARCHAR(100),
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO e2e_users (name, email) VALUES
  ('Alice', 'alice@test.com'),
  ('Bob', 'bob@test.com'),
  ('Charlie', 'charlie@test.com');

-- Oracle
CREATE TABLE e2e_users (
  id NUMBER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name VARCHAR2(50),
  email VARCHAR2(100),
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
-- 同上 INSERT
```

```sql
-- Converter 测试表 (MySQL)
CREATE TABLE IF NOT EXISTS e2e_orders (
  id INT PRIMARY KEY AUTO_INCREMENT,
  order_no VARCHAR(32),
  status VARCHAR(20),
  amount DECIMAL(10,2),
  created_at DATETIME,
  updated_at DATETIME
);
INSERT INTO e2e_orders (order_no, status, amount, created_at, updated_at) VALUES
  ('ORD-001', 'pending',   99.50,  '2024-03-15 08:30:00', '2024-03-15 08:30:00'),
  ('ORD-002', 'paid',      250.00, '2024-06-20 14:00:00', '2024-06-21 09:15:00'),
  ('ORD-003', 'shipped',   1280.00,'2024-11-01 00:00:00', '2024-11-03 16:45:00'),
  ('ORD-004', 'completed', 45.90,  '2025-01-10 12:00:00', '2025-01-12 18:30:00'),
  ('ORD-005', 'cancelled', 0.00,   '2025-07-04 23:59:59', '2025-07-05 00:00:01');
```

### 测试 YAML 文件

```yaml
# testdata/e2e_basic.yaml
user_list:
  view_name: "用户列表"
  db_type: "mysql"
  dsn: "root:root123456@tcp(127.0.0.1:3306)/default_db"
  sql: "SELECT id, name, email FROM e2e_users ORDER BY id"
  params: []
  view:
    enable: true
    enable_paging: true
    page_size: 2
    columns:
      - field: id
        header: "ID"
        align: "center"
      - field: name
        header: "姓名"
      - field: email
        header: "邮箱"
        style: dim

user_search:
  view_name: "用户搜索"
  db_type: "mysql"
  dsn: "root:root123456@tcp(127.0.0.1:3306)/default_db"
  sql: "SELECT id, name FROM e2e_users WHERE name LIKE CONCAT('%', {{keyword}}, '%')"
  params:
    - name: keyword
      prompt: "搜索关键词"
      default: "Ali"
  view:
    enable: true
    columns:
      - field: id
        header: "ID"
      - field: name
        header: "姓名"

disabled_view:
  view_name: "禁用视图"
  db_type: "mysql"
  dsn: "root:root123456@tcp(127.0.0.1:3306)/default_db"
  sql: "SELECT 1"
  params: []
  view:
    enable: false
```

```yaml
# testdata/e2e_multi_db.yaml
mysql_users:
  view_name: "MySQL用户"
  db_type: "mysql"
  dsn: "root:root123456@tcp(127.0.0.1:3306)/default_db"
  sql: "SELECT id, name FROM e2e_users ORDER BY id"
  params: []
  view:
    enable: true
    columns:
      - field: id
        header: "ID"
      - field: name
        header: "姓名"

pg_users:
  view_name: "PG用户"
  db_type: "postgres"
  dsn: "postgres://postgres:postgres123@127.0.0.1:5432/postgres_db?sslmode=disable"
  sql: "SELECT id, name FROM e2e_users ORDER BY id"
  params: []
  view:
    enable: true
    columns:
      - field: id
        header: "ID"
      - field: name
        header: "姓名"

oracle_users:
  view_name: "Oracle用户"
  db_type: "oracle"
  dsn: "oracle://appuser:App123!@127.0.0.1:1521/XEPDB1"
  sql: "SELECT id, name FROM e2e_users ORDER BY id"
  params: []
  view:
    enable: true
    columns:
      - field: id
        header: "ID"
      - field: name
        header: "姓名"
```

```yaml
# testdata/e2e_params.yaml
param_view:
  view_name: "参数测试"
  db_type: "mysql"
  dsn: "root:root123456@tcp(127.0.0.1:3306)/default_db"
  sql: "SELECT id, name FROM e2e_users WHERE id >= {{min_id}} AND name LIKE CONCAT('%', {{keyword}}, '%')"
  params:
    - name: min_id
      prompt: "最小ID"
    - name: keyword
      prompt: "关键词"
  view:
    enable: true
    columns:
      - field: id
        header: "ID"
      - field: name
        header: "姓名"
```

```yaml
# testdata/e2e_converter.yaml
order_list:
  view_name: "订单列表"
  db_type: "mysql"
  dsn: "root:root123456@tcp(127.0.0.1:3306)/default_db"
  sql: "SELECT order_no, status, amount, created_at, updated_at FROM e2e_orders ORDER BY id"
  params: []
  view:
    enable: true
    columns:
      - field: order_no
        header: "订单号"
      - field: status
        header: "状态"
        converter: status_to_cn
      - field: amount
        header: "金额"
        converter: money_format
      - field: created_at
        header: "创建时间"
        converter: datetime_to_iso
      - field: updated_at
        header: "更新时间"
        converter: datetime_to_cn

order_summary:
  view_name: "订单统计"
  db_type: "mysql"
  dsn: "root:root123456@tcp(127.0.0.1:3306)/default_db"
  sql: "SELECT status, COUNT(*) AS cnt, SUM(amount) AS total FROM e2e_orders GROUP BY status"
  params: []
  view:
    enable: true
    columns:
      - field: status
        header: "状态"
        converter: status_to_cn
      - field: cnt
        header: "数量"
      - field: total
        header: "总金额"
        converter: money_format
```

### Converter 函数注册表（Python 侧）

```python
CONVERTERS = {
    "datetime_to_iso": lambda v: v.strftime("%Y-%m-%d") if v else "",
    "datetime_to_cn":  lambda v: v.strftime("%Y年%m月%d日 %H:%M") if v else "",
    "status_to_cn":    lambda v: {"pending": "待支付", "paid": "已支付",
                                  "shipped": "已发货", "completed": "已完成",
                                  "cancelled": "已取消"}.get(v, v),
    "money_format":    lambda v: f"¥{v:,.2f}" if v is not None else "",
}
```

---

## M2: Python 通信与 YAML 解析

### E2E-M2-01: 单视图基本查询

**前置**: Go 二进制已编译，MySQL 已启动，e2e_users 表已建

**步骤**:
1. `yaml-view -c testdata/e2e_basic.yaml -v user_list`

**预期**:
- Python 自动拉起 Go 子进程
- stdout 输出表格，包含 3 行数据 (Alice, Bob, Charlie)
- 列标题为 ID / 姓名 / 邮箱
- 进程正常退出，Go 子进程被 /shutdown 关闭

---

### E2E-M2-02: 多视图并行查询 + 连接复用

**前置**: 同上

**步骤**:
1. `yaml-view -c testdata/e2e_basic.yaml`（不指定 -v，加载所有 enabled view）

**预期**:
- `user_list` 和 `user_search` 两个 view 并行查询
- 两者共用同一个 MySQL 连接（同 dsn 去重）
- `disabled_view` 不出现在输出中
- 两个 view 的结果各自独立展示

---

### E2E-M2-03: 参数默认值

**前置**: 同上

**步骤**:
1. `yaml-view -c testdata/e2e_basic.yaml -v user_search`

**预期**:
- `keyword` 参数使用 default 值 `Ali`
- 不弹出交互提示
- 结果只包含 Alice

---

### E2E-M2-04: --param 覆盖默认值

**步骤**:
1. `yaml-view -c testdata/e2e_basic.yaml -v user_search --param keyword=Bob`

**预期**:
- 结果只包含 Bob
- default 值被覆盖

---

### E2E-M2-05: 缺少参数时交互提示

**步骤**:
1. `yaml-view -c testdata/e2e_params.yaml`（无 --param）

**预期**:
- 终端提示输入 `最小ID` 和 `关键词`
- 输入 `1` 和 `Ali` 后返回 Alice

---

### E2E-M2-06: -v 过滤视图

**步骤**:
1. `yaml-view -c testdata/e2e_basic.yaml -v user_list,user_search`

**预期**:
- 只查询并展示 user_list 和 user_search
- disabled_view 即使 enable:true 也不出现（被 -v 过滤）

---

### E2E-M2-07: 目录加载

**前置**: `testdata/views/` 目录下放 2 个 yaml 文件

**步骤**:
1. `yaml-view -c testdata/views/`

**预期**:
- 递归加载目录下所有 .yaml 文件
- 所有 enabled view 合并展示

---

### E2E-M2-08: check 子命令

**步骤**:
1. `yaml-view check -c testdata/e2e_params.yaml`

**预期**:
- 输出提示：`param_view` 的 `min_id` 和 `keyword` 缺少 default
- 不执行任何查询
- 退出码 0（仅提示，不报错）

---

### E2E-M2-09: Go 进程生命周期

**步骤**:
1. 正常执行 `yaml-view -c testdata/e2e_basic.yaml -v user_list`
2. 验证 Go 子进程已退出（`ps aux | grep db-gateway` 无结果）

**预期**:
- 正常退出时 Python 调用 /shutdown，Go 进程消失
- 异常退出（kill -9 Python）时，Go 进程在 30min 空闲后自动回收

---

### E2E-M2-10: SQL 模板渲染

**步骤**:
1. `yaml-view -c testdata/e2e_basic.yaml -v user_search --param keyword=Charlie`

**预期**:
- `{{keyword}}` 被替换为 `?` 占位符
- 参数 `Charlie` 通过 params 数组传递给 Go
- 结果只包含 Charlie（验证参数化查询，非字符串拼接）

---

### E2E-M2-11: 查询失败不阻塞其他视图

**前置**: e2e_multi_db.yaml 中 Oracle 未启动或 DSN 错误

**步骤**:
1. `yaml-view -c testdata/e2e_multi_db.yaml`

**预期**:
- mysql_users 和 pg_users 正常展示
- oracle_users 展示错误信息（DB_ERROR 或 CONNECTION_LOST）
- 不因 Oracle 失败而退出

---

### E2E-M2-12: 会话超时（Python 侧 120s 兜底）

**前置**: 构造一个 YAML，SQL 为 `SELECT SLEEP(200)`，timeout 设为 200

**步骤**:
1. `yaml-view -c testdata/e2e_timeout.yaml`
2. 等待 120s

**预期**:
- Python 侧会话超时触发（默认 120s）
- 所有未完成查询标记失败
- 调用 /close 关闭所有连接
- 终端提示"会话超时"，进程正常退出（非挂死）

---

### E2E-M2-13: 查询超时部分行展示

**前置**: 构造大表（10000 行），SQL 无 WHERE 条件，timeout=2

**步骤**:
1. `yaml-view -c testdata/e2e_partial.yaml -v big_query`

**预期**:
- Go 返回 `QUERY_TIMEOUT` + `partial_rows`（已扫描的行）
- Python 展示已获取的部分行，标注"超时截断，已获取 N 行"
- 不显示空白或报错退出

---

### E2E-M2-14: YAML 格式错误与必填字段缺失

**步骤**:
1. `yaml-view -c testdata/e2e_invalid.yaml`（缺少 dsn 字段）
2. `yaml-view -c testdata/e2e_broken.yaml`（YAML 语法错误）

**预期**:
- 缺 dsn：提示 `view "xxx" missing required field: dsn`，不启动 Go
- YAML 语法错误：提示解析失败 + 行号，退出码 1
- 不拉起 Go 子进程

---

### E2E-M2-15: 不支持的 db_type

**前置**: YAML 中 `db_type: dameng`

**步骤**:
1. `yaml-view -c testdata/e2e_bad_driver.yaml -v dm_view`

**预期**:
- 提示 `unsupported driver: dameng`
- 该 view 标记失败，其他 view 不受影响

---

### E2E-M2-16: 空结果集

**前置**: SQL 条件不匹配任何行（`WHERE 1=0`）

**步骤**:
1. `yaml-view -c testdata/e2e_empty.yaml -v empty_view`

**预期**:
- 表格只显示列标题，无数据行
- 提示"0 行"或"无数据"
- 不报错

---

### E2E-M2-17: -v 指定不存在的 view

**步骤**:
1. `yaml-view -c testdata/e2e_basic.yaml -v nonexistent_view`

**预期**:
- 提示 `view "nonexistent_view" not found in config`
- 列出可用的 view key
- 退出码 1

---

### E2E-M2-18: 数据库不可达

**前置**: MySQL 容器停止

**步骤**:
1. `yaml-view -c testdata/e2e_basic.yaml -v user_list`

**预期**:
- /connect 失败，提示连接被拒绝（含 host:port）
- 不挂死，不重试无限循环
- 退出码 1

---

### E2E-M2-19: 目录加载 view key 冲突

**前置**: `testdata/conflict/` 下两个文件都定义了 `user_list` key

**步骤**:
1. `yaml-view -c testdata/conflict/`

**预期**:
- 后加载的文件覆盖先加载的（或报错提示冲突）
- 不出现重复 tab
- stderr 输出警告：`duplicate view key "user_list" in xxx.yaml, overridden by yyy.yaml`

---

## M3: TUI 渲染与交互

### E2E-M3-01: Rich 表格渲染

**步骤**:
1. `yaml-view -c testdata/e2e_basic.yaml -v user_list`（TUI 模式）

**预期**:
- 表格有边框线、列标题加粗
- `email` 列显示为 dim 样式
- `ID` 列居中对齐

---

### E2E-M3-02: Tab 切换

**前置**: e2e_basic.yaml 有 2 个 enabled view

**步骤**:
1. 启动 TUI
2. 按 `Tab` 或 `Ctrl+2` 切换到第二个 tab

**预期**:
- 默认展示第一个 tab（user_list）
- 切换后展示 user_search 的结果
- Tab 标签显示 view_name（"用户列表" / "用户搜索"）

---

### E2E-M3-03: 分页翻页

**前置**: user_list 的 page_size=2，共 3 条数据

**步骤**:
1. 启动 TUI，查看 user_list
2. 按 `n`（下一页）
3. 按 `p`（上一页）

**预期**:
- 第 1 页显示 Alice, Bob（2 行）
- 第 2 页显示 Charlie（1 行）
- 回到第 1 页显示 Alice, Bob
- 页码指示器显示 `1/2` → `2/2` → `1/2`

---

### E2E-M3-04: 异步加载状态

**前置**: 查询耗时较长（可用 SLEEP 模拟）

**步骤**:
1. 启动 TUI，立即切换到某个 tab

**预期**:
- 数据未到达时显示 loading 指示（如 spinner 或 "加载中..."）
- 数据到达后自动刷新为表格

---

### E2E-M3-05: 错误 Tab 展示

**前置**: 某 view 的 SQL 有语法错误

**步骤**:
1. 启动 TUI

**预期**:
- 错误 view 的 tab 显示错误标记（如红色 ✗）
- 切换到该 tab 显示错误详情（error code + message + hint）
- 其他 tab 不受影响

---

### E2E-M3-06: Converter — 日期时间转 ISO 格式

**前置**: e2e_orders 表已建，e2e_converter.yaml 就绪

**步骤**:
1. `yaml-view -c testdata/e2e_converter.yaml -v order_list`

**预期**:
- `created_at` 列原始值 `2024-03-15 08:30:00` 显示为 `2024-03-15`
- `updated_at` 列原始值 `2024-06-21 09:15:00` 显示为 `2024年06月21日 09:15`
- Go 返回原始 datetime 值，转换在 Python 侧完成
- 同一 view 中两列使用不同 converter 互不干扰

---

### E2E-M3-06b: Converter — 枚举值英转中

**步骤**:
1. `yaml-view -c testdata/e2e_converter.yaml -v order_list`

**预期**:

| Go 返回原始值 | 终端显示 |
|---|---|
| `pending` | `待支付` |
| `paid` | `已支付` |
| `shipped` | `已发货` |
| `completed` | `已完成` |
| `cancelled` | `已取消` |

- 未知枚举值（如 `refunded`）原样透传，不报错

---

### E2E-M3-06c: Converter — 金额格式化

**步骤**:
1. `yaml-view -c testdata/e2e_converter.yaml -v order_list`

**预期**:

| Go 返回原始值 | 终端显示 |
|---|---|
| `99.50` | `¥99.50` |
| `1280.00` | `¥1,280.00` |
| `0.00` | `¥0.00` |

- 千分位分隔符正确
- NULL 值显示为空字符串，不抛异常

---

### E2E-M3-06d: Converter — 聚合结果同样生效

**步骤**:
1. `yaml-view -c testdata/e2e_converter.yaml -v order_summary`

**预期**:
- `status` 列枚举转换生效（`待支付` / `已支付` 等）
- `total` 列金额格式化生效（`¥1,675.40`）
- converter 对 GROUP BY 聚合结果同样适用

---

### E2E-M3-06e: Converter — NULL 值与异常值防御

**前置**: 插入一条 `status=NULL, amount=NULL, created_at=NULL` 的记录

**步骤**:
1. 查询包含 NULL 行的 view

**预期**:
- `status_to_cn(NULL)` → 显示空字符串，不抛 KeyError
- `money_format(NULL)` → 显示空字符串
- `datetime_to_iso(NULL)` → 显示空字符串
- 其他正常行不受影响

---

### E2E-M3-06f: Converter — 未注册的 converter 名称

**前置**: YAML 中某 column 配置 `converter: nonexistent_func`

**步骤**:
1. 查询该 view

**预期**:
- 该列原样显示 Go 返回的原始值
- stderr 输出警告：`unknown converter: nonexistent_func, column will display raw value`
- 不中断查询，其他列正常

---

### E2E-M3-07: enable:false 完全隐藏

**步骤**:
1. `yaml-view -c testdata/e2e_basic.yaml`

**预期**:
- 只有 2 个 tab（user_list, user_search）
- disabled_view 不创建 tab，不可切换

---

### E2E-M3-08: 交互式参数输入（TUI）

**步骤**:
1. `yaml-view -c testdata/e2e_params.yaml`（TUI 模式，无 --param）

**预期**:
- 弹出参数输入界面
- 逐个提示输入 min_id 和 keyword
- 输入完成后执行查询并展示结果

---

### E2E-M3-09: view_name 缺省回退到顶级 key

**前置**: YAML 中某 view 不写 `view_name` 字段

```yaml
raw_key_view:
  db_type: "mysql"
  dsn: "..."
  sql: "SELECT 1"
  view:
    enable: true
```

**步骤**:
1. 启动 TUI

**预期**:
- Tab 标签显示 `raw_key_view`（顶级 key）
- 不报错、不显示空白标签

---

### E2E-M3-10: overflow: fold 长文本折叠

**前置**: 某列数据为 200 字符长文本，YAML 配置 `overflow: fold`

**步骤**:
1. 查询该 view

**预期**:
- 长文本在单元格内自动换行（fold），不截断
- 表格行高自适应
- 未配置 overflow 的列保持默认行为（截断 + 省略号）

---

### E2E-M3-11: enable_paging: false 全量展示

**前置**: view 配置 `enable_paging: false`，数据 50 行

**步骤**:
1. 查询该 view

**预期**:
- 一次性展示全部 50 行，无翻页交互
- 不显示页码指示器
- 不发送 page/page_size 参数给 Go（或 page=0）

---

### E2E-M3-12: maxRowsPerQuery 截断提示

**前置**: 表有 20000 行，Go CLI 模式 maxRowsPerQuery=10000

**步骤**:
1. `SELECT * FROM big_table`（无 LIMIT）

**预期**:
- 返回 10000 行，`truncated: true`
- Python 侧表格底部提示"结果已截断，共 10000 行（上限）"
- 不卡死、不 OOM

---

## M4: 国产库适配与压测

### E2E-M4-01: PostgreSQL 查询

**前置**: PG 已启动，e2e_users 表已建

**步骤**:
1. `yaml-view -c testdata/e2e_multi_db.yaml -v pg_users`

**预期**:
- 返回 3 行数据
- 分页 SQL 使用 `LIMIT ? OFFSET ?` 语法

---

### E2E-M4-02: Oracle 查询

**前置**: Oracle 已启动（注意启动慢，start_period=180s）

**步骤**:
1. `yaml-view -c testdata/e2e_multi_db.yaml -v oracle_users`

**预期**:
- 返回 3 行数据
- 分页 SQL 使用 `ROWNUM` 或 `OFFSET FETCH` 语法
- 驱动为 `sijms/go-ora`（纯 Go，无 Instant Client）

---

### E2E-M4-03: OpenGauss 查询

**前置**: OpenGauss 已启动

**步骤**:
1. 修改 e2e_multi_db.yaml 增加 opengauss view
2. `yaml-view -c testdata/e2e_multi_db.yaml -v og_users`

**预期**:
- 返回 3 行数据
- sha256 认证通过（openGauss-connector-go-pq 驱动）

---

### E2E-M4-04: 跨库视图（MySQL + PG + Oracle 同时）

**步骤**:
1. `yaml-view -c testdata/e2e_multi_db.yaml`

**预期**:
- 3 个 tab 各自展示对应数据库的结果
- 3 条连接并行建立（不同 dsn）
- 任一库失败不影响其他

---

### E2E-M4-05: 各方言分页正确性

**步骤**:
对每种数据库执行 page=1,page_size=2 和 page=2,page_size=2

**预期**:

| 数据库 | 第 1 页 | 第 2 页 | 分页语法 |
|---|---|---|---|
| MySQL | Alice, Bob | Charlie | `LIMIT 2 OFFSET 0/2` |
| PG | Alice, Bob | Charlie | `LIMIT 2 OFFSET 0/2` |
| Oracle | Alice, Bob | Charlie | `ROWNUM <= 2/4` |
| OpenGauss | Alice, Bob | Charlie | `LIMIT 2 OFFSET 0/2` |

---

### E2E-M4-06: 并发压测

**前置**: 准备 10 个 view 指向同一 MySQL

**步骤**:
1. `yaml-view -c testdata/e2e_stress.yaml`（10 个 view，同 dsn）

**预期**:
- 10 个查询通过同一条连接串行执行（同 dsn 去重）
- 全部成功返回，无 QUEUE_FULL（CLI 模式 QueueDepth=20 > 10）
- 总耗时 ≈ 单查询耗时 × 10（串行）

---

### E2E-M4-07: 队列满场景

**前置**: 构造 QueueDepth=2 的配置，提交 5 个慢查询

**步骤**:
1. 通过 Python 脚本直接调用 Go HTTP API（绕过 YAML）
2. 同时提交 5 个 `SELECT SLEEP(5)` 到同一 conn_id

**预期**:
- 前 3 个入队成功（1 执行中 + 2 队列中）
- 后 2 个立即返回 503 QUEUE_FULL
- 执行中的查询不受影响

---

### E2E-M4-08: 连接泄漏回收

**步骤**:
1. Python 调用 /connect 建立连接
2. kill -9 Python 进程（不调用 /shutdown）
3. 等待 30 分钟（或调低回收阈值测试）

**预期**:
- Go 侧后台 goroutine 检测到连接空闲 > 30min
- 自动关闭连接，释放资源
- 日志记录回收事件

---

### E2E-M4-09: 全局连接上限

**前置**: CLI 模式 GlobalMaxOpen=30

**步骤**:
1. 通过脚本建立 30 个不同 DSN 的连接
2. 尝试建立第 31 个

**预期**:
- 前 30 个成功
- 第 31 个返回错误 `global connection limit reached`

---

### E2E-M4-10: auth-token 鉴权

**步骤**:
1. `./db-gateway --auth-token secret123`
2. 不带 token 请求 → 401
3. 带 `X-Auth-Token: secret123` 请求 → 200

**预期**:
- 无 token 被拒绝
- 正确 token 通过
- Python 侧自动在 Header 中携带 token

---

## 用例优先级矩阵

| 用例 | 里程碑 | 优先级 | 自动化 |
|---|---|---|---|
| E2E-M2-01 单视图基本查询 | M2 | P0 | pytest |
| E2E-M2-02 多视图并行+连接复用 | M2 | P0 | pytest |
| E2E-M2-03 参数默认值 | M2 | P0 | pytest |
| E2E-M2-04 --param 覆盖 | M2 | P0 | pytest |
| E2E-M2-10 SQL 模板渲染 | M2 | P0 | pytest |
| E2E-M2-11 部分失败不阻塞 | M2 | P0 | pytest |
| E2E-M2-13 超时部分行展示 | M2 | P0 | pytest |
| E2E-M2-14 YAML 格式错误 | M2 | P0 | pytest |
| E2E-M2-16 空结果集 | M2 | P0 | pytest |
| E2E-M2-18 数据库不可达 | M2 | P0 | pytest |
| E2E-M2-05 交互提示 | M2 | P1 | 手动 |
| E2E-M2-06 -v 过滤 | M2 | P1 | pytest |
| E2E-M2-07 目录加载 | M2 | P1 | pytest |
| E2E-M2-08 check 子命令 | M2 | P1 | pytest |
| E2E-M2-09 进程生命周期 | M2 | P1 | pytest + ps |
| E2E-M2-12 会话超时 120s | M2 | P1 | pytest (慢) |
| E2E-M2-15 不支持的 db_type | M2 | P1 | pytest |
| E2E-M2-17 -v 不存在的 view | M2 | P1 | pytest |
| E2E-M2-19 目录 view key 冲突 | M2 | P2 | pytest |
| E2E-M3-01 表格渲染 | M3 | P0 | 手动/快照 |
| E2E-M3-02 Tab 切换 | M3 | P0 | 手动 |
| E2E-M3-03 分页翻页 | M3 | P0 | pytest (API层) |
| E2E-M3-05 错误 Tab | M3 | P0 | pytest (API层) |
| E2E-M3-06 Converter 日期转ISO | M3 | P0 | pytest |
| E2E-M3-06b Converter 枚举英转中 | M3 | P0 | pytest |
| E2E-M3-06c Converter 金额格式化 | M3 | P0 | pytest |
| E2E-M3-06e Converter NULL防御 | M3 | P0 | pytest |
| E2E-M3-07 enable:false 隐藏 | M3 | P0 | pytest |
| E2E-M3-09 view_name 缺省回退 | M3 | P0 | pytest |
| E2E-M3-11 enable_paging:false 全量 | M3 | P0 | pytest |
| E2E-M3-04 异步加载 | M3 | P1 | 手动 |
| E2E-M3-06d Converter 聚合结果 | M3 | P1 | pytest |
| E2E-M3-06f Converter 未注册名称 | M3 | P1 | pytest |
| E2E-M3-08 交互参数输入 | M3 | P1 | 手动 |
| E2E-M3-10 overflow:fold 长文本 | M3 | P1 | 手动/快照 |
| E2E-M3-12 maxRowsPerQuery 截断 | M3 | P1 | pytest |
| E2E-M4-01 PG 查询 | M4 | P0 | pytest |
| E2E-M4-02 Oracle 查询 | M4 | P0 | pytest |
| E2E-M4-04 跨库视图 | M4 | P0 | pytest |
| E2E-M4-05 方言分页 | M4 | P0 | pytest |
| E2E-M4-03 OpenGauss | M4 | P1 | pytest |
| E2E-M4-06 并发压测 | M4 | P1 | 脚本 |
| E2E-M4-07 队列满 | M4 | P1 | pytest |
| E2E-M4-08 连接泄漏回收 | M4 | P2 | 手动/慢测试 |
| E2E-M4-09 全局连接上限 | M4 | P2 | pytest |
| E2E-M4-10 auth-token | M4 | P2 | pytest |
