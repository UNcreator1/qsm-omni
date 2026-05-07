# Quantum Swarm V3 Production Hardening Plan

Date: 2026-05-04

This plan upgrades QSM from a local multi-node product builder into a production-grade autonomous build service. Current QSM can plan, hydrate a lake, run isolated rooms, verify products, collapse a winner, score force requirements, and run repeated autorun cycles. It is not yet enterprise production because hard isolation, benchmark evidence, terminal session supervision, and exact cost enforcement are still incomplete.

## Current Implemented Abilities

- Phase A planning/data lake gate checks local materials before Chop and writes planning artifacts into `.lake/`.
- Phase B room execution supports simulated, OpenCode, and LangChain harnesses with configurable positions and concurrency.
- Shared cache and wiki memory are consumed by nodes through `.qsm_memory/AGENTS.md` and `.qsm_memory/CACHE.md`.
- Phase C deterministic collapse uses verifier evidence, tests, force checklist scoring, lake interaction score, and grounded citation mapping.
- `qsm autorun` runs repeated local cycles and keeps `.state/autorun.json` plus per-cycle logs.
- `qsm cost` now writes `.state/cost_report.json` and `.state/cost_report.md`.
- `qsm sandbox` now writes `.state/sandbox_report.json` and `.state/sandbox_report.md`.
- `qsm benchmark` now runs local SWE-bench/Terminal-Bench-shaped smoke tasks and writes `.state/benchmark_report.json` and `.state/benchmark_report.md`.

## Research Anchors

- Docker Engine security documentation says containers are reasonably secure by default when processes run unprivileged, and recommends extra hardening such as AppArmor/SELinux-style layers. It also warns that Docker daemon/API exposure can become privilege escalation if misused.
- Docker's AI Sandbox security model uses microVM isolation, network proxy policy, isolated Docker engines, and credential proxying. This matches QSM's desired hard boundary better than plain local folders or plain host containers.
- Terminal-Bench 2.0 uses hard terminal tasks with unique environments, human-written solutions, and comprehensive verification tests. QSM benchmark tasks should imitate this contract locally, then later add official adapters.
- Agent of Empires shows a practical terminal management pattern for multiple coding agents: tmux-backed persistent sessions, git worktrees, optional Docker sandboxing, TUI/web dashboard, status detection, and diff review. QSM should not copy its code, but should absorb those architectural patterns for OpenCode node supervision.
- SWE-bench-style evaluation is valuable only when measured with cost, duration, pass rate, and reproducible tests. QSM should track internal benchmark evidence first and treat official SWE-bench scores as a later adapter, not as a marketing claim.

## Priority 1: Hard Sandboxing

Target: each node runs in a boundary stronger than a local room directory.

Implementation path:

1. Add `qsm sandbox` readiness detection. Done.
2. Add `internal/sandbox` execution profiles:
   - `room-only`: current local directories.
   - `docker`: container per room, non-root user, dropped capabilities, CPU/memory/pids limits, no host Docker socket, only room mount.
   - `microvm`: preferred production profile when Docker Sandbox or another VM-backed backend exists.
3. Add `-sandbox room|docker|microvm|auto` to `qsm run`, `autorun`, and `benchmark`.
4. Ensure secrets are never written into rooms. Prefer host-side provider proxy injection.
5. Add network policy per node:
   - default deny except 9Router/provider endpoint and approved package registries.
   - record all allowed domains in `.state/sandbox_report.json`.

Acceptance evidence:

- `.state/sandbox_report.json` shows `hard_sandbox_ready=true`.
- Each room status contains sandbox profile, resource limits, network policy, and cleanup result.
- A destructive sandbox probe cannot read files outside the mounted room.

## Priority 2: OpenCode Multi-Node Terminal Supervision

Target: OpenCode becomes equal to LangChain for real multi-node stress.

Implementation path:

1. Keep current parallel room executor as the orchestration layer.
2. Add a terminal supervisor:
   - tmux session per room when `tmux` exists.
   - persistent stdout/stderr capture.
   - stuck/waiting/idle/error state detection.
   - attach command recorded in room status.
