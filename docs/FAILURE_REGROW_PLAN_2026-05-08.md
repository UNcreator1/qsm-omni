# QSM Failure Diagnosis And Branch Regrowth Plan - 2026-05-08

## Summary

QSM should be able to cut a failed branch, learn why it failed, and regrow from the best safe checkpoint. This fits the Quantum Swarm model, but it must be evidence-driven rather than magical retry logic.

Grok was consulted through the existing Council bridge on port `8338` without restarting or modifying it. Grok agreed with the direction and warned against the main traps: complexity explosion, state corruption, cost/timeout loops, hallucination amplification, and non-reproducible regrowth.

Current QSM already has useful raw materials:

- per-node `BranchResult` in `.state/run_report.json`;
- command traces under `.rooms/*/.qsm_test/trace.jsonl`;
- test reports, stdout/stderr logs, cost reports, force score, lake score, and production gap reports;
- shared cache/lake memory and `failed_attempt` cache writes;
- Docker room isolation for QSM-owned verification;
- real-harness smoke as a separate live-agent evidence lane.

The missing subsystem is first-class **failure diagnosis -> failure memory -> checkpointed regrowth**.

## Goal

When a node fails, QSM should answer:

1. What failed?
2. Why did it fail?
3. Is the failure reproducible?
4. Is this a knowledge/materials gap or a normal engineering defect?
5. Which previous checkpoint can safely seed a regrown branch?
6. What lesson must the regrown branch cite before rebuilding?

The system must not claim arbitrary self-healing mastery. Regrowth is bounded, observable, cost-limited, and sandboxed.

## Failure Evidence Contract

Every failed position should write a mandatory sidecar:

```text
.rooms/pos-N/failure-evidence/
├── failure.json
├── evidence_manifest.json
├── file_manifest_before.json
├── file_manifest_after.json
├── product_diff.patch
├── logs/
│   ├── harness.stdout.log
│   ├── harness.stderr.log
│   └── qsm_test/*.log
└── traces/
    └── trace.jsonl
```

Minimum fields in `failure.json`:

```json
{
  "schema": "qsm.node_failure.v1",
  "failure_id": "uuid-or-content-hash",
  "objective_id": "obj-id",
  "position_id": "pos-03",
  "agent_id": "alpha",
  "agent_model": "provider/model",
  "harness_mode": "simulated|opencode|langchain",
  "sandbox_backend": "room|docker",
  "attempt": 1,
  "phase": "nodekit|harness_execute|verify|collapse",
  "classification": "compile_test_runtime",
  "confidence": 0.86,
  "root_cause_hypothesis": "pytest failed because generated parser accepts malformed input",
  "reproducible": true,
  "retryable": false,
  "research_required": false,
  "evidence_refs": ["sha256:...", "trace:command_end:2"],
  "memory_citations": ["cache_item:...", "wiki_item:..."],
  "created_at": "2026-05-08T00:00:00Z"
}
```

Evidence must be written atomically: temp directory first, then rename. The manifest hashes all evidence files so later regrowth can prove it used the same failure facts.

## Failure Classifier

Add a deterministic classifier first. LLM classification stays optional and only runs for ambiguous failures.

Failure enum:

- `requirement_misunderstanding`
- `compile_test_runtime`
- `sandbox_tooling`
- `model_route`
- `missing_knowledge_materials`
- `hallucinated_files_tests`
- `flaky_external_dependency`
- `cost_timeout`
- `other`

Rule examples:

- Exit code from `go test`, `pytest`, `npm test`, syntax traceback: `compile_test_runtime`.
- Docker daemon, mount, permission, network denial: `sandbox_tooling`.
- 429, 5xx, empty model content, route-health failure: `model_route`.
- Missing expected artifact or fake/hard-coded tests: `hallucinated_files_tests`.
- Repeated pass/fail variance across `qsm flake`: `flaky_external_dependency`.
- Budget exceeded or watchdog/no-progress timeout: `cost_timeout`.
- Missing API/library/domain knowledge, stale package docs, unknown requirement terms: `missing_knowledge_materials`.

The first implementation should live in a small package such as `internal/failure` and expose:

