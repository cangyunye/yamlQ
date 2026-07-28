# GoldenDB Driver Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use subagent-driven-development (recommended) to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `gd-mysql` and `gd-oracle` as first-class drivers in yamlQ, backed by existing `go-sql-driver/mysql` (GoldenDB speaks MySQL wire protocol for both modes). No new Go dependencies.

**Architecture:** Two registry entries share `sql.Open("mysql", dsn)`; only pagination dialect differs. Unit tests need zero database. Test data YAML files serve as integration specs. GoldenDB-MySQL is already compatible via the `mysql` driver — adding explicit entries for discoverability and consistency with OB.

**Tech Stack:** Go 1.25, `github.com/go-sql-driver/mysql` v1.10.0 (existing)

## Global Constraints

- Zero new Go module dependencies — both drivers reuse `go-sql-driver/mysql`
- No changes to Python code — Python just passes `"driver": "gd-mysql"` / `"gd-oracle"` as strings
- No changes to `conn/` or `server/` — they work generically over `*sql.DB`
- Pagination test must remain database-free (table-driven, no `sql.Open`)
- DSN format for GoldenDB: `user:password@tcp(host:port)/dbname?charset=utf8mb4`

---

## Files

| File | Action | Responsibility |
|------|--------|----------------|
| `db-gateway/driver/registry.go` | Modify | Add `gd-mysql` and `gd-oracle` entries |
| `db-gateway/driver/registry_test.go` | Modify | Add GoldenDB test cases |
| `db-gateway/dialect/pagination.go` | Modify | Add `gd-oracle` → Oracle-style pagination |
| `db-gateway/dialect/pagination_test.go` | Modify | Add GoldenDB pagination test cases |
| `testdata/gd_basic.yaml` | Create | YAML test data for GoldenDB-MySQL |
| `testdata/gd_oracle.yaml` | Create | YAML test data for GoldenDB-Oracle |
| `docs/ARCHITECTURE.md` | Modify | Update driver compatibility table |

---

### Task 1: Register `gd-mysql` and `gd-oracle` in driver registry

**Files:**
- Modify: `db-gateway/driver/registry.go:14-36`

**Interfaces:**
- Consumes: `OpenFunc` type (already defined), `sql.Open("mysql", dsn)` pattern
- Produces: `registry["gd-mysql"]` and `registry["gd-oracle"]` entries, both callable via `driver.Open()`

- [ ] **Step 1: Read current state**

Read `db-gateway/driver/registry.go` to confirm OB entries from prior work exist. The current map should have: `mysql`, `postgres`, `opengauss`, `oracle`, `ob-mysql`, `ob-oracle`.

- [ ] **Step 2: Add the two new entries**

Edit `db-gateway/driver/registry.go`, adding `gd-mysql` and `gd-oracle` after the `ob-oracle` entry:

```go
var registry = map[string]OpenFunc{
	"mysql": func(dsn string) (*sql.DB, error) {
		return sql.Open("mysql", dsn)
	},
	"postgres": func(dsn string) (*sql.DB, error) {
		return sql.Open("pgx", dsn)
	},
	"opengauss": func(dsn string) (*sql.DB, error) {
		return sql.Open("pgx", dsn)
	},
	"oracle": func(dsn string) (*sql.DB, error) {
		return sql.Open("oracle", dsn)
	},
	"ob-mysql": func(dsn string) (*sql.DB, error) {
		return sql.Open("mysql", dsn)
	},
	"ob-oracle": func(dsn string) (*sql.DB, error) {
		return sql.Open("mysql", dsn)
	},
	"gd-mysql": func(dsn string) (*sql.DB, error) {
		return sql.Open("mysql", dsn)
	},
	"gd-oracle": func(dsn string) (*sql.DB, error) {
		return sql.Open("mysql", dsn)
	},
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd db-gateway && go build ./...`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add db-gateway/driver/registry.go
git commit -m "feat: register gd-mysql and gd-oracle drivers"
```

---

### Task 2: Add GoldenDB-Oracle pagination dialect

**Files:**
- Modify: `db-gateway/dialect/pagination.go:10-17`

**Interfaces:**
- Consumes: `dialect.WrapPagination(driver, sql, page, pageSize)` — driver string now includes `gd-mysql`, `gd-oracle`
- Produces: correct pagination SQL for `gd-oracle`

- [ ] **Step 1: Read current state**

Read `db-gateway/dialect/pagination.go` to confirm OB cases from Task 2 of the OB plan exist.

- [ ] **Step 2: Add `gd-mysql` and `gd-oracle` to the switch**

Edit `db-gateway/dialect/pagination.go`:

```go
func WrapPagination(driver, sql string, page, pageSize int) string {
	if page <= 0 || pageSize <= 0 {
		return sql
	}
	offset := (page - 1) * pageSize
	switch driver {
	case "mysql", "opengauss", "postgres", "ob-mysql", "gd-mysql":
		return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, pageSize, offset)
	case "oracle", "ob-oracle", "gd-oracle":
		return fmt.Sprintf("%s OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", sql, offset, pageSize)
	default:
		return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, pageSize, offset)
	}
}
```

- [ ] **Step 3: Verify it compiles**

Run: `cd db-gateway && go build ./...`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add db-gateway/dialect/pagination.go
git commit -m "feat: add gd-oracle pagination dialect"
```

