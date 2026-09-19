from __future__ import annotations

from unittest import mock

import pytest

from yamlq.cli import _ConnHandle, _RETRYABLE_ERROR_CODES, _run_view
from yamlq.gateway import Gateway, GatewayError
from yamlq.parser import ViewConfig


def _view() -> ViewConfig:
    return ViewConfig(key="v", db_type="mysql", dsn="dsn://x", sql="SELECT 1")


def _handle(conn_id: str = "conn_1") -> _ConnHandle:
    gw = mock.Mock(spec=Gateway)
    gw.connect.return_value = conn_id
    return _ConnHandle(gw, "mysql", "dsn://x")


class TestConnHandle:
    def test_connects_lazily_once(self):
        gw = mock.Mock(spec=Gateway)
        gw.connect.return_value = "conn_9"
        h = _ConnHandle(gw, "mysql", "dsn://x")
        assert h.get() == "conn_9"
        assert h.get() == "conn_9"
        gw.connect.assert_called_once_with("mysql", "dsn://x")

    def test_refresh_reconnects(self):
        gw = mock.Mock(spec=Gateway)
        gw.connect.side_effect = ["conn_1", "conn_2"]
        h = _ConnHandle(gw, "mysql", "dsn://x")
        assert h.get() == "conn_1"
        assert h.get(refresh=True) == "conn_2"
        assert gw.connect.call_count == 2


class TestRunViewSelfHeal:
    def test_retries_once_on_connection_lost(self):
        h = _handle()
        gw = h._gw
        gw.query.side_effect = [
            {"error": {"code": "CONNECTION_LOST", "message": "bad conn"}},
            {"columns": ["id"], "rows": [[1]]},
        ]

        view, result = _run_view(gw, _view(), h, {}, timeout=30)

        assert result == {"columns": ["id"], "rows": [[1]]}
        assert gw.query.call_count == 2
        assert gw.query.call_args_list[0][0][0] == "conn_1"
        assert gw.query.call_args_list[1][0][0] == "conn_1"  # refreshed in place
        h._gw.connect.assert_called_with("mysql", "dsn://x")

    def test_retries_on_connection_not_found(self):
        h = _handle()
        gw = h._gw
        gw.query.side_effect = [
            {"error": {"code": "CONNECTION_NOT_FOUND", "message": "reaped"}},
            {"columns": [], "rows": []},
        ]

        _, result = _run_view(gw, _view(), h, {}, timeout=30)

        assert result == {"columns": [], "rows": []}
        assert gw.query.call_count == 2

    def test_no_retry_on_db_error(self):
        h = _handle()
        gw = h._gw
        gw.query.return_value = {"error": {"code": "DB_ERROR", "message": "syntax"}}

        _, result = _run_view(gw, _view(), h, {}, timeout=30)

        assert result["error"]["code"] == "DB_ERROR"
        assert gw.query.call_count == 1

    def test_keeps_original_error_when_reconnect_fails(self):
        h = _handle()
        gw = h._gw
        lost = {"error": {"code": "CONNECTION_LOST", "message": "bad conn"}}
        gw.query.side_effect = [lost, GatewayError("connect failed: down")]

        _, result = _run_view(gw, _view(), h, {}, timeout=30)

        assert result == lost
        assert gw.query.call_count == 2


def test_retryable_codes_membership():
    assert "CONNECTION_LOST" in _RETRYABLE_ERROR_CODES
    assert "CONNECTION_NOT_FOUND" in _RETRYABLE_ERROR_CODES
    assert "DB_ERROR" not in _RETRYABLE_ERROR_CODES
