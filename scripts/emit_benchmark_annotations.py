#!/usr/bin/env python3
"""Emit compact GitHub Actions annotations for failed QSM benchmark tasks."""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


def esc(value: Any) -> str:
    text = str(value or "")
    return (
        text.replace("%", "%25")
        .replace("\n", "%0A")
        .replace("\r", "%0D")
        .replace(":", "%3A")
        .replace(",", "%2C")
    )


def tail(path_value: Any, limit: int = 1200) -> str:
    if not path_value:
        return ""
    path = Path(str(path_value))
    if not path.exists() or not path.is_file():
        return ""
    text = path.read_text(errors="replace")
    text = text.strip()
    if len(text) <= limit:
        return text
    return text[-limit:]


def main() -> int:
    path = Path(sys.argv[1] if len(sys.argv) > 1 else ".state/benchmark_report.json")
    if not path.exists():
        print(f"::error title=QSM benchmark report missing::{esc(path)} was not created")
        return 0
    report = json.loads(path.read_text())
    for task in report.get("tasks", []):
        if task.get("passed"):
            continue
        msg = (
            f"suite={report.get('suite')} task={task.get('name')} exit={task.get('exit_code')} "
            f"nodes={task.get('succeeded_nodes')}/{task.get('requested_nodes')} "
            f"collapse={task.get('collapse_approved')} trace={task.get('trace_passed')} "
            f"manifest={task.get('manifest_passed')} cachewiki={task.get('lake_cache_citation_coverage')} "
            f"force={task.get('force_average')} error={task.get('error')}"
        )
        log_tail = tail(task.get("log_path"))
        if log_tail:
            msg += f"\nlog_tail:\n{log_tail}"
        print(f"::error title=QSM benchmark failed {esc(task.get('name'))}::{esc(msg)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