```go
Analyze(result swarm.BranchResult, room string) Report
```

The report should be safe to generate after a run:

```bash
qsm failure-analyze -root .
```

## Phase 1 Deployment Status

Phase 1 is now implemented as the first concrete failure-learning primitive:

- `internal/failure` analyzes `.state/run_report.json`.
- `qsm failure-analyze -root .` writes `.state/failure_report.json` and `.state/failure_report.md`.
- Each failed room receives `.rooms/pos-N/failure-evidence/failure.json`.
- Each failed node is classified into the first deterministic failure enum.
- Failure lessons are written into `.lake/cache` with kind `failure_lesson`.
- Full failure records are written into `.lake/failures/<failure-id>.json` and `.lake/failures/index.jsonl`.
- `qsm qa -refresh=true` now refreshes failure-learning evidence.
- Production QA includes a `failure-learning` evidence gate.

This does not regrow branches yet. It gives regrowth the evidence substrate it needs.

## Data Lake Storage

Failures become lake memory, not disposable logs.

Write:

```text
.lake/failures/failure-v1-<failure-id>.json
.lake/failures/index.jsonl
.lake/cache/<failure-derived-lesson>.json
```

Failure lake schema:

```json
{
  "schema": "qsm.failure_memory.v1",
  "failure_id": "uuid-or-content-hash",
  "objective_id": "obj-id",
  "position_id": "pos-03",
  "classification": "missing_knowledge_materials",
  "confidence": 0.92,
  "root_cause_hypothesis": "The branch used an outdated Vite command.",
  "evidence_refs": ["sha256:...", "trace:line:42"],
  "citations": ["cache_item:...", "wiki_item:...", "lake_artifact:..."],
  "quality_gates": {
    "evidence_completeness": 0.92,
    "reproducible": true,
    "dedup_score": 0.08,
    "passed": true
  },
  "lesson_learned": "Use npm create vite only during scaffolding; generated products should include package.json scripts and run npm test/build directly.",
  "regrow_seed": "pos-02-checkpoint-test",
  "created_at": "2026-05-08T00:00:00Z"
}
```

Quality gates for promotion into cache/wiki:

- evidence completeness >= 0.8;
- classification confidence >= 0.7, unless deterministic class;
- reproducible or independently validated by flake report;
- not duplicate of an existing failure lesson;
- includes at least one real evidence reference;
- includes cache/wiki/lake citation if the failure affected a plan/build/test decision.

## Checkpoints

Regrowth needs safe checkpoints.

Each position should checkpoint at stable phases:

- `plan`
- `scaffold`
- `build`
- `test`
- `verify`

Checkpoint layout:

```text
.rooms/pos-N/checkpoints/
├── plan.tar.gz
├── scaffold.tar.gz
├── build.tar.gz
├── test.tar.gz
└── manifest.json
```

Checkpoint primitive status:

- `internal/checkpoint` now writes deterministic `phase.tar.gz` room snapshots.
- Node execution writes `plan`, `build`, and `test` checkpoints.
- Checkpoints exclude recursive checkpoint archives, `.git`, `node_modules`, Python caches, and temp archives.
- `checkpoints/manifest.json` records phase, path, file count, bytes, hash, and timestamp.
- `qsm failure-analyze` points failures at the best existing checkpoint when one is available.

Manifest fields:

- checkpoint ID;
- phase;
- product path;
- hash tree;
- force/test/lake score snapshot;
- command trace offset;
- parent checkpoint ID.

Winner rooms must become read-only after collapse. Regrowth must copy from a checkpoint into a new room, never mutate the old room.

## Regrowth Flow

New command:

```bash
qsm regrow -root . -from-failures .state/failure_report.json -limit 3 -sandbox docker
```

Flow:

1. Read `.state/run_report.json`, `.state/verdict.json`, failure reports, and lake/cache memory.
2. Select failed branches that are worth regrowing:
   - not sandbox infra failure;
   - not pure provider outage unless another route exists;
   - not repeated identical failure above limit;
   - cost budget has room.
3. Pick seed:
   - best checkpoint from same branch if usable;
   - otherwise best successful sibling branch with similar product kind and citations.
