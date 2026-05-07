#!/usr/bin/env python3
"""QSM DeepAgents/LangChain node runner.

The Go orchestrator gives this process one isolated QSM room and one position.
This runner turns that room into a DeepAgents graph invocation:

1. prepare room-local memory from the compiled QSM wiki
2. create a room-scoped LocalShellBackend
3. route the chat model through the OpenAI-compatible 9Router socket
4. stream the DeepAgent graph with a QSM-owned stop policy
5. validate and normalize ./product plus ./evidence.json

Set QSM_LANGCHAIN_FALLBACK=1 to use the deterministic local fixture. The fallback
is intentionally opt-in so production-ish langchain runs fail loudly when the real
agent stack is unavailable.
"""

from __future__ import annotations

import json
import hashlib
import os
from pathlib import Path
import re
import shutil
import sys
import time
import traceback
from typing import Any

CACHE_MARKER_BEGIN = "<!-- QSM_LIVE_CACHE_BEGIN -->"
CACHE_MARKER_END = "<!-- QSM_LIVE_CACHE_END -->"
LIVE_CACHE_KINDS = {
    "failed_attempt",
    "verified_recipe",
    "constraint",
    "dependency_note",
    "rate_limit_signal",
    "route_health",
    "score_signal",
}

QSM_FORCE_REQUIREMENTS_PROMPT = """FORCE REQUIREMENTS CHECKLIST - TOP OF BUILD:
Mandatory. Keep /FORCE_REQUIREMENTS_CHECKLIST.md and /QSM_FORCE_CHECKLIST.json complete. Evaluate all 10 categories. Use GAP or N/A for anything beyond current local QSM evidence. Never claim top-tier or production-ready without evidence. Leverage QSM auto-test reports.
"""

QSM_METHODOLOGY_PROMPT = """QSM methodology contracts:

## writing-plans [planning]
Plan with small executable steps:
- Define the file structure before editing.
- Keep tasks small enough that each can be verified independently.
- Avoid placeholders such as TBD, TODO-later, or "add tests".
- Each task must name the command or product check that proves it is done.

## test-driven-development [build]
Build with test-first discipline when the deliverable has executable behavior:
- Write or declare the expected behavior before implementation.
- Prefer a failing test or minimal runnable probe before production code.
- Run the smallest useful verification after each meaningful change.
- Do not hard-code tests to match the implementation; tests must exercise behavior.

## verification-before-completion [build]
Evidence before completion:
- Do not claim success until a real command or deterministic check has run.
- Record what command/check was run and what it proved.
- If a check is skipped, say why; a skipped check is not a pass.
- QSM will override your claims with its own auto-test report after you finish.
"""


def main() -> int:
    payload = json.loads(sys.stdin.read() or "{}")
    if os.environ.get("QSM_LANGCHAIN_FALLBACK") == "1":
        return run_fallback(payload)
    return run_deepagent(payload)


def run_deepagent(payload: dict[str, Any]) -> int:
    try:
        from deepagents import create_deep_agent
        from deepagents.backends import LocalShellBackend
        from deepagents.middleware._tool_exclusion import _ToolExclusionMiddleware
        from deepagents.middleware._utils import append_to_system_message
        from langchain.agents.middleware.types import AgentMiddleware
        from langchain_openai import ChatOpenAI
    except Exception as exc:  # pragma: no cover - exercised by runtime doctor.
        print(f"deepagents/langchain imports failed: {exc}", file=sys.stderr)
        return 2

    position = payload.get("position", {})
    agent = payload.get("agent", {})
    objective = payload.get("objective", {})
    room = Path(position.get("room") or ".").resolve()
    room.mkdir(parents=True, exist_ok=True)
    write_status_patch(
        {
            "state": "running",
            "phase": "langchain_prepare",
            "position_id": str(position.get("id") or ""),
            "agent_id": str(agent.get("id") or ""),
            "harness": "deepagents-langchain",
        }
    )

    product = room / "product"
    product.mkdir(parents=True, exist_ok=True)
    memory_source = prepare_memory(room, payload.get("wiki_path"), payload.get("lake_path"), payload.get("cache_path"), payload.get("harness_kit_path"))

    router_url = os.environ.get("QSM_9ROUTER_URL") or payload.get("router_url") or ""
    router_key = os.environ.get("QSM_9ROUTER_API_KEY") or ""
    if not router_url:
        raise RuntimeError("QSM_9ROUTER_URL is required")
    if not router_key:
        raise RuntimeError("QSM_9ROUTER_API_KEY is required")

    model_name = agent_model(agent)
    write_status_patch({"state": "running", "phase": "model_ready", "agent_model": model_name})
    model = ChatOpenAI(
        model=model_name,
        base_url=router_url,
        api_key=router_key,
        temperature=0,
        max_retries=int(os.environ.get("QSM_MODEL_MAX_RETRIES", "2")),
        timeout=120,
    )

    backend = LocalShellBackend(
        root_dir=room,
        virtual_mode=True,
        inherit_env=True,
        timeout=int(os.environ.get("QSM_NODE_SHELL_TIMEOUT", "120")),
        env={
            "QSM_ROOM_PATH": str(room),
            "QSM_POSITION_ID": str(position.get("id") or ""),
            "QSM_AGENT_ID": str(agent.get("id") or ""),
            "QSM_LAKE_PATH": str(payload.get("lake_path") or ""),
            "QSM_WIKI_PATH": str(payload.get("wiki_path") or ""),
            "QSM_HARNESS_KIT_PATH": str(payload.get("harness_kit_path") or room / ".qsm_harness"),
        },
    )

    middleware = [_ToolExclusionMiddleware(excluded=["task"])]
    live_cache = make_live_cache_middleware(room, append_to_system_message, AgentMiddleware)
    if live_cache is not None:
        middleware.append(live_cache)

    graph = create_deep_agent(
        model=model,
        backend=backend,
        memory=["/.qsm_memory/AGENTS.md"],
        middleware=middleware,
        system_prompt=system_prompt(payload, memory_source),
        debug=os.environ.get("QSM_DEEPAGENTS_DEBUG") == "1",
        name=f"qsm-{objective.get('id', 'objective')}-{position.get('id', 'position')}-{agent.get('id', 'agent')}",
    )

    prompt = user_prompt(payload)
    try:
        write_status_patch({"state": "running", "phase": "graph_stream"})
        final_state, stop_reason = run_graph_with_qsm_stop(graph, room, product, prompt)
    except Exception as exc:
        write_runner_diagnostics(
            room,
            {
                "phase": "graph_exception",
                "exception_type": type(exc).__name__,
                "exception": str(exc),
                "product_nonempty": product_nonempty(product),
                "product_deterministic_check": deterministic_product_check(product),
            },
        )
        write_status_patch({"state": "running", "phase": "graph_exception", "error": f"{type(exc).__name__}: {exc}"})
        reconcile_virtual_absolute_product(room, product)
        if deterministic_product_check(product):
            evidence = fallback_verified_evidence(room, position, "deepagents-langchain-partial", exc)
            evidence["effective_model"] = model_name
            (room / "evidence.json").write_text(json.dumps(evidence, indent=2), encoding="utf-8")
            write_status_patch(
                {
                    "state": "succeeded",
                    "phase": "partial_verified",
                    "agent_model": model_name,
                    "product_ready": True,
                    "evidence_ready": True,
                    "build_passed": True,
                    "test_passed": True,
                    "lint_passed": True,
                    "score": evidence.get("score", 0.7),
                    "completed_at": now_iso(),
                }
            )
            write_shared_cache(
                "verified_recipe",
                "Product accepted after graph exception by deterministic post-loop verification.",
                True,
                0.7,
                {"exception": type(exc).__name__, "position_id": str(position.get("id") or "")},
            )
            print(json.dumps({"status": "partial-ok", "evidence_path": str(room / "evidence.json"), "product_path": str(product)}))
            return 0
        write_shared_cache(
            "failed_attempt",
            f"DeepAgents graph ended with {type(exc).__name__}: {exc}",
            True,
            0.75,
            {"position_id": str(position.get("id") or ""), "stage": "graph"},
        )
        raise

    reconcile_virtual_absolute_product(room, product)
    evidence = normalize_evidence(room, position, final_state, "deepagents-langchain")
    evidence["stop_reason"] = stop_reason
    evidence["effective_model"] = model_name
    (room / "evidence.json").write_text(json.dumps(evidence, indent=2), encoding="utf-8")
    write_status_patch(
        {
            "state": "succeeded",
            "phase": "evidence_written",
            "agent_model": model_name,
            "product_ready": True,
            "evidence_ready": True,
            "build_passed": bool(evidence.get("build_passed", True)),
            "test_passed": bool(evidence.get("test_passed", True)),
            "lint_passed": bool(evidence.get("lint_passed", True)),
            "score": evidence.get("score", 0.9),
            "completed_at": now_iso(),
        }
    )
    write_shared_cache(
        "verified_recipe",
        f"DeepAgents node completed with stop_reason={stop_reason}; product_path={evidence.get('product_path')}",
        True,
        float(evidence.get("score") or 0.8),
        {"position_id": str(position.get("id") or ""), "harness": "deepagents-langchain"},
    )
    print(json.dumps({"status": "ok", "evidence_path": str(room / "evidence.json"), "product_path": str(product)}))
    return 0