3. Add `qsm opencode-stress` or use `qsm benchmark -suite opencode-stress -harness opencode`.
4. Add OpenCode-specific rate-limit handling:
   - provider 429/backoff detection.
   - route switch through 9Router healthy pool.
   - DeepSeek direct fallback only when 9Router fails and user enabled fallback.

Acceptance evidence:

- 7 OpenCode nodes can run concurrently or under configured parallelism without status corruption.
- Every node has terminal logs, status transitions, test reports, and cost report lines.
- Failed/stuck nodes are retried or collapsed out with explicit reason.

## Priority 3: Cost/Token Accounting and Node Allocation

Target: node allocation is decided by budget and expected value, not only hardware.

Implementation path:

1. `qsm cost` estimates token usage from provider metadata when available, falling back to room evidence/log character estimates. Done.
2. Add provider-specific token capture to LangChain and OpenCode evidence.
3. Add budget flags:
   - `-max-cost`
   - `-max-cost-per-node`
   - `-cost-policy conservative|balanced|explore`
4. Modify capacity planner:
   - hardware capacity gives max safe nodes.
   - route health gives model availability.
   - cost policy allocates model/agent/node budgets.
5. Persist route/model ROI:
   - success rate.
   - average cost per success.
   - average test pass rate.
   - average collapse win rate.

Acceptance evidence:

- `.state/cost_report.json` has per-node tokens and cost.
- force checklist category 6 uses cost report evidence.
- route/build health can avoid models with poor cost-per-success.

## Priority 4: Benchmark Suites

Target: QSM can quantify itself like a real agent framework.

Implementation path:

1. Local smoke benchmark. Done:
   - SWE-bench-style local bugfix task.
   - Terminal-Bench-style local terminal product task.
2. OpenCode stress benchmark:
   - 3 to 7 rooms.
   - terminal-heavy tasks.
   - rate-limit/retry/cache evaluation.
3. Reliability benchmark:
   - repeated 24/7 autorun cycles.
   - intentionally unknown or low-context tasks.
   - score whether the system plans, researches, tests, discovers gaps, and records new formulas in the lake.
4. Official adapters:
   - Terminal-Bench adapter through its task format and verification scripts.
   - SWE-bench or SWE-bench-inspired adapter only after sandboxing is ready.

Acceptance evidence:

- `.state/benchmark_report.json` includes pass count, force score, lake score, cost, tokens, and logs for every task.
- 24/7 reliability report shows recovery rate, repeated-failure rate, cache improvement, and new-knowledge capture.
- No official benchmark score is claimed until run under its official harness.

## Priority 5: Data Lake as Enterprise Knowledge Brain

Target: the lake is not passive storage. It should behave like a materials warehouse plus self-improving research memory.

Implementation path:

1. Planning phase must prove materials are current:
   - local tool versions.
   - provider route health.
   - relevant docs/research freshness.
   - dependency and runtime suitability.
2. Every node must:
   - read latest wiki/cache before build.
   - cite cache item IDs used.
   - write verified recipes and failed attempts.
3. Add knowledge gap loop:
   - if task has low confidence or no matching lake items, trigger research/hydration before more build cycles.
   - compare divergent plans.
   - collapse into a reusable recipe or explicit unknown.
4. Add pruning/quality gates:
   - expire low-confidence or stale cache items.
   - promote repeated successful recipes.
   - demote repeated failed routes/patterns.

Acceptance evidence:

- `qsm lake-score` shows high refresh/write/citation coverage.
- planning report lists materials and freshness before Chop.
- benchmark tasks show improved pass rate or lower cost after repeated cache cycles.

## Deferred But Required Later

- SBOM generation.
- license compliance scan.
- SOC2/GDPR/HIPAA-style policy evidence.
- multi-tenant quotas and admin UI.
- official SWE-bench/Terminal-Bench scores.
- dashboard and distributed tracing exporter.

## Current Gate Result

Local product/service readiness: improving and testable.

Enterprise production readiness: not yet. The next hard gates are sandbox execution, OpenCode terminal supervision, and repeated benchmark/reliability evidence.
