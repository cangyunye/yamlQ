from __future__ import annotations

import os
import re
import sys
from dataclasses import dataclass, field
from pathlib import Path

import yaml


@dataclass
class ColumnConfig:
    field: str
    header: str = ""
    align: str = "left"
    style: str = ""
    overflow: str = "ellipsis"
    converter: str = ""
    width: int | None = None
    min_width: int | None = None
    max_width: int | None = None

    def __post_init__(self):
        if not self.header:
            self.header = self.field


@dataclass
class ParamConfig:
    name: str
    prompt: str = ""
    default: str | None = None

    def __post_init__(self):
        if not self.prompt:
            self.prompt = self.name


@dataclass
class ViewConfig:
    key: str
    view_name: str = ""
    description: str = ""
    db_type: str = ""
    dsn: str = ""
    sql: str = ""
    params: list[ParamConfig] = field(default_factory=list)
    enable: bool = True
    enable_paging: bool = False
    page_size: int = 20
    theme: str = "default"
    columns: list[ColumnConfig] = field(default_factory=list)

    def __post_init__(self):
        if not self.view_name:
            self.view_name = self.key

    @property
    def conn_key(self) -> tuple[str, str]:
        return (self.db_type, self.dsn)


class ConfigError(Exception):
    pass


def _parse_view(key: str, raw: dict) -> ViewConfig:
    if not isinstance(raw, dict):
        raise ConfigError(f'view "{key}": expected mapping, got {type(raw).__name__}')

    for required in ("db_type", "dsn", "sql"):
        if not raw.get(required):
            raise ConfigError(f'view "{key}": missing required field: {required}')

    view_section = raw.get("view", {}) or {}
    columns = [
        ColumnConfig(
            field=c.get("field", ""),
            header=c.get("header", ""),
            align=c.get("align", "left"),
            style=c.get("style", ""),
            overflow=c.get("overflow", "ellipsis"),
            converter=c.get("converter", ""),
            width=c.get("width"),
            min_width=c.get("min_width"),
            max_width=c.get("max_width"),
        )
        for c in (view_section.get("columns", []) or [])
        if c.get("field")
    ]

    params = [
        ParamConfig(
            name=p.get("name", ""),
            prompt=p.get("prompt", ""),
            default=p.get("default"),
        )
        for p in (raw.get("params", []) or [])
        if p.get("name")
    ]

    return ViewConfig(
        key=key,
        view_name=raw.get("view_name", ""),
        description=raw.get("description", ""),
        db_type=raw["db_type"],
        dsn=raw["dsn"],
        sql=raw["sql"].strip(),
        params=params,
        enable=view_section.get("enable", True),
        enable_paging=view_section.get("enable_paging", False),
        page_size=view_section.get("page_size", 20),
        theme=view_section.get("theme", "default"),
        columns=columns,
    )


def load_views(path: str) -> list[ViewConfig]:
    p = Path(path)
    if p.is_dir():
        files = sorted(p.rglob("*.yaml")) + sorted(p.rglob("*.yml"))
        if not files:
            raise ConfigError(f"no .yaml/.yml files found in {path}")
    elif p.is_file():
        files = [p]
    else:
        raise ConfigError(f"path not found: {path}")

    seen: dict[str, str] = {}
    views: list[ViewConfig] = []

    for f in files:
        try:
            with open(f, encoding="utf-8") as fh:
                data = yaml.safe_load(fh)
        except yaml.YAMLError as e:
            raise ConfigError(f"YAML parse error in {f}: {e}") from e

        if not isinstance(data, dict):
            raise ConfigError(f"{f}: top-level must be a mapping of views")

        for key, raw in data.items():
            if key in seen:
                print(
                    f'warning: duplicate view key "{key}" in {f}, '
                    f"overridden from {seen[key]}",
                    file=sys.stderr,
                )
                views = [v for v in views if v.key != key]
            seen[key] = str(f)
            views.append(_parse_view(key, raw))

    return views


_PARAM_RE = re.compile(r"\{\{(\w+)\}\}")


def render_sql(sql: str, param_values: dict[str, str]) -> tuple[str, list[str]]:
    names = _PARAM_RE.findall(sql)
    rendered = _PARAM_RE.sub("?", sql)
    values = [param_values.get(n, "") for n in names]
    return rendered, values


def check_views(views: list[ViewConfig]) -> list[str]:
    warnings: list[str] = []
    for v in views:
        missing = [p.name for p in v.params if p.default is None]
        if missing:
            warnings.append(
                f'view "{v.key}": params missing default: {", ".join(missing)}'
            )
    return warnings
