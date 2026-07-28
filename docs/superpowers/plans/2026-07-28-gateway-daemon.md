# Gateway Daemon Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Go gateway a reusable long-running daemon that outlives individual CLI invocations, avoiding repeated startup/teardown cost.

**Architecture:** Gateway writes a `.yamlq-gateway.env` file on every startup; Python checks env vars / this file before spawning. `--daemon` flag makes gateway ignore `/shutdown` and stay resident. `YAMLQ_GATEWAY_DAEMON=1` env tells Python not to kill gateway on exit. Existing one-shot behavior remains default.

**Tech Stack:** Go 1.25 (net/http), Python 3.11+ (subprocess / os.environ)

## Global Constraints

- Zero new Go module dependencies
- `--daemon` flag only available in `service` mode (not `cli` mode)
- Environment file written to CWD as `.yamlq-gateway.env`
- Default behavior (`cli` mode) unchanged: gateway starts with CLI, dies with CLI
- Python detects existing gateway via `YAMLQ_GATEWAY_URL` env var first, then `.yamlq-gateway.env` file
- Auth token from existing gateway must match if set

---

## Files

| File | Action | Responsibility |
|------|--------|----------------|
| `db-gateway/main.go` | Modify | Add `--daemon` flag, write env file on startup |
| `cli/yamlq/gateway.py` | Modify | Detect existing gateway, handle daemon/non-daemon lifecycle |
| `cli/yamlq/cli.py` | Modify | Pass `--daemon` flag when env `YAMLQ_GATEWAY_DAEMON=1` |
| `docs/ARCHITECTURE.md` | Modify | Document daemon mode |
| `scripts/yamlq-gateway.sh` | Create | Bash management script (start/stop/status/restart) |

---

### Task 1: Gateway — add `--daemon` flag and env file write

**Files:**
- Modify: `db-gateway/main.go`

**Interfaces:**
- Consumes: `flag.Bool("daemon", ...)`, current `fmt.Printf("listening on port %d\n", port)`
- Produces: `.yamlq-gateway.env` file in CWD with `YAMLQ_GATEWAY_URL` and `YAMLQ_GATEWAY_AUTH`; `/shutdown` handler only works in non-daemon mode

- [ ] **Step 1: Read current `db-gateway/main.go`**

Read the file to understand the current flag setup.

- [ ] **Step 2: Add `--daemon` flag and env file write logic**

```go
var daemonMode = flag.Bool("daemon", false, "run as daemon (stay resident after client disconnects)")

// After listening, before starting HTTP server:
if *daemonMode {
    *mode = "service"
    cfg = config.Service
    writeEnvFile(port, *authToken)
}

func writeEnvFile(port int, authToken string) {
    path := filepath.Join(os.Getenv("HOME"), ".yamlq-gateway.env")
    if cwd, err := os.Getwd(); err == nil {
        path = filepath.Join(cwd, ".yamlq-gateway.env")
    }
    lines := []string{
        fmt.Sprintf("YAMLQ_GATEWAY_URL=http://127.0.0.1:%d", port),
    }
    if authToken != "" {
        lines = append(lines, fmt.Sprintf("YAMLQ_GATEWAY_AUTH=%s", authToken))
    }
    if *daemonMode {
        lines = append(lines, "YAMLQ_GATEWAY_DAEMON=1")
    }
    os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}
```

- [ ] **Step 3: Remove env file on shutdown**

In the shutdown path (both SIGINT and `/shutdown`), add:
```go
func cleanupEnvFile() {
    path, _ := filepath.Abs(".yamlq-gateway.env")
    os.Remove(path)
}
```

Call `cleanupEnvFile()` before `os.Exit(0)` in both shutdown paths.

- [ ] **Step 4: In daemon mode, don't exit on `/shutdown`**

