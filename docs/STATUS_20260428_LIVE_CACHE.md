# Quantum Swarm Status - 2026-04-28 Live Cache Slice

## Result

Implemented live shared-cache consumption for the DeepAgents/LangChain harness.

When `-shared-cache` or `QSM_SHARED_CACHE=1` is enabled, each LangChain node now:

- receives an executor-seeded objective `constraint` item before node execution starts
- reads verified items from `.lake/cache`
- filters them by objective and approved cache kinds
- refreshes `.rooms/pos-N/.qsm_memory/CACHE.md`
- updates the marked live-cache section in `.rooms/pos-N/.qsm_memory/AGENTS.md`
- injects the latest verified cache facts into the model system prompt before model calls
- logs newly observed cache IDs to `deepagents.events.jsonl`

This turns the cache from a start-of-node snapshot into a live advisory layer for active DeepAgents nodes.

## Verification

Passed:

- `.venv/bin/python -m py_compile harness/langchain_runner.py`
- `go test ./...`
- `go build -o qsm ./cmd/qsm`
- direct smoke: verified cache item appears only when `QSM_SHARED_CACHE=1`, then updates `CACHE.md` and AGENTS.md
- end-to-end fallback run: `QSM_LANGCHAIN_FALLBACK=1 ./qsm run -harness langchain -positions 2 -parallel 2 -shared-cache`
- `./qsm doctor -root . -harness langchain` confirmed local runner dependencies, but real 9Router live check failed because `http://127.0.0.1:20128/v1/models` refused connection
- after starting 9Router on port `20128`, `./qsm doctor -root . -harness langchain` passed with `9router_live: 200 OK`
- real two-node LangChain/9Router wave passed with `QSM_LANGCHAIN_MODEL=oc/ling-2.6-flash-free`

Fallback run result:

- requested nodes: 2
- started nodes: 2
- succeeded nodes: 2
- failed nodes: 0
- shared cache summary: `verified_recipe:2`
- delivery: `deliveries/obj-1777342910`

Real 9Router run result:

- objective: `obj-1777344820`
- requested nodes: 2
- started nodes: 2
- succeeded nodes: 2
- failed nodes: 0
- shared cache summary: `constraint:1 verified_recipe:6`
- both rooms logged `cache_refresh` for objective constraint `dbbbbc3caadabe5b`
- delivery: `deliveries/obj-1777344820`

## Grok Review

Council/Grok reviewed the live-cache slice via:

`/Users/nexus/Downloads/NemoClaw/Council/attachments/qsm-live-cache-slice-20260428.tar.gz`

Grok assessment:

- low logic risk
- middleware model-call injection is concept-compliant
- branch isolation is preserved because only verified cache facts are shared, not sibling product files
- objective filtering and latest-N limits reduce pollution and prompt bloat
- remaining main gap is OpenCode live cache consumption

Full response:

`/Users/nexus/Downloads/NemoClaw/Council/grok_latest.txt`

## Known Issues

- OpenCode harness still reads cache as a startup file only.
- Real multi-node DeepAgents live refresh now passes with `oc/ling-2.6-flash-free`; other configured providers still need credential/behavior vetting.
- Cache pruning/expiry is not implemented yet.
- Hydrator, verifier, and collapse can produce more structured `constraint`, `score_signal`, and `dependency_note` items.
- Cache quality gates are basic; future versions should prevent low-value or duplicate lessons from accumulating.

## Next Plan

Active roadmap:

`docs/TOLARIA_247_KNOWLEDGE_BRAIN_PLAN.md`

1. Add route-health selection that tests model chat/tool compatibility before assigning agents. Implemented as `qsm route-health` and optional `qsm run -route-health`; latest gated run passed with `1/3` healthy routes and `2/2` nodes succeeded.
2. Add Tolaria vault staging, schema validation, and safe Git commit flow.
3. Add live-cache support for OpenCode through a supervisor loop or prompt refresh strategy.
4. Add cache pruning and duplicate suppression.
5. Enable shared cache by default for real harness modes after route-health validation.

Cleanup note:

- Old generated lake artifacts/cache and prior deliveries were archived under `.archive/cleanup-20260428T033928Z`.
- A second cleanup archived accidental `.archive` hydration under `.archive/post-route-health-hydration-cleanup-20260428T034459Z`.
- Active delivery retained: `deliveries/obj-1777347848`.
- Hydrator now skips `.archive` so archived generated files cannot pollute future wiki memory.

Deployment note:

- `./qsm deploy -root . -harness langchain -build=true -start-router=true` completed at `2026-04-28T03:50:38Z`.
- `./qsm doctor -root . -harness langchain` passed, including `9router_live: 200 OK`.
- 9Router is listening on `127.0.0.1:20128`.
- Post-deploy route-health currently reports `0/3` healthy default routes because `oc/ling-2.6-flash-free` is temporarily rate-limited, `oc/minimax-m2.5-free` returns reasoning with empty content, and `or/inclusionai/ling-2.6-1t:free` has no active provider credentials.
- Real gated runs should wait for route-health to show at least one healthy route or use a known-good explicit model override.

Free-pool replacement note:

