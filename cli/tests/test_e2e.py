from __future__ import annotations

import subprocess
import sys
from pathlib import Path

import pytest

TESTDATA = Path(__file__).resolve().parent.parent.parent / "testdata"
CLI_DIR = Path(__file__).resolve().parent.parent


def run_cli(*args: str, input_text: str = "", timeout: int = 60) -> subprocess.CompletedProcess:
    cmd = [sys.executable, "-m", "yamlq", *args]
    return subprocess.run(
        cmd,
        capture_output=True,
        text=True,
        input=input_text,
        timeout=timeout,
        cwd=str(CLI_DIR),
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
        ["mysql", "-h", "127.0.0.1", "-u", "root", "-proot123456", "default_db"],
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
            'view_a:\n  db_type: "mysql"\n  dsn: "root:root123456@tcp(127.0.0.1:3306)/default_db"\n'
            '  sql: "SELECT 1 AS val"\n  view:\n    enable: true\n'
        )
        (tmp_path / "b.yaml").write_text(
            'view_b:\n  db_type: "mysql"\n  dsn: "root:root123456@tcp(127.0.0.1:3306)/default_db"\n'
            '  sql: "SELECT 2 AS val"\n  view:\n    enable: true\n'
        )
        r = run_cli("-c", str(tmp_path))
        assert r.returncode == 0, r.stderr
        assert "view_a" in r.stdout
        assert "view_b" in r.stdout

    def test_duplicate_key_warning(self, tmp_path):
        content = 'dup:\n  db_type: "mysql"\n  dsn: "root:root123456@tcp(127.0.0.1:3306)/default_db"\n  sql: "SELECT 1"\n  view:\n    enable: true\n'
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
        r = run_cli("-c", str(TESTDATA / "e2e_multi_db.yaml"), "-v", "oracle_users")
        assert r.returncode == 0, r.stderr
        assert "Alice" in r.stdout
        assert "Oracle用户" in r.stdout

    def test_cross_db_all_views(self):
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
