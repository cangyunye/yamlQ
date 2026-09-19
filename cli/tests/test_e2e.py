from __future__ import annotations

import os
import re
import subprocess
import sys
import time
from pathlib import Path

import pytest
import requests

TESTDATA = Path(__file__).resolve().parent.parent.parent / "testdata"
CLI_DIR = Path(__file__).resolve().parent.parent

DEFAULT_MYSQL_DSN = "root:root123456@tcp(127.0.0.1:3306)/default_db"
DEFAULT_PG_DSN = "postgres://postgres:postgres123@127.0.0.1:5432/postgres_db?sslmode=disable"
DEFAULT_ORACLE_DSN = "oracle://appuser:App123!@127.0.0.1:1521/XEPDB1"

MYSQL_DSN_RE = re.compile(r"^(?P<user>[^:@/]+):(?P<pw>.*?)@tcp\((?P<host>[^:)]+):(?P<port>\d+)\)/(?P<db>[^?]+)")


def mysql_dsn() -> str:
    return os.environ.get("YAMLQ_TEST_MYSQL_DSN", DEFAULT_MYSQL_DSN)


def pg_dsn() -> str:
    return os.environ.get("YAMLQ_TEST_PG_DSN", DEFAULT_PG_DSN)


def opengauss_dsn() -> str | None:
    return os.environ.get("YAMLQ_TEST_OPENGAUSS_DSN") or None


def oracle_dsn() -> str | None:
    return os.environ.get("YAMLQ_TEST_ORACLE_DSN") or None


def ob_mysql_dsn() -> str | None:
    return os.environ.get("YAMLQ_TEST_OB_MYSQL_DSN") or None


def ob_oracle_dsn() -> str | None:
    return os.environ.get("YAMLQ_TEST_OB_ORACLE_DSN") or None


# Placeholder DSNs committed in the OceanBase fixtures (testdata/ob_*.yaml).
PLACEHOLDER_OB_MYSQL_DSN = "root:password@tcp(192.168.1.100:2883)/test_db?charset=utf8mb4"
PLACEHOLDER_OB_ORACLE_DSN = "mysql://scott@oracle_tenant:password@192.168.1.100:2883/test_db"
PLACEHOLDER_OB_ORACLE_DSN_CLUSTER = PLACEHOLDER_OB_ORACLE_DSN + "?cluster=obcluster"


def mysql_cli_args() -> list[str]:
    m = MYSQL_DSN_RE.match(mysql_dsn())
    if not m:
        pytest.exit(f"cannot parse YAMLQ_TEST_MYSQL_DSN: {mysql_dsn()}", returncode=2)
    return [
        "mysql", "-h", m["host"], "-P", m["port"], "-u", m["user"],
        f"-p{m['pw']}", m["db"],
    ]


OG_VIEW_TEMPLATE = """og_users:
  view_name: "OG用户"
  db_type: "opengauss"
  dsn: "{dsn}"
  sql: "SELECT id, name FROM e2e_users ORDER BY id"
  params: []
  view:
    enable: true
    columns:
      - field: id
        header: "ID"
      - field: name
        header: "姓名"
"""


def _inject_opengauss_view(text: str, dsn: str) -> str:
    return text.rstrip() + "\n\n" + OG_VIEW_TEMPLATE.format(dsn=dsn)


