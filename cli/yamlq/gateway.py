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


GATEWAY_ENV_FILE = ".yamlq-gateway.env"
HOME_GATEWAY_ENV_FILE = Path.home() / ".yamlq" / "gateway.env"


def _load_gateway_env() -> dict[str, str]:
    # CWD-level discovery first (project-scoped daemon), then the user-wide
    # file that `yamlq serve` writes under ~/.yamlq/.
    for env_file in (Path.cwd() / GATEWAY_ENV_FILE, HOME_GATEWAY_ENV_FILE):
        if env_file.exists():
            result = {}
            for line in env_file.read_text().strip().splitlines():
                if "=" in line:
                    k, v = line.split("=", 1)
                    result[k] = v
            return result
    return {}


def _find_existing_gateway() -> dict | None:
    url = os.environ.get("YAMLQ_GATEWAY_URL")
    if url:
        auth = os.environ.get("YAMLQ_GATEWAY_AUTH", "")
        return {"url": url, "auth": auth}
    env = _load_gateway_env()
    if "YAMLQ_GATEWAY_URL" in env:
        return {"url": env["YAMLQ_GATEWAY_URL"], "auth": env.get("YAMLQ_GATEWAY_AUTH", "")}
    return None


def _find_binary() -> str:
    base = Path(__file__).resolve().parent.parent.parent / "db-gateway"
    for name in ("db-gateway", "db-gateway.exe"):
        candidate = base / name
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
        self._owned = True
        self._session = requests.Session()
        if auth_token:
            self._session.headers["X-Auth-Token"] = auth_token

    def start(self) -> None:
        existing = _find_existing_gateway()
        if existing:
            if self._auth_token and self._auth_token != existing["auth"]:
                raise GatewayError(
                    f"auth token mismatch: CLI has {self._auth_token!r}, "
                    f"but existing gateway expects {existing['auth']!r}"
                )
            self._base_url = existing["url"]
            self._auth_token = existing["auth"]
            self._owned = False
            self._proc = None
            if existing["auth"]:
                self._session.headers["X-Auth-Token"] = existing["auth"]
            try:
                resp = self._session.get(f"{self._base_url}/ping", timeout=2)
                resp.raise_for_status()
                return
            except requests.RequestException:
                pass

        self._owned = True
        self._spawn_gateway()

    def _spawn_gateway(self) -> None:
        binary = _find_binary()
        if os.environ.get("YAMLQ_GATEWAY_DAEMON") == "1":
            cmd = [binary, "serve"]
            if self._auth_token:
                cmd.append(f"--auth-token={self._auth_token}")
            self._owned = False
        else:
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

    def is_owned(self) -> bool:
        """True when this client spawned the gateway process itself.

        Attached (daemon) clients must not close pools or shut the process
        down: the pool is meant to outlive a single CLI run.
        """
        return self._owned

    def serve(self, port: int = 0, idle_timeout: int = 600) -> int:
        """Run the gateway as a resident daemon in the foreground.

        stdout/stderr pass through so the user sees the listening line.
        Returns the gateway process exit code; Ctrl+C reaches the child too
        (same console) and it shuts down gracefully.
        """
        binary = _find_binary()
        cmd = [binary, "serve"]
        if port:
            cmd.append(f"--port={port}")
        if idle_timeout > 0:
            cmd.append(f"--conn-idle-timeout={idle_timeout}")
        if self._auth_token:
            cmd.append(f"--auth-token={self._auth_token}")

        proc = subprocess.Popen(cmd)
        try:
            proc.wait()
        except KeyboardInterrupt:
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait()
                raise GatewayError("gateway did not exit after Ctrl+C; killed")
        return proc.returncode

    def stop(self) -> None:
        if not self._owned or self._proc is None:
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