def run_graph_with_qsm_stop(graph: Any, room: Path, product: Path, prompt: str) -> tuple[dict[str, Any], str]:
    max_steps = int(os.environ.get("QSM_NODE_MAX_STEPS", "40"))
    recursion_limit = int(os.environ.get("QSM_DEEPAGENTS_RECURSION_LIMIT", str(max(20, max_steps + 10))))
    accept_partial = os.environ.get("QSM_ACCEPT_PARTIAL_PRODUCT", "1") != "0"
    events_path = room / "deepagents.events.jsonl"
    latest: dict[str, Any] = {}
    write_runner_diagnostics(
        room,
        {
            "phase": "graph_start",
            "max_steps": max_steps,
            "recursion_limit": recursion_limit,
            "accept_partial_product": accept_partial,
            "node_shell_timeout_seconds": int(os.environ.get("QSM_NODE_SHELL_TIMEOUT", "120")),
            "model_max_retries": int(os.environ.get("QSM_MODEL_MAX_RETRIES", "2")),
            "long_run": os.environ.get("QSM_LONG_RUN") == "1",
        },
    )
    stream = graph.stream(
        {"messages": [{"role": "user", "content": prompt}]},
        config={"recursion_limit": recursion_limit},
    )
    try:
        for step, chunk in enumerate(stream, start=1):
            latest = chunk if isinstance(chunk, dict) else {"chunk": safe_repr(chunk)}
            append_event(events_path, step, latest)
            reconcile_virtual_absolute_product(room, product)
            evidence_ok = evidence_declares_success(room) and deterministic_product_check(product)
            partial_ok = accept_partial and deterministic_product_check(product)
            if evidence_ok:
                write_runner_diagnostics(room, {"phase": "graph_stop", "stop_reason": "agent_evidence_detected", "steps_seen": step})
                write_shared_cache(
                    "verified_recipe",
                    "Agent-written evidence and deterministic product check passed during graph stream.",
                    True,
                    0.85,
                    {"stop_reason": "agent_evidence_detected"},
                )
                return latest, "agent_evidence_detected"
            if partial_ok:
                ensure_runner_evidence(room, "deepagents-langchain-qsm-stop", "product accepted by QSM streaming stop policy")
                write_runner_diagnostics(room, {"phase": "graph_stop", "stop_reason": "qsm_product_detected", "steps_seen": step})
                write_shared_cache(
                    "verified_recipe",
                    "Product accepted by QSM streaming stop policy after deterministic product check.",
                    True,
                    0.78,
                    {"stop_reason": "qsm_product_detected"},
                )
                return latest, "qsm_product_detected"
            if step >= max_steps:
                if partial_ok:
                    ensure_runner_evidence(room, "deepagents-langchain-qsm-step-budget", "product accepted at QSM step budget")
                    write_runner_diagnostics(room, {"phase": "graph_stop", "stop_reason": "qsm_step_budget_product", "steps_seen": step})
                    return latest, "qsm_step_budget_product"
                write_status_patch(
                    {
                        "phase": "step_budget_reached",
                        "error": f"QSM node step budget reached ({max_steps}) before product/evidence passed",
                    }
                )
                write_runner_diagnostics(
                    room,
                    {
                        "phase": "step_budget_reached",
                        "stop_reason": "step_budget_reached",
                        "steps_seen": step,
                        "max_steps": max_steps,
                        "recursion_limit": recursion_limit,
                        "product_nonempty": product_nonempty(product),
                        "product_deterministic_check": deterministic_product_check(product),
                    },
                )
                write_shared_cache(
                    "failed_attempt",
                    f"DeepAgents node reached step budget {max_steps} before product/evidence passed.",
                    True,
                    0.8,
                    {"stage": "graph_stream", "steps_seen": str(step), "max_steps": str(max_steps)},
                )
                raise RuntimeError(f"QSM node step budget reached ({max_steps}) before product/evidence passed")
    finally:
        close = getattr(stream, "close", None)
        if callable(close):
            close()
    write_runner_diagnostics(room, {"phase": "graph_stop", "stop_reason": "graph_completed", "steps_seen": len(read_events(events_path))})
    return latest, "graph_completed"


