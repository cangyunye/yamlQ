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
    cd "$YAMLQ_DIR"

    if is_running; then
        echo "gateway already running (PID $(get_pid))"
        exit 0
    fi

    args=("serve")
    [ -n "$AUTH_TOKEN" ] && args+=("--auth-token" "$AUTH_TOKEN")
    [ -n "$PORT" ] && args+=("--port" "$PORT")

    nohup "$GATEWAY_BIN" "${args[@]}" > "$YAMLQ_DIR/gateway.log" 2>&1 &
    pid=$!

    # wait for env file to appear
    for i in $(seq 1 10); do
        if [ -f "$ENV_FILE" ]; then
            echo "$pid" > "$PID_FILE"
            echo "gateway started (PID $pid)"
            cat "$ENV_FILE"
            exit 0
        fi
        sleep 0.3
    done

    echo "$pid" > "$PID_FILE"
    echo "warning: gateway started but env file not found yet" >&2
    echo "check logs: $YAMLQ_DIR/gateway.log" >&2
}

cmd_stop() {
    cd "$YAMLQ_DIR"
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
    cd "$YAMLQ_DIR"
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
