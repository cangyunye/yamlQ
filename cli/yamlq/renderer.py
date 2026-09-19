from __future__ import annotations

from rich.console import Console
from rich.table import Table

from yamlq.converters import apply_converter
from yamlq.parser import ViewConfig

console = Console()

THEMES: dict[str, dict] = {
    "default": {
        "header_style": "bold cyan",
        "border_style": "blue",
        "title_style": "bold white",
        "row_styles": ["", "dim"],
    },
    "ocean": {
        "header_style": "bold white on dark_blue",
        "border_style": "cyan",
        "title_style": "bold cyan",
        "row_styles": ["", "on grey15"],
    },
    "sunset": {
        "header_style": "bold black on yellow",
        "border_style": "red",
        "title_style": "bold yellow",
        "row_styles": ["", "on grey11"],
    },
    "forest": {
        "header_style": "bold white on dark_green",
        "border_style": "green",
        "title_style": "bold green",
        "row_styles": ["", "on grey11"],
    },
    "mono": {
        "header_style": "bold",
        "border_style": "white",
        "title_style": "bold",
        "row_styles": None,
    },
}


def render_table(view: ViewConfig, result: dict) -> None:
    if result.get("error"):
        err = result["error"]
        if isinstance(err, dict):
            console.print(f"[red]✗ {err.get('code', 'ERROR')}: {err.get('message', '')}[/red]")
            if err.get("hint"):
                console.print(f"[dim]  hint: {err['hint']}[/dim]")
        else:
            console.print(f"[red]✗ {err}[/red]")
        return

    columns = result.get("columns") or []
    rows = result.get("rows") or []

    theme = THEMES.get(view.theme, THEMES["default"])

    table = Table(
        title=f"{view.view_name}  @{view.db_type}+{view.dsn}",
        title_justify="left",
        show_lines=False,
        header_style=theme["header_style"],
        border_style=theme["border_style"],
        title_style=theme["title_style"],
        row_styles=theme["row_styles"],
    )

    # Column-name matching must tolerate case: Oracle-style backends (oracle,
    # ob-oracle, openGauss A-mode) report uppercased identifiers while user
    # column configs are usually lowercase. Exact match wins, then casefold.
    col_cfgs = {c.field: c for c in view.columns}
    col_cfgs_folded = {k.casefold(): v for k, v in col_cfgs.items()}

    def cfg_for(col_name: str):
        cfg = col_cfgs.get(col_name)
        if cfg is None:
            cfg = col_cfgs_folded.get(col_name.casefold())
        return cfg

    for col_name in columns:
        cfg = cfg_for(col_name)
        header = cfg.header if cfg else col_name
        justify = cfg.align if cfg else "left"
        style = cfg.style if cfg else ""
        overflow = cfg.overflow if cfg else "ellipsis"
        kwargs: dict = {}
        if cfg and cfg.width:
            kwargs["width"] = cfg.width
        if cfg and cfg.min_width:
            kwargs["min_width"] = cfg.min_width
        if cfg and cfg.max_width:
            kwargs["max_width"] = cfg.max_width
        table.add_column(header, justify=justify, style=style, overflow=overflow, **kwargs)

    for row in rows:
        cells = []
        for i, val in enumerate(row):
            col_name = columns[i] if i < len(columns) else ""
            cfg = cfg_for(col_name)
            if cfg and cfg.converter:
                val = apply_converter(cfg.converter, val)
            cells.append(str(val) if val is not None else "")
        table.add_row(*cells)

    console.print(table)

    truncated = result.get("truncated", False)
    row_count = len(rows)
    if truncated:
        console.print(f"[yellow]⚠ 结果已截断，显示 {row_count} 行（上限）[/yellow]")
    else:
        console.print(f"[dim]{row_count} 行[/dim]")
