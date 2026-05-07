# QSM Operational Runbook - 2026-05-07

## Purpose

This runbook defines the local operations evidence required before QSM can move from local-production to hosted production or Omni-Creator Alpha.

## Approval Gates

Every external release must pass these approval gate checks:

- `qsm qa -profile production -sandbox docker -image qsm-omni-sandbox:local -refresh=true` in hosted CI.
- `qsm qa -profile omni-alpha -sandbox docker -image qsm-omni-sandbox:local -refresh=true` for Omni-Creator Alpha claims.
- `.state/production_gap_report.json` must keep `production_ready=false` and `top_tier_ready=false` whenever required gates fail.
- Local release evidence is allowed only for `local-production` with `QSM_ALLOW_LOCAL_RELEASE_EVIDENCE=1`.
- `production` and `omni-alpha` must reject local waiver evidence.

## RPO And RTO

- RPO target for QSM evidence: less than 5 minutes of lost state. Evidence producers write JSON/Markdown reports under `.state/`, `.rooms/*/.qsm_test/`, and `.benchmarks/**/.state/`.
- RTO target for local recovery: less than 15 minutes to rerun `qsm sandbox`, `qsm benchmark`, `qsm self-improve`, and `qsm qa` after Docker/Colima restart.
- If Docker daemon is unavailable, QSM must report infra-unavailable and must not claim hard sandbox readiness.

## Recovery Procedure

1. Restart Docker/Colima outside QSM when needed.
2. Run `./qsm sandbox -probe -backend docker -image qsm-omni-sandbox:local -profile omni`.
3. Run `./qsm stress -sandbox docker -nodes 12 -parallel 6 -large-repo-files 1000`.
4. Run `./qsm benchmark -suite omni-contract -sandbox docker -image qsm-omni-sandbox:local -positions 2 -parallel 2`.
5. Run `./qsm self-improve -suite omni-contract -cycles 3 -sandbox docker -image qsm-omni-sandbox:local -positions 2 -parallel 2`.
6. Run `QSM_ALLOW_LOCAL_RELEASE_EVIDENCE=1 ./qsm qa -profile local-production -sandbox docker -image qsm-omni-sandbox:local -refresh=true`.
7. Run strict `production` and `omni-alpha` QA without local waiver to confirm the remaining hosted CI/top-tier gaps.

## Evidence Retention

GitHub Actions must upload `.state/*.json`, `.state/*.md`, benchmark reports, and benchmark logs with explicit artifact retention. Local runs should preserve `.state/production_gap_report.json` after every strict QA check.
