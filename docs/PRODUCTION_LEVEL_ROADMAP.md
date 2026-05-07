# Quantum Swarm V3 Production-Level Roadmap

Date: 2026-05-03

This roadmap turns the current Quantum Swarm V3 prototype into a production-grade local agentic build system. It is based on the current repo state, the latest smoke test, the Force Requirements checklist, and Grok Council critique.

## Current Honest Status

QSM V3 is a capable local orchestration prototype.

It currently has:

- `qsm` CLI for `run`, `status`, `doctor`, `route-health`, `capacity`, `wiki`, and related flows.
- Capacity planning for local hardware.
- Route-health and build-health tracking for 9Router/model routes.
- Simulated, LangChain, and OpenCode harness paths.
- Isolated room directories under `.rooms/pos-N`.
- Shared `.lake`, cache, and wiki memory.
- QSM methodology contracts inspired by Superpowers.
- Force Requirements artifacts in every room:
  - `FORCE_REQUIREMENTS_CHECKLIST.md`
  - `QSM_FORCE_CHECKLIST.json`
- QSM-owned auto-test reports:
  - `.qsm_test/qsm_test_report.json`
- Static-web product verifier.
- JavaScript syntax checks through `node --check`.
- Grounded citation mapper from lake/cache/wiki artifacts.
- Room status and run reports.
- Deterministic collapse into `deliveries/`.

Latest verified smoke:

- Command path: `go test ./...`, Python harness compile, `go build`, simulated QSM snake-game run.
- Result: one node succeeded, collapse approved, product delivered.
- Auto-test report: static-web product, `1/1` command passed.
- Force checklist: present with all 10 categories, defaulting to `GAP`.

This is not yet top-tier production.

Main gaps:

- Real multi-node OpenCode/LangChain stress testing is not complete.
- Force checklist is created and schema-checked, but not audited/scored.
- Browser Playwright smoke is skipped unless browser binaries exist locally.
- Security/compliance scans are not wired.
- Large repo and benchmark testing are not wired.
- 24/7 daemon/self-healing operation is not complete.
- Room isolation is path-based, not Docker/VM sandboxed.

## Product Principle

QSM must not pretend.

If enterprise evidence is unavailable, the system must write `GAP`, `LOCAL-GAP`, `PARTIAL`, or `N/A`, not `PASS`.

Collapse may deliver local products, but QSM may only claim production/top-tier readiness when the Force Requirements checklist has evidence-backed PASS states and no hard-gate failures.

## Phase A: Strategic Reservoir As Procurement-Grade Planning

This is the most important design correction.

Phase A must work like planning and purchasing materials before construction. The swarm should not begin parallel construction until the project lake contains the requirements, constraints, materials, dependencies, risks, tests, cost expectations, and acceptance gates.

This phase must also verify material quality. It is not enough to list a framework, API, model, package, or tool. QSM must check whether that material is current, available, maintained, compatible, secure enough, and the best practical choice for the objective before the build begins.

### Goal

Prevent mid-build plan churn by forcing the objective through a serious planning and materials-gathering gate before `Chop`.

### Required Lake Artifacts

Before positions are created, `.lake` must contain:

- `objective_contract`
  - user request
  - scope
  - non-goals
  - expected deliverable type
  - acceptance criteria
- `requirements_inventory`
  - functional requirements
  - non-functional requirements
  - Force Requirements category relevance
- `materials_inventory`
  - frameworks/libraries/tools needed
  - local runtime dependencies
  - browser/test/security tools needed
  - API/model routes needed
- `materials_quality_audit`
  - current stable version or pinned version
  - last verified date/time
  - source used for verification
  - maintenance signal, such as latest release, active docs, or working install
  - license or usage constraint where relevant
  - compatibility with local OS/runtime/harness
  - security/deprecation warnings
  - known alternatives considered
  - selected material and reason
- `resource_freshness_report`
  - official docs or primary source checked when network is available
  - local version checked when tool is installed
  - package/model/API availability checked before use
  - stale-resource threshold and decision
  - `fresh`, `stale`, `unverified`, or `blocked` status for every material
- `repo_archaeology`
  - existing files relevant to the build
  - architecture boundaries
  - test commands already available
- `risk_register`
  - unknowns
  - likely failure modes
  - security/privacy hazards
  - cost/rate-limit hazards
- `test_strategy`
  - QSM-owned tests
  - node-suggested tests
  - browser tests
  - security checks
  - benchmark/eval checks
