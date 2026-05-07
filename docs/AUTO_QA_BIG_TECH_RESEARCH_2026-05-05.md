# Auto-Test / Auto-QA Big-Tech-Level Research Review

Date: 2026-05-05

## Verdict

Status: **PARTIAL**

Quantum Swarm V3 now has a serious local-first QA foundation: QSM owns the verifier, records command evidence, rejects testless code products, requires behavior smoke for static web, warns on fake tests, and kills stalled nodes with a no-progress watchdog.

It does **not** yet match big-tech / top-tier 2026 coding-agent QA level. The current force score of `5.7/10` is honest.

## Grok Council Result

Grok was queried through the Council bridge on port `8338` without restarting or modifying the port.

Grok verdict:

- `PARTIAL`
- Current QSM auto-QA is stronger than early agent prototypes.
- It is not yet at 2026 big-tech coding-agent QA level.
- The design is sound for the swarm philosophy, but it needs hard sandboxing, official benchmark adapters, coverage/mutation/fuzz/flake gates, trace/replay observability, and CI/security release gates.
- Follow-up Grok review required a **sandbox-first ordering**: do not build mutation, coverage, or deeper 24/7 QA on top of room-only execution.

Council files:

- Request packet: `/Users/nexus/Downloads/NemoClaw/Council/attachments/qsm-autoqa-bigtech-review-20260505.md`
- Latest Grok output: `/Users/nexus/Downloads/NemoClaw/Council/grok_latest.txt`

## Fact-Checked External Standards

OpenAI's Codex description sets a strong coding-agent baseline: independent tasks run in isolated environments; the agent can run test harnesses, linters, and type checkers; and users get terminal/test evidence for review. Source: [OpenAI Codex launch](https://openai.com/index/introducing-codex/).

OpenAI's agent eval guidance treats traces, graders, datasets, and eval runs as the core surfaces for improving agent quality and finding workflow-level regressions. Source: [OpenAI agent evals](https://developers.openai.com/api/docs/guides/agent-evals).

