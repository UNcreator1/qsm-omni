# QSM Production Deployment Status - 2026-05-06

## Result

QSM reached local production QA readiness on this machine with Docker-backed command execution.

- Production QA: `PASS`
- Gates: `22/22`
- Force score: `8.55/10`
- Top-tier: `false`
- Hard sandbox: Docker probe passed
- CI release evidence: local-dev waiver, not hosted CI

## Evidence Files

- `.state/qa_report.json`
- `.state/production_gap_report.json`
- `.state/sandbox_report.json`
- `.state/trace_report.json`
- `.state/stress_report.json`
- `.state/recovery_report.json`
- `.state/contributor_smoke_report.json`
- `.state/coverage_report.json`
- `.state/flake_report.json`
- `.state/mutation_report.json`
- `.state/cost_budget_report.json`
- `.state/ci_release_report.json`

## Verified Run

- Objective: `obj-1778055067`
- Harness: `simulated`
- Sandbox: `docker`
- Nodes: `7/7` succeeded
- Parallelism: `4`
- Lake cache citation coverage: `100%`
- Decision citation coverage: `100%`
- Delivered product: `deliveries/obj-1778055067`

## Docker Sandbox

The production run used:

```bash
QSM_SANDBOX_DOCKER_IMAGE=node:22-alpine
```

The probe verified:

- inside-room read succeeds
- outside-room read is blocked
- network is blocked
- timeout is killed
- Docker daemon is reachable through Colima

The full Playwright image remains the richer target for browser-heavy production validation, but the `node:22-alpine` image is enough for the current static-web manifest smoke, coverage contract, flake detection, mutation probe, recovery probe, and stress probe.

## Caveats

This is local production readiness, not top-tier enterprise readiness.

- CI evidence used `QSM_ALLOW_LOCAL_RELEASE_EVIDENCE=1`.
- Hosted GitHub Actions has not run this exact production QA yet.
- Force score is above the production threshold but below top-tier threshold.
- MicroVM isolation, SBOM/license/compliance, multi-tenant admin, and official SWE-bench/Terminal-Bench adapters remain future work.
