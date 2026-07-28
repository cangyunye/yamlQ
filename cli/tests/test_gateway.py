from __future__ import annotations

import os
import subprocess
from pathlib import Path
from unittest import mock

import requests
import pytest
from yamlq.gateway import Gateway, GatewayError, _find_existing_gateway, _load_gateway_env


class TestLoadGatewayEnv:
    def test_returns_empty_when_no_file(self, monkeypatch, tmp_path):
        monkeypatch.chdir(tmp_path)
        assert _load_gateway_env() == {}

    def test_parses_file_correctly(self, monkeypatch, tmp_path):
        monkeypatch.chdir(tmp_path)
        (tmp_path / ".yamlq-gateway.env").write_text(
            "YAMLQ_GATEWAY_URL=http://127.0.0.1:9999\nYAMLQ_GATEWAY_AUTH=token123\n"
        )
        result = _load_gateway_env()
        assert result == {
            "YAMLQ_GATEWAY_URL": "http://127.0.0.1:9999",
            "YAMLQ_GATEWAY_AUTH": "token123",
        }

    def test_skips_lines_without_equals(self, monkeypatch, tmp_path):
        monkeypatch.chdir(tmp_path)
        (tmp_path / ".yamlq-gateway.env").write_text(
            "YAMLQ_GATEWAY_URL=http://127.0.0.1:9999\ngarbage\n"
        )
        result = _load_gateway_env()
        assert result == {"YAMLQ_GATEWAY_URL": "http://127.0.0.1:9999"}


class TestFindExistingGateway:
    def test_returns_none_when_no_env_var_or_file(self, monkeypatch, tmp_path):
        monkeypatch.chdir(tmp_path)
        monkeypatch.delenv("YAMLQ_GATEWAY_URL", raising=False)
        assert _find_existing_gateway() is None

    def test_reads_from_env_var(self, monkeypatch):
        monkeypatch.setenv("YAMLQ_GATEWAY_URL", "http://127.0.0.1:8888")
        monkeypatch.delenv("YAMLQ_GATEWAY_AUTH", raising=False)
        result = _find_existing_gateway()
        assert result == {"url": "http://127.0.0.1:8888", "auth": ""}

    def test_reads_from_env_var_with_auth(self, monkeypatch):
        monkeypatch.setenv("YAMLQ_GATEWAY_URL", "http://127.0.0.1:8888")
        monkeypatch.setenv("YAMLQ_GATEWAY_AUTH", "secret123")
        result = _find_existing_gateway()
        assert result == {"url": "http://127.0.0.1:8888", "auth": "secret123"}

    def test_reads_from_dot_env_file(self, monkeypatch, tmp_path):
        monkeypatch.chdir(tmp_path)
        monkeypatch.delenv("YAMLQ_GATEWAY_URL", raising=False)
        monkeypatch.delenv("YAMLQ_GATEWAY_AUTH", raising=False)
        (tmp_path / ".yamlq-gateway.env").write_text(
            "YAMLQ_GATEWAY_URL=http://127.0.0.1:7777\nYAMLQ_GATEWAY_AUTH=file_token\n"
        )
        result = _find_existing_gateway()
        assert result == {"url": "http://127.0.0.1:7777", "auth": "file_token"}

    def test_env_var_takes_precedence_over_file(self, monkeypatch, tmp_path):
        monkeypatch.chdir(tmp_path)
        monkeypatch.setenv("YAMLQ_GATEWAY_URL", "http://127.0.0.1:6666")
        monkeypatch.setenv("YAMLQ_GATEWAY_AUTH", "env_token")
        (tmp_path / ".yamlq-gateway.env").write_text(
            "YAMLQ_GATEWAY_URL=http://127.0.0.1:7777\nYAMLQ_GATEWAY_AUTH=file_token\n"
        )
        result = _find_existing_gateway()
        assert result == {"url": "http://127.0.0.1:6666", "auth": "env_token"}


