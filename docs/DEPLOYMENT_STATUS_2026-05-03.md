# Quantum Swarm V3 Deployment Status - 2026-05-03

## Current Productized Capabilities

Quantum Swarm V3 is now a local product framework for deterministic multi-position builds, with a serious Phase A planning gate before Chop.

Implemented and verified:

- `qsm plan`: creates a procurement-style Data Lake plan before build execution.
- `qsm run`: enforces the planning/materials gate by default before Chop.
- `qsm autorun`: runs repeated supervised build cycles, writes `.state/autorun.json`, and stores per-cycle logs under `.state/autorun/`.
- `qsm autorun-plist`: generates a macOS `launchd` plist for 20-minute 24/7 local autorun without loading it automatically.
- Atomic Data Lake artifact writes: prevents corrupted JSON when parallel writers emit the same deterministic artifact.
- Force Requirements artifacts in every room: `FORCE_REQUIREMENTS_CHECKLIST.md` and `QSM_FORCE_CHECKLIST.json`.
- QSM-owned auto-test verification: framework tests products directly instead of trusting node-written claims.
- Browser smoke verification: Playwright + Chromium installed locally; static web products are served through a temporary local HTTP server and checked in headless Chromium.
- Built-in security scan: committed secrets and high-risk dynamic execution fail the node; medium issues become warnings.
- Generated-product dependency gates: `npm audit --audit-level=high` for Node lockfiles, `go vet`/optional `govulncheck` for Go modules, and optional `pip-audit` for Python requirements.
- Evidence-based Force Requirements scoring: each run writes `.state/force_score.json`, `.state/force_score.md`, and a Data Lake `force_requirements_score` artifact. Local readiness is scored honestly and does not claim top-tier without benchmark evidence.
- Force local collapse gate: delivery is blocked if mandatory local categories such as reliability, observability, or security are `FAIL`/`GAP`.
- Status observability: `qsm status` reports plan readiness, node accounting, command pass counts, cache summary, security counts, room health, and build-health by model/route.
- Shared cache consumption: `qsm autorun` enables verified cache by default, and room status reports cache refresh counts.
- DeepSeek fallback import: direct fallback now prefers `DEEPSEEK_API_KEY` from `/Users/nexus/Downloads/NemoClaw/.env` over other provider keys.

## Verified Smoke Results

Real harness deploy check:

```text
Command: qsm deploy -harness langchain
Result: real harness ready, managed 9Router live at http://127.0.0.1:20128/v1
Grok/Council port 8338: not touched
```

Real route-health check:

```text
Harness: langchain
Healthy routes via 9Router/free pool: 0/3
oc/ling-2.6-flash-free: failed, no longer free
oc/minimax-m2.5-free: failed, empty content
or/inclusionai/ling-2.6-1t:free: failed, missing OR credentials
DeepSeek direct fallback: 1/1 healthy after importing DEEPSEEK_API_KEY from /Users/nexus/Downloads/NemoClaw/.env
Result: real fallback route approved by route-health
```

Real LangChain one-node smoke:

```text
Request: Real DeepSeek smoke: build a snake game
Harness: langchain
Fallback: deepseek-chat
Nodes: requested=1 started=1 succeeded=1 failed=0 accounted=true
QSM verifier: cmds=2/2
Security: critical=0 high=0 medium=0
Delivery: deliveries/obj-1777814001
```

Real LangChain multi-position stress:

```text
Request: Real multi-position stress: build a polished snake game
Harness: langchain
Fallback: deepseek-chat
Positions: 3
Parallel: 2
Nodes: requested=3 started=3 succeeded=3 failed=0 accounted=true
QSM verifier per room: cmds=2/2
Security per room: critical=0 high=0 medium=0
Shared cache: constraint=1 route_health=4 verified_recipe=9
Force score: average=5.8 top_tier=false
Delivery: deliveries/obj-1777814080
```