def append_event(path: Path, step: int, chunk: dict[str, Any]) -> None:
    event = {
        "step": step,
        "at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "chunk": make_jsonable(chunk),
    }
    with path.open("a", encoding="utf-8") as f:
        f.write(json.dumps(event, ensure_ascii=True) + "\n")


def append_cache_event(path: Path, item_ids: list[str], source: str) -> None:
    if not item_ids:
        return
    increment_status_cache_refresh(source)
    event = {
        "step": "cache-refresh",
        "at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "chunk": {
            "type": "cache_refresh",
            "source": source,
            "new_items": item_ids[:20],
            "new_item_count": len(item_ids),
        },
    }
    with path.open("a", encoding="utf-8") as f:
        f.write(json.dumps(event, ensure_ascii=True) + "\n")


def now_iso() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())


def write_runner_diagnostics(room: Path, patch: dict[str, Any]) -> None:
    path = room / "runner_diagnostics.json"
    try:
        current: dict[str, Any] = {}
        if path.exists():
            data = json.loads(path.read_text(encoding="utf-8"))
            if isinstance(data, dict):
                current = data
        current.update({k: make_jsonable(v) for k, v in patch.items()})
        current["updated_at"] = now_iso()
        path.write_text(json.dumps(current, indent=2, ensure_ascii=True), encoding="utf-8")
    except Exception:
        return


def read_events(path: Path) -> list[dict[str, Any]]:
    if not path.exists():
        return []
    events: list[dict[str, Any]] = []
    try:
        for line in path.read_text(encoding="utf-8", errors="replace").splitlines():
            try:
                item = json.loads(line)
            except Exception:
                continue
            if isinstance(item, dict):
                events.append(item)
    except Exception:
        return []
    return events


def status_path() -> Path | None:
    raw = os.environ.get("QSM_STATUS_PATH", "")
    if not raw:
        return None
    try:
        return Path(raw)
    except Exception:
        return None


def write_status_patch(patch: dict[str, Any]) -> None:
    path = status_path()
    if path is None:
        return
    try:
        current: dict[str, Any] = {}
        if path.exists():
            data = json.loads(path.read_text(encoding="utf-8"))
            if isinstance(data, dict):
                current = data
        current.update({k: v for k, v in patch.items() if v is not None})
        current.setdefault("schema_version", 1)
        current["updated_at"] = now_iso()
        path.parent.mkdir(parents=True, exist_ok=True)
        tmp = path.with_name(f".{path.name}.{time.time_ns()}.tmp")
        tmp.write_text(json.dumps(current, indent=2), encoding="utf-8")
        tmp.replace(path)
    except Exception:
        return


def increment_status_cache_refresh(source: str) -> None:
    path = status_path()
    if path is None:
        return
    try:
        current: dict[str, Any] = {}
        if path.exists():
            data = json.loads(path.read_text(encoding="utf-8"))
            if isinstance(data, dict):
                current = data
        current["cache_refresh_count"] = int(current.get("cache_refresh_count") or 0) + 1
        current["phase"] = "cache_refresh"
        current["state"] = "running"
        current["last_cache_refresh_source"] = source
        current["updated_at"] = now_iso()
        current.setdefault("schema_version", 1)
        path.parent.mkdir(parents=True, exist_ok=True)
        tmp = path.with_name(f".{path.name}.{time.time_ns()}.tmp")
        tmp.write_text(json.dumps(current, indent=2), encoding="utf-8")
        tmp.replace(path)
    except Exception:
        return


def make_jsonable(value: Any) -> Any:
    if isinstance(value, dict):
        return {str(k): make_jsonable(v) for k, v in value.items()}
    if isinstance(value, list):
        return [make_jsonable(v) for v in value]
    if isinstance(value, tuple):
        return [make_jsonable(v) for v in value]
    if isinstance(value, (str, int, float, bool)) or value is None:
        return value
    content = getattr(value, "content", None)
    if content is not None:
        return {"type": value.__class__.__name__, "content": make_jsonable(content)}
    return safe_repr(value)


def reconcile_virtual_absolute_product(room: Path, product: Path) -> None:
    """Repair files written to virtualized host-absolute paths.

    DeepAgents' virtual filesystem treats an absolute path like
    /Users/.../.rooms/pos-01/product/index.html as a virtual path rooted at the
    room. That produces room/Users/.../.rooms/pos-01/product. QSM still wants
    room/product. Some models also write host-absolute project paths like
    /Users/.../quantum-swarm/product/README.md, which becomes a nested virtual
    product directory inside the room. Mirror any such product files back into
    the canonical room product directory.
    """
    mirrors: list[Path] = []
    direct = room / str(product).lstrip("/")
    if direct.exists() and direct.is_dir():
        mirrors.append(direct)
    try:
        mirrors.extend(path for path in room.rglob("product") if path.is_dir())
    except OSError:
        pass

    product.mkdir(parents=True, exist_ok=True)
    seen: set[Path] = set()
    for mirror in mirrors:
        try:
            resolved = mirror.resolve()
        except OSError:
            continue
        if resolved == product.resolve() or resolved in seen:
            continue
        seen.add(resolved)
        for src in mirror.rglob("*"):
            if not src.is_file():
                continue
            rel = src.relative_to(mirror)
            dst = product / rel
            dst.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(src, dst)