---

### Task 3: Add GoldenDB test cases to existing unit tests

**Files:**
- Modify: `db-gateway/driver/registry_test.go`
- Modify: `db-gateway/dialect/pagination_test.go`

**Interfaces:**
- Consumes: `driver.Supported()`, `driver.Open()`, `dialect.WrapPagination()`

- [ ] **Step 1: Read existing test files**

Read both test files to understand the current test structure before editing.

- [ ] **Step 2: Add `gd-mysql` and `gd-oracle` to `TestSupported`**

Edit `db-gateway/driver/registry_test.go`, adding two cases to the table in `TestSupported`:

```go
{name: "gd-mysql", input: "gd-mysql", want: true},
{name: "gd-oracle", input: "gd-oracle", want: true},
```

- [ ] **Step 3: Add `gd-mysql` and `gd-oracle` to `TestOpenReturnsDB`**

Add two cases to the table:

```go
{name: "gd-mysql", driver: "gd-mysql", dsn: "root:pass@tcp(127.0.0.1:3306)/test"},
{name: "gd-oracle", driver: "gd-oracle", dsn: "root:pass@tcp(127.0.0.1:3306)/test"},
```

- [ ] **Step 4: Run registry tests to verify**

Run: `cd db-gateway && go test ./driver/ -v`
Expected: 14/14 passing (8 original OB cases + 4 new GoldenDB cases)

Output should show:
```
=== RUN   TestSupported/gd-mysql
=== RUN   TestSupported/gd-oracle
=== RUN   TestOpenReturnsDB/gd-mysql
=== RUN   TestOpenReturnsDB/gd-oracle
--- PASS
```

- [ ] **Step 5: Add GoldenDB pagination test cases**

Append to `db-gateway/dialect/pagination_test.go`:

```go
func TestWrapPaginationGDMysql(t *testing.T) {
	sql := "SELECT * FROM users ORDER BY id"
	got := WrapPagination("gd-mysql", sql, 2, 20)
	want := "SELECT * FROM users ORDER BY id LIMIT 20 OFFSET 20"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapPaginationGDOracle(t *testing.T) {
	sql := "SELECT * FROM users ORDER BY id"
	got := WrapPagination("gd-oracle", sql, 3, 15)
	want := "SELECT * FROM users ORDER BY id OFFSET 30 ROWS FETCH NEXT 15 ROWS ONLY"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 6: Run pagination tests**

Run: `cd db-gateway && go test ./dialect/ -v`
Expected: 7/7 passing (5 original OB tests + 2 new GoldenDB)

- [ ] **Step 7: Commit**

```bash
git add db-gateway/driver/registry_test.go db-gateway/dialect/pagination_test.go
git commit -m "test: add GoldenDB driver registration and pagination tests"
```

---

### Task 4: Provide test data YAML files

**Files:**
- Create: `testdata/gd_basic.yaml`
- Create: `testdata/gd_oracle.yaml`

**Interfaces:**
- Consumes: yamlQ YAML config format (`db_type`, `dsn`, `sql`, `view` blocks)

- [ ] **Step 1: Create `testdata/gd_basic.yaml` for MySQL-mode GoldenDB**

```yaml
gd_user_list:
  view_name: "GoldenDB 用户列表"
  description: "GoldenDB MySQL 模式用户查询"
  db_type: "gd-mysql"
  dsn: "root:password@tcp(192.168.1.100:3306)/test_db?charset=utf8mb4"
  sql: |
    SELECT user_id, user_name, status, created_at
    FROM t_user
    WHERE user_name LIKE CONCAT('%', {{keyword}}, '%')
  params:
    - name: keyword
      prompt: "搜索关键词"
      default: ""
  view:
    enable: true
    theme: "ocean"
    columns:
      - field: user_id
        header: "用户ID"
        align: "center"
      - field: user_name
        header: "用户名"
        min_width: 10
      - field: status
        header: "状态"
        converter: status_to_cn
      - field: created_at
        header: "创建时间"
        converter: datetime_to_iso

gd_order_stats:
  view_name: "GoldenDB 订单统计"
  description: "GoldenDB MySQL 模式聚合查询"
  db_type: "gd-mysql"
  dsn: "root:password@tcp(192.168.1.100:3306)/test_db?charset=utf8mb4"
  sql: |
    SELECT DATE(create_time) AS dt, COUNT(*) AS cnt, SUM(amount) AS total
    FROM t_order
    WHERE create_time >= {{start_date}}
    GROUP BY DATE(create_time)
    ORDER BY dt DESC
  params:
    - name: start_date
      prompt: "起始日期 (YYYY-MM-DD)"
      default: "2026-01-01"
  view:
    enable: true
    theme: "forest"
    columns:
      - field: dt
        header: "日期"
      - field: cnt
        header: "订单数"
        align: "right"
      - field: total
        header: "总金额"
        align: "right"
        converter: money_format