@pytest.fixture(autouse=True)
def _override_fixture_dsns(tmp_path_factory, monkeypatch):
    """Rewrite committed fixtures with env-provided local DSNs, and inject an
    openGauss view into the multi-db fixture when its DSN is provided.

    The rewritten copy lives in its own tmp dir — sharing the per-test
    tmp_path would leak rewritten fixtures into directory-loading tests.
    """
    mysql, pg, og = os.environ.get("YAMLQ_TEST_MYSQL_DSN"), os.environ.get("YAMLQ_TEST_PG_DSN"), opengauss_dsn()
    oracle = oracle_dsn()
    ob_mysql, ob_oracle = ob_mysql_dsn(), ob_oracle_dsn()
    if not (mysql or pg or og or oracle or ob_mysql or ob_oracle):
        yield
        return
    target = tmp_path_factory.mktemp("testdata-rewrite")
    for f in TESTDATA.glob("*.yaml"):
        text = f.read_text(encoding="utf-8")
        if mysql:
            text = text.replace(DEFAULT_MYSQL_DSN, mysql)
        if pg:
            text = text.replace(DEFAULT_PG_DSN, pg)
        if oracle:
            text = text.replace(DEFAULT_ORACLE_DSN, oracle)
        if ob_oracle:
            # Cluster variant first: it is a superset of the base string.
            text = text.replace(PLACEHOLDER_OB_ORACLE_DSN_CLUSTER, ob_oracle)
            text = text.replace(PLACEHOLDER_OB_ORACLE_DSN, ob_oracle)
        if ob_mysql:
            text = text.replace(PLACEHOLDER_OB_MYSQL_DSN, ob_mysql)
        if og and "multi_db" in f.name:
            text = _inject_opengauss_view(text, og)
        (target / f.name).write_text(text, encoding="utf-8")
    monkeypatch.setattr(sys.modules[__name__], "TESTDATA", target)
    yield


def run_cli(*args: str, input_text: str = "", timeout: int = 60, env: dict | None = None) -> subprocess.CompletedProcess:
    cmd = [sys.executable, "-m", "yamlq", *args]
    return subprocess.run(
        cmd,
        capture_output=True,
        text=True,
        input=input_text,
        timeout=timeout,
        cwd=str(CLI_DIR),
        env=env,
    )


@pytest.fixture(scope="session", autouse=True)
def seed_db():
    sql = """
    CREATE TABLE IF NOT EXISTS e2e_users (
      id INT PRIMARY KEY AUTO_INCREMENT,
      name VARCHAR(50),
      email VARCHAR(100),
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
    DELETE FROM e2e_users;
    INSERT INTO e2e_users (name, email) VALUES
      ('Alice', 'alice@test.com'),
      ('Bob', 'bob@test.com'),
      ('Charlie', 'charlie@test.com');

    CREATE TABLE IF NOT EXISTS e2e_orders (
      id INT PRIMARY KEY AUTO_INCREMENT,
      order_no VARCHAR(32),
      status VARCHAR(20),
      amount DECIMAL(10,2),
      created_at DATETIME,
      updated_at DATETIME
    );
    DELETE FROM e2e_orders;
    INSERT INTO e2e_orders (order_no, status, amount, created_at, updated_at) VALUES
      ('ORD-001', 'pending',   99.50,  '2024-03-15 08:30:00', '2024-03-15 08:30:00'),
      ('ORD-002', 'paid',      250.00, '2024-06-20 14:00:00', '2024-06-21 09:15:00'),
      ('ORD-003', 'shipped',   1280.00,'2024-11-01 00:00:00', '2024-11-03 16:45:00'),
      ('ORD-004', 'completed', 45.90,  '2025-01-10 12:00:00', '2025-01-12 18:30:00'),
      ('ORD-005', 'cancelled', 0.00,   '2025-07-04 23:59:59', '2025-07-05 00:00:01');
    """
    subprocess.run(
        mysql_cli_args(),
        input=sql,
        capture_output=True,
        text=True,
        timeout=10,
    )


class TestBasicQuery:
    def test_single_view(self):
        r = run_cli("-c", str(TESTDATA / "e2e_basic.yaml"), "-v", "user_list")
        assert r.returncode == 0, r.stderr
        assert "Alice" in r.stdout
        assert "Bob" in r.stdout
        assert "Charlie" in r.stdout
        assert "用户列表" in r.stdout

    def test_multi_view_parallel(self):
        r = run_cli("-c", str(TESTDATA / "e2e_basic.yaml"))
        assert r.returncode == 0, r.stderr
        assert "Alice" in r.stdout
        assert "用户列表" in r.stdout
        assert "用户搜索" in r.stdout
        assert "禁用视图" not in r.stdout

    def test_empty_result(self):
        r = run_cli("-c", str(TESTDATA / "e2e_empty.yaml"), "-v", "empty_view")
        assert r.returncode == 0, r.stderr
        assert "0 行" in r.stdout


