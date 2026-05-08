# Quantum Swarm V3 Scratch

This is a from-scratch implementation of the Quantum Swarm Team V3 concept. It is not copied from the previous prototype.

## Run

```bash
go run ./cmd/qsm run -request "Build an auditable CLI product" -positions auto
```

The run creates:

- `.lake/artifacts/*.json` for phase evidence
- `.rooms/pos-N/` for isolated divergent positions
- `.state/run_report.json` for node accounting and failure/success counts
- `.state/verdict.json` for the deterministic collapse result
- `deliveries/<objective-id>/` for the winning product
- `internal/wiki/wiki.md` for the compiled common truth layer

## Commands

```bash
go run ./cmd/qsm capacity
go run ./cmd/qsm doctor -harness opencode
go run ./cmd/qsm harness-readiness -root . -harness all
go run ./cmd/qsm deploy -harness opencode
go run ./cmd/qsm stop
go run ./cmd/qsm status
go run ./cmd/qsm synthesize -request "..."
go run ./cmd/qsm hydrate -root .
go run ./cmd/qsm wiki -root .
go run ./cmd/qsm run -request "..." -positions auto -parallel auto -harness simulated
go run ./cmd/qsm trace -root .
go run ./cmd/qsm cost-budget -root .
go run ./cmd/qsm coverage -root . -sandbox room
go run ./cmd/qsm flake -root . -sandbox room -runs 3
go run ./cmd/qsm mutation -root . -sandbox room
go run ./cmd/qsm benchmark -root . -suite terminal-contract -sandbox room
go run ./cmd/qsm benchmark -root . -suite omni-contract -sandbox docker
go run ./cmd/qsm self-improve -root . -suite omni-contract -cycles 3 -sandbox docker
go run ./cmd/qsm stress -root . -sandbox docker -nodes 7 -parallel 4
go run ./cmd/qsm recovery -root . -sandbox docker
go run ./cmd/qsm contributor-smoke -root .
go run ./cmd/qsm ci-bootstrap -root . -provider github
go run ./cmd/qsm ci-release -root .
go run ./cmd/qsm ops-readiness -root .
go run ./cmd/qsm compliance -root .
go run ./cmd/qsm qa -root . -profile alpha
go run ./cmd/qsm production-gap -root .
```

## QA Profiles

