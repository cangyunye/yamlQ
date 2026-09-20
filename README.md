# yamlQ

基于 YAML 配置的终端多数据库查询工具。支持 MySQL、PostgreSQL、Oracle，一条命令查多库、多视图并行、表格化展示。

## 快速开始

### 构建

```bash
task build          # 编译 Go 网关 + 安装 Python CLI
```

### 使用

```bash
# 查询单个视图
yamlq -c config.yaml -v user_list

# 查询所有启用的视图（并行）
yamlq -c config.yaml

# 加载目录下所有 YAML
yamlq -c ./views/

# 传参覆盖默认值
yamlq -c config.yaml -v user_search --param keyword=Bob

# 交互式 TUI（Tab 切换、翻页）
yamlq -c config.yaml --tui

# 校验配置（检查哪些参数缺少默认值）
yamlq check -c config.yaml
```

#### 版本查看

```bash
yamlq --version
# yamlq v0.2.0-9-ge7679a9-dirty (commit e7679a9, built 2026-09-20T23:25:08+0800)
```

## 常驻网关（serve）

默认情况下，每次 `yamlq run` 都会启动一个随用随走的网关进程。频繁执行时，可把网关变成常驻守护进程，数据库连接池跨命令复用，免去每次的 TCP 握手与认证开销：

```bash
# 启动常驻网关（前台运行，Ctrl+C 退出）
yamlq serve

# 自定义端口 / 连接池空闲回收时间 / 鉴权 token
yamlq serve --port 7788 --conn-idle-timeout 1800 --auth-token my-secret
```

`yamlq serve` 启动后会把发现信息写入 `~/.yamlq/gateway.env`（以及当前目录的 `.yamlq-gateway.env`），此后所有 `yamlq run` 自动发现并复用该网关：

- 连接池按 `驱动+DSN` 幂等复用，第二次 run 零握手开销；
- 运行结束不再关闭连接池，由网关按 `--conn-idle-timeout`（默认 600 秒）回收空闲池；
- 网关每 30 秒对空闲池做活性探测，失效连接自动清理，下次 `/connect` 透明重建；CLI 侧查询遇到 `CONNECTION_LOST` 也会自动重连重试一次。

安全提示：常驻网关监听 `127.0.0.1`，长期存活，建议配合 `--auth-token` 使用；token 会写入发现文件（本机用户可读）。

## CLI 参数

| 参数 | 说明 |
|---|---|
| `-c` | YAML 文件或目录（目录递归加载 `.yaml/.yml`） |
| `-v` | 过滤视图 key，逗号分隔 |
| `--param` | `key=value` 覆盖参数默认值，可多次使用 |
| `--tui` | 启动交互式 TUI 模式 |
| `--timeout` | 单查询超时秒数（默认 30） |
| `--session-timeout` | 会话总超时秒数（默认 120） |
| `--mode` | 网关模式 `cli`（默认）或 `service` |
| `--auth-token` | 网关鉴权 token |
| `--verbose` | 详细日志 |
| `--conn-idle-timeout` | `serve` 模式连接池空闲回收秒数（默认 600，0 关闭） |

## YAML 配置

一个 YAML 文件包含多个视图，每个视图 = 一个数据源 + 一条 SQL：

```yaml
user_list:
  view_name: "用户列表"           # Tab 标签名（缺省用 key）
  description: "所有用户"         # 可选描述
  db_type: "mysql"               # mysql | postgres | oracle | opengauss
  dsn: "root:pass@tcp(127.0.0.1:3306)/mydb"
  sql: |
    SELECT id, name, status, created_at
    FROM users
    WHERE name LIKE CONCAT('%', {{keyword}}, '%')
  params:
    - name: keyword
      prompt: "搜索关键词"
      default: ""                # 有 default 则静默使用，无则交互提示
  view:
    enable: true                 # false 则跳过不查询
    enable_paging: true          # TUI 模式下启用翻页
    page_size: 20
    theme: "ocean"               # default | ocean | sunset | forest | mono
    columns:
      - field: id
        header: "ID"
        align: "center"          # left | center | right
        width: 6                 # 固定列宽
      - field: name
        header: "姓名"
        min_width: 8             # 最小列宽
      - field: status
        header: "状态"
        # 方式 A：内置 converter（仅 status_to_cn 一组固定映射）
        # converter: status_to_cn
        # 方式 B：YAML 自定义枚举映射（任意字段任意取值, 推荐）
        enum_map:
          pending: 待支付
          paid: 已支付
          shipped: 已发货
          completed: 已完成
          cancelled: 已取消
          refunded: 已退款
        max_width: 10            # 最大列宽（超出折叠/截断）
      - field: created_at
        header: "创建时间"
        converter: datetime_to_iso
        style: dim
        overflow: fold
```

