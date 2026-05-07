# Superpowers Reference Integration

Date: 2026-05-03

QSM reviewed `obra/superpowers` as a reference framework for agent behavior. The useful conclusion is that Superpowers is not a model router, room harness, or collapse engine. It is a policy and skills layer that shapes how coding agents plan, test, debug, review, and claim completion.

## Decision

QSM should borrow the methodology, not depend on the external plugin at runtime.

Reasons:

- QSM already owns routing through 9Router, room isolation, harness execution, shared lake/cache, deterministic auto-test reports, and collapse.
- Superpowers' strength is behavior discipline: planning, TDD, verification-before-completion, systematic debugging, and review loops.
- Installing plugins inside every room would add external moving parts and make reproducibility harder.
- QSM nodes need phase-specific contracts, not the entire Superpowers bootstrap on every prompt.

## Implemented

QSM now has a native methodology package:

- `internal/methodology`
- contracts for synthesis, planning, build, debug, review, and collapse
- prompt rendering for phase-specific contract injection

The build-room prompts now include:

- `writing-plans`
- `test-driven-development`
- `verification-before-completion`

The synthesis phase now drops a verified `methodology_contract` artifact into the lake so later hydration/wiki layers can see the governing behavior contract.

## Mapping

| Superpowers idea | QSM mapping |
| --- | --- |
| brainstorming | Phase A synthesis and Council intake |
| writing-plans | Chop/planning contract |
| test-driven-development | Room build loop |
| verification-before-completion | Room evidence plus QSM-owned auto-test report |
| systematic-debugging | Retry and failed-attempt cache handling |
| requesting-code-review | Auditor nodes |
| finishing branch | Deterministic collapse and delivery |

## Guardrail

Agent self-discipline is useful, but it is not proof. The QSM-owned auto-test service remains the authority:

- agent evidence is advisory;
- `.qsm_test/qsm_test_report.json` is executable truth;
- collapse should prefer branches with deterministic build/test/lint evidence.
