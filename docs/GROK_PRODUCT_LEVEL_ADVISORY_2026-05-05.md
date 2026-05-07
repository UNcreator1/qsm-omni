# Grok Product-Level Advisory - 2026-05-05

## Context

Grok was consulted through the existing Council bridge on port `8338`. The bridge was already online and was not restarted or modified.

Attachment sent:

`/Users/nexus/Downloads/NemoClaw/Council/attachments/qsm-product-level-current-20260505.md`

## Verified Local Facts Before Advisory

- 9Router was activated successfully at `http://127.0.0.1:20128/v1`.
- `/v1/models` was live and returned multiple routes.
- Initial `qsm route-health -models auto` failed `0/3` because it only tested stale default agent routes.
- Broader `qsm route-health -models free -limit 16` found healthy routes:
  - `wombo`
  - `oc/nemotron-3-super-free`
- Broader `qsm route-health -models all -limit 16` found healthy routes:
  - `wombo`
  - `oc/nemotron-3-super-free`
  - `kr/claude-sonnet-4.5`
- A live LangChain/9Router smoke completed with `2/2` nodes succeeded and delivered `deliveries/obj-1777948276`.

## Grok's Main Judgment

Grok agreed that the real immediate blockers are:

1. Route-health defaults were stale and too narrow.
2. Lake citation coverage was too weak for a self-improving memory system.
3. Product-level trust still needs reliability, hard sandbox enforcement, recovery metrics, and cost accounting.

Grok also argued that some top-tier checklist items are over-scoped for local-first alpha:

- 100+ concurrent sessions
- full SLA/failover
- full dashboard/exporter stack
- official SWE-bench/Terminal-Bench/ROI evidence

Current state should be called local alpha/internal-dev ready, not enterprise product ready.

## Grok's Ranked Next Steps

1. Dynamic route-health and fallback overhaul.
2. Enforce lake citation and memory consumption in node outputs.
3. Harden response-shape parsing for reasoning-only or nonstandard model responses.
4. Enforce hard sandbox execution and measure recovery.
5. Add lightweight observability and benchmark adapters.

## Deployed Immediately From Advisory

Implemented two slices of step 1:

- `qsm route-health -models auto` now discovers current free/combo routes from `/v1/models` when using the LangChain harness and no explicit `QSM_LANGCHAIN_MODEL` override is set.
- The old default-agent route list is now only a fallback if discovery fails.
- Added a regression test so `auto` cannot silently regress into probing only stale hardcoded routes.
- Healthy discovered routes are now reliability-sorted using route-health plus build-health, so a route with weaker recent build history loses priority to a more stable route.

Validation:

```bash
go test ./...
go build -o qsm ./cmd/qsm
./qsm route-health -root . -harness langchain -models auto -limit 16 -timeout 30s
```

Latest `auto` route-health after patch:

```text
Route health: 1/5 healthy
[OK] oc/nemotron-3-super-free latency=6716ms shape=openai_chat_json build=2/2 100%
```

Follow-up verification:

- A live run with `-route-health-models=free` succeeded `2/2` through 9Router and delivered `deliveries/obj-1777948276`.
- A later `auto` run initially selected `wombo` and failed after reaching the QSM node step budget.
- After reliability sorting, a new `auto` run selected `oc/nemotron-3-super-free` first, proving selection priority was fixed.
- That final run then stalled in `cache_refresh` without progress for several minutes and was terminated manually. This exposes the next blocker: the harness supervisor needs a no-progress timeout/retry loop for silent nodes.

`wombo` was healthy in some scans and in the successful 2-node live run, but flaky in later runs due to empty-content or step-budget behavior. That supports Grok's recommendation to add response-shape hardening and circuit-breaker/blacklist behavior next.

## Product-Level Go/No-Go

Go:

- Local alpha/internal-dev use with live 9Router routes.
- Real LangChain node execution after route-health selects healthy discovered models.
- Data lake maintenance and strict promotion gates.

No-go:

- Enterprise/product promise.
- 24/7 unattended reliability claim.
- Hard security isolation claim.
- Self-improving memory claim until nodes cite lake/wiki item IDs consistently.

## Next Implementation Plan

1. Add route circuit breaker:
   - Track repeated HTTP/model-shape failures.
   - Skip known-bad routes for a TTL.
   - Prefer healthy build-health plus fresh route-health.

2. Add response-shape hardening:
   - Capture raw response shape samples safely.
   - Support providers that return content in alternate fields.
   - Keep reasoning-only responses failed unless a usable answer can be extracted.

3. Add harness no-progress timeout:
   - Detect rooms stuck in the same phase without status updates.
   - Kill and retry the node with the next healthy route.
   - Record timeout evidence in build-health and lake cache.

4. Enforce node lake citations:
   - Require evidence metadata to list consumed cache/wiki IDs.
   - Penalize or reject evidence with zero citation coverage.
   - Promote only recipes with both reusable content and provenance.

5. Run live long-harness stress:
   - 3, 5, then 7 positions.
   - 9Router-first, DeepSeek fallback only when no healthy router routes exist.
   - Measure success/fail/retry/cost/citation coverage.

6. Add minimal product observability:
   - Route latency and health counts.
   - Node success/failure/retry counts.
   - Lake citation coverage.
   - Cost/token per objective.