class TestParams:
    def test_default_value(self):
        r = run_cli("-c", str(TESTDATA / "e2e_basic.yaml"), "-v", "user_search")
        assert r.returncode == 0, r.stderr
        assert "Alice" in r.stdout
        assert "Bob" not in r.stdout

    def test_param_override(self):
        r = run_cli(
            "-c", str(TESTDATA / "e2e_basic.yaml"),
            "-v", "user_search",
            "--param", "keyword=Bob",
        )
        assert r.returncode == 0, r.stderr
        assert "Bob" in r.stdout
        assert "Alice" not in r.stdout

    def test_sql_template_parameterized(self):
        r = run_cli(
            "-c", str(TESTDATA / "e2e_basic.yaml"),
            "-v", "user_search",
            "--param", "keyword=Charlie",
        )
        assert r.returncode == 0, r.stderr
        assert "Charlie" in r.stdout
        assert "Alice" not in r.stdout


class TestViewFilter:
    def test_filter_views(self):
        r = run_cli(
            "-c", str(TESTDATA / "e2e_basic.yaml"),
            "-v", "user_list,user_search",
        )
        assert r.returncode == 0, r.stderr
        assert "用户列表" in r.stdout
        assert "用户搜索" in r.stdout

    def test_nonexistent_view(self):
        r = run_cli(
            "-c", str(TESTDATA / "e2e_basic.yaml"),
            "-v", "nonexistent_view",
        )
        assert r.returncode == 1
        assert "not found" in r.stderr


class TestCheck:
    def test_check_missing_defaults(self):
        r = run_cli("check", "-c", str(TESTDATA / "e2e_params.yaml"))
        assert r.returncode == 0
        assert "min_id" in r.stderr
        assert "keyword" in r.stderr

    def test_check_all_ok(self):
        r = run_cli("check", "-c", str(TESTDATA / "e2e_basic.yaml"))
        assert r.returncode == 0
        assert "all views OK" in r.stdout


class TestErrorHandling:
    def test_invalid_yaml_missing_dsn(self):
        r = run_cli("-c", str(TESTDATA / "e2e_invalid.yaml"))
        assert r.returncode == 1
        assert "missing required field" in r.stderr

    def test_unsupported_driver(self):
        r = run_cli("-c", str(TESTDATA / "e2e_bad_driver.yaml"), "-v", "bad_view")
        assert r.returncode == 0
        assert "unsupported driver" in r.stdout or "CONNECT_FAILED" in r.stdout

    def test_config_not_found(self):
        r = run_cli("-c", "/nonexistent/path.yaml")
        assert r.returncode == 1
        assert "not found" in r.stderr


class TestConverter:
    def test_status_enum_to_cn(self):
        r = run_cli("-c", str(TESTDATA / "e2e_converter.yaml"), "-v", "order_list")
        assert r.returncode == 0, r.stderr
        assert "待支付" in r.stdout
        assert "已支付" in r.stdout
        assert "已发货" in r.stdout
        assert "已完成" in r.stdout
        assert "已取消" in r.stdout

    def test_money_format(self):
        r = run_cli("-c", str(TESTDATA / "e2e_converter.yaml"), "-v", "order_list")
        assert r.returncode == 0, r.stderr
        assert "¥99.50" in r.stdout
        assert "¥1,280.00" in r.stdout

    def test_datetime_to_iso(self):
        r = run_cli("-c", str(TESTDATA / "e2e_converter.yaml"), "-v", "order_list")
        assert r.returncode == 0, r.stderr
        assert "2024-03-15" in r.stdout

    def test_summary_converter(self):
        r = run_cli("-c", str(TESTDATA / "e2e_converter.yaml"), "-v", "order_summary")
        assert r.returncode == 0, r.stderr
        assert "待支付" in r.stdout
        assert "¥" in r.stdout