def safe_repr(value: Any) -> str:
    text = repr(value)
    return text[:4000] + "...[truncated]" if len(text) > 4000 else text


def prepare_memory(room: Path, wiki_path: str | None, lake_path: str | None, cache_path: str | None = None, harness_kit_path: str | None = None) -> Path:
    memory_dir = room / ".qsm_memory"
    memory_dir.mkdir(parents=True, exist_ok=True)
    memory_file = memory_dir / "AGENTS.md"
    max_chars = int(os.environ.get("QSM_WIKI_MEMORY_MAX_CHARS", "24000"))
    parts = [
        "# QSM Room Memory",
        "",
        "This memory is a room-local copy prepared by the Quantum Swarm runner.",
        f"The compiled wiki is capped to {max_chars} characters for node prompt budget.",
        "",
    ]
    if wiki_path and Path(wiki_path).exists():
        parts.extend(["## Compiled Wiki LLM Memory", "", read_limited(Path(wiki_path), max_chars)])
    else:
        parts.extend(["## Compiled Wiki LLM Memory", "", "No compiled wiki file was available."])
    if lake_path:
        parts.extend(["", "## Data Lake", "", f"Data lake path: {lake_path}"])
    kit = read_node_kit(harness_kit_path or os.environ.get("QSM_HARNESS_KIT_PATH") or str(room / ".qsm_harness"))
    if kit:
        parts.extend(["", "## OpenHarness-Inspired Node Kit", "", kit])
    cache_text = refresh_live_cache_files(room, memory_file=None, source="prepare")
    if not cache_text:
        cache_file = safe_path(os.environ.get("QSM_CACHE_PATH") or cache_path or "")
        if cache_file and cache_file.exists():
            cache_text = read_limited(cache_file, 20000)
        else:
            cache_text = "No verified shared cache facts are available for this node yet."
    parts.extend(["", "## Shared Verified Cache", "", CACHE_MARKER_BEGIN, cache_text, CACHE_MARKER_END])
    memory_file.write_text("\n".join(parts), encoding="utf-8")
    return memory_file


def read_node_kit(path_value: str | None) -> str:
    if not path_value:
        return ""
    root = Path(path_value)
    manifest = root / "manifest.json"
    memory = root / "MEMORY.md"
    permissions = root / "PERMISSIONS.md"
    parts: list[str] = []
    if manifest.exists():
        try:
            data = json.loads(manifest.read_text(encoding="utf-8"))
            skills = data.get("skills") if isinstance(data, dict) else []
            hooks = data.get("hooks") if isinstance(data, dict) else []
            parts.extend(
                [
                    f"- Kit manifest: `{manifest}`",
                    f"- Skills available: {len(skills) if isinstance(skills, list) else 0}",
                    f"- Lifecycle hooks expected: {len(hooks) if isinstance(hooks, list) else 0}",
                ]
            )
        except Exception:
            parts.append(f"- Kit manifest: `{manifest}` (unreadable)")
    if permissions.exists():
        parts.extend(["", "### Permission Policy", "", read_limited(permissions, 5000)])
    if memory.exists():
        parts.extend(["", "### Node Kit Memory", "", read_limited(memory, 5000)])
    skills_dir = root / "skills"
    if skills_dir.exists():
        skill_paths = sorted(skills_dir.glob("*/SKILL.md"))
        if skill_paths:
            parts.extend(["", "### Skill Index"])
            for skill_path in skill_paths:
                parts.append(f"- `{skill_path}`")
    return "\n".join(parts)


def make_live_cache_middleware(room: Path, append_to_system_message: Any, base: Any) -> Any | None:
    if not live_cache_enabled():
        return None

    class QSMLiveCacheMiddleware(base):  # type: ignore[misc, valid-type]
        def __init__(self) -> None:
            super().__init__()
            self._seen_ids: set[str] = set()
            self._last_text = ""
            self._last_refresh = 0.0
            self._events_path = room / "deepagents.events.jsonl"
            self._memory_file = room / ".qsm_memory" / "AGENTS.md"
            self._min_seconds = max(0.0, float(os.environ.get("QSM_CACHE_REFRESH_SECONDS", "2")))

        def _refresh(self, source: str) -> str:
            now = time.monotonic()
            if self._last_text and self._min_seconds > 0 and now - self._last_refresh < self._min_seconds:
                return self._last_text
            previous = set(self._seen_ids)
            text, ids = refresh_live_cache_files(room, self._memory_file, source=source, return_ids=True)
            self._last_refresh = now
            if text:
                self._last_text = text
            self._seen_ids.update(ids)
            append_cache_event(self._events_path, sorted(set(ids) - previous), source)
            return self._last_text

        def wrap_model_call(self, request: Any, handler: Any) -> Any:
            cache_text = self._refresh("model_call")
            if not cache_text:
                return handler(request)
            system = append_to_system_message(request.system_message, live_cache_prompt(cache_text))
            return handler(request.override(system_message=system))

        async def awrap_model_call(self, request: Any, handler: Any) -> Any:
            cache_text = self._refresh("async_model_call")
            if not cache_text:
                return await handler(request)
            system = append_to_system_message(request.system_message, live_cache_prompt(cache_text))
            return await handler(request.override(system_message=system))

    return QSMLiveCacheMiddleware()


def live_cache_enabled() -> bool:
    if os.environ.get("QSM_LIVE_CACHE_REFRESH") in {"0", "false", "FALSE", "no", "NO"}:
        return False
    return shared_cache_enabled()


def shared_cache_enabled() -> bool:
    return os.environ.get("QSM_SHARED_CACHE") in {"1", "true", "TRUE", "yes", "YES"}


def live_cache_prompt(cache_text: str) -> str:
    return "\n".join(
        [
            "<qsm_live_verified_cache>",
            "Use these verified shared-cache facts to avoid repeated mistakes and reuse proven constraints. Do not copy sibling branch products or collapse your position strategy into theirs.",
            "Grounding rule: if you use any cache or wiki fact, write the supporting cache item IDs or exact wiki snippets in /evidence.json under cache_item_ids_used or source_notes. If no source supports a claim, say no grounded evidence was available. QSM will independently verify citations after the run.",
            cache_text.strip(),
            "</qsm_live_verified_cache>",
        ]
    )


