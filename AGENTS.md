# yamlq — AGENTS.md

## Repo structure
- `cli/` — Python app (uv + hatchling), entrypoint `yamlq/__main__.py:main`
- `db-gateway/` — Go HTTP gateway (module `yamlq/db-gateway`), entrypoint `main.go`
- Python depends on Go binary at runtime: spawns as subprocess

## Build & test
```bash
make build            # default: mysql + postgres + opengauss + oracle only
make build-all        # all drivers + serve subcommand (-tags all)
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
- **Go tests need real MySQL** at `127.0.0.1:3306` (DSN `root:root123456@tcp(127.0.0.1:3306)/default_db`).
- **Python tests are E2E only** — spin up Go binary, hit real MySQL. Test fixture seeds DB via `mysql` CLI.
- **`cli/tests/test_e2e.py:run_cli`** invokes `python -m yamlq` (not pip-installed `yamlq` binary).
- **Architecture**: Python parses YAML, spawns Go gateway on `127.0.0.1:0` (random port), Go handles DB. Port discovered via Go stdout `"listening on port N"`.
- **Timeout model**: `--timeout` = per-query (Go `context.WithTimeout`), `--session-timeout` = Python `ThreadPoolExecutor` deadline (default 120s).
- **Error codes** (Go): `QUEUE_FULL` → 503, `QUERY_TIMEOUT` → partial + 200, `CONNECTION_LOST` / `DB_ERROR` → 200 with error detail.
- **Converters** are Python-only (4 built-in: `datetime_to_iso`, `datetime_to_cn`, `status_to_cn`, `money_format`).
- **No CI** configured.
