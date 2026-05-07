# Quantum Swarm V3 Deployment - 2026-04-27

## Deployed State

The LangChain/DeepAgents harness is now deployed with a QSM-owned streaming stop policy.

Implemented changes:

- Replaced blocking `graph.invoke(...)` with `graph.stream(...)`.
- Added `QSM_NODE_MAX_STEPS` node-loop budget.
- Added per-room `deepagents.events.jsonl` event capture.
- Added deterministic product/evidence checks after each graph event.
- Added repair for DeepAgents virtual filesystem absolute-path mirroring.
- Strengthened static-web verification so referenced local assets must exist.
- Rebuilt `./qsm`.

## Verification

Commands passed:

```bash
.venv/bin/python -m py_compile harness/langchain_runner.py
go test ./...
go build -o qsm ./cmd/qsm
./qsm deploy -harness langchain -wait 30s
./qsm doctor -harness langchain
```

`qsm doctor -harness langchain` reported all prerequisites ready, including 9Router live at:

```text
http://127.0.0.1:20128/v1
```

## Real Harness Smokes

### One-Node Static Snake Build

Command:

```bash
QSM_LANGCHAIN_MODEL=oc/minimax-m2.5-free QSM_NODE_MAX_STEPS=60 ./qsm run \
  -harness langchain \
  -positions 1 \
  -parallel 1 \
  -timeout 5m \
  -retries 0 \
  -request "Build a tiny static snake game in HTML/CSS/JS. Put all files in ./product and write evidence.json."
```

Result:

```text
Nodes: requested=1 started=1 succeeded=1 failed=0 parallel=1 accounted=true
Collapse: winner=pos-01 approved=true
Delivered product: deliveries/obj-1777273685
```

Delivered files:

```text
deliveries/obj-1777273685/index.html
deliveries/obj-1777273685/style.css
deliveries/obj-1777273685/game.js
```

### Two-Node Parallel Smoke

Command:

```bash
QSM_LANGCHAIN_MODEL=oc/minimax-m2.5-free QSM_NODE_MAX_STEPS=25 ./qsm run \
  -harness langchain \
  -positions 2 \
  -parallel 2 \
  -timeout 4m \
  -retries 0 \
  -request "Deployment smoke: each node creates ./product/README.md with one line and ./evidence.json, then exits immediately."
```

Result:

```text
Nodes: requested=2 started=2 succeeded=2 failed=0 parallel=2 accounted=true
Collapse: winner=pos-01 approved=true
Delivered product: deliveries/obj-1777273799
```

## Current Limitations

- `LocalShellBackend` is still not a hard sandbox. Production needs Docker, VM, or another isolated backend.
- Static-web verification is useful but still shallow. Browser/canvas smoke tests are not wired into collapse yet.
- Scheduler-level rate-limit and model backoff are still needed before larger parallel swarms.
- Go evidence rewriting currently normalizes `evidence.json` and drops some runner-specific metadata such as `stop_reason`.

## Next Step

Implement product-type verifiers, starting with static web:

- validate referenced assets
- run JS syntax checks
- optionally run a browser smoke against `index.html`
- store verifier output in `evidence.json`

After that, scale from 2-node smoke to hardware-planned positions with model backoff enabled.
