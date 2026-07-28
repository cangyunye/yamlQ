from __future__ import annotations

import atexit
import os
import shutil
import signal
import subprocess
import sys
import time
from pathlib import Path

import requests


class GatewayError(Exception):
    pass


def _find_binary() -> str:
    candidate = Path(__file__).resolve().parent.parent.parent / "db-gateway" / "db-gateway"
    if candidate.is_file():
        return str(candidate)
    found = shutil.which("db-gateway")
    if found:
        return found
    raise GatewayError(
        "db-gateway binary not found. Build it first: cd db-gateway && go build -o db-gateway ."
    )


class Gateway:
    def __init__(self, mode: str = "cli", auth_token: str = "", verbose: bool = False):
        self._mode = mode
        self._auth_token = auth_token
        self._verbose = verbose
        self._proc: subprocess.Popen | None = None
        self._base_url = ""
        self._session = requests.Session()
        if auth_token:
            self._session.headers["X-Auth-Token"] = auth_token

    def start(self) -> None:
        binary = _find_binary()
        cmd = [binary, f"--mode={self._mode}"]
        if self._auth_token:
            cmd.append(f"--auth-token={self._auth_token}")

        self._proc = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE if not self._verbose else None,
            text=True,
        )
        atexit.register(self.stop)

        line = self._proc.stdout.readline().strip()
        if "listening on port" not in line:
            stderr = self._proc.stderr.read() if self._proc.stderr else ""
            raise GatewayError(f"db-gateway failed to start: {line} {stderr}")

        port = int(line.split("port")[-1].strip())
        self._base_url = f"http://127.0.0.1:{port}"
        if self._verbose:
            print(f"[gateway] started on port {port}, pid={self._proc.pid}", file=sys.stderr)

    def stop(self) -> None:
        if self._proc is None:
            return
        try:
            self._session.post(f"{self._base_url}/shutdown", timeout=3)
        except Exception:
            pass
        try:
            self._proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self._proc.kill()
        self._proc = None

    def _url(self, path: str) -> str:
        return f"{self._base_url}{path}"

    def connect(self, driver: str, dsn: str, **pool_kwargs) -> str:
        body = {"driver": driver, "dsn": dsn, **pool_kwargs}
        resp = self._session.post(self._url("/connect"), json=body, timeout=30)
        data = resp.json()
        if data.get("error"):
            raise GatewayError(f"connect failed: {data['error']}")
        return data["conn_id"]

    def query(
        self,
        conn_id: str,
        sql: str,
        params: list | None = None,
        page: int = 0,
        page_size: int = 0,
        timeout: int = 30,
    ) -> dict:
        body = {
            "conn_id": conn_id,
            "sql": sql,
            "params": params or [],
            "page": page,
            "page_size": page_size,
            "timeout": timeout,
        }
        resp = self._session.post(self._url("/query"), json=body, timeout=timeout + 10)
        return resp.json()

    def close(self, conn_id: str) -> None:
        self._session.post(self._url("/close"), json={"conn_id": conn_id}, timeout=5)

    def ping(self, conn_id: str) -> dict:
        resp = self._session.get(self._url("/ping"), params={"conn_id": conn_id}, timeout=5)
        return resp.json()
