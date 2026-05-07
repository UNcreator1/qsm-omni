# QSM Data Lake Enterprise Deployment Plan

Date: 2026-05-05

This is the full lifecycle for making the QSM data lake serious enough to act like a materials warehouse and self-improving knowledge brain.

## Step 1: Intake

Inputs enter through:

- zero-shot agent hypotheses,
- local repository hydration,
- route-health probes,
- node evidence,
- QSM verifier reports,
- human-approved research.

Gate:

- Every item must have source, kind, confidence, verification status, and objective ID when relevant.

## Step 2: Plan Materials Gate

Before Chop, `qsm plan` checks:

- local tools,
- harness readiness,
- wiki presence,
- route-health availability for real runs,
- freshness of critical resources.

Gate:

- Critical blockers stop Chop.
- Optional gaps become warnings.

## Step 3: Active Cache Selection

Nodes receive only ranked verified cache through:

```text
.rooms/pos-N/.qsm_memory/CACHE.md
```

Selection prefers:

- objective constraints,
- fresh route health,
- verified recipes,
- failed attempts,
- dependency notes.

Gate:

- stale/duplicate/low-confidence cache must not pollute active node memory.

## Step 4: Maintenance

Command:

```bash
qsm lake-maintain -root .
qsm lake-maintain -root . -apply=true
```

Behavior:

- reports stale route-health,
- reports low-confidence items,
- reports duplicate lower-rank items,
- moves bad items to `.lake/quarantine/cache`,
- never deletes by default.

Artifacts:

```text
.state/lake_maintenance_report.json
.state/lake_maintenance_report.md
.lake/quarantine/cache/
```

## Step 5: Promotion

Command:

```bash
qsm lake-promote -root .
qsm lake-promote -root . -apply=true
```

Behavior:

- scans repeated verified recipes,
- rejects low-signal operational messages,
- accepts high-signal reusable practices,
- writes curated best practices into `.lake/curated`,
- writes verified `curated_best_practice` artifacts into `.lake/artifacts`,
- recompiles `internal/wiki/wiki.md`.

Artifacts:

```text
.state/lake_promotion_report.json
.state/lake_promotion_report.md
.lake/curated/best_practices.json
.lake/curated/best_practices.md
```

## Step 6: Node Usage

Each node must:

- read wiki/cache before build,
- use `.qsm_harness/skills/lake/SKILL.md`,
- cite `cache_item_ids_used` or `source_notes` in `evidence.json`,
- write either a `verified_recipe` or a `failed_attempt`.

Gate:

- `qsm lake-score` must show refresh/write coverage.
- Enterprise target requires citation coverage >= 50%.

## Step 7: Collapse

Collapse reads:

- branch results,
- QSM tests,
- citations,
- force score,
- lake interaction score.

Gate:

- winner must pass deterministic product verification.
- readiness claims must match force evidence.

## Step 8: Continuous Operation

Recommended 24/7 cycle:

```bash
qsm route-health -root . -harness langchain -models auto
qsm lake-maintain -root . -apply=true
qsm run -root . -harness langchain -positions auto -parallel auto -shared-cache=true -route-health=true
qsm lake-score -root .
qsm lake-promote -root . -apply=true
qsm force-score -root .
```

## Current Deployment Status

- Maintenance gate: deployed.
- Promotion gate: deployed.
- Hard sandbox gate: not deployed.
- SBOM/license/compliance: deferred.
- Official SWE-bench/Terminal-Bench adapters: not deployed.

The lake is now operationally self-cleaning and promotion-aware, but it still needs better node citation discipline and stronger real-harness evidence before enterprise readiness.