- `qsm route-health -models free` now discovers 9Router free/combo candidates from `/v1/models`.
- It prioritizes `wombo`, `oc/*`, and chat-capable `openrouter/*:free` routes.
- `qsm run -route-health -route-health-models free` can synthesize replacement agents from healthy discovered routes.
- Latest free-pool run approved a product with `oc/nemotron-3-super-free`, while `wombo` passed chat-health but failed the build loop. The next scheduler refinement should track build-health separately from chat-health.
- Active delivery retained: `deliveries/obj-1777349311`.

DeepSeek product-route note:

- DeepSeek key source was found at `/Users/nexus/Downloads/NemoClaw/scripts/Deepseek_keys`; the key was used only in-process and not copied into QSM state/docs.
- Direct API probe passed against `https://api.deepseek.com/chat/completions` with `deepseek-chat`.
- QSM route-health passed with `QSM_9ROUTER_URL=https://api.deepseek.com`, `QSM_LANGCHAIN_MODEL=deepseek-chat`: `1/1` healthy, OpenAI chat JSON shape.
- Full LangChain/QSM smoke passed with DeepSeek forced as the model: requested `3`, started `3`, succeeded `3`, failed `0`, parallel `2`, collapse approved.
- DeepSeek delivery retained: `deliveries/obj-1777349965`.

DeepSeek emergency fallback note:

- DeepSeek is not wired into 9Router and should not be added to the free/provider pool.
- `qsm run` now supports `-deepseek-fallback`, `-deepseek-key-file`, and `-deepseek-model`.
- Fallback only activates when `-route-health` is enabled and the primary 9Router probe finds zero healthy routes.
- The fallback reads the key file at runtime, switches the LangChain harness directly to `https://api.deepseek.com`, and sets the active model to `deepseek-chat`.
- `.state/route_health.json` records `primary_url`, `effective_url`, `fallback_used`, and `fallback_model`, but stores no key material.
- Forced-primary-failure smoke passed with `QSM_9ROUTER_URL=http://127.0.0.1:9/v1`: fallback engaged, requested `1`, started `1`, succeeded `1`, failed `0`, collapse approved.
- Latest fallback delivery retained: `deliveries/obj-1777428879`.

AoE-inspired room status note:

- Added per-room status files at `.rooms/pos-N/.qsm_status/status.json`.
- Status files record objective, position, agent, model, harness, state, phase, attempt, cache refresh count, product/evidence readiness, pass flags, score, error, and timestamps.
- `qsm status` now includes a `Room health` section and `qsm status -json` includes `room_statuses`.
- Executor writes queued/running/final status and cache refresh state.
- OpenCode and LangChain harnesses write launch/run/verify/final status.
- LangChain runner can update the status file through `QSM_STATUS_PATH`; live-cache refresh events increment the status cache counter.
- OpenCode now has a supervisor refresh loop for `-shared-cache` that rewrites `.qsm_memory/CACHE.md` during execution and appends `.qsm_status/events.jsonl`.
- Verification passed: `go test ./...`, `go build -o qsm ./cmd/qsm`, Python compile for `harness/langchain_runner.py`, simulated status smoke, and LangChain fallback status smoke.
- Latest LangChain fallback status delivery retained: `deliveries/obj-1777430909`.

Build-health deployment note:

- Added `.state/build_health.json`, a persistent model-level ledger built from room status and run results.
- `qsm run` updates build-health after every execution and deduplicates observations by objective/position.
- `qsm status` now prints a `Build health` section and includes build health in JSON output.
- `qsm route-health` now annotates routes with known build success rate and blocks auto-selection only after repeated build failures with zero successes.
- Latest build-health smoke passed with `1 requested / 1 started / 1 succeeded / 0 failed`, delivery `deliveries/obj-1777432860`.
- Deploy passed: `./qsm deploy -root . -harness langchain -build=true -start-router=true -wait 45s` rebuilt `qsm`, started 9Router on `127.0.0.1:20128`, and reported real harness ready.
- `./qsm doctor -root . -harness langchain` passed including `9router_live: 200 OK`.
- Current free-pool route-health after deploy: `2/5` healthy (`wombo`, `oc/nemotron-3-super-free`). `oc/ling-2.6-flash-free` has historical build-health `1/1`, but the provider now returns `400` because the free model is no longer available.

9Router-first policy smoke:

- First policy wave ran while 9Router was unreachable; DeepSeek fallback engaged correctly and delivered `deliveries/obj-1777434339`.
- After redeploying 9Router, `./qsm doctor -root . -harness langchain` passed and free-pool route-health found `wombo` and `oc/nemotron-3-super-free` healthy.
- A second `qsm run -route-health -route-health-models free -deepseek-fallback -shared-cache` selected only 9Router routes; DeepSeek did not engage.
- Result: requested `2`, started `2`, succeeded `2`, failed `0`, collapse approved.
- Build-health now records `wombo: 1/1`, `oc/nemotron-3-super-free: 1/1`, `oc/ling-2.6-flash-free: 1/1` historical but currently unavailable, and `deepseek-chat: 1/2`.
- Latest 9Router-first delivery retained: `deliveries/obj-1777434452`.
