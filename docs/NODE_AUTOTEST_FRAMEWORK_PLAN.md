# QSM Node Auto-Test Framework Plan

Date: 2026-05-02

This plan defines the quality gate that every Quantum Swarm V3 node must pass before collapse can treat its product as real. It combines the current QSM architecture, Codex-style tool execution, and Grok Council advice, but corrects claims that were not supported by tool facts.

## Goal

The core rule is simple:

> A node may write code, tests, and evidence, but QSM owns the test runner and the verdict.

Prompt rules and self-verification help agents stay organized, but they are not proof. A product is only eligible for collapse after QSM executes deterministic checks inside the room and records command outputs, exit codes, and test counts.

## Current Gap

Current QSM verification is useful but shallow:

- Product path exists and is a non-empty directory.
- Static web products must have `index.html`, a script/canvas/JS surface, and existing local assets.
- JavaScript files are syntax-checked with `node --check`.

This catches broken packaging and syntax, but it does not prove behavior. It also lets harnesses mark `TestPassed=true` too early if the product merely exists.

## 2026 Big-Tech QA Review

Update on 2026-05-05: the current implementation is now stronger than the original gap statement. QSM rejects testless code products, requires static-web behavior/browser smoke, records command evidence, and has a no-progress node watchdog.

Grok Council plus independent research still classify it as **PARTIAL**, not big-tech-level QA yet. The detailed review is recorded in `docs/AUTO_QA_BIG_TECH_RESEARCH_2026-05-05.md`.

The next production-grade QA gates are:

- hard Docker/microVM command isolation;
- official-shaped SWE-bench/Terminal-Bench task adapters;
- coverage, mutation, property/fuzz, and hidden negative-control gates;
- flake detection and quarantine;
- full model/tool/file-edit trace records in the lake;
- Playwright browser matrix with trace-on-failure;
- CI/release gate that fails if any production QA check is skipped.

## Verified Facts

