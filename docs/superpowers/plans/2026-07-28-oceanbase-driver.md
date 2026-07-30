# OceanBase Driver Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use subagent-driven-development (recommended) to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `ob-mysql` and `ob-oracle` as first-class drivers in yamlQ.
- `ob-mysql` backed by existing `go-sql-driver/mysql` (OB MySQL tenant speaks MySQL wire protocol).
- `ob-oracle` backed by existing `sijms/go-ora/v2` (OB Oracle tenant requires Oracle protocol).

**Architecture:** `ob-mysql` uses `sql.Open("mysql", dsn)`; `ob-oracle` uses `sql.Open("oracle", dsn)`. Only pagination dialect differs between them. Unit tests need zero database. Test data YAML files serve as integration specs.

**Tech Stack:** Go 1.25, `github.com/go-sql-driver/mysql` v1.10.0 (existing), `github.com/sijms/go-ora/v2` (existing)

## Global Constraints

- Zero new Go module dependencies — `ob-mysql` reuses `go-sql-driver/mysql`, `ob-oracle` reuses `sijms/go-ora/v2`
- No changes to Python code — Python just passes `"driver": "ob-mysql"` / `"ob-oracle"` as strings
- No changes to `conn/` or `server/` — they work generically over `*sql.DB`
- Pagination test must remain database-free (table-driven, no `sql.Open`)
- DSN format for OB-MySQL: `user@tenant:password@tcp(host:port)/dbname?charset=utf8mb4`
- DSN format for OB-Oracle: `user@tenant/password@host:port/service_name` (go-ora simple connection string)

---

## Files

| File | Action | Responsibility |
|------|--------|----------------|
| `db-gateway/driver/registry.go` | Modify | Add `ob-mysql` and `ob-oracle` entries |
| `db-gateway/driver/registry_test.go` | Create | Unit tests for registration (no DB) |
| `db-gateway/dialect/pagination.go` | Modify | Add `ob-oracle` → Oracle-style pagination |
| `db-gateway/dialect/pagination_test.go` | Modify | Add OB pagination test cases |
| `testdata/ob_basic.yaml` | Create | YAML test data for OB-MySQL |
| `testdata/ob_oracle.yaml` | Create | YAML test data for OB-Oracle |
| `docs/ARCHITECTURE.md` | Modify | Update driver compatibility table |

---

### Task 1: Register `ob-mysql` and `ob-oracle` in driver registry

**Files:**
- Modify: `db-gateway/driver/registry.go:14-26`

**Interfaces:**
- Consumes: `OpenFunc` type (already defined), `sql.Open("mysql", dsn)` pattern
- Produces: `registry["ob-mysql"]` and `registry["ob-oracle"]` entries, both callable via `driver.Open()`

- [ ] **Step 1: Add the two new entries to `registry`**

Edit `db-gateway/driver/registry.go`, adding `ob-mysql` and `ob-oracle` cases after the `opengauss` line:

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
		return sql.Open("oracle", dsn)
	},
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd db-gateway && go build ./...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add db-gateway/driver/registry.go
git commit -m "feat: register ob-mysql and ob-oracle drivers"
```

---

### Task 2: Add OB-Oracle pagination dialect

**Files:**
- Modify: `db-gateway/dialect/pagination.go:10-17`

**Interfaces:**
- Consumes: `driver` string from `conn/manager.go` → `dialect.WrapPagination(mc.Driver, ...)`
- Produces: correct pagination SQL for `ob-oracle`

- [ ] **Step 1: Add `ob-oracle` case to the switch**

Edit `db-gateway/dialect/pagination.go`:

```go
func WrapPagination(driver, sql string, page, pageSize int) string {
	if page <= 0 || pageSize <= 0 {
		return sql
	}
	offset := (page - 1) * pageSize
	switch driver {
	case "mysql", "opengauss", "postgres", "ob-mysql":
		return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, pageSize, offset)
	case "oracle", "ob-oracle":
		return fmt.Sprintf("%s OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", sql, offset, pageSize)
	default:
		return fmt.Sprintf("%s LIMIT %d OFFSET %d", sql, pageSize, offset)
	}
}
```

Note: `ob-mysql` grouped with MySQL (same LIMIT/OFFSET syntax).
`ob-oracle` grouped with Oracle (OFFSET...FETCH syntax).

- [ ] **Step 2: Verify it compiles**

Run: `cd db-gateway && go build ./...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add db-gateway/dialect/pagination.go
git commit -m "feat: add ob-oracle pagination dialect"
```

---

### Task 3: Write unit tests for registration and pagination

**Files:**
- Create: `db-gateway/driver/registry_test.go`
- Modify: `db-gateway/dialect/pagination_test.go`

**Interfaces:**
- Consumes: `driver.Open()`, `driver.Supported()`, `dialect.WrapPagination()`
- Produces: verification that all four known OB cases work

- [ ] **Step 1: Create `db-gateway/driver/registry_test.go`**

```go
package driver

