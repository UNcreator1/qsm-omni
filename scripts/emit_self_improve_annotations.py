#!/usr/bin/env python3
"""Emit compact GitHub Actions annotations for QSM self-improvement failures."""

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


def emit_error(title: str, message: str) -> None:
    print(f"::error title={esc(title)}::{esc(message)}")


def main() -> int:
    path = Path(sys.argv[1] if len(sys.argv) > 1 else ".state/self_improvement_report.json")
    if not path.exists():
        emit_error("QSM self-improve report missing", f"{path} was not created")
        return 0

    report = json.loads(path.read_text())
    for err in report.get("errors", []):
        emit_error("QSM self-improve gate", err)

    for cycle in report.get("task_summaries", []):
        cycle_id = cycle.get("cycle")
        for task in cycle.get("tasks", []):
            if task.get("passed"):
                continue
            total_nodes = int(task.get("succeeded_nodes") or 0) + int(task.get("failed_nodes") or 0)
            msg = (
                f"cycle={cycle_id} task={task.get('name')} exit={task.get('exit_code')} "
                f"nodes={task.get('succeeded_nodes')}/{total_nodes} "
                f"collapse={task.get('collapse_approved')} trace={task.get('trace_passed')} "
                f"manifest={task.get('manifest_passed')} "
                f"cachewiki={task.get('lake_cache_citation_coverage')} "
                f"force={task.get('force_average')} error={task.get('error')} "
                f"log_tail={(task.get('log_tail') or '')[-1200:]}"
            )
            emit_error(f"QSM failed {task.get('name')}", msg)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