- `cost_plan`
  - expected models/routes
  - token/call budget
  - max retry/parallelism policy
- `force_requirements_baseline`
  - 10 categories initialized with current expected evidence
  - `GAP` where evidence is unavailable
- `chop_readiness_verdict`
  - `approved: true/false`
  - blockers
  - allowed number of positions
  - required harness mode

### Hard Gate

No `Chop` unless `chop_readiness_verdict.approved == true`.

No real harness run unless route-health has at least one healthy route or an explicitly approved fallback.

No build may start with an unverified critical material. A critical material is any framework, package manager, browser tool, model route, runtime, security scanner, or API needed to produce or verify the deliverable.

Allowed exceptions:

- non-critical optional tools may be marked `GAP`;
- unavailable browser/security tools may allow local-dev builds but must block production profiles;
- enterprise-only materials may be marked `LOCAL-GAP` when the local profile does not require them.

### Implementation Tasks

1. Add `internal/planning` package.
2. Define structured `PlanLake` artifact types.
3. Add `qsm plan -request ...` command.
4. Make `qsm run` call planning first unless `-skip-plan` is explicitly passed.
5. Add `.state/plan_report.json`.
6. Add `plan_readiness` to `qsm status`.
7. Add material/resource freshness checks:
   - local command version probes;
   - package/runtime availability probes;
   - official source URL and checked timestamp fields;
   - route-health for model/API materials;
   - browser/security tool availability for product profiles.
8. Add tests proving `Chop` refuses when required planning artifacts or critical material checks are missing.

### Acceptance Criteria

- `qsm plan` produces all required lake artifacts.
- `qsm run` refuses to start positions if the planning gate fails.
- Planning output includes materials inventory and test strategy.
- Planning output includes material quality and freshness reports.
- Critical materials are either `fresh` or explicitly blocked before `Chop`.
- The plan records alternatives considered for major frameworks/tools.
- A simple static-web request can pass planning in under 30 seconds.
- A request requiring unavailable tools returns a clear blocker before any build room starts.

## Phase 1: Real Multi-Node Harness Reliability

### Goal

Prove real LangChain/OpenCode nodes can run concurrently with route-health, shared cache, retries, and deterministic accounting.

### Current Gap

Simulated path is stable. Real single-node route smoke has worked, but multi-node real harness reliability is not proven.

### Implementation Tasks

1. Add `qsm test-harness`.
2. Add stress profiles:
   - `single-real`
   - `three-node`
   - `seven-node-local`
   - `route-failover`
3. Record:
   - node started/succeeded/failed
   - attempts
   - model route
   - duration
   - token/cost if available
   - cache refresh count
   - test report summary
4. Add deterministic ordering for shared-cache write/read windows.
5. Add real harness stop policy:
   - max tool calls
   - max model turns
   - max wall-clock
   - stop on valid product + evidence + QSM test report

### Hard Gate

Real harness production mode remains disabled unless:

- 5 consecutive `qsm test-harness -profile three-node` runs pass.
- Success rate is at least 90%.
- All nodes are accounted for.
- No stale room status is reused.
- Failed routes are excluded from the next run.

### Acceptance Criteria

- `qsm test-harness -harness langchain -positions 3 -parallel 2` reaches 90% success over 5 runs.
- Same target for OpenCode once the OpenCode stack is available.
- Failures produce cache lessons and build-health updates.

## Phase 2: Force Auditor And Collapse Scoring

### Goal

Turn the Force Requirements checklist from a presence/schema artifact into an evidence-backed audit gate.

### Current Gap

Every room has `QSM_FORCE_CHECKLIST.json`, but categories default to `GAP`.

### Implementation Tasks

1. Add `internal/forceaudit`.
2. Parse `QSM_FORCE_CHECKLIST.json`.
3. Read:
   - `.qsm_test/qsm_test_report.json`
   - `evidence.json`
   - room status
   - route/build health
   - security scan reports
   - benchmark reports
4. Auto-fill local evidence:
   - Maintainability/DX
   - Observability/countability
   - Usability/portability
   - Self-healing evidence when retries/backoff are exercised
5. Keep enterprise-scale items as `LOCAL-GAP` unless benchmark/stress data proves them.
6. Add force audit summary to `BranchResult`.
7. Update `collapse.Audit` and `ConsensusEngine`.

### Hard Gates

Branch fails collapse if:

- Checklist JSON missing or invalid.
- Any local-critical category is `FAIL`.
- Any QSM-owned deterministic test fails.
- Security scan has high/critical findings.
- Checklist claims `PASS` without evidence.