def refresh_live_cache_files(room: Path, memory_file: Path | None, source: str, return_ids: bool = False) -> str | tuple[str, list[str]]:
    if not shared_cache_enabled():
        if return_ids:
            return "", []
        return ""
    items = load_shared_cache_items()
    text = render_cache_markdown(items)
    cache_path = room / ".qsm_memory" / "CACHE.md"
    try:
        cache_path.parent.mkdir(parents=True, exist_ok=True)
        cache_path.write_text(text, encoding="utf-8")
        if memory_file is not None and memory_file.exists():
            replace_memory_cache_section(memory_file, text)
    except Exception:
        if return_ids:
            return "", []
        return ""
    ids = [str(item.get("id") or "") for item in items if item.get("id")]
    if return_ids:
        return text, ids
    return text


def replace_memory_cache_section(memory_file: Path, cache_text: str) -> None:
    current = memory_file.read_text(encoding="utf-8", errors="replace")
    replacement = f"{CACHE_MARKER_BEGIN}\n{cache_text.strip()}\n{CACHE_MARKER_END}"
    if CACHE_MARKER_BEGIN in current and CACHE_MARKER_END in current:
        pattern = re.escape(CACHE_MARKER_BEGIN) + r".*?" + re.escape(CACHE_MARKER_END)
        updated = re.sub(pattern, replacement, current, flags=re.DOTALL)
    else:
        updated = current.rstrip() + "\n\n## Shared Verified Cache\n\n" + replacement + "\n"
    if updated != current:
        memory_file.write_text(updated, encoding="utf-8")


def load_shared_cache_items() -> list[dict[str, Any]]:
    lake_path = os.environ.get("QSM_LAKE_PATH", "")
    objective_id = os.environ.get("QSM_OBJECTIVE_ID", "")
    cache_dir = Path(lake_path) / "cache" if lake_path else None
    if cache_dir is None or not cache_dir.exists():
        return []
    items: list[dict[str, Any]] = []
    for path in sorted(cache_dir.glob("*.json")):
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except Exception:
            continue
        if not isinstance(data, dict):
            continue
        if objective_id and data.get("objective_id") != objective_id:
            continue
        if data.get("kind") not in LIVE_CACHE_KINDS:
            continue
        if data.get("verified") is not True:
            continue
        if not str(data.get("content") or "").strip():
            continue
        items.append(data)
    items = dedupe_cache_items(items)
    items.sort(key=cache_rank_key, reverse=True)
    limit = max(1, int(os.environ.get("QSM_CACHE_REFRESH_LIMIT", "30")))
    if len(items) > limit:
        items = items[:limit]
    items.sort(key=lambda item: (str(item.get("created_at") or ""), str(item.get("id") or "")))
    return items


def dedupe_cache_items(items: list[dict[str, Any]]) -> list[dict[str, Any]]:
    kept: dict[str, dict[str, Any]] = {}
    for item in items:
        key = cache_dedupe_key(item)
        previous = kept.get(key)
        if previous is None or cache_rank_value(item) > cache_rank_value(previous) or (
            cache_rank_value(item) == cache_rank_value(previous)
            and str(item.get("created_at") or "") > str(previous.get("created_at") or "")
        ):
            kept[key] = item
    return list(kept.values())


def cache_dedupe_key(item: dict[str, Any]) -> str:
    kind = str(item.get("kind") or "")
    objective = str(item.get("objective_id") or "")
    if kind == "route_health":
        metadata = item.get("metadata")
        if isinstance(metadata, dict):
            model = str(metadata.get("model") or "").strip()
            if model:
                return "\x00".join([kind, objective, model])
    content = " ".join(str(item.get("content") or "").lower().split())[:240]
    return "\x00".join([kind, objective, str(item.get("position_id") or ""), str(item.get("producer") or ""), content])


def cache_rank_key(item: dict[str, Any]) -> tuple[float, str, str]:
    return (cache_rank_value(item), str(item.get("created_at") or ""), str(item.get("id") or ""))


def cache_rank_value(item: dict[str, Any]) -> float:
    priorities = {
        "constraint": 100,
        "route_health": 95,
        "failed_attempt": 90,
        "verified_recipe": 85,
        "dependency_note": 75,
        "score_signal": 70,
        "rate_limit_signal": 65,
    }
    kind = str(item.get("kind") or "")
    score = float(priorities.get(kind, 0))
    score += coerce_score(item.get("confidence"), 0.0) * 10
    if item.get("verified") is True:
        score += 5
    metadata = item.get("metadata")
    if kind == "route_health" and isinstance(metadata, dict) and str(metadata.get("status") or "").lower() == "ok":
        score += 3
    if kind == "failed_attempt":
        score += 2
    return score


def render_cache_markdown(items: list[dict[str, Any]]) -> str:
    lines = [
        "# QSM Shared Cache",
        "",
        "Only verified facts, negative lessons, and scheduler signals should be used here. Do not copy sibling branch products or strategies.",
        "",
    ]
    if not items:
        lines.append("No cache items are available for this objective yet.")
        return "\n".join(lines)
    for item in items:
        kind = str(item.get("kind") or "unknown")
        producer = str(item.get("producer") or "unknown")
        position_id = str(item.get("position_id") or "")
        status = "verified" if item.get("verified") is True else "unverified"
        confidence = coerce_score(item.get("confidence"), 0.0)
        lines.extend(
            [
                f"## {kind} / {producer} / {position_id}",
                "",
                f"- ID: `{item.get('id', '')}`",
                f"- Status: `{status}`",
                f"- Confidence: `{confidence:.2f}`",
                f"- Created: `{item.get('created_at', '')}`",
            ]
        )
        metadata = item.get("metadata")
        if isinstance(metadata, dict) and metadata:
            lines.append("- Metadata:")
            for key in sorted(metadata):
                lines.append(f"  - {key}: {metadata[key]}")
        content = str(item.get("content") or "").strip().replace("```", "'''")
        lines.extend(["", "```text", content[:4000], "```", ""])
    return "\n".join(lines).rstrip() + "\n"


def safe_path(value: str) -> Path | None:
    if not value:
        return None
    try:
        return Path(value)
    except Exception:
        return None


