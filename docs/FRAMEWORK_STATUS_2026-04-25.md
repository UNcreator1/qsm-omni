# Quantum Swarm V3 Framework Status - 2026-04-25

## Summary

The framework is now much closer to a real Quantum Swarm Team implementation, but it is not production-complete yet.

The Go orchestrator can now:

- deploy/check the local 9Router socket
- verify OpenCode, 9Router config, Python, DeepAgents, and LangChain prerequisites
- run node rooms through a LangChain/DeepAgents Python harness
- account for requested/started/succeeded/failed nodes honestly
- capture stdout/stderr/evidence per room
- ask the Council as an advisory reasoning layer
- keep deterministic collapse separate from Council advice

The latest real smoke test did not collapse successfully. It proved the DeepAgents path is being reached, but the node failed with a LangGraph recursion-limit error before producing a valid product.

## Important Files

- Runner: `/Users/nexus/Downloads/NemoClaw/go_claw/quantum-swarm-v3-scratch/harness/langchain_runner.py`
- Go harness: `/Users/nexus/Downloads/NemoClaw/go_claw/quantum-swarm-v3-scratch/internal/swarm/harness.go`
- Runtime config/doctor: `/Users/nexus/Downloads/NemoClaw/go_claw/quantum-swarm-v3-scratch/internal/runtime/config.go`
- Council client: `/Users/nexus/Downloads/NemoClaw/go_claw/quantum-swarm-v3-scratch/internal/council/council.go`
- DeepAgents mapping note: `/Users/nexus/Downloads/NemoClaw/go_claw/quantum-swarm-v3-scratch/docs/DEEPAGENTS_NODE_LOOP.md`
- Latest run report: `/Users/nexus/Downloads/NemoClaw/go_claw/quantum-swarm-v3-scratch/.state/run_report.json`
- Latest room evidence: `/Users/nexus/Downloads/NemoClaw/go_claw/quantum-swarm-v3-scratch/.rooms/pos-01/evidence.json`

## What Was Fixed

1. Real DeepAgents runner added.

   `harness/langchain_runner.py` no longer defaults to a deterministic fake. It now creates a DeepAgents graph using `create_deep_agent`, `LocalShellBackend`, and `ChatOpenAI` pointed at 9Router.

2. Explicit fallback only.

   The old deterministic snake-game generator is still available only with:

   ```bash
   QSM_LANGCHAIN_FALLBACK=1
   ```

3. 9Router model routing added.

   The runner uses:

   - `QSM_9ROUTER_URL`
   - `QSM_9ROUTER_API_KEY`
   - QSM agent provider/model

   It also supports a smoke-test override:

   ```bash
   QSM_LANGCHAIN_MODEL=oc/minimax-m2.5-free
   ```

4. Absolute path handling fixed.

   Earlier, `.venv/bin/python` was resolved relative to `.rooms/pos-01`, causing:

   ```text
   fork/exec .venv/bin/python: no such file or directory
   ```

   Runtime config now resolves workspace paths as absolute paths.

5. Room nesting bug fixed.

   Earlier, the runner launched inside `.rooms/pos-01` and then resolved `.rooms/pos-01` again, creating a nested fake room:

   ```text
   .rooms/pos-01/.rooms/pos-01
   ```

   The Go harness now converts `p.Room` to an absolute path before execution.

6. Memory budget added.

   A full compiled wiki was around 12 MB and caused a huge prompt:

   ```text
   requested about 3429414 tokens
   ```

   The runner now caps room-local wiki memory with `QSM_WIKI_MEMORY_MAX_CHARS`, defaulting to `60000`.

7. Hydration exclusions added.

   Local repo hydration now skips generated/runtime directories:

   - `.venv`
   - `.lake`
   - `.rooms`
   - `.state`
   - `deliveries`
   - `node_modules`
   - build/cache folders
   - compiled `internal/wiki/wiki.md`

   Latest hydration dropped from thousands of files to 32 files.

8. Council advisory layer integrated.

   `qsm council` can call the local Council scripts and save:

   ```text
   .state/council_advice.json
   ```

   Council advice is intentionally advisory only. It does not decide deterministic collapse.

