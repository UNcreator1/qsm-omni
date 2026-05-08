# QSM Omni-Creator Alpha Status - 2026-05-07

> Superseded by `docs/OMNI_CREATOR_ALPHA_STATUS_2026-05-08.md`, where hosted CI run #17 closes the `ci-release` blocker and makes `omni-alpha` a blocking gate.

## Current Level

QSM now has the core Omni-Creator Alpha gates implemented and Docker-backed local-production QA is green. The repo is **not yet externally production-ready** and **not yet Omni-Creator Alpha** for one honest reason: hosted CI release evidence is still missing. The corrected force scorer now reports `9.6/10` with visible per-category scores, but production claims remain blocked until GitHub Actions produces CI evidence without the local waiver.

Implemented:

- `qsm ci-bootstrap -provider github`
- `deploy/qsm-omni-sandbox.Dockerfile`
- `qsm sandbox -probe -profile omni`
- `qsm_project_manifest.v1.json` branch contract
- Collapse rejection for invalid product manifests
- `qsm benchmark -suite omni-contract`
- `qsm self-improve -suite omni-contract`
- QA profiles: `local-production`, `production`, `omni-alpha`

Verified locally:

- `go test ./...` passed.
- `go build -o qsm ./cmd/qsm` passed.
- `docker build -t qsm-omni-sandbox:local -f deploy/qsm-omni-sandbox.Dockerfile .` passed.
- `./qsm sandbox -probe -backend docker -image qsm-omni-sandbox:local -profile omni -json` passed with `hard_sandbox_ready=true`, `readiness_level=omni-docker-probed`, outside-room access blocked, network blocked, timeout killed, and Go/Node/Python/pytest/Playwright runtime probes passing.
- `./qsm benchmark -suite omni-contract -sandbox docker -image qsm-omni-sandbox:local -positions 2 -parallel 2 -timeout 5m` passed `6/6` product classes.
- `./qsm self-improve -suite omni-contract -cycles 3 -sandbox docker -image qsm-omni-sandbox:local -positions 2 -parallel 2 -timeout 5m` passed with `6 -> 6`, repeated failure rate `0.00`, and three cache-cited lessons.
- `./qsm stress -root . -sandbox docker -nodes 12 -parallel 6 -large-repo-files 1000` passed with a real 1000-file synthetic repo verification.
- `./qsm ops-readiness -root .` passed with CI artifact retention, launchd/autonomy evidence, approval-gate runbook coverage, and truthful production-gap reporting.
- `./qsm compliance -root .` passed with Docker sandbox policy verification and local module/file inventory; it warns that the project has no explicit license file.
- DeepSeek fallback route-health passed when `DEEPSEEK_API_KEY` was mapped to `QSM_9ROUTER_API_KEY` for the command only. The key was not written into 9Router config.
- `QSM_ALLOW_LOCAL_RELEASE_EVIDENCE=1 ./qsm qa -profile local-production -sandbox docker -image qsm-omni-sandbox:local -refresh=true` passed `23/23` gates.
- `./qsm qa -profile production -sandbox docker -image qsm-omni-sandbox:local -refresh=true` fails honestly only on hosted CI release evidence.
- `./qsm qa -profile omni-alpha -sandbox docker -image qsm-omni-sandbox:local -refresh=true` passed `26` gates and failed `1` required gate: hosted CI release evidence.
- `./qsm force-score -root . -json` reports average `9.55`, with `self_improve_passed=true` and `ci_release_ci=false`.
- `./qsm production-gap -root .` reports only one failed production gate: `ci-release`.

Important implementation fix from this deployment:

- Docker runner private `/tmp` now uses `exec` in its tmpfs options so `go test` can execute compiled test binaries while keeping the room-only mount, non-root user, no Docker socket, blocked network, dropped capabilities, and resource limits.
- `qsm qa` now accepts `-image` and uses the full omni sandbox probe automatically for `omni-alpha`, preventing QA from accidentally judging `node:22-bookworm` instead of `qsm-omni-sandbox:local`.
- Self-improvement now treats a saturated 6/6 contract run with zero repeated failures as stable improvement evidence, while still requiring promoted lessons to cite `cache_item:` or `wiki_item:`.
- Docker runner now propagates per-command environment variables into the container with `--env`, which fixed manifest/stress commands that rely on command-scoped env values.
- GitHub Actions production workflow now passes `-image qsm-omni-sandbox:local` to production/omni QA and uploads QSM artifacts with explicit retention.
- Force scoring now exposes `evidence_score` per checklist category, consumes self-improvement evidence, and reserves the final external-production blocker for real hosted CI evidence instead of making the `9.5` threshold mathematically unreachable.

## Honest Claim Boundary

Do not claim QSM is an omni framework creator for arbitrary software yet.

The current alpha target means:

- 6 contract-covered product classes.
- Docker-backed sandbox execution.
- Hosted CI evidence without local waiver.
- Measurable self-improvement evidence.
- Force score `>= 9.5`.

Until those all pass, `production_ready` and `top_tier_ready` must remain false for external claims.

## Current Remaining To-Dos

1. Push/run the generated GitHub Actions workflow so hosted CI produces `.state/ci_release_report.json` without `QSM_ALLOW_LOCAL_RELEASE_EVIDENCE`.
2. Keep `production_ready=false` and `top_tier_ready=false` until CI evidence exists for the same production/omni QA path.
3. Start 9Router before route-health if 9Router combo evidence must count; direct DeepSeek fallback is healthy only when the key is mapped to `QSM_9ROUTER_API_KEY` for that command.
4. Later hardening after Omni-Creator Alpha: explicit license choice, stronger compliance scanning, microVM isolation, PR automation, and external official benchmark/ROI evidence.