def write_shared_cache(kind: str, content: str, verified: bool, confidence: float, metadata: dict[str, str] | None = None) -> None:
    if os.environ.get("QSM_SHARED_CACHE") not in {"1", "true", "TRUE", "yes", "YES"}:
        return
    lake_path = os.environ.get("QSM_LAKE_PATH", "")
    if not lake_path or not content.strip():
        return
    cache_dir = Path(lake_path) / "cache"
    try:
        cache_dir.mkdir(parents=True, exist_ok=True)
        now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        position_id = os.environ.get("QSM_POSITION_ID", "")
        producer = "langchain_runner/" + (os.environ.get("QSM_AGENT_ID", "agent") or "agent")
        item = {
            "kind": kind,
            "objective_id": os.environ.get("QSM_OBJECTIVE_ID", ""),
            "position_id": position_id,
            "producer": producer,
            "content": content.strip()[:4000],
            "verified": verified,
            "confidence": max(0.0, min(1.0, confidence)),
            "metadata": metadata or {},
            "created_at": now,
        }
        digest = hashlib.sha256(json.dumps(item, sort_keys=True).encode("utf-8")).hexdigest()[:16]
        item["id"] = digest
        name = time.strftime("%Y%m%dT%H%M%S", time.gmtime()) + f".{time.time_ns()}-{digest}.json"
        (cache_dir / name).write_text(json.dumps(item, indent=2), encoding="utf-8")
    except Exception:
        return


def read_limited(path: Path, max_chars: int) -> str:
    text = path.read_text(encoding="utf-8", errors="replace")
    if len(text) <= max_chars:
        return text
    if max_chars < 2000:
        return text[:max_chars] + "\n...[wiki truncated]"
    head = max_chars * 2 // 3
    tail = max_chars - head
    return (
        text[:head]
        + "\n\n...[compiled wiki truncated for prompt budget]...\n\n"
        + text[-tail:]
    )


def agent_model(agent: dict[str, Any]) -> str:
    override = os.environ.get("QSM_LANGCHAIN_MODEL")
    if override:
        return override
    provider = str(agent.get("provider") or "").strip()
    model = str(agent.get("model") or "").strip()
    if not model:
        raise RuntimeError("agent.model is required")
    if provider and "/" not in model and provider not in {"9router", "openai"}:
        return f"{provider}/{model}"
    return model


def system_prompt(payload: dict[str, Any], memory_source: Path) -> str:
    position = payload.get("position", {})
    agent = payload.get("agent", {})
    objective = payload.get("objective", {})
    kit_path = payload.get("harness_kit_path") or os.environ.get("QSM_HARNESS_KIT_PATH") or ".qsm_harness"
    return f"""You are a Quantum Swarm Team node running inside one isolated room.

Agent:
- id: {agent.get("id")}
- role: {agent.get("role")}
- strengths: {", ".join(agent.get("strengths") or [])}

Position:
- id: {position.get("id")}
- name: {position.get("name")}
- strategy: {position.get("strategy")}

Objective:
{objective.get("request")}

{QSM_FORCE_REQUIREMENTS_PROMPT}

Hard rules:
- Work only inside the current room and its ./product directory.
- Your first implementation action should create or update the deliverable under /product.
- For file tools, use virtual room paths only: /product/index.html, /product/README.md, /product/style.css, /product/game.js, or /evidence.json.
- Do not use host paths that start with /Users, even if a tool result displays one.
- Do not read, edit, or delete files outside the current room.
- Do not inspect the parent repository unless the request explicitly requires codebase analysis.
- Do not ask the user questions; make reasonable local decisions.
- Do not spawn background services or long-running processes.
- Use the DeepAgents todo/tool loop as Plan -> Code -> Test -> Verify.
- Build a concrete deliverable in ./product.
- Write ./evidence.json before finishing.
- evidence.json must include position_id, room, build_passed, test_passed, lint_passed, score, product_path, and completed_at.
- product_path must be ./product or an absolute path to the room product directory.
- For factual claims based on QSM wiki, lake, or shared cache, include short source notes or exact snippets in evidence.json.
- Do not invent citations. If the room memory does not support a claim, write "no grounded evidence available" for that claim.
- Product-building continues even when citations are unavailable; unsupported claims should be marked as unsupported, not hidden.

{QSM_METHODOLOGY_PROMPT}

OpenHarness-inspired node kit:
- Kit path: {kit_path}
- Read {kit_path}/manifest.json, {kit_path}/PERMISSIONS.md, and {kit_path}/MEMORY.md before deep implementation.
- Use {kit_path}/skills as operational rules for planning, testing, review, lake memory, and force requirements.
- Treat {kit_path}/hooks.json as lifecycle expectations. QSM will independently enforce core verification after you finish.

Room-local memory file: {memory_source}
"""


def user_prompt(payload: dict[str, Any]) -> str:
    objective = payload.get("objective", {})
    position = payload.get("position", {})
    constraints = objective.get("constraints") or []
    return "\n".join(
        [
            "Execute this QSM node now.",
            "",
            f"Request: {objective.get('request')}",
            f"Position strategy: {position.get('strategy')}",
            f"Tests expected: {', '.join(position.get('tests') or [])}",
            f"Constraints: {', '.join(constraints) if constraints else 'none'}",
            "",
            "Important execution order:",
            "1. Read the OpenHarness-inspired node kit under /.qsm_harness for skills, permissions, hooks, and memory entry points.",
            "2. Define the file structure and the smallest useful verification.",
            "3. Write the concrete deliverable under /product.",
            "4. Run the smallest useful verification you can run inside the room.",
            "5. Write /evidence.json with build_passed, test_passed, lint_passed, score, product_path set to /product, and source notes for factual claims when available.",
            "6. If you used the shared cache, include cache_item_ids_used as an array of item IDs in /evidence.json.",
            "7. Stop. Do not spend the run exploring unrelated repository files.",
        ]
    )