class TestDirectoryLoad:
    def test_load_directory(self, tmp_path):
        (tmp_path / "a.yaml").write_text(
            f'view_a:\n  db_type: "mysql"\n  dsn: "{mysql_dsn()}"\n'
            '  sql: "SELECT 1 AS val"\n  view:\n    enable: true\n'
        )
        (tmp_path / "b.yaml").write_text(
            f'view_b:\n  db_type: "mysql"\n  dsn: "{mysql_dsn()}"\n'
            '  sql: "SELECT 2 AS val"\n  view:\n    enable: true\n'
        )
        r = run_cli("-c", str(tmp_path))
        assert r.returncode == 0, r.stderr
        assert "view_a" in r.stdout
        assert "view_b" in r.stdout

    def test_duplicate_key_warning(self, tmp_path):
        content = f'dup:\n  db_type: "mysql"\n  dsn: "{mysql_dsn()}"\n  sql: "SELECT 1"\n  view:\n    enable: true\n'
        (tmp_path / "a.yaml").write_text(content)
        (tmp_path / "b.yaml").write_text(content)
        r = run_cli("-c", str(tmp_path))
        assert "duplicate view key" in r.stderr


class TestMultiDB:
    def test_postgres_query(self):
        r = run_cli("-c", str(TESTDATA / "e2e_multi_db.yaml"), "-v", "pg_users")
        assert r.returncode == 0, r.stderr
        assert "Alice" in r.stdout
        assert "Bob" in r.stdout
        assert "Charlie" in r.stdout
        assert "PG用户" in r.stdout

    def test_oracle_query(self):
        if not oracle_dsn():
            pytest.skip("YAMLQ_TEST_ORACLE_DSN not set (no local Oracle)")
        r = run_cli("-c", str(TESTDATA / "e2e_multi_db.yaml"), "-v", "oracle_users")
        assert r.returncode == 0, r.stderr
        assert "Alice" in r.stdout
        assert "Oracle用户" in r.stdout

    def test_cross_db_all_views(self):
        if not oracle_dsn():
            pytest.skip("YAMLQ_TEST_ORACLE_DSN not set (no local Oracle)")
        r = run_cli("-c", str(TESTDATA / "e2e_multi_db.yaml"))
        assert r.returncode == 0, r.stderr
        assert "MySQL用户" in r.stdout
        assert "PG用户" in r.stdout
        assert "Oracle用户" in r.stdout

    def test_partial_failure(self):
        r = run_cli("-c", str(TESTDATA / "e2e_multi_db.yaml"))
        assert r.returncode == 0
        assert "Alice" in r.stdout


class TestThemeAndLayout:
    def test_title_left_aligned_with_datasource(self):
        r = run_cli("-c", str(TESTDATA / "e2e_basic.yaml"), "-v", "user_list")
        assert r.returncode == 0, r.stderr
        assert "用户列表" in r.stdout
        assert "@mysql+" in r.stdout

    def test_theme_applied(self):
        r = run_cli("-c", str(TESTDATA / "e2e_basic.yaml"), "-v", "user_list")
        assert r.returncode == 0, r.stderr
        assert "用户列表" in r.stdout

    def test_column_width_config(self):
        r = run_cli("-c", str(TESTDATA / "e2e_basic.yaml"), "-v", "user_list")
        assert r.returncode == 0, r.stderr
        assert "ID" in r.stdout
        assert "姓名" in r.stdout
        assert "邮箱" in r.stdout

    def test_converter_theme_combined(self):
        r = run_cli("-c", str(TESTDATA / "e2e_converter.yaml"), "-v", "order_list")
        assert r.returncode == 0, r.stderr
        assert "待支付" in r.stdout
        assert "¥99.50" in r.stdout
        assert "订单列表" in r.stdout


class TestOpenGauss:
    @pytest.fixture(autouse=True)
    def _require_og_dsn(self):
        if not opengauss_dsn():
            pytest.skip("YAMLQ_TEST_OPENGAUSS_DSN not set")

    def test_opengauss_query(self):
        r = run_cli("-c", str(TESTDATA / "e2e_multi_db.yaml"), "-v", "og_users")
        assert r.returncode == 0, r.stderr
        assert "Alice" in r.stdout
        assert "Bob" in r.stdout
        assert "OG用户" in r.stdout


def _wait_for_daemon_url(cwd: Path, timeout: float = 20.0) -> str | None:
    env_file = cwd / ".yamlq-gateway.env"
    deadline = time.time() + timeout
    while time.time() < deadline:
        if env_file.exists():
            for line in env_file.read_text().splitlines():
                if line.startswith("YAMLQ_GATEWAY_URL="):
                    return line.split("=", 1)[1].strip()
        time.sleep(0.2)
    return None


