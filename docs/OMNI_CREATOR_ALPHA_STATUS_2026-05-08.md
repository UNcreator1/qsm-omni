# QSM Omni-Creator Alpha Status - 2026-05-08

## Current Level

QSM has reached the defined **Omni-Creator Alpha** gate for this repo.

This means the framework can create, test, sandbox, trace, score, self-improve, and deliver the six contract-covered project classes in hosted CI:

- static web
- CLI tool
- Go service
- Python package
- Node fullstack
- data transform

The claim is still bounded: this is contract-covered Omni Alpha, not proof of arbitrary software mastery or enterprise multi-tenant SaaS readiness.

## Hosted CI Evidence

Latest blocking proof:

- Workflow: `QSM Production QA`
- Run: [#17](https://github.com/UNcreator1/qsm-omni/actions/runs/25544380416)
- Commit: `e23f878`
- Status: `Success`
- Duration: `8m 5s`
- Evidence artifact: `qsm-production-evidence`
- Artifact digest: `sha256:a02da50e170d026d6ba0df2b014d59615a70d0d6b9f93d1b1254eb7f4da95502`

Run #17 made `omni-alpha` blocking rather than advisory. The workflow now fails if:

- `production` QA fails
- `omni-alpha` QA fails
- Docker hard sandbox probe fails
- `omni-contract` is not 6/6
- self-improvement evidence fails
- trace, coverage, mutation, flake, cost-budget, compliance, recovery, stress, ops, or CI-release reports are missing/failing
- force score is below the `omni-alpha` threshold

## What Changed Since 2026-05-07

- Hosted CI evidence now exists; the old `ci-release` blocker is closed.
- `omni-alpha` is no longer advisory in the workflow.
- Python Docker QA no longer depends on host Python paths.
- Python syntax checks avoid `compileall` bytecode writes in non-root Docker rooms.
- Pytest runs with cache writes disabled for sandbox compatibility.
- Benchmark failures now hard-fail with GitHub annotations.
- Node fullstack products are classified as Node/fullstack when `package.json` and `index.html` coexist.
- Mutation testing skips QA, smoke, coverage, test, and spec files before mutating implementation code.
- The workflow writes a production-gap report after the blocking Omni Alpha QA step.
- `qsm harness-readiness` and `qsm real-harness-smoke` now provide the first separated live-agent evidence lane without weakening the simulated contract gate.
- `qsm failure-analyze` now turns failed nodes into typed failure evidence, lake failure records, and reusable `failure_lesson` cache items.
- Node execution now emits `plan`, `build`, and `test` room checkpoints so future regrowth can restart from a safe seed instead of blindly retrying.

## Honest Claim Boundary

Safe claim:

> QSM is an Omni-Creator Alpha framework for six contract-covered project classes, with Docker-backed hosted CI evidence and blocking production/omni QA gates.

Unsafe claim:

> QSM can build any arbitrary project/software at enterprise production level without domain-specific contracts, real harness stress, or external benchmark evidence.

## Remaining Hardening Beyond Alpha

1. Run `.github/workflows/qsm-real-harness-smoke.yml` with real 9Router and harness secrets, starting with `execute=false` readiness/route-health evidence, then `execute=true` one-node live delivery evidence. Use the workflow `runner=self-hosted` input when the real harness paths are local.
2. Add real-harness OpenCode/LangChain stress as a blocking suite, not only simulated harness contracts.
3. Add official SWE-bench/Terminal-Bench adapters or clearly named local equivalents with comparable scoring fields.
4. Add route-health/cost accounting for live provider calls in every real harness path.
5. Add CodeQL/Semgrep-style static analysis and stronger SBOM/license policy.
6. Evaluate microVM isolation after Docker maturity.
7. Add PR automation and approval gates for autonomous merge workflows.
8. Run repeated 24/7 reliability campaigns and publish trend reports.
9. Promote failure learning into full checkpointed branch regrowth: cut failed branches, seed from the last safe checkpoint, inject cited lake lessons, and rebuild in a fresh room.