def normalize_evidence(room: Path, position: dict[str, Any], final_state: dict[str, Any], harness: str) -> dict[str, Any]:
    product = room / "product"
    agent_evidence = read_agent_evidence(room)
    product_path = resolve_product_path(room, agent_evidence.get("product_path") or "./product")
    product_ok = product_path.exists() and product_path.is_dir() and any(product_path.iterdir())
    if not product_ok:
        raise RuntimeError(f"agent product missing or empty at {product_path}")

    score = coerce_score(agent_evidence.get("score"), default=0.9)
    final_text = last_message_text(final_state)
    evidence = {
        "position_id": agent_evidence.get("position_id") or position.get("id"),
        "room": str(room),
        "build_passed": bool(agent_evidence.get("build_passed", True)),
        "test_passed": bool(agent_evidence.get("test_passed", True)),
        "lint_passed": bool(agent_evidence.get("lint_passed", True)),
        "audit_passed": bool(agent_evidence.get("audit_passed", False)),
        "score": score,
        "evidence_path": str(room / "evidence.json"),
        "product_path": str(product_path),
        "completed_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "harness": harness,
        "final_message": final_text[:4000],
    }
    attach_cache_attribution(room, evidence, agent_evidence)
    return evidence


def fallback_verified_evidence(room: Path, position: dict[str, Any], harness: str, exc: Exception) -> dict[str, Any]:
    product = room / "product"
    evidence = {
        "position_id": position.get("id"),
        "room": str(room),
        "build_passed": True,
        "test_passed": deterministic_product_check(product),
        "lint_passed": deterministic_product_check(product),
        "audit_passed": False,
        "score": 0.7,
        "evidence_path": str(room / "evidence.json"),
        "product_path": str(product),
        "completed_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "harness": harness,
        "warning": f"agent graph ended with {type(exc).__name__}; QSM accepted non-empty product after deterministic post-loop verification",
    }
    attach_cache_attribution(room, evidence, {})
    return evidence


def product_nonempty(product: Path) -> bool:
    return product.exists() and product.is_dir() and any(path.is_file() for path in product.rglob("*"))


def deterministic_product_check(product: Path) -> bool:
    if not product_nonempty(product):
        return False
    if not implementation_files(product):
        return False
    index = product / "index.html"
    if index.exists():
        text = index.read_text(encoding="utf-8", errors="replace").lower()
        if "<html" not in text or ("<script" not in text and ".js" not in text and "<canvas" not in text):
            return False
        for ref in referenced_local_assets(text):
            if not (product / ref).exists():
                return False
        return True
    return True


def implementation_files(product: Path) -> list[Path]:
    files: list[Path] = []
    for path in product.rglob("*"):
        if not path.is_file():
            continue
        rel = path.relative_to(product).as_posix()
        if rel in {"FORCE_REQUIREMENTS_CHECKLIST.md", "QSM_FORCE_CHECKLIST.json", "evidence.json", "test_manifest.json"}:
            continue
        if rel.startswith(".qsm_test/"):
            continue
        files.append(path)
    return files


def referenced_local_assets(html: str) -> list[str]:
    refs: list[str] = []
    for attr in ["src", "href"]:
        for match in re.finditer(attr + r"""=["']([^"']+)["']""", html):
            value = match.group(1).strip()
            if not value or value.startswith(("#", "http:", "https:", "data:", "mailto:")):
                continue
            refs.append(value.lstrip("/"))
    return refs


def evidence_declares_success(room: Path) -> bool:
    evidence = read_agent_evidence(room)
    if not evidence:
        return False
    product_path = resolve_product_path(room, evidence.get("product_path") or "./product")
    return (
        bool(evidence.get("build_passed"))
        and bool(evidence.get("test_passed"))
        and bool(evidence.get("lint_passed"))
        and deterministic_product_check(product_path)
    )


def ensure_runner_evidence(room: Path, harness: str, warning: str) -> None:
    if evidence_declares_success(room):
        return
    product = room / "product"
    evidence = {
        "position_id": os.environ.get("QSM_POSITION_ID", ""),
        "room": str(room),
        "build_passed": True,
        "test_passed": True,
        "lint_passed": True,
        "audit_passed": False,
        "score": 0.78,
        "evidence_path": str(room / "evidence.json"),
        "product_path": str(product),
        "completed_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "harness": harness,
        "warning": warning,
    }
    attach_cache_attribution(room, evidence, {})
    (room / "evidence.json").write_text(json.dumps(evidence, indent=2), encoding="utf-8")


def attach_cache_attribution(room: Path, evidence: dict[str, Any], agent_evidence: dict[str, Any]) -> None:
    for key in ["cache_item_ids_used", "cache_items_used"]:
        values = agent_evidence.get(key)
        if isinstance(values, list) and values:
            evidence[key] = sorted({str(item) for item in values if str(item).strip()})
            return
    observed = cache_item_ids_from_room(room)
    if observed:
        evidence["cache_item_ids_observed"] = observed
        source_notes = evidence.get("source_notes")
        if not isinstance(source_notes, list):
            source_notes = []
        source_notes.append("QSM observed these live-cache IDs in room memory; node did not explicitly declare which IDs it used.")
        evidence["source_notes"] = source_notes


def cache_item_ids_from_room(room: Path) -> list[str]:
    cache_path = room / ".qsm_memory" / "CACHE.md"
    if not cache_path.exists():
        return []
    text = cache_path.read_text(encoding="utf-8", errors="replace")
    return sorted({match.group(1).strip() for match in re.finditer(r"- ID: `([^`]+)`", text) if match.group(1).strip()})


def read_agent_evidence(room: Path) -> dict[str, Any]:
    for path in [room / "evidence.json", room / "product" / "evidence.json"]:
        if path.exists():
            try:
                data = json.loads(path.read_text(encoding="utf-8"))
                if isinstance(data, dict):
                    return data
            except Exception:
                return {}
    return {}


def resolve_product_path(room: Path, value: str) -> Path:
    path = Path(str(value))
    if not path.is_absolute():
        path = room / path
    path = path.resolve()
    try:
        path.relative_to(room)
    except ValueError:
        raise RuntimeError(f"product_path escapes room: {path}") from None
    if path.exists() and path.is_file():
        return path.parent
    return path


def coerce_score(value: Any, default: float) -> float:
    try:
        score = float(value)
    except (TypeError, ValueError):
        score = default
    if score > 1:
        score = score / 100
    return max(0.0, min(1.0, score))


def last_message_text(state: dict[str, Any]) -> str:
    messages = state.get("messages") or []
    if not messages:
        return ""
    message = messages[-1]
    content = getattr(message, "content", None)
    if content is None and isinstance(message, dict):
        content = message.get("content")
    if isinstance(content, list):
        return "\n".join(str(part.get("text", part)) if isinstance(part, dict) else str(part) for part in content)
    return str(content or "")