import "testing"

func TestSupported(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		want   bool
	}{
		{name: "mysql", input: "mysql", want: true},
		{name: "postgres", input: "postgres", want: true},
		{name: "oracle", input: "oracle", want: true},
		{name: "opengauss", input: "opengauss", want: true},
		{name: "ob-mysql", input: "ob-mysql", want: true},
		{name: "ob-oracle", input: "ob-oracle", want: true},
		{name: "unsupported", input: "sqlite", want: false},
		{name: "empty", input: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Supported(tc.input)
			if got != tc.want {
				t.Errorf("Supported(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestOpenReturnsDB(t *testing.T) {
	cases := []struct {
		name   string
		driver string
		dsn    string
	}{
		{name: "ob-mysql", driver: "ob-mysql", dsn: "root:pass@tcp(127.0.0.1:2883)/test"},
		{name: "ob-oracle", driver: "ob-oracle", dsn: "user@tenant/pass@127.0.0.1:2883/test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, err := Open(tc.driver, tc.dsn)
			if err != nil {
				t.Fatalf("Open(%q, %q) unexpected error: %v", tc.driver, tc.dsn, err)
			}
			if db == nil {
				t.Fatalf("Open(%q, %q) returned nil *sql.DB", tc.driver, tc.dsn)
			}
			db.Close()
		})
	}
}
```

Note: `sql.Open()` only validates DSN syntax, does not connect — safe to call with fake credentials. `db.Close()` cleans up the pool.

- [ ] **Step 2: Run registry tests to verify they pass**

Run: `cd db-gateway && go test ./driver/ -v`
Expected:

```
=== RUN   TestSupported/mysql
=== RUN   TestSupported/postgres
=== RUN   TestSupported/oracle
=== RUN   TestSupported/opengauss
=== RUN   TestSupported/ob-mysql
=== RUN   TestSupported/ob-oracle
=== RUN   TestSupported/unsupported
=== RUN   TestSupported/empty
--- PASS: TestSupported (0.00s)
=== RUN   TestOpenReturnsDB/ob-mysql
=== RUN   TestOpenReturnsDB/ob-oracle
--- PASS: TestOpenReturnsDB (0.00s)
PASS
```

- [ ] **Step 3: Add OB pagination test cases to `pagination_test.go`**

Append after existing tests (or add as new functions):

```go
func TestWrapPaginationOBMySQL(t *testing.T) {
	sql := "SELECT * FROM orders ORDER BY id"
	got := WrapPagination("ob-mysql", sql, 2, 20)
	want := "SELECT * FROM orders ORDER BY id LIMIT 20 OFFSET 20"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWrapPaginationOBOracle(t *testing.T) {
	sql := "SELECT * FROM orders ORDER BY id"
	got := WrapPagination("ob-oracle", sql, 3, 15)
	want := "SELECT * FROM orders ORDER BY id OFFSET 30 ROWS FETCH NEXT 15 ROWS ONLY"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 4: Run pagination tests**

Run: `cd db-gateway && go test ./dialect/ -v`
Expected: 5 tests pass (3 original + 2 new)

- [ ] **Step 5: Run full Go test suite**

Run: `cd db-gateway && go test ./... -short -timeout 30s`
Expected: driver + dialect tests pass. server tests will be skipped or fail (need real MySQL — expected).

- [ ] **Step 6: Commit**

```bash
git add db-gateway/driver/registry_test.go db-gateway/dialect/pagination_test.go
git commit -m "test: add OB driver registration and pagination tests"
```

---

### Task 4: Provide test data YAML files

**Files:**
- Create: `testdata/ob_basic.yaml`
- Create: `testdata/ob_oracle.yaml`

**Interfaces:**
- Consumes: yamlQ YAML config format (`db_type`, `dsn`, `sql`, `view` blocks)
- Produces: drop-in test YAMLs for manual validation with real OB

- [ ] **Step 1: Create `testdata/ob_basic.yaml` for MySQL-tenant OB**

```yaml
ob_user_list:
  view_name: "OB 用户列表"
  description: "OceanBase MySQL 租户用户查询"
  db_type: "ob-mysql"
  dsn: "root:password@tcp(192.168.1.100:2883)/test_db?charset=utf8mb4"
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

ob_order_stats:
  view_name: "OB 订单统计"
  description: "OceanBase MySQL 租户聚合查询"
  db_type: "ob-mysql"
  dsn: "root:password@tcp(192.168.1.100:2883)/test_db?charset=utf8mb4"
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

- [ ] **Step 2: Create `testdata/ob_oracle.yaml` for Oracle-tenant OB**

```yaml
ob_emp_list:
  view_name: "OB 员工列表"
  description: "OceanBase Oracle 租户员工查询"
  db_type: "ob-oracle"
  dsn: "scott@oracle_tenant/password@192.168.1.100:2883/test_db"
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

ob_dept_summary:
  view_name: "OB 部门汇总"
  description: "OceanBase Oracle 租户分析查询"
  db_type: "ob-oracle"
  dsn: "scott@oracle_tenant/password@192.168.1.100:2883/test_db"
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
git add testdata/ob_basic.yaml testdata/ob_oracle.yaml
git commit -m "test: add OB test data YAML files"
```

---

### Task 5: Update architecture documentation

**Files:**
- Modify: `docs/ARCHITECTURE.md:21-27`

- [ ] **Step 1: Update driver compatibility table**

Edit the table in `docs/ARCHITECTURE.md`, replacing rows 21-27:

```markdown
| db_type | Go 驱动 | 备注 |
|---|---|---|
| `mysql` | `go-sql-driver/mysql` | 兼容 GoldenDB-MySQL |
| `ob-mysql` | `go-sql-driver/mysql` | OceanBase MySQL 租户，MySQL 协议 |
| `ob-oracle` | `sijms/go-ora/v2` | OceanBase Oracle 租户，Oracle 协议，Oracle SQL 方言 |
| `postgres` | `jackc/pgx/v5/stdlib` | |
| `oracle` | `sijms/go-ora/v2` | 纯 Go，免 Instant Client |
| `opengauss` | `jackc/pgx/v5/stdlib` | PG 协议兼容 |
```

- [ ] **Step 2: Commit**

```bash
git add docs/ARCHITECTURE.md
git commit -m "docs: add OB driver documentation"
```

---

### How to test without a real OceanBase

All unit tests in tasks 1-3 run without any database. They verify:
- Driver names are registered → `Supported()` returns correct booleans
- `Open()` returns a valid `*sql.DB` (no connection made, just pool object)
- Pagination SQL is correctly generated for each driver variant

For end-to-end testing with a real OB instance:

1. **Use the testdata YAML files** from Task 4 as config
2. **Replace DSN** with your OB credentials
3. **Create sample tables** using one of these scripts:

**OB-MySQL tenant:**
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

**OB-Oracle tenant:**
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

3. **Run:**
```bash
make build
yamlq -c testdata/ob_basic.yaml
yamlq -c testdata/ob_oracle.yaml
```