Real LangChain seven-position stress:

```text
Request: Real seven-position stress: build a polished snake game
Harness: langchain
Fallback: deepseek-chat
Positions: 7
Parallel: 2
Nodes: requested=7 started=7 succeeded=4 failed=3 accounted=true
Winner: pos-02
Collapse: approved=true
Shared cache: constraint=1 failed_attempt=5 route_health=4 verified_recipe=14
Force score: average=5.5 top_tier=false
Delivery: deliveries/obj-1777814298
```

Real LangChain seven-position retest after harness/smoke hardening:

```text
Request: Real seven-position final retest after smoke hardening
Harness: langchain
Fallback: deepseek-chat
Positions: 7
Parallel: 2
Nodes: requested=7 started=7 succeeded=6 failed=1 accounted=true
Winner: pos-01
Collapse: approved=true
Shared cache: constraint=1 failed_attempt=2 route_health=4 verified_recipe=18
Force score: average=5.8 top_tier=false
Delivery: deliveries/obj-1777814997
```

Retest failure taxonomy:

```text
pos-04: LangChain step budget reached before product/evidence passed.
```

Seven-position failure taxonomy:

```text
pos-01: LangChain step budget reached before product/evidence passed.
pos-03: LangChain step budget reached before product/evidence passed.
pos-06: QSM browser smoke failed blank-canvas detection.
```

Follow-up fix already applied:

```text
Browser smoke now samples the whole canvas through a downscaled probe instead of only the top-left 64x64 pixels.
Regression tests cover centered canvas drawing and blank canvas failure.
Optional button click smoke is non-fatal; canvas/content errors remain fatal.
LangChain nonzero exits are salvageable when QSM's independent product verifier passes.
```

Latest simulated product smoke:

```text
Request: Browser smoke fixed: build a snake game
Harness: simulated
Positions: 1
Result: approved=true
Nodes: requested=1 started=1 succeeded=1 failed=0 accounted=true
QSM verifier: cmds=2/2
Security: critical=0 high=0 medium=0
Delivery: deliveries/obj-1777795240
```

Force score output:

```text
.state/force_score.json
.state/force_score.md
```

Latest autorun smoke:

```text
Command: qsm autorun ... -max-cycles 1
Result: exit=0
State: .state/autorun.json
Log: .state/autorun/cycle-0001.log
```

Launchd artifact:

```text
.state/qsm.autorun.plist
```

Verification commands passed:

```text
go test ./...
go build -o qsm ./cmd/qsm
.venv/bin/python -m py_compile harness/langchain_runner.py
npm audit --prefix . --json
```

Local QSM package audit result:

```text
0 vulnerabilities
```

Latest real LangChain long-run production harness smoke:

```text
Command: qsm run ... -positions 7 -parallel 2 -harness langchain -route-health=true -deepseek-fallback=true -shared-cache=true -long-run
Node runtime: max_steps=90 recursion_limit=120 shell_timeout=180s model_retries=4
Route: 9Router checked first; direct DeepSeek fallback engaged because it was the only healthy route.
Planning: approved=true materials=7 artifacts=10 blockers=0 warnings=0
Result: approved=true
Nodes: requested=7 started=7 succeeded=7 failed=0 accounted=true
QSM verifier: cmds=14/14
Security: critical=0 high=0 medium=0
Delivery: deliveries/obj-1777815984
Force score: average=5.85 top_tier=false
```

Latest enterprise Data Lake attribution smoke:

```text
Command: qsm run ... -positions 7 -parallel 2 -harness langchain -route-health=true -deepseek-fallback=true -shared-cache=true -long-run
Result: approved=true
Nodes: requested=7 started=7 succeeded=7 failed=0 accounted=true
Delivery: deliveries/obj-1777819898
Lake interaction: avg_node_score=100.0 refresh_coverage=100% write_coverage=100% cache_attribution_coverage=100% enterprise_ready=true
Cache: constraint=1 route_health=4 verified_recipe=21
Force score: average=5.8 top_tier=false
```