In ``server.go`, the `/shutdown` handler should check a daemon flag and reply with a warning instead of calling `shutdownFunc` when in daemon mode.

However, for simplicity, keep `/shutdown` working in both modes — in daemon mode, CLI clients that spawned a non-daemon gateway won't send shutdown anyway. The env file cleanup on SIGINT is sufficient for graceful daemon stop.

- [ ] **Step 5: Add imports**

Add `"os"`, `"path/filepath"`, `"strings"` to imports if not already present.

- [ ] **Step 6: Verify it compiles**

Run: `cd db-gateway && go build ./...`
Expected: no errors

- [ ] **Step 7: Commit**

```bash
git add db-gateway/main.go
git commit -m "feat: add --daemon flag and .yamlq-gateway.env file"
```

---

### Task 2: Python gateway.py — detect and connect to existing gateway

**Files:**
- Modify: `cli/yamlq/gateway.py`

**Interfaces:**
- Consumes: `os.environ`, `Path.cwd()`, current `Gateway` class (`start()`, `stop()`, `query()`, etc.)
- Produces: `Gateway.start()` checks for existing gateway before spawning; new `_find_existing_gateway()` static method; daemon-aware `stop()`

- [ ] **Step 1: Read current `cli/yamlq/gateway.py`**

Read the full file to understand current `Gateway` class structure.

- [ ] **Step 2: Add existing gateway detection**

Add to the `Gateway` class (or as module-level helpers):

```python
import os
from pathlib import Path
from urllib.parse import urlparse

GATEWAY_ENV_FILE = ".yamlq-gateway.env"

def _load_gateway_env() -> dict[str, str]:
    """Read .yamlq-gateway.env from CWD."""
    env_file = Path.cwd() / GATEWAY_ENV_FILE
    if not env_file.exists():
        return {}
    result = {}
    for line in env_file.read_text().strip().splitlines():
        if "=" in line:
            k, v = line.split("=", 1)
            result[k] = v
    return result

def _find_existing_gateway() -> dict | None:
    """Check env var first, then env file. Returns connection info or None."""
    url = os.environ.get("YAMLQ_GATEWAY_URL")
    if url:
        auth = os.environ.get("YAMLQ_GATEWAY_AUTH", "")
        return {"url": url, "auth": auth}
    env = _load_gateway_env()
    if "YAMLQ_GATEWAY_URL" in env:
        return {"url": env["YAMLQ_GATEWAY_URL"], "auth": env.get("YAMLQ_GATEWAY_AUTH", "")}
    return None
```

- [ ] **Step 3: Modify `Gateway.start()` to reuse existing gateway**

```python
class Gateway:
    def start(self) -> None:
        existing = _find_existing_gateway()
        if existing:
            self._base_url = existing["url"]
            self._auth_token = existing["auth"]
            self._owned = False  # don't kill on stop
            self._proc = None
            # verify it's alive
            try:
                resp = requests.get(f"{self._base_url}/ping", timeout=2)
                resp.raise_for_status()
                return
            except requests.RequestException:
                pass  # stale env file, fall through to spawn

        # existing code: spawn gateway subprocess
        self._owned = True
        self._spawn_gateway()
```

- [ ] **Step 4: Modify `Gateway.stop()` to skip shutdown for non-owned gateways**

```python
def stop(self) -> None:
    if not self._owned:
        return  # don't kill reused gateway
    # existing shutdown logic...
    if self._proc:
        self._send_shutdown()
        self._proc.wait(timeout=5)
```

- [ ] **Step 5: Determine daemon mode for spawned gateways**

In `_spawn_gateway()`, check `os.environ.get("YAMLQ_GATEWAY_DAEMON")`:
- If `"1"`: pass `--daemon` flag to gateway subprocess, set `self._owned = False` (don't kill on stop)
- If not set: current behavior (one-shot, kill on stop)

```python
def _spawn_gateway(self) -> None:
    cmd = [self._binary_path, "--mode", self._mode]
    if self._auth_token:
        cmd.extend(["--auth-token", self._auth_token])
    if os.environ.get("YAMLQ_GATEWAY_DAEMON") == "1":
        cmd.append("--daemon")
        self._owned = False
    # ... rest of existing spawn logic
