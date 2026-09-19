from __future__ import annotations

from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field

from rich.text import Text
from textual import work
from textual.app import App, ComposeResult
from textual.containers import Vertical
from textual.reactive import reactive
from textual.widget import Widget
from textual.widgets import DataTable, Footer, Header, Input, LoadingIndicator, Static, TabbedContent, TabPane

from yamlq.converters import apply_converter
from yamlq.gateway import Gateway
from yamlq.parser import ViewConfig, render_sql


@dataclass
class ViewResult:
    columns: list[str] = field(default_factory=list)
    rows: list[list] = field(default_factory=list)
    truncated: bool = False
    error: dict | str | None = None
    total_pages: int = 1


class ParamDialog(Widget):
    DEFAULT_CSS = """
    ParamDialog {
        height: auto;
        padding: 1 2;
        border: round $accent;
        margin: 1 4;
    }
    ParamDialog .title {
        text-style: bold;
        margin-bottom: 1;
    }
    ParamDialog Input {
        margin-bottom: 1;
    }
    """

    def __init__(self, view: ViewConfig, callback, **kwargs):
        super().__init__(**kwargs)
        self.view = view
        self.callback = callback
        self._inputs: dict[str, Input] = {}

    def compose(self) -> ComposeResult:
        yield Static(f"参数输入 — {self.view.view_name}", classes="title")
        for p in self.view.params:
            yield Static(f"{p.prompt}:")
            inp = Input(placeholder=p.name, value=str(p.default) if p.default else "", id=f"param-{p.name}")
            self._inputs[p.name] = inp
            yield inp
        yield Static("[dim]按 Enter 提交[/dim]")

    def on_input_submitted(self, event: Input.Submitted) -> None:
        values = {}
        for name, inp in self._inputs.items():
            values[name] = inp.value
        self.callback(values)


class ViewPane(Widget):
    DEFAULT_CSS = """
    ViewPane {
        height: 1fr;
    }
    ViewPane .status-bar {
        height: 1;
        dock: bottom;
        background: $surface;
        color: $text-muted;
        padding: 0 1;
    }
    """

    page = reactive(1)

    def __init__(self, view: ViewConfig, result: ViewResult | None = None, **kwargs):
        super().__init__(**kwargs)
        self.view = view
        self.result = result
        self._table = DataTable()

    def compose(self) -> ComposeResult:
        if self.result is None:
            yield LoadingIndicator()
            yield Static("加载中...", classes="status-bar")
        elif self.result.error:
            err = self.result.error
            if isinstance(err, dict):
                msg = f"✗ {err.get('code', 'ERROR')}: {err.get('message', '')}"
                hint = err.get("hint", "")
                if hint:
                    msg += f"\n  hint: {hint}"
            else:
                msg = f"✗ {err}"
            yield Static(Text(msg, style="red"))
        else:
            yield self._table
            yield Static("", classes="status-bar", id="status")

    def on_mount(self) -> None:
        if self.result and not self.result.error:
            self._build_table()

    def _build_table(self) -> None:
        r = self.result
        col_cfgs = {c.field: c for c in self.view.columns}

        self._table.clear(columns=True)
        for col_name in r.columns:
            cfg = col_cfgs.get(col_name)
            label = cfg.header if cfg else col_name
            self._table.add_column(label, key=col_name)

        page_size = self.view.page_size if self.view.enable_paging else len(r.rows) or 1
        start = (self.page - 1) * page_size
        end = start + page_size
        page_rows = r.rows[start:end]

        for row in page_rows:
            cells = []
            for i, val in enumerate(row):
                col_name = r.columns[i] if i < len(r.columns) else ""
                cfg = col_cfgs.get(col_name)
                if cfg and cfg.converter:
                    val = apply_converter(cfg.converter, val)
                cells.append(str(val) if val is not None else "")
            self._table.add_row(*cells)

        total = len(r.rows)
        if self.view.enable_paging and total > 0:
            self.total_pages = (total + page_size - 1) // page_size
        else:
            self.total_pages = 1

        self._update_status(total, len(page_rows))

    def _update_status(self, total: int, shown: int) -> None:
        try:
            status = self.query_one(f"#status-{self.view.key}", Static)
        except Exception:
            try:
                status = self.query_one("#status", Static)
            except Exception:
                return
        r = self.result
        parts = [f"{total} 行"]
        if self.view.enable_paging and self.total_pages > 1:
            parts.append(f"第 {self.page}/{self.total_pages} 页")
        if r and r.truncated:
            parts.append("⚠ 结果已截断")
        status.update(" | ".join(parts))

    def watch_page(self) -> None:
        if self.result and not self.result.error:
            self._build_table()

    def next_page(self) -> None:
        if self.page < self.total_pages:
            self.page += 1

    def prev_page(self) -> None:
        if self.page > 1:
            self.page -= 1


