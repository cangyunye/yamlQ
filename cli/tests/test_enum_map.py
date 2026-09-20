from __future__ import annotations

from pathlib import Path

import pytest
import yaml

from yamlq.converters import format_cell
from yamlq.parser import ColumnConfig, ConfigError, load_views


def _view_yaml(column_body: str) -> Path:
    tmp = Path(__file__).resolve().parent / "tmp_enum_map.yaml"
    tmp.write_text(
        "v:\n"
        "  db_type: mysql\n"
        "  dsn: root@tcp(127.0.0.1:3306)/db\n"
        "  sql: SELECT 1\n"
        "  view:\n"
        "    columns:\n"
        f"      {column_body}\n",
        encoding="utf-8",
    )
    return tmp


class TestParserEnumMap:
    def test_parses_string_keys(self, tmp_path):
        cfg = _view_yaml('- field: status\n        header: 状态\n        enum_map:\n          PENDING: 待支付\n          PAID: 已支付')
        try:
            view = load_views(str(cfg))[0]
        finally:
            cfg.unlink(missing_ok=True)
        col = view.columns[0]
        assert col.enum_map == {"PENDING": "待支付", "PAID": "已支付"}

    def test_numeric_keys_become_strings(self, tmp_path):
        cfg = _view_yaml('- field: active\n        enum_map:\n          0: 停售\n          1: 在售')
        try:
            view = load_views(str(cfg))[0]
        finally:
            cfg.unlink(missing_ok=True)
        assert view.columns[0].enum_map == {"0": "停售", "1": "在售"}

    def test_empty_mapping_is_fine(self, tmp_path):
        cfg = _view_yaml('- field: x\n        enum_map: {}')
        try:
            view = load_views(str(cfg))[0]
        finally:
            cfg.unlink(missing_ok=True)
        assert view.columns[0].enum_map == {}

    def test_non_mapping_enum_map_rejected(self, tmp_path):
        cfg = _view_yaml('- field: x\n        enum_map: [a, b]')
        try:
            with pytest.raises(ConfigError, match="enum_map must be a mapping"):
                load_views(str(cfg))
        finally:
            cfg.unlink(missing_ok=True)

    def test_defaults_when_absent(self):
        col = ColumnConfig(field="x")
        assert col.enum_map == {}
        assert col.converter == ""


class TestFormatCell:
    MAP = {"PENDING": "待支付", "PAID": "已支付", "0": "停售", "1": "在售"}

    def test_maps_string_value(self):
        assert format_cell("PAID", enum_map=self.MAP) == "已支付"

    def test_unmapped_falls_back_to_raw(self):
        assert format_cell("REFUNDED", enum_map=self.MAP) == "REFUNDED"

    def test_numeric_value_matches_string_key(self):
        assert format_cell(1, enum_map=self.MAP) == "在售"
        assert format_cell(1.0, enum_map=self.MAP) == "在售"
        assert format_cell("1", enum_map=self.MAP) == "在售"
        assert format_cell(0, enum_map=self.MAP) == "停售"

    def test_none_is_empty(self):
        assert format_cell(None, enum_map=self.MAP) == ""

    def test_enum_map_wins_over_named_converter(self):
        assert format_cell("PAID", converter="status_to_cn", enum_map=self.MAP) == "已支付"

    def test_converter_still_applies_without_enum_map(self):
        assert format_cell("pending", converter="status_to_cn") == "待支付"
        assert format_cell(99.5, converter="money_format") == "¥99.50"

    def test_raw_value_when_no_config(self):
        assert format_cell(None) == ""
        assert format_cell("abc") == "abc"

    def test_datetime_cn_iso_string(self):
        assert format_cell("1981-02-20T00:00:00Z", converter="datetime_to_cn") == "1981年02月20日 00:00"
        assert format_cell("2025-01-06T10:12:33.000000", converter="datetime_to_cn") == "2025年01月06日 10:12"
        assert format_cell("2024-03-15 08:30:00", converter="datetime_to_cn") == "2024年03月15日 08:30"

    def test_datetime_iso_iso_string(self):
        assert format_cell("1980-12-17T00:00:00Z", converter="datetime_to_iso") == "1980-12-17"

    def test_unparseable_datetime_string_passthrough(self):
        assert format_cell("not-a-date", converter="datetime_to_cn") == "not-a-date"
