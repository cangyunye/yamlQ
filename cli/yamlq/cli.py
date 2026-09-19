from __future__ import annotations

import argparse
import sys
import threading
from concurrent.futures import ThreadPoolExecutor, as_completed

from yamlq import __version__, __commit__
from yamlq.gateway import Gateway, GatewayError
from yamlq.parser import ConfigError, ViewConfig, check_views, load_views, render_sql
from yamlq.renderer import console, render_table


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="yamlq", description="YAML-driven multi-database query tool")
    sub = parser.add_subparsers(dest="command")

    run_p = sub.add_parser("run", help="execute views (default)")
    _add_config_args(run_p)
    _add_common_args(run_p)

    check_p = sub.add_parser("check", help="validate views config")
    _add_config_args(check_p)

    serve_p = sub.add_parser("serve", help="run the gateway as a resident daemon")
    serve_p.add_argument("--port", type=int, default=0, help="listen port (default: random)")
    serve_p.add_argument("--conn-idle-timeout", type=int, default=600, help="close pools idle longer than N seconds (0 disables)")
    serve_p.add_argument("--auth-token", default="", help="auth token for gateway")
    serve_p.add_argument("--verbose", action="store_true", help="verbose logging")

    _add_common_args(parser)
    return parser


def _add_config_args(p: argparse.ArgumentParser) -> None:
    # Separate dests for the positional and the flag: a nargs="?" positional
    # applies its default even when unconsumed, which would clobber a value
    # set via -c if both shared dest="config".
    p.add_argument("config_pos", nargs="?", default=None,
                   help="YAML file or directory")
    p.add_argument("-c", "--config", dest="config_opt", default=None,
                   help="YAML file or directory")


def _resolve_config(args: argparse.Namespace) -> str | None:
    return args.config_opt or args.config_pos


def _add_common_args(parser: argparse.ArgumentParser) -> None:
    parser.add_argument("--version", action="store_true", help="print version and exit")
    parser.add_argument("-v", "--views", help="comma-separated view keys to run")
    parser.add_argument("--param", action="append", default=[], help="key=value param override")
    parser.add_argument("--mode", default="cli", choices=["cli", "service"], help="gateway mode")
    parser.add_argument("--auth-token", default="", help="auth token for gateway")
    parser.add_argument("--verbose", action="store_true", help="verbose logging")
    parser.add_argument("--timeout", type=int, default=30, help="per-query timeout in seconds")
    parser.add_argument("--session-timeout", type=int, default=120, help="overall session timeout")
    parser.add_argument("--tui", action="store_true", help="launch interactive TUI mode")


def _parse_params(param_list: list[str]) -> dict[str, str]:
    result = {}
    for item in param_list:
        if "=" not in item:
            print(f"warning: ignoring malformed --param: {item}", file=sys.stderr)
            continue
        k, v = item.split("=", 1)
        result[k] = v
    return result


def _resolve_params(view: ViewConfig, overrides: dict[str, str]) -> dict[str, str]:
    values: dict[str, str] = {}
    for p in view.params:
        if p.name in overrides:
            values[p.name] = overrides[p.name]
        elif p.default is not None:
            values[p.name] = str(p.default)
        else:
            try:
                values[p.name] = input(f"{p.prompt} [{p.name}]: ")
            except (EOFError, KeyboardInterrupt):
                print(file=sys.stderr)
                sys.exit(1)
    return values


_RETRYABLE_ERROR_CODES = {"CONNECTION_LOST", "CONNECTION_NOT_FOUND"}


class _ConnHandle:
    """Tracks one (driver, dsn) pool for a run.

    On CONNECTION_LOST/CONNECTION_NOT_FOUND the server-side pool is gone
    (reaped or closed); `get(refresh=True)` re-runs the now-idempotent
    /connect so a dead pool is transparently rebuilt.
    """

    def __init__(self, gw: Gateway, driver: str, dsn: str):
        self._gw = gw
        self._driver = driver
        self._dsn = dsn
        self._conn_id: str | None = None
        self._lock = threading.Lock()

    def get(self, refresh: bool = False) -> str:
        with self._lock:
            if refresh or self._conn_id is None:
                self._conn_id = self._gw.connect(self._driver, self._dsn)
            return self._conn_id

    def conn_id(self) -> str | None:
        with self._lock:
            return self._conn_id


def _run_view(gw: Gateway, view: ViewConfig, handle: _ConnHandle, param_overrides: dict[str, str], timeout: int) -> tuple[ViewConfig, dict]:
    try:
        params = _resolve_params(view, param_overrides)
        sql, values = render_sql(view.sql, params)
        result = gw.query(handle.get(), sql, values, page=0, page_size=0, timeout=timeout)
        err = (result or {}).get("error") or {}
        if err.get("code") in _RETRYABLE_ERROR_CODES:
            try:
                result = gw.query(handle.get(refresh=True), sql, values, page=0, page_size=0, timeout=timeout)
            except GatewayError:
                pass  # keep the original connection-lost error
        return view, result
    except Exception as e:
        return view, {"error": {"code": "QUERY_FAILED", "message": str(e)}}