Data Lake enterprise hardening now includes:

```text
Ranked cache memory selection:
- objective-scoped verified cache only
- kind priority for constraints, route health, failures, recipes, dependency notes
- route-health dedupe by model
- duplicate recipe/failure suppression before prompt injection

Lake interaction scoring:
- .state/lake_interaction_score.json
- .state/lake_interaction_score.md
- qsm lake-score -root .
- per-node refresh count, observed cache IDs, written cache IDs, cache attribution, and quality score

Attribution preservation:
- Python runner records cache_item_ids_observed when QSM stops early
- Go evidence normalization preserves cache attribution metadata into run_report.json
- enterprise_ready requires refresh coverage, write coverage, cache attribution coverage, and node score threshold
```

Hardening applied after the first long-run attempt:

```text
Long-run node budget is now a first-class CLI/service profile:
- -long-run
- -node-max-steps
- -deepagents-recursion-limit
- -node-shell-timeout
- -model-max-retries

Runner diagnostics are written to each room as runner_diagnostics.json.
Checklist-only products are rejected as missing deliverable files.
The security scanner no longer flags "No eval()" in docs/comments as dynamic execution.
```

## Phase A Data Lake Gate

The planning gate now writes these artifacts before Chop:

- `objective_contract`
- `requirements_inventory`
- `materials_inventory`
- `materials_quality_audit`
- `resource_freshness_report`
- `risk_register`
- `test_strategy`
- `cost_plan`
- `force_requirements_baseline`
- `chop_readiness_verdict`

The gate blocks real harness execution if:

- a critical local material is missing or unverified,
- real harness mode lacks fresh route-health,
- no model route is healthy within the route-health freshness window.

This matches the intended "purchase materials before construction" concept: positions should not start building until the materials, tools, routes, memory, and verification strategy are known.

## Remaining Production Gaps

Still not enterprise-production complete:

- Real LangChain multi-node execution now has a clean 7/7 long-run smoke, but larger stress, longer duration, and repeated-cycle reliability are still needed.
- Real OpenCode multi-node execution still needs comparable long-run stress.
- Route-health is available. Current free 9Router routes are not healthy in the latest smoke, but direct DeepSeek fallback from `.env` is healthy and has passed one-node, three-position, and seven-position LangChain runs.
- Security scan is useful but still basic; SBOM, license checks, stronger sandbox policy, and secret-redaction logs should be added.
- Long-run tuning fixed the former 40-step budget bottleneck for the latest 7-position smoke; repeated-cycle self-repair metrics are not measured yet.
- Data Lake interaction now passes the current local enterprise gate on a 7-position real run; repeated-cycle measurement is still needed before claiming 24/7 enterprise memory reliability.
- `qsm autorun` is a local service loop, not yet a macOS `launchd` installer or cloud supervisor.
- Room isolation is filesystem/path isolation, not a hard VM/container sandbox.
- SWE-bench/Terminal-Bench style benchmarking is not wired.
- Force Requirements checklist is now evidence-scored locally, but enterprise categories still correctly remain below top-tier until benchmarks, sandboxing, SLA, cost accounting, and ROI evidence exist.
- Human approval gates for risky actions are not a first-class workflow object yet.

## Recommended Next Route

1. Add hard sandbox option:
   - Start with macOS sandbox-exec or container profile when available.
   - Keep current room isolation as local-dev fallback.

2. Add approved launchctl install/uninstall helpers:
   - Current state generates `.state/qsm.autorun.plist`.
   - Loading/unloading it should remain explicit because it creates a recurring background process.

3. Run higher real harness stress:
   - Repeat 7 positions across multiple autorun cycles.
   - Raise the clean-node target from one successful 7/7 smoke to sustained 7/7 across repeated cycles.
   - Compare success/fail/cost by model.
