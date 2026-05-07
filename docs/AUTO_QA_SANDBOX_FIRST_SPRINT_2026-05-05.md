# Auto-QA Sandbox-First Sprint

Date: 2026-05-05

## Status

This is the active sprint after Grok's adversarial review. Verdict remains `PARTIAL`: QSM has strong local-alpha QA, but production QA must stay blocked until sandbox and observability evidence are real.

## Sprint Rule

Hard sandbox execution is Phase 0.

Do not promote coverage, mutation, flake, browser matrix, or 24/7 reliability claims before command execution is isolated and observable.

## Phase 0 Deliverables

- `internal/sandbox` distinguishes Docker CLI presence from Docker daemon reachability.
- `qsm sandbox -probe -backend room|docker|auto` records inside-room access, outside-room blocking, network blocking, timeout killing, and resource policy.
- `internal/sandbox.Runner` supports `room` and `docker` backends.
- `internal/tester` routes QSM-owned and manifest commands through the selected sandbox runner.
- `.qsm_test/trace.jsonl` records test command start/end events.
- `qsm qa -sandbox room|docker|auto` refreshes sandbox evidence and production fails unless Docker/microVM hard sandbox probe passes.

## Expected Local Result

- `qsm qa -profile alpha -sandbox room` may pass with warnings.
- `qsm qa -profile production -sandbox docker` must fail if Docker daemon is unavailable or the Docker probe fails.
- Remaining production gates for trace report, benchmark adapter, lake citations, coverage, mutation, flake, cost budget, and CI remain failed until later phases.

## Next Phase

After Docker probe passes, move to trace lake plus official-shaped benchmark adapter. Coverage and mutation stay deferred until sandbox execution is reliable.