Soft gates:

- Enterprise-only items may be `LOCAL-GAP` without blocking local product delivery.
- `LOCAL-GAP` blocks top-tier/production-readiness claims.

### Acceptance Criteria

- A room with missing checklist fails audit.
- A room with `PASS` but no evidence is downgraded or failed.
- A room with passing auto-tests gets local test evidence linked.
- `qsm status` shows force audit summary.

## Phase 3: Browser And UI Verification

### Goal

Make web products behavior-checked, not just syntax-checked.

### Current Gap

Playwright smoke is dependency-gated and skipped when browser binaries are not installed.

### Implementation Tasks

1. Add `qsm doctor --browser`.
2. Add `qsm install-browser-tools` or documented local setup command.
3. Add Playwright browser discovery:
   - package available
   - Chromium executable available
   - sandbox compatibility
4. Add static-web smoke:
   - load `index.html`
   - fail on console/page errors
   - check visible content
   - click primary controls
   - canvas non-blank pixel sample
   - screenshot artifact
5. Add mobile viewport smoke.
6. Write `.qsm_test/browser_report.json`.

### Hard Gate

For web/JS products:

- If browser tools are installed, browser smoke must pass.
- If browser tools are not installed, product may pass local syntax gate but force checklist must mark browser E2E as `GAP`.
- Production profile requires browser smoke.

### Acceptance Criteria

- Snake game passes browser smoke with screenshot.
- Broken JS fails browser smoke.
- Blank canvas fails if game claims canvas gameplay.
- `qsm status` shows browser smoke state.

## Phase 4: Security And Compliance Scanning

### Goal

Add basic product security checks before delivery.

### Implementation Tasks

1. Add `internal/security`.
2. Add secret scanning:
   - regex baseline
   - optional `gitleaks` integration
3. Add language/package scanners:
   - Go: `govulncheck` and optional `gosec`
   - Node: `npm audit` when `package-lock.json` exists
   - Python: `pip-audit` when dependency files exist
   - Container/SBOM: optional `trivy` and `syft`
   - Cross-language: optional `semgrep`
4. Add `.qsm_security/security_report.json`.
5. Add security findings to Force Audit.

### Hard Gate

No production delivery if:

- hard-coded secrets are detected;
- high/critical vulnerabilities are found;
- generated code attempts path escape or unsafe collapse action.

### Acceptance Criteria

- A fixture containing fake API secret is blocked.
- A clean static product passes baseline secret scan.
- Security report is included in evidence and status.

## Phase 5: Hard Sandbox Per Room

### Goal

Move from path-based isolation to real execution isolation.

### Current Gap

LocalShellBackend and room prompts are not a true security sandbox.

### Implementation Options

Preferred:

- Docker/Podman per room.

Fallback:

- macOS sandbox-exec profile if available.
- Restricted subprocess wrapper with allowlisted commands.

### Implementation Tasks

1. Add `internal/sandbox`.
2. Add `qsm doctor --sandbox`.
3. Add sandbox profile:
   - room mounted read/write
   - repo root read-only or unavailable
   - no host home access
   - network disabled by default
   - explicit network allow flag for research phase only
4. Make real harnesses run inside sandbox.
5. Keep `.lake` access controlled through copied scoped materials, not broad filesystem access.

### Hard Gate

Production profile refuses real harness execution if sandbox is unavailable.

### Acceptance Criteria

- Attempted write outside room fails.
- Attempted read of host secret path fails.
- Product build still succeeds inside sandbox.

## Phase 6: Observability, Replay, And Cost Accounting

### Goal

Make every action countable and replayable.

### Implementation Tasks

1. Add `internal/trace`.
2. Define event schema:
   - objective
   - route probe
   - room phase
   - tool call
   - file edit
   - command run
   - test result
   - force audit result
   - collapse decision
3. Write `.state/trace.jsonl`.
4. Add `qsm trace`.
5. Add `qsm dashboard` or static HTML report.
6. Add cost accounting:
   - tokens in/out when route returns usage
   - model route
   - cost estimate
   - cost per successful branch

### Hard Gate

Production profile requires:

- run report;
- room status;
- test report;
- force audit;
- trace file;
- collapse verdict.

### Acceptance Criteria

- `qsm trace obj-id` reconstructs the run.
- `qsm status --json` includes countable metrics.
- Missing trace blocks production profile.

## Phase 7: Long-Running 24/7 Self-Healing

### Goal

Turn QSM from a command runner into a durable service.

