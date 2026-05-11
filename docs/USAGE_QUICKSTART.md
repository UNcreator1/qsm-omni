# QSM Usage Quickstart

This is the practical operator guide for running QSM locally.

## Current Honest Scope

QSM is currently an Omni-Creator Alpha framework:

- It can run contract-covered local product builds.
- It can test, trace, score, archive, and deliver products.
- It keeps `.lake` as shared memory.
- It can diagnose failed nodes and seed regrow rooms from checkpoints.

It is not yet a claim that QSM can build any arbitrary software at enterprise production level without new contracts, real harness stress, and domain-specific validation.

## One-Time Build

```bash
cd /Users/nexus/Downloads/NemoClaw/UNcreator1/qsm-omni
go test ./...
go build -o qsm ./cmd/qsm
```

## Clean For A New Test

Use this before testing a new product request. It keeps `.lake` intact.

```bash
mkdir -p archived-deliveries
if [ -d deliveries ]; then
  find deliveries -mindepth 1 -maxdepth 1 -exec mv {} archived-deliveries/ \;
fi
rm -rf .rooms .state .benchmarks
mkdir -p .rooms .state deliveries
```

Do not delete `.lake` unless you intentionally want to erase the memory brain.

## Basic Local Alpha Run

This uses the deterministic simulated harness. It is the safest way to check the framework loop.

```bash
./qsm run \
  -request "product_kind=cli-tool. Build a tiny tested CLI tool." \
  -harness simulated \
  -positions 3 \
  -parallel 3 \
  -shared-cache=true
```

Then inspect:

```bash
./qsm status
./qsm qa -profile alpha -sandbox room -refresh=true
```

Important outputs:

- `.state/run_report.json`
- `.state/verdict.json`
- `.state/qa_report.json`
- `.rooms/pos-N/`
- `deliveries/<objective-id>/`

## Failure Learning

After any run, QSM can turn failed nodes into typed learning evidence:

```bash
./qsm failure-analyze -root .
```

Outputs:

- `.state/failure_report.json`
- `.state/failure_report.md`
- `.rooms/pos-N/failure-evidence/failure.json`
- `.lake/failures/*.json`
- `.lake/cache/*failure_lesson*.json`

Failure classes include:

- `compile_test_runtime`
- `sandbox_tooling`
- `model_route`
- `missing_knowledge_materials`
- `hallucinated_files_tests`
- `cost_timeout`
- `other`

## Regrow From Failure

When failed nodes have checkpoints, seed fresh regrow rooms:

```bash
./qsm regrow -root . -from-failures .state/failure_report.json -limit 3
```

Outputs:

- `.state/regrow_report.json`
- `.state/regrow_report.md`
- `.rooms/regrow-<position>-rN/`
- `.rooms/regrow-<position>-rN/.qsm_regrow/seed.json`
- `.rooms/regrow-<position>-rN/.qsm_memory/REGROW.md`

Current limitation: `qsm regrow` prepares regrow rooms, but does not yet execute the regrown branch automatically.

## Docker Production-Shaped Run

Docker is required for hard sandbox production-style evidence.

```bash
docker build -t qsm-omni-sandbox:local -f deploy/qsm-omni-sandbox.Dockerfile .
./qsm sandbox -root . -probe -backend docker -image qsm-omni-sandbox:local -profile omni
```

If the probe passes:

```bash
QSM_ALLOW_LOCAL_RELEASE_EVIDENCE=1 \
./qsm qa -root . -profile local-production -sandbox docker -image qsm-omni-sandbox:local -refresh=true
```

Hosted CI, not local waiver, is the source of truth for external production claims.

## Omni Contract Benchmark

Run the six supported project-kind contracts:

```bash
./qsm benchmark -suite omni-contract -sandbox docker -image qsm-omni-sandbox:local
./qsm self-improve -suite omni-contract -cycles 3 -sandbox docker -image qsm-omni-sandbox:local
./qsm qa -profile omni-alpha -sandbox docker -image qsm-omni-sandbox:local -refresh=true
```

The six v1 product kinds are:

- `static-web`
- `cli-tool`
- `go-service`
- `python-package`
- `node-fullstack`
- `data-transform`

## Real Harness Readiness

Simulated mode proves orchestration. Real OpenCode/LangChain nodes require 9Router and harness dependencies.

```bash
./qsm harness-readiness -root . -harness all
./qsm real-harness-smoke -root . -harness langchain -sandbox docker -execute=false
```

Only use `-execute=true` once readiness and route health are green.

## Good Daily Loop

```bash
# 1. clean transient state but keep memory
rm -rf .rooms .state .benchmarks && mkdir -p .rooms .state deliveries

# 2. run a product objective
./qsm run -request "product_kind=static-web. Build a tested browser app." -harness simulated -positions 3 -parallel 3 -shared-cache=true

# 3. refresh evidence
./qsm qa -profile alpha -sandbox room -refresh=true

# 4. if nodes failed, learn and seed regrow
./qsm failure-analyze -root .
./qsm regrow -root . -from-failures .state/failure_report.json -limit 3
```