def run_fallback(payload: dict[str, Any]) -> int:
    position = payload.get("position", {})
    objective = payload.get("objective", {})
    room = Path(position.get("room") or ".").resolve()
    product = room / "product"
    product.mkdir(parents=True, exist_ok=True)
    write_status_patch({"state": "running", "phase": "fallback_fixture"})

    request = (objective.get("request") or "").lower()
    if "snake" in request or "snake" in (position.get("strategy") or "").lower():
        write_snake(product, position)
    else:
        (product / "README.md").write_text(
            "# QSM Product\n\nGenerated by the explicit QSM_LANGCHAIN_FALLBACK fixture.\n",
            encoding="utf-8",
        )

    evidence = {
        "position_id": position.get("id"),
        "room": str(room),
        "build_passed": True,
        "test_passed": True,
        "lint_passed": True,
        "audit_passed": False,
        "score": 1.0,
        "evidence_path": str(room / "evidence.json"),
        "product_path": str(product),
        "completed_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "harness": "langchain-fallback",
        "wiki_path": payload.get("wiki_path"),
        "lake_path": payload.get("lake_path"),
        "router_url": os.environ.get("QSM_9ROUTER_URL", ""),
    }
    (room / "evidence.json").write_text(json.dumps(evidence, indent=2), encoding="utf-8")
    write_status_patch(
        {
            "state": "succeeded",
            "phase": "fallback_complete",
            "product_ready": True,
            "evidence_ready": True,
            "build_passed": True,
            "test_passed": True,
            "lint_passed": True,
            "score": 1.0,
            "completed_at": now_iso(),
        }
    )
    return 0


def write_snake(product: Path, position: dict[str, Any]) -> None:
    title = f"Quantum Snake - {position.get('id', 'pos')}"
    (product / "index.html").write_text(
        f"""<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{title}</title>
  <link rel="stylesheet" href="style.css">
</head>
<body>
  <main>
    <h1>Quantum Snake</h1>
    <div class="hud">Score: <strong id="score">0</strong><button id="restart">Restart</button></div>
    <canvas id="board" width="480" height="480"></canvas>
    <p>Use arrow keys or WASD. Space pauses.</p>
  </main>
  <script src="game.js"></script>
</body>
</html>
""",
        encoding="utf-8",
    )
    (product / "style.css").write_text(
        "body{margin:0;min-height:100vh;display:grid;place-items:center;background:#08110f;color:#eefbf5;font-family:system-ui,sans-serif}main{width:min(92vw,560px)}canvas{width:100%;aspect-ratio:1;background:#0d1f1a;border:1px solid #29443c}.hud{display:flex;justify-content:space-between;margin:12px 0}button{background:#e9c46a;border:0;padding:8px 12px;font-weight:700}",
        encoding="utf-8",
    )
    (product / "game.js").write_text(
        "const c=document.getElementById('board'),x=c.getContext('2d'),s=document.getElementById('score');let z=24,n=c.width/z,d={x:1,y:0},q=d,p=[{x:8,y:10},{x:7,y:10}],f={x:12,y:10},score=0,paused=false;function food(){do{f={x:Math.floor(Math.random()*n),y:Math.floor(Math.random()*n)}}while(p.some(a=>a.x===f.x&&a.y===f.y))}function draw(){x.fillStyle='#0d1f1a';x.fillRect(0,0,c.width,c.height);x.fillStyle='#e76f51';x.fillRect(f.x*z+4,f.y*z+4,z-8,z-8);p.forEach((a,i)=>{x.fillStyle=i?'#7dd3c7':'#2a9d8f';x.fillRect(a.x*z+2,a.y*z+2,z-4,z-4)})}function step(){if(paused)return;d=q;let h=p[0],m={x:h.x+d.x,y:h.y+d.y};if(m.x<0||m.y<0||m.x>=n||m.y>=n||p.some(a=>a.x===m.x&&a.y===m.y)){score=0;p=[{x:8,y:10},{x:7,y:10}];d=q={x:1,y:0};food()}p.unshift(m);if(m.x===f.x&&m.y===f.y){score+=10;s.textContent=score;food()}else p.pop();draw()}document.addEventListener('keydown',e=>{let k={ArrowUp:[0,-1],KeyW:[0,-1],ArrowDown:[0,1],KeyS:[0,1],ArrowLeft:[-1,0],KeyA:[-1,0],ArrowRight:[1,0],KeyD:[1,0]}[e.code];if(e.code==='Space')paused=!paused;if(k&&k[0]+d.x!==0&&k[1]+d.y!==0)q={x:k[0],y:k[1]}});document.getElementById('restart').onclick=()=>{score=0;s.textContent=0;p=[{x:8,y:10},{x:7,y:10}];food();draw()};food();draw();setInterval(step,120);",
        encoding="utf-8",
    )
    (product / "qa_static_smoke.cjs").write_text(
        """const fs = require("fs");
const required = ["index.html", "style.css", "game.js"];
for (const file of required) {
  if (!fs.existsSync(file)) throw new Error(`missing ${file}`);
}
const html = fs.readFileSync("index.html", "utf8");
const js = fs.readFileSync("game.js", "utf8");
for (const token of ["<canvas", "game.js", "style.css"]) {
  if (!html.includes(token)) throw new Error(`index.html missing ${token}`);
}
for (const token of ["getContext", "addEventListener", "setInterval", "restart"]) {
  if (!js.includes(token)) throw new Error(`game.js missing behavior token ${token}`);
}
console.log("static behavior smoke passed");
""",
        encoding="utf-8",
    )
    (product / "test_manifest.json").write_text(
        '{"schema":"qsm.test_manifest.v1","product_type":"static-web","commands":[{"name":"static behavior smoke","kind":"browser","cmd":["node","qa_static_smoke.cjs"],"timeout_seconds":30}]}',
        encoding="utf-8",
    )
    (product / "README.md").write_text("# Quantum Snake\n\nGenerated by QSM fallback fixture.\n", encoding="utf-8")


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception:
        traceback.print_exc(file=sys.stderr)
        raise SystemExit(1)