class TestDaemonReuse:
    def test_two_runs_attach_to_same_daemon(self, tmp_path):
        home = tmp_path / "home"
        home.mkdir()
        daemon_env = {
            **os.environ,
            "USERPROFILE": str(home),
            "HOME": str(home),
        }
        serve_proc = subprocess.Popen(
            [sys.executable, "-m", "yamlq", "serve"],
            cwd=str(tmp_path),
            env=daemon_env,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        try:
            url = _wait_for_daemon_url(tmp_path)
            assert url, "daemon did not write its discovery file in time"

            run_env = {**os.environ, "YAMLQ_GATEWAY_URL": url}
            r1 = run_cli("-c", str(TESTDATA / "e2e_basic.yaml"), "-v", "user_list", env=run_env)
            assert r1.returncode == 0, r1.stderr
            assert "Alice" in r1.stdout

            # The attach run must not tear the daemon down.
            assert serve_proc.poll() is None

            r2 = run_cli("-c", str(TESTDATA / "e2e_basic.yaml"), "-v", "user_list", env=run_env)
            assert r2.returncode == 0, r2.stderr
            assert "Alice" in r2.stdout
            assert serve_proc.poll() is None

            # The pool created by run 1 is reused verbatim: the now-idempotent
            # /connect returns the same conn_id the run just used.
            a = requests.post(f"{url}/connect", json={"driver": "mysql", "dsn": mysql_dsn()}, timeout=10).json()
            b = requests.post(f"{url}/connect", json={"driver": "mysql", "dsn": mysql_dsn()}, timeout=10).json()
            assert not a.get("error") and not b.get("error"), (a, b)
            assert a["conn_id"] == b["conn_id"]
        finally:
            try:
                requests.post(f"{url}/shutdown", timeout=5)
            except Exception:
                pass
            try:
                serve_proc.wait(timeout=10)
            except Exception:
                serve_proc.kill()


class TestOceanBase:
    @pytest.fixture(autouse=True)
    def _require_ob(self):
        if not ob_mysql_dsn() and not ob_oracle_dsn():
            pytest.skip("YAMLQ_TEST_OB_MYSQL_DSN / YAMLQ_TEST_OB_ORACLE_DSN not set")

    def test_ob_mysql_user_list(self):
        r = run_cli("-c", str(TESTDATA / "ob_basic.yaml"), "-v", "ob_user_list")
        assert r.returncode == 0, r.stderr
        assert "OB 用户列表" in r.stdout
        assert "Alice" in r.stdout
        assert "已支付" in r.stdout
        assert "2026-01-05" in r.stdout

    def test_ob_mysql_user_list_param(self):
        r = run_cli(
            "-c", str(TESTDATA / "ob_basic.yaml"),
            "-v", "ob_user_list",
            "--param", "keyword=Bob",
        )
        assert r.returncode == 0, r.stderr
        assert "Bob" in r.stdout
        assert "Alice" not in r.stdout

    def test_ob_mysql_order_stats(self):
        r = run_cli("-c", str(TESTDATA / "ob_basic.yaml"), "-v", "ob_order_stats")
        assert r.returncode == 0, r.stderr
        assert "OB 订单统计" in r.stdout
        assert "¥" in r.stdout
        assert "1,280" in r.stdout

    def test_ob_oracle_emp_list(self):
        if not ob_oracle_dsn():
            pytest.skip("YAMLQ_TEST_OB_ORACLE_DSN not set")
        r = run_cli("-c", str(TESTDATA / "ob_oracle.yaml"), "-v", "ob_emp_list")
        assert r.returncode == 0, r.stderr
        assert "OB 员工列表" in r.stdout
        assert "SMITH" in r.stdout
        assert "1980年12月17日" in r.stdout

    def test_ob_oracle_dept_summary(self):
        if not ob_oracle_dsn():
            pytest.skip("YAMLQ_TEST_OB_ORACLE_DSN not set")
        r = run_cli("-c", str(TESTDATA / "ob_oracle.yaml"), "-v", "ob_dept_summary")
        assert r.returncode == 0, r.stderr
        assert "OB 部门汇总" in r.stdout
        assert "RESEARCH" in r.stdout
        assert "¥3,775.00" in r.stdout