```

- [ ] **Step 6: Verify imports**

Ensure `requests` is imported (already is).

- [ ] **Step 7: Commit**

```bash
git add cli/yamlq/gateway.py
git commit -m "feat: detect and reuse existing gateway daemon"
```

---

### Task 3: Update architecture documentation

**Files:**
- Modify: `docs/ARCHITECTURE.md`

- [ ] **Step 1: Add daemon mode documentation**

Add a new section in `docs/ARCHITECTURE.md` after the driver table:

```markdown
## 6. Gateway 常驻模式

### 环境变量 / 文件发现

| 机制 | 路径 | 说明 |
|---|---|---|
| `YAMLQ_GATEWAY_URL` | 环境变量 | 直接指定已运行 gateway 地址 |
| `.yamlq-gateway.env` | 当前目录 | gateway 启动时自动写入 |

`.yamlq-gateway.env` 文件内容:
```
YAMLQ_GATEWAY_URL=http://127.0.0.1:54321
YAMLQ_GATEWAY_AUTH=my-token
YAMLQ_GATEWAY_DAEMON=1
```

### 启动模式

| 模式 | 命令 | 行为 |
|---|---|---|
| 默认（一次性） | `yamlq config.yaml` | Python 拉起 gateway，退出时关停 |
| 常驻（daemon） | `YAMLQ_GATEWAY_DAEMON=1 yamlq config.yaml` | Python 拉起 gateway 并常驻，退出不影响 gateway |
| 独立启动 | `./db-gateway --mode service --daemon` | 独立运行，yamlq 通过 env 文件发现并复用 |
| 直接复用 | `YAMLQ_GATEWAY_URL=http://127.0.0.1:54321 yamlq config.yaml` | 跳过发现，直连指定 gateway |
```

Renumber the existing sections (7=术语表, 8=关键决策) accordingly.

- [ ] **Step 2: Commit**

```bash
git add docs/ARCHITECTURE.md
git commit -m "docs: add gateway daemon mode documentation"
```

---

### Task 4: Bash management script for standalone gateway

**Files:**
- Create: `scripts/yamlq-gateway.sh`

- [ ] **Step 1: Create the bash script**

Create `scripts/yamlq-gateway.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

YAMLQ_DIR="$(cd "$(dirname "$0")/.." && pwd)"
GATEWAY_BIN="$YAMLQ_DIR/db-gateway/db-gateway"
ENV_FILE="$YAMLQ_DIR/.yamlq-gateway.env"
PID_FILE="$YAMLQ_DIR/.yamlq-gateway.pid"

usage() {
    cat <<EOF
Usage: $(basename "$0") {start|stop|status|restart}

  start   Start gateway daemon (background, writes .yamlq-gateway.env)
  stop    Stop gateway daemon (sends SIGTERM, cleans up env file)
  status  Show gateway daemon status (PID, URL, uptime)
  restart Stop + start

Options:
  --port PORT       Listen port (default: random)
  --auth-token TOK  Auth token for API access
  --bin PATH        Path to gateway binary (default: $GATEWAY_BIN)
EOF
    exit 1
}

PORT=""
AUTH_TOKEN=""
CMD=""

while [ $# -gt 0 ]; do
    case "$1" in
        start|stop|status|restart) CMD="$1"; shift ;;
        --port) PORT="$2"; shift 2 ;;
        --auth-token) AUTH_TOKEN="$2"; shift 2 ;;
        --bin) GATEWAY_BIN="$2"; shift 2 ;;
        *) usage ;;
    esac
done

[ -z "$CMD" ] && usage
[ -x "$GATEWAY_BIN" ] || { echo "error: gateway binary not found: $GATEWAY_BIN"; exit 1; }

get_pid() {
    [ -f "$PID_FILE" ] && cat "$PID_FILE" || echo ""
}

