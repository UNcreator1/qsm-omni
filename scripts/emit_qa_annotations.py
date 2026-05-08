#!/usr/bin/env python3
"""Emit compact GitHub Actions annotations for failed QSM QA gates."""

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
    path = Path(sys.argv[1] if len(sys.argv) > 1 else ".state/qa_report.json")
    if not path.exists():
        emit_error("QSM QA report missing", f"{path} was not created")
        return 0

    report = json.loads(path.read_text())
    for gate in report.get("gates", []):
        if gate.get("status") != "FAIL" or not gate.get("required"):
            continue
        msg = (
            f"profile={report.get('profile')} gate={gate.get('id')} "
            f"evidence={gate.get('evidence')} recommendation={gate.get('recommendation')}"
        )
        emit_error(f"QSM QA failed {gate.get('id')}", msg)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