- Codex-style coding agents are strongest when they can read/edit files and run real test harnesses, linters, and type checkers, with terminal/test outputs available as evidence. Source: [OpenAI Codex introduction](https://openai.com/index/introducing-codex/).
- Agent loops repeatedly run tools and feed tool output back into the model; OpenAI describes Codex as scanning files, editing, running tests, and repeating. Source: [OpenAI WebSocket agent workflow article](https://openai.com/index/speeding-up-agentic-workflows-with-websockets/).
- Go tests are conventionally executed with `go test`; files ending in `_test.go` contain test functions. Source: [Go add a test tutorial](https://go.dev/doc/tutorial/add-a-test).
- `go test -json` emits machine-readable JSON, not JUnit XML. Source: [cmd/go documentation](https://pkg.go.dev/cmd/go).
- Pytest discovers `test_*.py` and `*_test.py` files and can run all discovered tests from the command line. Source: [pytest invocation docs](https://docs.pytest.org/en/stable/how-to/usage.html).
- `node --check` syntax-checks a script without executing it, and Node has a built-in `--test` runner. Source: [Node CLI docs](https://nodejs.org/api/cli.html).
- `npm test` runs the command defined in the package's `scripts.test` field. Source: [npm test docs](https://docs.npmjs.com/cli/v11/commands/npm-test/).
- Playwright can run browser tests headlessly from the command line with `npx playwright test`. Source: [Playwright running tests docs](https://playwright.dev/docs/running-tests).

## Grok Council Result

Grok agreed with the main architecture: node-authored tests are not enough, and QSM needs independent deterministic tests after product creation.

Corrections applied:

- Grok claimed `go test -json > junit.xml`; this is wrong. QSM should parse Go JSON natively, then optionally export JUnit later.
- Grok cited several external sources without URLs or verifiable details. Those claims are treated as advisory, not factual input.
- Grok recommended coverage gates immediately. For QSM V3, coverage should be Phase 2 because generated micro-products may not have stable coverage tooling.

## Per-Node Contract

Each room should produce:

```text
.rooms/pos-N/
├── product/
├── evidence.json
├── test_manifest.json        # optional node suggestion, never trusted alone
└── .qsm_test/
    ├── qsm_test_report.json  # written by QSM
    ├── commands/
    ├── logs/
    └── hidden/
```

Node responsibilities:

- Build the requested product in `./product`.
- Optionally write tests under `./product/tests` or declare test commands in `test_manifest.json`.
- Report what it believes it tested in `evidence.json`.

QSM responsibilities:

- Detect product type and language/tooling.
- Generate or select independent checks that the node did not author.
- Run all commands from a controlled runner.
- Record command, working directory, duration, exit code, stdout/stderr log paths, and parsed test counts.
- Override `BuildPassed`, `TestPassed`, `LintPassed`, and `Score` from real results.

## Manifest Schema

The node may suggest commands, but QSM decides whether to run them.

```json
{
  "schema": "qsm.test_manifest.v1",
  "product_type": "static-web|go|python|node|generic",
  "commands": [
    {
      "name": "unit tests",
      "cmd": ["pytest", "-q"],
      "cwd": "product",
      "kind": "test",
      "timeout_seconds": 60
    }
  ],
  "expected_files": ["README.md"],
  "web": {
    "entry": "index.html",
    "requires_interaction": true
  }
}
```

Rules:

- Commands must be arrays, not shell strings.
- `cwd` must resolve inside the room.
- Network access is disabled for product tests unless explicitly allowed by QSM.
- Manifest commands can only add checks, not remove QSM-owned checks.

## Test Report Schema

QSM writes this, not the node:

```json
{
  "schema": "qsm.test_report.v1",
  "passed": true,
  "product_type": "static-web",
  "summary": {
    "commands": 4,
    "passed_commands": 4,
    "failed_commands": 0,
    "tests": 3,
    "passed_tests": 3,
    "failed_tests": 0
  },
  "commands": [
    {
      "name": "javascript syntax",
      "cmd": ["node", "--check", "app.js"],
      "cwd": "product",
      "exit_code": 0,
      "duration_ms": 82,
      "stdout_path": ".qsm_test/logs/js-check.stdout.log",
      "stderr_path": ".qsm_test/logs/js-check.stderr.log"
    }
  ],
  "warnings": [],
  "errors": []
}
```

`BranchResult.Verification` should include the high-level product shape checks. `BranchResult.TestReport` should include executable command truth. Collapse must use both.

## Deterministic Gates

### All Products

Required:

- Product directory exists and is non-empty.
- No absolute local-machine paths in deliverable references.
- No missing local assets.
- Evidence exists in strict mode.
- QSM test report exists and is internally consistent.

### Go

Detection:

- `go.mod`, `*.go`, or `*_test.go` in product.

Commands:

- `go test ./... -json` when `go.mod` exists.
- `go test . -json` for single-package products without module support if possible.
- `gofmt -w` should not be run by the verifier; use `gofmt -l` as a lint check to avoid mutating node output.

### Python

Detection:

- `*.py`, `pyproject.toml`, `requirements.txt`, or `tests/`.

Commands:

- `python -m compileall -q product` for syntax/import-level compilation.
- `python -m pytest -q` if pytest is available and tests are present.
- If pytest is missing, mark a warning and still run syntax checks.

### Node / JavaScript / TypeScript

Detection:

- `package.json`, `*.js`, `*.mjs`, `*.cjs`, `*.ts`, or `tests/`.

Commands:

- `node --check` for plain JS files that Node can parse.
- `npm test -- --runInBand` or plain `npm test` only if package scripts include `test`.
- `node --test` if Node test files exist and no package test script is defined.
- TypeScript requires a local compiler or package script; otherwise warn rather than invent a compiler.

### Static Web

Detection:

- `index.html`.

Commands/checks:

- Existing HTML/asset/JS syntax checks.
- Phase 1: generated Node smoke script using Playwright if installed:
  - open `file://.../index.html`;
  - fail on console errors;
  - assert body text exists;
  - if canvas exists, assert non-zero dimensions and non-blank pixel sample;
  - click one visible button if available and assert no crash.
- If Playwright is unavailable, mark browser smoke as skipped warning, not pass.

## Anti-Fake-Test Design

The anti-fake barrier has three layers:

1. Node-authored tests are informational.
2. QSM-owned hidden probes are generated after the node finishes.
3. Collapse scores only QSM-executed results.

Initial fake-test heuristics:

- Warn if test files contain no assertions.
- Warn if tests only assert constants such as `true`, `1 == 1`, or static snapshots with no product import/load.
- Warn if `evidence.json` claims tests passed but QSM did not run a matching command.
- Fail if a manifest command tries to run outside the room.

Phase 2 anti-fake checks:

- Metamorphic probes: run the same function/CLI/UI with varied inputs.
- Negative controls: malformed input must fail gracefully.
- Mutation smoke: temporarily alter one generated output or config in a copy and verify the test can fail.

## Collapse Scoring

Suggested scoring:

```text
score = 0.25 build_shape + 0.45 deterministic_tests + 0.15 lint_syntax + 0.10 audit + 0.05 citations
```

Hard fail conditions:

- Product verification fails.
- Any required deterministic command fails.
- Test report missing in strict-test mode.
- Command attempted to escape the room.

Cache flow:

- Passing command recipes become `verified_recipe` cache entries.
- Failures become `failed_attempt` entries with normalized error signature and log path.
- Sibling nodes may consume failure summaries from live cache, but they must still pass their own room-local test run.

## Implementation Slice

Phase 1 should be small and shippable:

1. Add `internal/tester` package.
   - `Manifest`
   - `TestReport`
   - `CommandResult`
   - product detector
   - command runner with timeout and cwd guard
2. Extend `BranchResult` with `TestReport *tester.TestReport`.
3. After `requireProduct`, run `tester.Verify(room, productPath)`.
4. Override `TestPassed` and `LintPassed` from QSM test report.
5. Persist `.qsm_test/qsm_test_report.json` and mirror summary into `evidence.json`.
6. Add `qsm status` output: commands passed/failed and test counts.
7. Add fixtures/tests:
   - static web missing asset fails;
   - JS syntax error fails;
   - Python syntax error fails;
   - fake evidence saying tests passed is overridden by command failure;
   - Go sample with `go test ./... -json` passes;
   - manifest command outside room is rejected.

## Phase 1 Implementation Status

Implemented on 2026-05-02:

- Added `internal/tester` as the QSM-owned auto-test service.
- Added `qsm.test_manifest.v1` parsing for optional node-suggested checks.
- Added `qsm.test_report.v1` reports written to `.qsm_test/qsm_test_report.json`.
- Added deterministic command execution with guarded CWD, timeouts, stdout/stderr log capture, exit codes, and parsed test counts where available.
- Added product detection for generic, static web, Go, Python, and Node/JavaScript products.
- Added Go gates using `go test ... -json` and `gofmt -l`.
- Added Python gates using `python -m compileall` and pytest when tests exist.
- Added JavaScript gates using `node --check`, `npm test` when a package test script exists, and `node --test` when native test files exist.
- Added dependency-gated Playwright smoke: QSM runs browser smoke only when the Playwright package and Chromium executable are both already available; otherwise it records a warning and does not install dependencies during verification.
- Wired QSM test reports into OpenCode, LangChain, and simulated harnesses after product verification.
- Extended `BranchResult`, room status, `qsm status`, and cache publication with test report summaries.
- Added regression tests proving broken JavaScript, Python syntax errors, and escaping manifest commands fail even if an agent claims success.

Important current behavior:

- Generic products with no executable surface can pass with `commands=0`. This is intentional for documents/reports, but stricter product-type requirements should be added per deliverable class.
- Browser smoke is skipped unless Playwright and its browser binary already exist locally. QSM does not run `npx playwright install` during verification because that would mutate the environment and may require network.

Phase 2:

- Playwright browser smoke with screenshots/log capture.
- Coverage collection where tooling is already present.
- Metamorphic and mutation smoke checks.

Phase 3:

- Independent test-generator node that writes hidden probes from the objective and product API.
- Test quality reranker.
- JUnit export for CI integrations.

## Definition of Done

QSM can call itself production-ready for node auto-test when:

- No branch can set `TestPassed=true` without QSM-executed command evidence.
- Every collapse candidate has `.qsm_test/qsm_test_report.json`.
- `qsm status` shows command/test counts.
- Broken products fail deterministically even if `evidence.json` claims success.
- The full repo passes `go test ./...` after implementation.