```

- [ ] **Step 2: Create `testdata/gd_oracle.yaml` for Oracle-mode GoldenDB**

```yaml
gd_emp_list:
  view_name: "GoldenDB 员工列表"
  description: "GoldenDB Oracle 模式员工查询"
  db_type: "gd-oracle"
  dsn: "scott:password@tcp(192.168.1.100:3306)/test_db?charset=utf8mb4"
  sql: |
    SELECT empno, ename, job, sal, hiredate
    FROM emp
    WHERE ename LIKE '%' || {{keyword}} || '%'
  params:
    - name: keyword
      prompt: "搜索关键词"
      default: ""
  view:
    enable: true
    theme: "sunset"
    columns:
      - field: empno
        header: "员工号"
        align: "center"
      - field: ename
        header: "姓名"
      - field: job
        header: "职位"
      - field: sal
        header: "薪资"
        align: "right"
        converter: money_format
      - field: hiredate
        header: "入职日期"
        converter: datetime_to_cn

gd_dept_summary:
  view_name: "GoldenDB 部门汇总"
  description: "GoldenDB Oracle 模式分析查询"
  db_type: "gd-oracle"
  dsn: "scott:password@tcp(192.168.1.100:3306)/test_db?charset=utf8mb4"
  sql: |
    SELECT d.dname,
           COUNT(e.empno) AS emp_count,
           NVL(SUM(e.sal), 0) AS total_sal
    FROM dept d
    LEFT JOIN emp e ON e.deptno = d.deptno
    GROUP BY d.dname
    ORDER BY total_sal DESC
  params: []
  view:
    enable: true
    theme: "default"
    columns:
      - field: dname
        header: "部门"
      - field: emp_count
        header: "人数"
        align: "right"
      - field: total_sal
        header: "薪资总额"
        align: "right"
        converter: money_format
```

- [ ] **Step 3: Commit**

```bash
git add testdata/gd_basic.yaml testdata/gd_oracle.yaml
git commit -m "test: add GoldenDB test data YAML files"
```

---

### Task 5: Update architecture documentation

**Files:**
- Modify: `docs/ARCHITECTURE.md:26-33`

- [ ] **Step 1: Read current state**

Read `docs/ARCHITECTURE.md` to confirm the OB doc changes from prior work.

- [ ] **Step 2: Update driver compatibility table**

Edit the table in `docs/ARCHITECTURE.md`:

```markdown
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
```

Note: `mysql` row stripped the old compatibility note since GoldenDB-MySQL now has a dedicated entry.

- [ ] **Step 3: Commit**

```bash
git add docs/ARCHITECTURE.md
git commit -m "docs: add GoldenDB driver documentation"
```

---

### How to test without a real GoldenDB

All unit tests in tasks 1-3 run without any database. They verify:
- Driver names are registered → `Supported()` returns correct booleans
- `Open()` returns a valid `*sql.DB` (no connection made, just pool object)
- Pagination SQL is correctly generated for each driver variant

For end-to-end testing with a real GoldenDB instance:

1. **Use the testdata YAML files** from Task 4 as config
2. **Replace DSN** with your GoldenDB credentials
3. **Create sample tables** using one of these scripts:

**GoldenDB MySQL mode:**
```sql
CREATE TABLE t_user (
  user_id   INT PRIMARY KEY,
  user_name VARCHAR(100),
  status    VARCHAR(20),
  created_at DATETIME
);
INSERT INTO t_user VALUES (1, 'Alice', 'active', '2026-01-15 08:30:00');
INSERT INTO t_user VALUES (2, 'Bob', 'inactive', '2026-03-20 14:00:00');

CREATE TABLE t_order (
  id          INT PRIMARY KEY,
  amount      DECIMAL(10,2),
  create_time DATETIME
);
INSERT INTO t_order VALUES (1, 100.00, '2026-07-01 10:00:00');
INSERT INTO t_order VALUES (2, 250.50, '2026-07-02 11:30:00');
```

**GoldenDB Oracle mode:**
```sql
CREATE TABLE emp (
  empno    NUMBER PRIMARY KEY,
  ename    VARCHAR2(100),
  job      VARCHAR2(50),
  sal      NUMBER(10,2),
  hiredate DATE,
  deptno   NUMBER
);
CREATE TABLE dept (
  deptno NUMBER PRIMARY KEY,
  dname  VARCHAR2(100)
);
INSERT INTO dept VALUES (10, 'ACCOUNTING');
INSERT INTO dept VALUES (20, 'RESEARCH');
INSERT INTO emp VALUES (1001, 'Alice', 'MANAGER', 8000, DATE '2025-01-15', 10);
INSERT INTO emp VALUES (1002, 'Bob', 'ANALYST', 6000, DATE '2025-03-20', 20);
```

4. **Run:**
```bash
make build
yamlq -c testdata/gd_basic.yaml
yamlq -c testdata/gd_oracle.yaml
```