### 多数据源示例

同一文件内声明多个视图，指向不同数据库，并行查询：

```yaml
mysql_report:
  db_type: "mysql"
  dsn: "root:pass@tcp(127.0.0.1:3306)/db1"
  sql: "SELECT * FROM orders"
  view: { enable: true }

pg_report:
  db_type: "postgres"
  dsn: "postgres://user:pass@127.0.0.1:5432/db2?sslmode=disable"
  sql: "SELECT * FROM analytics"
  view: { enable: true }

oracle_report:
  db_type: "oracle"
  dsn: "oracle://user:pass@127.0.0.1:1521/XEPDB1"
  sql: "SELECT * FROM legacy_data"
  view: { enable: true }
```

### SQL 参数

- 使用 `{{param_name}}` 声明占位符，运行时替换为 `?` 参数化查询
- **不要**将 `{{param}}` 放在引号内（如 `'%{{x}}%'`），应使用数据库函数：
  - MySQL: `LIKE CONCAT('%', {{x}}, '%')`
  - PG: `LIKE '%' || {{x}} || '%'`

### 内置主题

| 名称 | 表头 | 边框 | 斑马纹 |
|---|---|---|---|
| `default` | 青色加粗 | 蓝色 | 有 |
| `ocean` | 白字深蓝底 | 青色 | 有 |
| `sunset` | 黑字黄底 | 红色 | 有 |
| `forest` | 白字深绿底 | 绿色 | 有 |
| `mono` | 白色加粗 | 白色 | 无 |

### 列级枚举翻译 `enum_map`

任何字段都可以在列配置里声明一张"原值 → 显示文本"的 YAML 映射（自定义枚举列表）:

```yaml
view:
  columns:
    - field: priority
      header: 优先级
      enum_map:
        HIGH: 高
        MEDIUM: 中
        LOW: 低
    - field: active          # 数值原值也可映射, key 用字符串即可
      header: 上架
      enum_map:
        "0": 停售
        "1": 在售
```

规则:

- 查询结果中的原值（字符串化后）命中 key 则显示映射文本, 未命中则原样显示, 永不丢数据;
- 数值原值 `1` / `1.0` / `"1"` 归一到同一 key, 因此枚举 key 一律按字符串写即可（建议加引号）;
- 同一列同时声明 `enum_map` 与 `converter` 时, `enum_map` 优先;
- Oracle/MySQL/PG 返回的 NUMBER 都是字符串/数字原值, 无需转换器配合。

### 内置 Converter


| 名称 | 功能 | 示例 |
|---|---|---|
| `datetime_to_iso` | 日期 → `2024-03-15` | `2024-03-15 08:30:00` → `2024-03-15` |
| `datetime_to_cn` | 日期 → 中文格式 | → `2024年03月15日 08:30` |
| `status_to_cn` | 枚举英转中 | `pending` → `待支付` |
| `money_format` | 金额千分位 | `1280.00` → `¥1,280.00` |

未注册的 converter 名称会输出警告并原样显示。

### DSN 格式

| db_type | DSN 格式 |
|---|---|
| `mysql` | `user:pass@tcp(host:3306)/db` |
| `postgres` | `postgres://user:pass@host:5432/db?sslmode=disable` |
| `oracle` | `oracle://user:pass@host:1521/service` |
| `opengauss` | `postgres://user:pass@host:5433/db?sslmode=disable` |

## 项目结构

```
yamlq/
├── db-gateway/       # Go HTTP 数据库网关
├── cli/              # Python CLI + TUI
├── oracle/           # Oracle 种子 SQL (演示表 + e2e_users)
├── testdata/         # E2E 测试 YAML
├── docs/             # 架构设计、ADR、测试计划
└── Taskfile.yml
```

## 开发

```bash
task test             # 运行全部测试（Go + Python E2E）
task test-go          # 仅 Go 测试
task test-python      # 仅 Python E2E 测试
task build            # 编译
```

详细架构设计见 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)。