is_running() {
    local pid=$(get_pid)
    [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null
}

cmd_start() {
    if is_running; then
        echo "gateway already running (PID $(get_pid))"
        exit 0
    fi

    args=("--mode" "service" "--daemon")
    [ -n "$AUTH_TOKEN" ] && args+=("--auth-token" "$AUTH_TOKEN")
    [ -n "$PORT" ] && args+=("--port" "$PORT")

    nohup "$GATEWAY_BIN" "${args[@]}" > /dev/null 2>&1 &
    pid=$!
    echo "$pid" > "$PID_FILE"

    # wait for env file to appear
    for i in $(seq 1 10); do
        if [ -f "$ENV_FILE" ]; then
            echo "gateway started (PID $pid)"
            cat "$ENV_FILE"
            exit 0
        fi
        sleep 0.3
    done

    echo "warning: gateway started but env file not found yet" >&2
    echo "check logs: $YAMLQ_DIR/gateway.log" >&2
}

cmd_stop() {
    local pid=$(get_pid)
    if [ -z "$pid" ] || ! kill -0 "$pid" 2>/dev/null; then
        echo "gateway not running"
        rm -f "$PID_FILE" "$ENV_FILE"
        exit 0
    fi

    echo "stopping gateway (PID $pid)..."
    kill "$pid" 2>/dev/null || true
    for i in $(seq 1 10); do
        if ! kill -0 "$pid" 2>/dev/null; then
            break
        fi
        sleep 0.3
    done
    kill -9 "$pid" 2>/dev/null || true
    rm -f "$PID_FILE" "$ENV_FILE"
    echo "gateway stopped"
}

cmd_status() {
    local pid=$(get_pid)
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
        echo "gateway: running (PID $pid)"
        local url=""
        [ -f "$ENV_FILE" ] && url=$(grep YAMLQ_GATEWAY_URL "$ENV_FILE" | cut -d= -f2)
        [ -n "$url" ] && echo "  url: $url"
        if [ -f "$ENV_FILE" ] && grep -q YAMLQ_GATEWAY_AUTH "$ENV_FILE"; then
            echo "  auth: $(grep YAMLQ_GATEWAY_AUTH "$ENV_FILE" | cut -d= -f2)"
        fi
    else
        echo "gateway: not running"
        rm -f "$PID_FILE"
    fi
}

case "$CMD" in
    start)   cmd_start ;;
    stop)    cmd_stop ;;
    status)  cmd_status ;;
    restart) cmd_stop; cmd_start ;;
esac
```

- [ ] **Step 2: Make it executable**

Run: `chmod +x scripts/yamlq-gateway.sh`

- [ ] **Step 3: Commit**

```bash
git add scripts/yamlq-gateway.sh
git commit -m "feat: add gateway daemon management script"
```

---

### How to test

**Unit tests (Python):**
```python
def test_find_existing_gateway_env_var(monkeypatch):
    monkeypatch.setenv("YAMLQ_GATEWAY_URL", "http://127.0.0.1:9999")
    monkeypatch.setenv("YAMLQ_GATEWAY_AUTH", "test123")
    result = _find_existing_gateway()
    assert result == {"url": "http://127.0.0.1:9999", "auth": "test123"}

def test_find_existing_gateway_env_file(tmp_path, monkeypatch):
    env_file = tmp_path / ".yamlq-gateway.env"
    env_file.write_text("YAMLQ_GATEWAY_URL=http://127.0.0.1:8888\nYAMLQ_GATEWAY_AUTH=abc\n")
    monkeypatch.chdir(tmp_path)
    result = _find_existing_gateway()
    assert result == {"url": "http://127.0.0.1:8888", "auth": "abc"}

def test_no_gateway_found():
    result = _find_existing_gateway()
    assert result is None
```

**Manual E2E:**
```bash
# Start daemon manually
./db-gateway --mode service --daemon --auth-token test123
# → writes .yamlq-gateway.env

# In another terminal, run CLI — reuses existing gateway
yamlq -c testdata/e2e_basic.yaml

# Stop daemon
kill $(lsof -ti:$(grep PORT .yamlq-gateway.env | cut -d: -f3))

# Daemon mode via env var
YAMLQ_GATEWAY_DAEMON=1 yamlq -c testdata/e2e_basic.yaml
# → gateway stays alive after CLI exits
```
