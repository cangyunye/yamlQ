from __future__ import annotations

import sys
from datetime import date, datetime
from decimal import Decimal
from typing import Any, Callable

ConverterFunc = Callable[[Any], str]

_STATUS_MAP = {
    "pending": "待支付",
    "paid": "已支付",
    "shipped": "已发货",
    "completed": "已完成",
    "cancelled": "已取消",
}


def _parse_datetime(v: str) -> datetime | None:
    # The Go gateway serialises time.Time to ISO-8601, so converter inputs are
    # strings in practice (e.g. "2021-03-15T09:00:00Z").
    try:
        return datetime.fromisoformat(v.strip().replace("Z", "+00:00"))
    except ValueError:
        return None


def _datetime_to_iso(v: Any) -> str:
    if isinstance(v, (datetime, date)):
        return v.strftime("%Y-%m-%d")
    if isinstance(v, str) and v:
        parsed = _parse_datetime(v)
        if parsed is not None:
            return parsed.strftime("%Y-%m-%d")
        return v[:10]
    return ""


def _datetime_to_cn(v: Any) -> str:
    if isinstance(v, (datetime, date)):
        return v.strftime("%Y年%m月%d日 %H:%M")
    if isinstance(v, str) and v:
        parsed = _parse_datetime(v)
        if parsed is not None:
            return parsed.strftime("%Y年%m月%d日 %H:%M")
        return v
    return ""


def _status_to_cn(v: Any) -> str:
    if v is None:
        return ""
    return _STATUS_MAP.get(str(v), str(v))


def _money_format(v: Any) -> str:
    if v is None:
        return ""
    try:
        return f"¥{Decimal(str(v)):,.2f}"
    except Exception:
        return str(v)


BUILTIN_CONVERTERS: dict[str, ConverterFunc] = {
    "datetime_to_iso": _datetime_to_iso,
    "datetime_to_cn": _datetime_to_cn,
    "status_to_cn": _status_to_cn,
    "money_format": _money_format,
}

_warned: set[str] = set()


def apply_converter(name: str, value: Any) -> str:
    fn = BUILTIN_CONVERTERS.get(name)
    if fn is None:
        if name not in _warned:
            print(
                f"warning: unknown converter: {name}, column will display raw value",
                file=sys.stderr,
            )
            _warned.add(name)
        return str(value) if value is not None else ""
    try:
        return fn(value)
    except Exception:
        return str(value) if value is not None else ""