## Latest Real Smoke Test

Command used:

```bash
QSM_LANGCHAIN_MODEL=oc/minimax-m2.5-free ./qsm run \
  -harness langchain \
  -positions 1 \
  -parallel 1 \
  -timeout 4m \
  -retries 0 \
  -request "Build a tiny static snake game in HTML/CSS/JS. Put all files in ./product and write evidence.json."
```

Result:

```text
Nodes: requested=1 started=1 succeeded=0 failed=1 parallel=1 accounted=true
Collapse: winner=pos-01 approved=false reason=no branch survived deterministic collapse gates
```

Failure:

```text
langgraph.errors.GraphRecursionError: Recursion limit of 80 reached without hitting a stop condition.
```

Latest room did not contain a final product directory after this run.

## Issues Still Open

1. DeepAgents graph does not stop reliably.

   The model can continue tool/planning loops until LangGraph hits `GraphRecursionError`. QSM needs a stronger node completion contract.

2. Product acceptance after partial graph failure is incomplete.

   A previous run produced product files but still failed because the graph did not stop cleanly. A post-loop adapter was started, but the latest run failed before product files existed. This needs a more robust stream-based or checkpoint-based finalizer.

3. Rate limits remain real.

   `oc/ling-2.6-flash-free` hit provider 429 limits. The runner has model override and retry knobs now, but QSM still needs scheduler-level rate-limit/backoff across nodes.

4. DeepAgents default middleware still includes subagent machinery.

   The `task` tool is excluded from the exposed tool list, but DeepAgents still builds subagent middleware internally. For QSM, one node should equal one graph unless QSM explicitly enables nested agents.

5. LocalShellBackend is not a true sandbox.

   `LocalShellBackend(root_dir=room, virtual_mode=True)` scopes filesystem tools, but shell commands are not securely sandboxed. Production execution needs Docker, VM, or a stricter backend.

6. Deterministic verification is still shallow.

   Current post-loop product checks only prove that a product exists and, for HTML, includes basic HTML/script/canvas structure. It does not yet run browser checks, tests, lint, or security audit per deliverable type.

7. Multi-node LangChain swarm is not yet validated.

   Single-node DeepAgents path reaches 9Router. Multi-node parallel LangChain execution should wait until scheduler backoff and node stop conditions are stronger.

## Next Implementation Steps

1. Replace `graph.invoke(...)` with a streamed DeepAgents execution loop.

   Capture tool events as they happen and stop when QSM sees:

   - non-empty `./product`
   - valid `./evidence.json`
   - deterministic post-loop verification passes

2. Add QSM node stop policy.

   Suggested defaults:

   - max model turns per node
   - max tool calls per node
   - max wall-clock per node
   - stop immediately after evidence and product pass

3. Add scheduler-level model backoff.

   Track provider/model 429s and delay or switch models before launching more nodes.

4. Make model routing configurable in Go.

   Add flags/env support for per-run model override instead of relying only on `QSM_LANGCHAIN_MODEL`.

5. Add deterministic product verification by product type.

   For static web apps:

   - check `index.html`
   - run a browser/canvas smoke if possible
   - verify JS syntax
   - verify no missing local assets

6. Re-run smoke:

   ```bash
   QSM_LANGCHAIN_MODEL=oc/minimax-m2.5-free ./qsm run \
     -harness langchain \
     -positions 1 \
     -parallel 1 \
     -timeout 4m \
     -retries 0 \
     -request "Build a tiny static snake game in HTML/CSS/JS. Put all files in ./product and write evidence.json."
   ```

7. Only after single-node success, test:

   ```bash
   ./qsm run -harness langchain -positions 2 -parallel 2 ...
   ```

## Bottom Line

The framework is no longer just generating examples. It now reaches a real DeepAgents/LangChain node through 9Router and records failures correctly. The current blocker is node-loop control: DeepAgents needs to be bounded and collapsed into QSM evidence deterministically instead of relying on the model to stop perfectly.