4. Create new room:
   - `.rooms/regrow-pos-N-r1`;
   - copy checkpoint product state;
   - inject `failure_memory.json`, relevant cache/wiki citations, and repair instructions.
5. Run a constrained build/test loop.
6. Write new `BranchResult` with `regrown_from`, `failure_ids_used`, and `checkpoint_id`.
7. Collapse only against regrown and original successful candidates.

Termination guards:

- max regrowth rounds per objective;
- max cost/duration;
- max repeated classification per objective;
- no regrow from non-reproducible failures unless flake detector marks it safe to retry;
- no research unless classification permits it.

## Research Trigger Rules

Do not hydrate blindly.

Research is allowed only when:

- classification is `missing_knowledge_materials`; or
- classification confidence < 0.6 and the branch shows `requirement_misunderstanding` or `hallucinated_files_tests`.

Research is blocked for:

- compile/test/runtime failures;
- Docker/sandbox/tool failures;
- model/route failures;
- pure timeout/cost failures;
- flaky external dependencies.

Research order:

1. Search `.lake/cache`, `.lake/artifacts`, and `internal/wiki`.
2. Search local repo/docs.
3. Only if still missing and `QSM_ALLOW_RESEARCH=1`, run targeted web/provider research.
4. Store results as cited lake/cache memory with freshness and source metadata.

This matches the “purchase materials carefully before construction” idea: research fills a missing material list, not every random crack in a build.

## Implementation Plan

### Phase 1: Failure Evidence And Classifier

Add:

- `internal/failure` package;
- `qsm failure-analyze -root .`;
- `.state/failure_report.json/.md`;
- `.rooms/*/failure-evidence/`;
- `.lake/failures/`.

Acceptance:

- failing sample nodes produce deterministic failure reports;
- report explains class, phase, retryability, reproducibility, evidence paths;
- production-gap shows failure-regrowth readiness as missing until reports exist.

### Phase 2: Failure Memory Into Lake

Add:

- failure memory writer;
- deduplication against prior failure items;
- promotion of high-quality lessons into `.lake/cache`;
- lake-score accounting for failure-derived citations.

Acceptance:

- `qsm lake-score` distinguishes normal cache citations from failure memory citations;
- self-improve can cite failure lessons in later cycles;
- no lesson is promoted without evidence refs.

### Phase 3: Checkpoints

Add:

- checkpoint writer in node execution lifecycle;
- checkpoint manifest and hash validation;
- read-only marking for winner rooms after collapse.

Acceptance:

- checkpoints are created per phase;
- corrupt checkpoint is rejected;
- winner rooms are not mutated by regrow.

### Phase 4: Regrow Command

Add:

- `qsm regrow`;
- seed selection;
- bounded regrowth execution;
- collapse over original successes plus regrown candidates.

Acceptance:

- one injected failure can be diagnosed, regrown, retested, and collapsed;
- regrow report includes source failure IDs and checkpoint IDs;
- repeated failure stops cleanly instead of looping.

### Phase 5: Targeted Hydration

Add:

- missing-materials research gate;
- `QSM_ALLOW_RESEARCH`;
- local-first lake/wiki/repo search;
- targeted web/provider research only after local search fails.

Acceptance:

- compile/test failures do not trigger research;
- missing knowledge failure does trigger a cited material item;
- regrown branch must cite that material.

### Phase 6: QA Gates

Add gates:

- `failure-evidence`;
- `failure-memory`;
- `checkpoint-integrity`;
- `regrow-recovery`;
- `research-discipline`.

Acceptance:

- local-production may warn;
- production requires evidence reports;
- top-tier requires regrow recovery and live-harness smoke/stress.

## Unsafe Shortcuts

Rejected:

- mutating a failed room in place;
- mutating a winner room;
- unlimited retries/regrowth;
- researching every failure;
- using uncited LLM guesses as failure truth;
- promoting failure lessons without reproducibility/evidence;
- letting regrowth bypass Docker sandbox;
- allowing node-authored tests to prove their own repair without QSM-owned verification.

## Immediate Next Step

Implement Phase 1 only:

```bash
qsm failure-analyze -root .
```

This is the right first slice because QSM cannot regrow safely until it can name failures with evidence.
