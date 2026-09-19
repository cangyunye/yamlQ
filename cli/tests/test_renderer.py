from __future__ import annotations

from yamlq import renderer
from yamlq.parser import ColumnConfig, ViewConfig


def _view(columns: list[ColumnConfig]) -> ViewConfig:
    return ViewConfig(
        key="v", db_type="ob-oracle", dsn="dsn://x", sql="SELECT 1",
        columns=columns,
    )


def _render(view: ViewConfig, result: dict) -> str:
    with renderer.console.capture() as capture:
        renderer.render_table(view, result)
    return capture.get()


class TestColumnCaseMatching:
    def test_uppercase_result_matches_lowercase_config(self):
        # Oracle-style backends report uppercased identifiers.
        view = _view([ColumnConfig(field="sal", header="薪资", converter="money_format")])
        out = _render(view, {"columns": ["SAL"], "rows": [[3775]]})
        assert "薪资" in out
        assert "¥3,775.00" in out

    def test_exact_match_still_wins(self):
        view = _view([
            ColumnConfig(field="sal", header="小写"),
            ColumnConfig(field="SAL", header="大写"),
        ])
        out = _render(view, {"columns": ["SAL"], "rows": [[1]]})
        assert "大写" in out
        assert "小写" not in out

    def test_unmatched_column_falls_back_to_field_name(self):
        view = _view([ColumnConfig(field="other", header="无关")])
        out = _render(view, {"columns": ["SAL"], "rows": [[1]]})
        assert "SAL" in out
        assert "无关" not in out
