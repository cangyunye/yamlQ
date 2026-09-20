from __future__ import annotations

import subprocess
import sys
from pathlib import Path

import pytest

REPO = Path(__file__).resolve().parent.parent.parent
TESTDATA = REPO / "testdata"
ORACLE_SEED_SCOTT = REPO / "oracle" / "seed_scott.sql"
ORACLE_SEED_APPUSER = REPO / "oracle" / "seed_appuser.sql"
CLI_DIR = REPO / "cli"

SCOTT_DSN = "scott/tiger@//localhost:1521/XEPDB1"
APPUSER_DSN = "appuser/App123!@//localhost:1521/XEPDB1"


def _oracle_reachable() -> bool:
    probe = "SELECT 1 FROM dual;"
    try:
        r = subprocess.run(
            ["docker", "exec", "-i", "oracle", "sqlplus", "-s", SCOTT_DSN],
            input=probe.encode(),
            capture_output=True,
            timeout=15,
        )
    except (FileNotFoundError, subprocess.TimeoutExpired):
        return False
    return r.returncode == 0 and b"1" in r.stdout


@pytest.fixture(scope="module", autouse=True)
def seed_oracle():
    if not _oracle_reachable():
        pytest.skip("oracle container (docker exec) not reachable — skipping Oracle E2E")
    for seed_file, dsn in ((ORACLE_SEED_SCOTT, SCOTT_DSN), (ORACLE_SEED_APPUSER, APPUSER_DSN)):
        if not seed_file.is_file():
            pytest.fail(f"missing seed file: {seed_file}")
        r = subprocess.run(
            ["docker", "exec", "-i", "oracle", "sqlplus", "-s", dsn],
            input=seed_file.read_bytes(),
            capture_output=True,
            timeout=120,
        )
        assert r.returncode == 0, r.stderr.decode(errors="replace")


def run_cli(*args: str, timeout: int = 90) -> subprocess.CompletedProcess:
    return subprocess.run(
        [sys.executable, "-m", "yamlq", *args],
        capture_output=True,
        text=True,
        timeout=timeout,
        cwd=str(CLI_DIR),
    )


def demo(*views: str, extra: tuple = ()) -> subprocess.CompletedProcess:
    argv = ["-c", str(TESTDATA / "oracle_demo.yaml")]
    if views:
        argv += ["-v", ",".join(views)]
    return run_cli(*argv, *extra)


class TestOracleEnumMap:
    """YAML enum_map 翻译: 字符串枚举(job/status/priority) + 数值枚举(active)"""

    def test_emp_job_and_dept_mapped(self):
        r = demo("emp_list")
        assert r.returncode == 0, r.stderr
        for label in ("经理", "总裁", "分析师", "销售", "文员"):
            assert label in r.stdout
        for dept in ("财务部", "研发部", "销售部"):
            assert dept in r.stdout
        assert "SALESMAN" not in r.stdout

    def test_product_numeric_active_mapped(self):
        r = demo("product_list")
        assert r.returncode == 0, r.stderr
        assert "在售" in r.stdout
        assert "停售" in r.stdout
        assert "台式整机" in r.stdout
        assert "外设" in r.stdout
        assert "生活电器" in r.stdout
        # 数值枚举原值不再以 0/1 出现在 上架 列
        assert "\n│   1" in r.stdout and "在售" in r.stdout

    def test_order_status_mapped_aggregate_and_list(self):
        r = demo("order_list", "order_summary")
        assert r.returncode == 0, r.stderr
        for label in ("已支付", "已发货", "待支付", "已完成", "已取消"):
            assert label in r.stdout
        assert "PENDING" not in r.stdout
        assert "¥3,302.00" in r.stdout  # PAID 总额 658+745+1899

    def test_task_two_enum_fields(self):
        r = demo("task_board")
        assert r.returncode == 0, r.stderr
        for label in ("高", "中", "低", "待办", "进行中", "已完成", "已阻塞"):
            assert label in r.stdout
        assert "BLOCKED" not in r.stdout


class TestOracleConvertersAndWidth:
    """money / datetime 转换 + max_width+fold 终端宽度处理"""

    def test_money_format(self):
        r = demo("emp_list")
        assert r.returncode == 0, r.stderr
        assert "¥5,000.00" in r.stdout
        assert "¥800.00" in r.stdout

    def test_datetime_to_cn_full_date(self):
        r = demo("emp_list")
        assert r.returncode == 0, r.stderr
        assert "1981年02月20日" in r.stdout  # ALLEN
        assert "1987年04月19日" in r.stdout  # SCOTT

    def test_datetime_to_iso(self):
        r = demo("task_board")
        assert r.returncode == 0, r.stderr
        assert "2025-08-05" in r.stdout
        assert "2025-10-01" in r.stdout

    def test_wide_text_folds_not_truncates(self):
        """备注列 max_width+fold: 首尾两个词都必须完整出现(折行而非省略号截断)"""
        r = demo("product_list")
        assert r.returncode == 0, r.stderr
        assert "国AA级照度" in r.stdout
        assert "色温三档" in r.stdout
        assert "ΔE<2" in r.stdout  # product 2 最后一段也保留
        assert "…" not in r.stdout

    def test_long_title_wrapped_without_loss(self):
        r = demo("task_board")
        assert r.returncode == 0, r.stderr
        assert "表格过宽时按列宽折行," in r.stdout
        assert "避免截断关键信息" in r.stdout


class TestOracleParams:
    def test_default_keyword(self):
        r = demo("emp_search")
        assert r.returncode == 0, r.stderr
        assert "CLARK" in r.stdout
        assert "KING" in r.stdout
        assert "SMITH" not in r.stdout

    def test_param_override(self):
        r = demo("emp_search", extra=("--param", "keyword=O"))
        assert r.returncode == 0, r.stderr
        assert "JONES" in r.stdout
        assert "CLARK" not in r.stdout
        assert "KING" not in r.stdout


class TestOracleAllViews:
    def test_all_views_run_together(self):
        r = demo()
        assert r.returncode == 0, r.stderr
        for marker in ("员工列表", "部门汇总", "商品清单", "订单列表", "订单状态汇总", "任务看板", "员工搜索"):
            assert marker in r.stdout
        assert "✗" not in r.stdout
        assert "error" not in r.stderr.lower()

    def test_title_shows_database_marker_not_credentials(self):
        r = demo("emp_list")
        assert "@oracle+127.0.0.1:1521/XEPDB1" in r.stdout
        assert "scott:tiger" not in r.stdout
        assert "tiger@" not in r.stdout

    def test_column_headers_chinese(self):
        r = demo("task_board")
        for h in ("任务", "负责人", "优先级", "状态", "截止日期"):
            assert h in r.stdout