class YamlViewApp(App):
    TITLE = "yamlq"
    CSS = """
    TabbedContent {
        height: 1fr;
    }
    """
    BINDINGS = [
        ("q", "quit", "退出"),
        ("n", "next_page", "下一页"),
        ("p", "prev_page", "上一页"),
        ("1", "tab_0", "Tab 1"),
        ("2", "tab_1", "Tab 2"),
        ("3", "tab_2", "Tab 3"),
        ("4", "tab_3", "Tab 4"),
        ("5", "tab_4", "Tab 5"),
        ("6", "tab_5", "Tab 6"),
        ("7", "tab_6", "Tab 7"),
        ("8", "tab_7", "Tab 8"),
        ("9", "tab_8", "Tab 9"),
    ]

    def __init__(
        self,
        views: list[ViewConfig],
        gateway: Gateway | None,
        param_overrides: dict[str, str],
        timeout: int = 30,
    ):
        super().__init__()
        self.views = views
        self.gateway = gateway
        self.param_overrides = param_overrides
        self.timeout = timeout
        self._results: dict[str, ViewResult] = {}
        self._pending_params: dict[str, dict[str, str]] = {}

    def compose(self) -> ComposeResult:
        yield Header()
        if not self.views:
            yield Static("未指定配置文件，请使用 yamlq <配置文件> 启动或按 q 退出", id="welcome")
        else:
            with TabbedContent(initial=self.views[0].key if self.views else ""):
                for view in self.views:
                    with TabPane(f"{view.view_name}  @{view.db_type}+{view.dsn}", id=view.key):
                        yield ViewPane(view, id=f"pane-{view.key}")
        yield Footer()

    def on_mount(self) -> None:
        if not self.views or self.gateway is None:
            return
        needs_input = [
            v for v in self.views
            if any(p.default is None and p.name not in self.param_overrides for p in v.params)
        ]
        ready = [v for v in self.views if v not in needs_input]

        if ready:
            self._fetch_views(ready)

        for view in needs_input:
            self._show_param_dialog(view)

    def _show_param_dialog(self, view: ViewConfig) -> None:
        def on_submit(values: dict[str, str]):
            self._pending_params[view.key] = values
            try:
                self.query_one(ParamDialog).remove()
            except Exception:
                pass
            self._fetch_views([view])

        dialog = ParamDialog(view, on_submit, id="param-dialog")
        self.mount(dialog)

    @work(thread=True)
    def _fetch_views(self, views: list[ViewConfig]) -> None:
        conn_cache: dict[tuple[str, str], str] = {}

        with ThreadPoolExecutor(max_workers=min(len(views), 10)) as pool:
            futures = {
                pool.submit(self._fetch_one, v, conn_cache): v
                for v in views
            }
            for future in as_completed(futures):
                view = futures[future]
                try:
                    result = future.result()
                except Exception as e:
                    result = ViewResult(error={"code": "INTERNAL", "message": str(e)})
                self._results[view.key] = result
                self.call_from_thread(self._update_pane, view.key, result)

        # Attached (daemon) gateways keep pools across runs; only close the
        # pools of a gateway this process spawned itself.
        if self.gateway.is_owned():
            for conn_id in conn_cache.values():
                try:
                    self.gateway.close(conn_id)
                except Exception:
                    pass

    def _fetch_one(self, view: ViewConfig, conn_cache: dict) -> ViewResult:
        ck = view.conn_key
        if ck not in conn_cache:
            conn_cache[ck] = self.gateway.connect(view.db_type, view.dsn)
        conn_id = conn_cache[ck]

        overrides = {**self.param_overrides, **self._pending_params.get(view.key, {})}
        values: dict[str, str] = {}
        for p in view.params:
            if p.name in overrides:
                values[p.name] = overrides[p.name]
            elif p.default is not None:
                values[p.name] = str(p.default)
            else:
                values[p.name] = ""

        sql, param_vals = render_sql(view.sql, values)
        data = self.gateway.query(conn_id, sql, param_vals, timeout=self.timeout)

        if data.get("error"):
            return ViewResult(error=data["error"])
        return ViewResult(
            columns=data.get("columns") or [],
            rows=data.get("rows") or [],
            truncated=data.get("truncated", False),
        )

    def _update_pane(self, key: str, result: ViewResult) -> None:
        try:
            pane = self.query_one(f"#pane-{key}", ViewPane)
        except Exception:
            return
        pane.result = result
        pane.remove_children()
        if result.error:
            err = result.error
            if isinstance(err, dict):
                msg = f"✗ {err.get('code', 'ERROR')}: {err.get('message', '')}"
                hint = err.get("hint", "")
                if hint:
                    msg += f"\n  hint: {hint}"
            else:
                msg = f"✗ {err}"
            pane.mount(Static(Text(msg, style="red")))
        else:
            pane._table = DataTable()
            pane.mount(pane._table)
            pane.mount(Static("", classes="status-bar", id=f"status-{key}"))
            pane._build_table()

    def action_next_page(self) -> None:
        pane = self._active_pane()
        if pane:
            pane.next_page()

    def action_prev_page(self) -> None:
        pane = self._active_pane()
        if pane:
            pane.prev_page()

    def _switch_tab(self, index: int) -> None:
        try:
            tc = self.query_one(TabbedContent)
            tabs = list(tc.tabs)
            if 0 <= index < len(tabs):
                tc.active = tabs[index].id
        except Exception:
            pass

    def action_tab_0(self) -> None: self._switch_tab(0)
    def action_tab_1(self) -> None: self._switch_tab(1)
    def action_tab_2(self) -> None: self._switch_tab(2)
    def action_tab_3(self) -> None: self._switch_tab(3)
    def action_tab_4(self) -> None: self._switch_tab(4)
    def action_tab_5(self) -> None: self._switch_tab(5)
    def action_tab_6(self) -> None: self._switch_tab(6)
    def action_tab_7(self) -> None: self._switch_tab(7)
    def action_tab_8(self) -> None: self._switch_tab(8)

    def _active_pane(self) -> ViewPane | None:
        try:
            tc = self.query_one(TabbedContent)
            active_id = tc.active
            if active_id:
                return self.query_one(f"#pane-{active_id}", ViewPane)
        except Exception:
            pass
        return None
