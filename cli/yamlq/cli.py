from __future__ import annotations

import argparse
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed

from yamlq import __version__, __commit__
from yamlq.gateway import Gateway, GatewayError
from yamlq.parser import ConfigError, ViewConfig, check_views, load_views, render_sql
from yamlq.renderer import console, render_table


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="yamlq", description="YAML-driven multi-database query tool")
    sub = parser.add_subparsers(dest="command")

    run_p = sub.add_parser("run", help="execute views (default)")
    run_p.add_argument("config", nargs="?", default=None, help="YAML file or directory")
    _add_common_args(run_p)

    check_p = sub.add_parser("check", help="validate views config")
    check_p.add_argument("config", help="YAML file or directory")

    _add_common_args(parser)
    return parser


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


def _run_view(gw: Gateway, view: ViewConfig, conn_id: str, param_overrides: dict[str, str], timeout: int) -> tuple[ViewConfig, dict]:
    try:
        params = _resolve_params(view, param_overrides)
        sql, values = render_sql(view.sql, params)
        result = gw.query(conn_id, sql, values, page=0, page_size=0, timeout=timeout)
        return view, result
    except Exception as e:
        return view, {"error": {"code": "QUERY_FAILED", "message": str(e)}}


def cmd_run(args: argparse.Namespace) -> int:
    try:
        views = load_views(args.config)
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
    conn_ids: list[str] = []
    try:
        with ThreadPoolExecutor(max_workers=min(len(groups), 10)) as pool:
            futures = {}
            for ck, group_views in groups.items():
                try:
                    conn_id = gw.connect(ck[0], ck[1])
                    conn_ids.append(conn_id)
                except GatewayError as e:
                    for v in group_views:
                        results.append((v, {"error": {"code": "CONNECT_FAILED", "message": str(e)}}))
                    continue
                for v in group_views:
                    futures[pool.submit(_run_view, gw, v, conn_id, param_overrides, args.timeout)] = v

            for future in as_completed(futures, timeout=args.session_timeout):
                results.append(future.result())
    except TimeoutError:
        print("session timeout reached, cancelling remaining queries", file=sys.stderr)
    except KeyboardInterrupt:
        print(file=sys.stderr)
    finally:
        for cid in conn_ids:
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


def cmd_check(args: argparse.Namespace) -> int:
    try:
        views = load_views(args.config)
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

    if len(sys.argv) > 1 and sys.argv[1] not in ("run", "check", "-h", "--help", "--version"):
        sys.argv.insert(1, "run")

    args = parser.parse_args()

    if getattr(args, "version", False):
        print(f"yamlq {__version__} (commit {__commit__})")
        sys.exit(0)

    if args.command == "check":
        sys.exit(cmd_check(args))

    if not args.config:
        sys.exit(_run_tui_no_config())

    sys.exit(cmd_run(args))