class TestGatewayStart:
    def test_reuses_existing_gateway_when_alive(self):
        gw = Gateway()
        with (
            mock.patch("yamlq.gateway._find_existing_gateway") as mock_find,
            mock.patch.object(gw._session, "get") as mock_get,
        ):
            mock_find.return_value = {"url": "http://127.0.0.1:5555", "auth": "test_token"}
            mock_response = mock.Mock()
            mock_response.raise_for_status.return_value = None
            mock_get.return_value = mock_response

            gw.start()

            assert gw._base_url == "http://127.0.0.1:5555"
            assert gw._auth_token == "test_token"
            assert gw._owned is False
            assert gw._proc is None
            mock_get.assert_called_once_with("http://127.0.0.1:5555/ping", timeout=2)

    def test_raises_on_auth_token_mismatch(self):
        gw = Gateway(auth_token="cli_token")
        with mock.patch("yamlq.gateway._find_existing_gateway") as mock_find:
            mock_find.return_value = {"url": "http://127.0.0.1:5555", "auth": "other_token"}
            with pytest.raises(GatewayError, match="auth token mismatch"):
                gw.start()

    def test_falls_through_when_existing_gateway_stale(self):
        gw = Gateway()
        with (
            mock.patch("yamlq.gateway._find_existing_gateway") as mock_find,
            mock.patch.object(gw._session, "get") as mock_get,
            mock.patch.object(gw, "_spawn_gateway") as mock_spawn,
        ):
            mock_find.return_value = {"url": "http://127.0.0.1:5555", "auth": "test_token"}
            mock_get.side_effect = requests.RequestException("connection refused")

            gw.start()

            mock_get.assert_called_once_with("http://127.0.0.1:5555/ping", timeout=2)
            mock_spawn.assert_called_once()
            assert gw._owned is True

    def test_no_existing_calls_spawn(self):
        gw = Gateway()
        with (
            mock.patch("yamlq.gateway._find_existing_gateway") as mock_find,
            mock.patch.object(gw, "_spawn_gateway") as mock_spawn,
        ):
            mock_find.return_value = None
            gw.start()
            mock_spawn.assert_called_once()


class TestGatewayStop:
    def test_does_not_kill_non_owned_gateway(self):
        gw = Gateway()
        gw._owned = False
        gw._proc = mock.Mock(spec=subprocess.Popen)
        gw._base_url = "http://127.0.0.1:5555"

        with mock.patch.object(gw._session, "post") as mock_post:
            gw.stop()
            mock_post.assert_not_called()
            gw._proc.wait.assert_not_called()

    def test_kills_owned_gateway(self):
        gw = Gateway()
        gw._owned = True
        proc = mock.Mock(spec=subprocess.Popen)
        gw._proc = proc
        gw._base_url = "http://127.0.0.1:5555"

        with mock.patch.object(gw._session, "post") as mock_post:
            gw.stop()
            mock_post.assert_called_once()
            proc.wait.assert_called_once_with(timeout=5)

    def test_stop_noop_when_proc_none(self):
        gw = Gateway()
        gw._owned = True
        gw._proc = None
        gw.stop()


class TestDaemonMode:
    def test_spawns_with_daemon_flag(self, monkeypatch):
        monkeypatch.setenv("YAMLQ_GATEWAY_DAEMON", "1")
        gw = Gateway()
        with (
            mock.patch("yamlq.gateway._find_binary") as mock_binary,
            mock.patch("yamlq.gateway.subprocess.Popen") as mock_popen,
        ):
            mock_binary.return_value = "/fake/db-gateway"
            proc = mock.MagicMock()
            proc.stdout.readline.return_value = "listening on port 9999"
            proc.pid = 12345
            mock_popen.return_value = proc

            gw.start()

            cmd = mock_popen.call_args[0][0]
            assert "--daemon" in cmd
            assert gw._owned is False

    def test_daemon_flag_not_present_by_default(self, monkeypatch):
        monkeypatch.delenv("YAMLQ_GATEWAY_DAEMON", raising=False)
        gw = Gateway()
        with (
            mock.patch("yamlq.gateway._find_binary") as mock_binary,
            mock.patch("yamlq.gateway.subprocess.Popen") as mock_popen,
        ):
            mock_binary.return_value = "/fake/db-gateway"
            proc = mock.MagicMock()
            proc.stdout.readline.return_value = "listening on port 9999"
            proc.pid = 12346
            mock_popen.return_value = proc

            gw.start()

            cmd = mock_popen.call_args[0][0]
            assert "--daemon" not in cmd
            assert gw._owned is True