def cmd_run(args: argparse.Namespace) -> int:
    try:
        views = load_views(_resolve_config(args))
    except ConfigError as e:
        print(f"error: {e}", file=sys.stderr)
        return 1

    if args.views:
        wanted = set(args.views.split(","))
        available = {v.key for v in views}
        missing = wanted - available
        if missing:
            print(f"error: view(s) not found: {', '.join(sorted(missing))}", file=sys.stderr)
            print(f"available views: {', '.join(sorted(available))}", file=sys.stderr)
            return 1
        views = [v for v in views if v.key in wanted]

    views = [v for v in views if v.enable]
    if not views:
        print("no enabled views to run", file=sys.stderr)
        return 0

    param_overrides = _parse_params(args.param)

    gw = Gateway(mode=args.mode, auth_token=args.auth_token, verbose=args.verbose)
    try:
        gw.start()
    except GatewayError as e:
        print(f"error: {e}", file=sys.stderr)
        return 1

    if getattr(args, "tui", False):
        return _run_tui(views, gw, param_overrides, args.timeout)

    groups: dict[tuple[str, str], list[ViewConfig]] = {}
    for v in views:
        groups.setdefault(v.conn_key, []).append(v)

    results: list[tuple[ViewConfig, dict]] = []
    handles: list[_ConnHandle] = []
    try:
        with ThreadPoolExecutor(max_workers=min(len(groups), 10)) as pool:
            futures = {}
            for ck, group_views in groups.items():
                handle = _ConnHandle(gw, ck[0], ck[1])
                try:
                    handle.get()
                except GatewayError as e:
                    for v in group_views:
                        results.append((v, {"error": {"code": "CONNECT_FAILED", "message": str(e)}}))
                    continue
                handles.append(handle)
                for v in group_views:
                    futures[pool.submit(_run_view, gw, v, handle, param_overrides, args.timeout)] = v

            for future in as_completed(futures, timeout=args.session_timeout):
                results.append(future.result())
    except TimeoutError:
        print("session timeout reached, cancelling remaining queries", file=sys.stderr)
    except KeyboardInterrupt:
        print(file=sys.stderr)
    finally:
        # Attached (daemon) gateways keep their pools across runs; only a
        # gateway we spawned ourselves for this run gets torn down.
        if gw.is_owned():
            for handle in handles:
                cid = handle.conn_id()
                if cid:
                    try:
                        gw.close(cid)
                    except Exception:
                        pass

    view_order = {v.key: i for i, v in enumerate(views)}
    results.sort(key=lambda r: view_order.get(r[0].key, 999))

    for view, result in results:
        render_table(view, result)
        console.print()

    return 0


def _run_tui(views: list[ViewConfig], gw: Gateway, param_overrides: dict[str, str], timeout: int) -> int:
    from yamlq.tui.app import YamlViewApp
    app = YamlViewApp(views, gw, param_overrides, timeout)
    app.run()
    gw.stop()
    return 0


def _run_tui_no_config() -> int:
    from yamlq.tui.app import YamlViewApp
    app = YamlViewApp([], None, {}, 30)
    app.run()
    return 0


def cmd_serve(args: argparse.Namespace) -> int:
    gw = Gateway(mode="service", auth_token=args.auth_token, verbose=args.verbose)
    try:
        return gw.serve(port=args.port, idle_timeout=args.conn_idle_timeout)
    except GatewayError as e:
        print(f"error: {e}", file=sys.stderr)
        return 1


def cmd_check(args: argparse.Namespace) -> int:
    config = _resolve_config(args)
    if not config:
        print("error: check requires a config file (-c or positional)", file=sys.stderr)
        return 1
    try:
        views = load_views(config)
    except ConfigError as e:
        print(f"error: {e}", file=sys.stderr)
        return 1

    warnings = check_views(views)
    if warnings:
        for w in warnings:
            print(w, file=sys.stderr)
    else:
        print("all views OK")
    return 0


def main() -> None:
    parser = build_parser()

    if len(sys.argv) > 1 and sys.argv[1] not in ("run", "check", "serve", "-h", "--help", "--version"):
        sys.argv.insert(1, "run")

    args = parser.parse_args()

    if getattr(args, "version", False):
        print(f"yamlq {__version__} (commit {__commit__})")
        sys.exit(0)

    if args.command == "check":
        sys.exit(cmd_check(args))

    if args.command == "serve":
        sys.exit(cmd_serve(args))

    if not _resolve_config(args):
        sys.exit(_run_tui_no_config())

    sys.exit(cmd_run(args))