### Implementation Tasks

1. Add `qsm serve`.
2. Add durable queue:
   - file-backed first
   - SQLite later
3. Add run scheduler:
   - route-health before run
   - capacity-aware concurrency
   - backoff/circuit breakers
   - resume interrupted rooms
4. Add health endpoint:
   - `/healthz`
   - `/metrics`
   - `/runs`
5. Add launchd profile for local Mac 24/7 mode.
6. Add injected-failure tests:
   - kill node process
   - route 429
   - missing browser
   - corrupt evidence
   - sandbox denial

### Hard Gate

24/7 mode is not production until:

- 48-hour stress test passes;
- 100+ runs complete;
- injected failures are recovered or cleanly quarantined;
- no manual restart required.

### Acceptance Criteria

- `qsm serve` resumes after process restart.
- Failed run state is not lost.
- Circuit breaker blocks bad routes.
- Self-healing rate is reported.

## Phase 8: Benchmarks And Large Repo Evaluation

### Goal

Measure QSM honestly against real work, not toy demos.

### Implementation Tasks

1. Add `qsm benchmark`.
2. Create local benchmark suite:
   - static web product
   - Go bugfix
   - Python bugfix
   - Node bugfix
   - documentation transformation
   - multi-file refactor
   - large repo archaeology task
3. Add adapters for:
   - SWE-bench style tasks where feasible
   - Terminal-Bench style shell tasks where feasible
   - internal repo test suite
4. Metrics:
   - success rate
   - time to completion
   - cost per task
   - retries
   - test failures
   - code churn/revert within 14 days if git history exists

### Hard Gate

No top-tier claim unless:

- benchmark suite is public or reproducible;
- success rate target is met;
- force checklist categories are evidence-backed;
- failures are included in report, not hidden.

Initial local target:

- 10 benchmark runs;
- 5 positions where capacity allows;
- at least 70% success;
- zero unaccounted nodes.

## Phase 9: Product Delivery And Team Workflow

### Goal

Make QSM usable as a product, not only as a local experiment.

### Implementation Tasks

1. Add project templates.
2. Add GitHub/GitLab connector layer.
3. Add PR creation with approval gate.
4. Add human review checkpoints.
5. Add team config:
   - roles
   - quotas
   - allowed routes
   - secrets policy
6. Add documentation:
   - quickstart
   - real harness setup
   - sandbox setup
   - browser setup
   - security setup
   - 24/7 setup

### Hard Gate

No autonomous merge without:

- passing tests;
- passing security;
- passing force audit local-critical categories;
- human approval unless explicitly configured.

## Production Profiles

### `local-dev`

Allows:

- simulated harness;
- missing browser as warning;
- force checklist `GAP`;
- path-based isolation.

Purpose:

- development and demos.

### `local-production`

Requires:

- real harness route-health;
- QSM auto-tests;
- valid force audit;
- browser smoke for web products;
- security scan baseline;
- trace file.

Allows:

- enterprise categories marked `LOCAL-GAP`.

### `enterprise-production`

Requires:

- hard sandbox;
- 24/7 durable service;
- benchmark evidence;
- security/compliance evidence;
- observability dashboard;
- disaster recovery story;
- cost tracking;
- 48-hour stress test.

## Suggested Priority Order

1. Phase A procurement-grade planning gate.
2. Real multi-node harness stress.
3. Force Auditor scoring.
4. Browser verification.
5. Security scans.
6. Hard sandbox.
7. Observability and trace replay.
8. 24/7 self-healing service.
9. Benchmarks and large repo evaluation.
10. Product/team workflow.

## Immediate Next Build Slice

Build Phase A planning gate first.

Why:

- It reduces plan churn.
- It makes the lake serious.
- It gives every later phase better inputs.
- It lets QSM refuse bad or under-specified builds before wasting model/API cycles.

Minimum implementation:

- `internal/planning`
- `qsm plan`
- `.state/plan_report.json`
- required lake artifacts
- `materials_quality_audit`
- `resource_freshness_report`
- `chop_readiness_verdict`
- `qsm run` refuses to `Chop` if planning gate fails in production profile

First acceptance test:

```bash
./qsm plan -root . -request "build a snake game"
./qsm run -root . -request "build a snake game" -positions 1 -parallel 1 -harness simulated
```

Expected:

- plan report exists;
- lake has requirements/materials/material-quality/freshness/risk/test/cost artifacts;
- Chop starts only after planning approval;
- product still delivers;
- QSM status shows plan readiness.
