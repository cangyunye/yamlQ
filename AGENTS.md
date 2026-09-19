# yamlq — AGENTS.md

## Repo structure
- `cli/` — Python app (uv + hatchling), entrypoint `yamlq/__main__.py:main`
- `db-gateway/` — Go HTTP gateway (module `yamlq/db-gateway`), entrypoint `main.go`
- Python depends on Go binary at runtime: spawns as subprocess

## Build & test
```bash
make build            # default: mysql + postgres + opengauss + oracle only
make build-all        # all drivers (-tags all); serve subcommand is always available
make test             # test-go + test-python (default build)
make test-go          # go test ./... -timeout 60s (default build)
make test-go-all      # go test -tags all ./... -timeout 60s (all drivers)
make test-python      # .venv/bin/python -m pytest tests/ -v --timeout=120 (from cli/)
make clean            # rm binary + __pycache__
```

## Quirks & gotchas
- **No lint/typecheck config** — no ruff.toml, no golangci, no pre-commit. Only `make test` verifies.
- **Go 1.25** — uses `http.NewServeMux` method-pattern routing (`"POST /connect"`).
- **Go binary** built at `db-gateway/db-gateway`, gitignored. Must rebuild after Go changes.
- **Python toolchain** is `uv` (not pip/poetry). `.venv/` lives in `cli/`.
- **python ≥3.11** required.
- **Go tests need real MySQL**; DSN defaults to `root:root123456@tcp(127.0.0.1:3306)/default_db`, override with `YAMLQ_TEST_MYSQL_DSN` (server_test.go).
- **Python tests are E2E only** — spin up Go binary, hit real MySQL. Test fixture seeds DB via `mysql` CLI. DSNs overridable: `YAMLQ_TEST_MYSQL_DSN` (Go-format), `YAMLQ_TEST_PG_DSN`, `YAMLQ_TEST_OPENGAUSS_DSN` (injects an og view into the multi-db fixture), `YAMLQ_TEST_ORACLE_DSN`, `YAMLQ_TEST_OB_MYSQL_DSN`, `YAMLQ_TEST_OB_ORACLE_DSN` (unset → the matching tests skip). OB tests need the `-tags all` binary (ob drivers).
- **Renderer column matching is case-insensitive-fallback** — Oracle-style backends (oracle, ob-oracle, openGauss A-mode) return uppercased column names while user column configs are usually lowercase; exact match wins, then casefold.
- **Local test DSNs live in `.env.local`** (gitignored, source it before running tests) — never commit credentials.
- **`cli/tests/test_e2e.py:run_cli`** invokes `python -m yamlq` (not pip-installed `yamlq` binary).
- **Architecture**: Python parses YAML, spawns Go gateway on `127.0.0.1:0` (random port), Go handles DB. Port discovered via Go stdout `"listening on port N"`.
- **Resident gateway**: `yamlq serve` runs the gateway as a daemon, writes `~/.yamlq/gateway.env` + cwd `.yamlq-gateway.env`; later `yamlq run` auto-attaches (env var `YAMLQ_GATEWAY_URL` > cwd file > home file, verified via `/ping`). Attached runs never close pools — `/connect` is idempotent per `driver+DSN` (Manager.keys), and a reaper (serve mode, `--conn-idle-timeout`, default 600s) closes idle pools and probes liveness every 30s. CLI retries once on `CONNECTION_LOST`/`CONNECTION_NOT_FOUND`.
- **Timeout model**: `--timeout` = per-query (Go `context.WithTimeout`), `--session-timeout` = Python `ThreadPoolExecutor` deadline (default 120s).
- **Error codes** (Go): `QUEUE_FULL` → 503, `QUERY_TIMEOUT` → partial + 200, `CONNECTION_LOST` / `DB_ERROR` → 200 with error detail.
- **Converters** are Python-only (4 built-in: `datetime_to_iso`, `datetime_to_cn`, `status_to_cn`, `money_format`).
- **No CI** configured.