Terminal-Bench frames terminal-agent evaluation as a repeatable task contract: instruction, sandbox, verifier, oracle, and script-checked outcomes rather than confident final messages. Source: [Terminal-Bench](https://terminalbench.lol/).

Braintrust documents a production eval loop: promote experiments, run evals in CI/CD, score production traces, and feed interesting traces back into datasets. Source: [Braintrust evaluations](https://www.braintrust.dev/docs/evaluate).

Playwright's current test guidance emphasizes real assertions, isolated browser contexts, trace artifacts, and CI retry traces for debugging. Sources: [Playwright writing tests](https://playwright.dev/docs/writing-tests), [Playwright trace viewer](https://playwright.dev/docs/trace-viewer-intro).

GitHub CodeQL represents production security scanning expectations: code is analyzed with a query engine and findings surface as code-scanning alerts, including CI workflows. Source: [GitHub CodeQL code scanning](https://docs.github.com/en/code-security/concepts/code-scanning/codeql/about-code-scanning-with-codeql).

## Current QSM Strengths

- QSM-owned verifier writes `.qsm_test/qsm_test_report.json`; nodes cannot self-certify success.
- Command results record argv, cwd, exit code, duration, stdout/stderr log paths, command kind, and origin.
- Code products require a real test surface and at least one passing test command.
- Static-web products require a browser/behavior smoke and JavaScript syntax/lint check.
- Manifest commands are path-contained and additive.
- Fake-test heuristics catch no-assertion and constant-truth tests.
- Basic secret/dynamic-exec scan exists.
- No-progress watchdog cancels and retries silent node stalls.
- Local benchmark suite passed `2/2`, with clear warning that it is not official SWE-bench or Terminal-Bench evidence.

## Gaps To Big-Tech-Level QA

1. **Hard sandbox is not enforced.**
   Local room isolation is not equivalent to the isolated cloud/container environments used by serious coding-agent systems.

2. **No official benchmark adapter.**
   QSM has local SWE-bench/Terminal-Bench-inspired tasks, but no official or official-shaped adapter run evidence.

3. **No coverage, mutation, property, or fuzz gates.**
   Passing tests can still be shallow. Big systems need evidence that tests exercise behavior and catch regressions.

4. **No flake detection or quarantine.**
   Retries exist, but the system does not classify flaky tests, isolate them, or track recurrence.

5. **No trace/replay quality loop.**
   QSM logs command evidence, but it does not yet emit full model/tool/file-edit traces with graders, replay, datasets, or production-to-eval feedback.

6. **No CI/release gate.**
   There is no single `qsm qa --profile production` release gate that runs all required checks and fails promotion deterministically.

7. **Browser QA is too shallow.**
   Static-web smoke is useful, but not a browser matrix with accessibility, mobile viewport, performance, visual-regression, and trace-on-failure artifacts.

8. **Security/compliance is basic.**
   Basic scans exist; SBOM/license/CodeQL-style scans are not wired.

## Node Collapse Gates To Add

These should become mandatory before a node can win collapse in a production profile.

1. `sandbox_gate`
   Every command runs inside Docker or microVM policy. No command can touch host paths outside the room.

2. `benchmark_contract_gate`
   Every benchmark task has instruction, isolated environment, verifier script, expected artifact contract, and oracle/reference notes where possible.

3. `test_depth_gate`
   Code products require unit/integration tests, coverage threshold, and hidden behavior probes generated after build.

4. `mutation_or_negative_gate`
   At least one mutation/negative-control probe must fail when the product is intentionally broken in a copied sandbox.

5. `flake_gate`
   Critical checks run with controlled retry. Any nondeterministic result is recorded as flaky and cannot silently pass.

6. `trace_gate`
   Every LLM call, tool call, file edit, test command, retry, and collapse decision emits a trace record into the lake.

7. `browser_quality_gate`
   Static web products run Playwright with assertions, console-error failure, trace-on-retry, mobile/desktop viewport checks, and nonblank canvas/visual checks when relevant.

8. `security_gate`
   Critical/high findings from secret scan, dependency audit, and CodeQL/Semgrep-style static analysis fail production collapse.

9. `ci_release_gate`
   `qsm qa --profile production` must run locally and in CI with the same report schema.

10. `eval_regression_gate`
    Every production change runs against a saved dataset of QSM build tasks and compares pass rate, cost, latency, flake rate, and hallucination/unsupported-claim rate.

## Prioritized Build Plan

Update after Grok adversarial sprint review: the active plan is now sandbox-first. Hard sandbox execution is Phase 0 and must precede trace, official-shaped benchmarks, coverage, mutation, flake, and 24/7 stress. The active sprint note is `docs/AUTO_QA_SANDBOX_FIRST_SPRINT_2026-05-05.md`.

### Step 1: Production QA Profile

Add `qsm qa --profile alpha|beta|production`.

Production profile should fail if any mandatory gate is skipped. Alpha can warn/skip when optional tools are missing.

Status: implemented as `qsm qa -profile alpha|beta|production`. The command writes `.state/qa_report.json` and `.state/qa_report.md`, exits non-zero on failed required gates, and keeps production-only evidence such as trace/replay, coverage, mutation, flake, and CI release gates as hard production blockers.

### Step 2: Hard Sandbox Runner

Add a test command executor interface:

- `room-only` for alpha.
- `docker` for local beta.
- `microvm` later.

All manifest and QSM-owned commands should flow through the executor.

### Step 3: Coverage + Hidden Probe Generator

For Go/Python/Node:

- parse test counts;
- collect coverage where toolchain supports it;
- generate QSM-owned hidden probes from product entry points and requested acceptance criteria;
- fail production profile below threshold.

### Step 4: Mutation / Negative Control

Copy the product to `.qsm_test/mutation_work/`, apply safe mutations, run selected tests, and require at least one test to detect the break.

### Step 5: Flake Classifier

Run critical checks twice only when needed. Classify:

- deterministic pass;
- deterministic fail;
- flaky pass/fail;
- infra failure.

Collapse must not treat flaky as clean success.

### Step 6: Trace Lake

Create `internal/trace` and write structured JSONL:

- objective ID;
- room;
- node;
- model/route;
- prompt hash;
- tool command;
- file edit summary;
- command output paths;
- cost/tokens;
- QA result;
- collapse reason.

This becomes the local equivalent of LangSmith/Braintrust/OpenAI trace/eval surfaces.

### Step 7: Browser QA Upgrade

Use Playwright when installed:

- desktop and mobile viewports;
- console/pageerror failure;
- assertions against visible UI;
- canvas nonblank pixel check;
- screenshot and trace on failure;
- accessibility smoke where possible.

If Playwright is unavailable, production profile fails instead of silently accepting a static smoke substitute.

### Step 8: Official-Shaped Benchmark Adapter

Add `qsm benchmark --suite terminal-contract` with:

- instruction;
- sandbox root;
- verifier script;
- oracle notes;
- hidden expected artifacts;
- JSON report.

Then add official SWE-bench/Terminal-Bench adapters when dependencies and datasets are available.

### Step 9: Security Scan Layer

Wire optional scanners by availability:

- built-in secret/dynamic-exec scan;
- `npm audit` / `pip-audit` / `govulncheck` where applicable;
- Semgrep or CodeQL profile;
- SBOM later.

### Step 10: Release Gate

Collapse cannot promote to production delivery unless:

- force score passes threshold;
- all mandatory QA profile gates pass;
- every node is accounted for;
- lake citations and trace records exist;
- no stale running rooms remain;
- cost ceiling is respected.

## Current Claim Allowed

QSM can honestly claim:

> Local-alpha swarm framework with QSM-owned deterministic QA and anti-fake-test gates.

QSM cannot yet claim:

> Big-tech-level production coding-agent framework.

That claim should wait until hard sandbox, trace/eval loop, official-shaped benchmarks, coverage/mutation/flake gates, and production QA profile all pass repeatedly.