`qsm qa` is the release gate for the current run. It writes `.state/qa_report.json` and `.state/qa_report.md`. Production and Omni-Creator Alpha claims are only valid when the same Docker-backed gates pass in hosted CI without `QSM_ALLOW_LOCAL_RELEASE_EVIDENCE`. As of 2026-05-08, GitHub Actions run [#17](https://github.com/UNcreator1/qsm-omni/actions/runs/25544380416) passes the blocking `production` and `omni-alpha` profiles with the `qsm-omni-sandbox:local` image.

Evidence producers write the production gate inputs directly:

- `qsm sandbox -probe` writes `.state/sandbox_report.*` and separates Docker CLI, daemon, image, and probe readiness.
- `qsm trace` aggregates `.rooms/*/.qsm_test/trace.jsonl` into `.state/trace_report.*`.
- `qsm cost-budget` enforces token, cost, duration, and cost-per-success ceilings.
- `qsm coverage`, `qsm flake`, and `qsm mutation` generate deterministic Auto-QA reports from existing room test commands.
- `qsm benchmark -suite terminal-contract` runs local SWE/Terminal-style contract tasks.
- `qsm benchmark -suite omni-contract` runs the six Omni-Creator Alpha product contracts: static web, CLI, Go service, Python package, Node fullstack, and data transform.
- `qsm self-improve -suite omni-contract -cycles 3` reruns the contract suite, promotes measured lessons into lake/cache/wiki memory, and requires no regression. If the suite is already saturated at 6/6 with zero repeated failures, stable cache-cited learning counts as self-improvement evidence; otherwise force-score improvement is required.
- `qsm stress`, `qsm recovery`, and `qsm contributor-smoke` prove concurrent command execution, failure-capture/recovery, and new-contributor build/test readiness.
- `qsm ci-bootstrap -provider github` writes the GitHub Actions production QA workflow.
- `qsm ci-release` records commit/branch/CI/latest-QA evidence.
- `qsm harness-readiness` records live OpenCode/LangChain prerequisite and 9Router readiness evidence without turning simulated contract success into a live-agent claim.
- `qsm ops-readiness` validates CI artifact retention, launchd/autonomy evidence, approval-gate runbook coverage, and truthful production-gap reporting.
- `qsm compliance` validates Docker sandbox policy and writes a local SBOM/license inventory report.
- `qsm production-gap` summarizes remaining production and top-tier gaps.

- `alpha`: requires run report, all nodes accounted, at least one successful node, QSM-owned test evidence, no critical/high security findings, and force score >= 5.0. Missing production items are warnings.
- `beta`: adds approved collapse, benchmark evidence, cost accounting, lake citation coverage, and force score >= 7.0.
- `local-production`: uses the production-shaped gates with a local release waiver. It never marks external production readiness.
- `production`: requires official-shaped benchmark evidence, Docker/microVM hard sandbox readiness, trace/replay, coverage, mutation/negative-control, flake, cost-budget, hosted CI release evidence, and force score >= 8.5. Local waiver is rejected.
- `omni-alpha`: requires production plus `omni-contract` 6/6, self-improvement evidence, omni sandbox runtime probe, and force score >= 9.5.

Example:

```bash
./qsm sandbox -root . -probe -backend docker -image qsm-omni-sandbox:local -profile omni
./qsm qa -root . -profile production -sandbox docker -image qsm-omni-sandbox:local
./qsm qa -root . -profile omni-alpha -sandbox docker -image qsm-omni-sandbox:local
```

Recommended production sandbox image:

```bash
docker build -t qsm-omni-sandbox:local -f deploy/qsm-omni-sandbox.Dockerfile .
./qsm sandbox -root . -probe -backend docker -image qsm-omni-sandbox:local -profile omni
QSM_ALLOW_LOCAL_RELEASE_EVIDENCE=1 ./qsm qa -root . -profile local-production -sandbox docker -image qsm-omni-sandbox:local
```

Fast local Docker smoke image:

```bash
docker pull node:22-alpine
QSM_SANDBOX_DOCKER_IMAGE=node:22-alpine ./qsm sandbox -root . -probe -backend docker
QSM_ALLOW_LOCAL_RELEASE_EVIDENCE=1 QSM_SANDBOX_DOCKER_IMAGE=node:22-alpine ./qsm qa -root . -profile local-production -sandbox docker
```

Every delivered product branch must now include `product/qsm_project_manifest.v1.json`. Collapse rejects a branch when the manifest is missing, the product kind is unsupported, expected artifacts are absent, test commands are missing, or memory citations do not include `cache_item:` or `wiki_item:`.

## Capacity Planning

Node count is calculated from local hardware instead of hardcoded. The planner estimates active node cost as:

- 9Router API socket/result buffers
- OpenCode CLI terminal process
- Harness agent loop supervisor
- Wiki LLM working context

On a 16 GiB / 10 CPU machine, the default conservative profile recommends 7 active nodes/positions.

## Execution Accounting

`qsm run` now executes all positions through a bounded executor instead of aborting on the first node error. Each node writes a `BranchResult`, and `.state/run_report.json` records:

- requested, started, succeeded, and failed node counts
- effective parallelism
- per-node agent/model assignment
- per-node error text when a harness fails
- whether every requested node was accounted for before collapse

Use:

```bash
go run ./cmd/qsm status
```

to inspect the last run in a human-readable form.

For real harnesses, `-parallel auto` is conservative and caps concurrent live nodes to reduce provider rate-limit failures. Use `-parallel N` to force a larger wave. Retryable failures such as 429s, temporary timeouts, and missing product artifacts are retried with:

```bash
go run ./cmd/qsm run -request "..." -harness opencode -retries 2 -retry-backoff 15s
```

## Local Deployment

`qsm deploy` builds the local binary, starts 9Router as a managed background process when needed, waits for `/v1/models`, and writes `.state/deploy.json`.

```bash
go run ./cmd/qsm deploy -harness opencode
./qsm doctor -harness opencode
```

Managed 9Router logs and PID are written to:

- `.state/9router.log`
- `.state/9router.pid`

Stop the managed router with:

```bash
./qsm stop
```

## Current Scope

`simulated` mode is local and deterministic. It proves the orchestration flow but does not run live agents.

Real nodes require:

- `QSM_9ROUTER_URL`, defaulting to `http://localhost:20128/v1`
- `QSM_9ROUTER_API_KEY`
- `QSM_OPENCODE_PATH` for `-harness opencode`
- `QSM_LANGCHAIN_RUNNER` for `-harness langchain`
- compiled Wiki memory at `internal/wiki/wiki.md`

Use this to check whether real agents can run:

```bash
go run ./cmd/qsm doctor -harness opencode
```

Once the doctor is green:

```bash
go run ./cmd/qsm run -request "Build a small product" -positions auto -parallel auto -harness opencode
```
