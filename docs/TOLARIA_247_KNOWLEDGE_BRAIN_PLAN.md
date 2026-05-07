# Tolaria 24/7 Knowledge Brain Build Plan

Status: planned after the 2026-04-28 live-cache milestone.

This document turns the Tolaria "24/7 knowledge brain" concept into a safe Quantum Swarm V3 build route.

## Concept Verdict

The concept matches Quantum Swarm V3:

```text
knowledge task
  -> local divergent positions
  -> isolated .rooms/pos-N proposals
  -> verified .lake material and shared cache
  -> deterministic collapse
  -> Tolaria vault staging
  -> schema validation
  -> Git commit and push
```

Each node is a local superposition position. Nodes do not directly edit or push the Tolaria vault. They produce proposals, evidence, and cache lessons. Collapse and verification decide what becomes vault truth.

Important boundary:

- "Local swarm" means execution is local on one Mac.
- Live crawling Reddit, HN, arXiv, or web sources still uses network.
- A no-network mode must use pre-downloaded exports, PDFs, browser archives, RSS dumps, or local MolmoWeb captures.

## Target Layout

```text
tolaria-swarm/
├── vault/                  # Tolaria Git repo
│   ├── raw/
│   ├── longchat/
│   ├── wiki/
│   ├── graph/
│   └── AGENT.md
├── qsm/                    # Quantum Swarm V3
│   ├── .lake/
│   ├── .rooms/pos-N/
│   ├── .state/
│   ├── deliveries/
│   └── internal/wiki/
├── config.yaml
├── orchestrator.sh
└── launchd/
```

## Position Model

The swarm should create divergent positions by task type, not by cloning the same prompt blindly:

| Position | Role | Output |
|---|---|---|
| `longchat-ingest` | Read cached/live longchat sources such as Reddit or HN | raw notes, source map, candidate entities |
| `paper-ingest` | Read papers or arXiv exports | claims, citations, methods, unresolved terms |
| `qwen-restructure` | Convert raw material into Tolaria frontmatter, Types, and Relationships | structured YAML and wiki patches |
| `gap-detection` | Find missing definitions, weak links, contradictions | gap report and `constraint` cache items |
| `type-healer` | Repair Tolaria Types and Relationship consistency | graph patch proposal |
| `contradiction-audit` | Adversarial review of proposed changes | audit findings and score signals |
| `synthesis` | Merge the strongest material into readable wiki updates | final proposal package |

Each room writes only inside its own room:

```text
.rooms/pos-N/proposal/
├── raw_notes.md
├── entities.yaml
├── relationships.yaml
├── wiki_patch.md
├── source_manifest.json
└── evidence.json
```

## Tolaria Data Contracts

### Entity Proposal

```yaml
id: stable-slug
type: Concept | Person | Project | Paper | Tool | Claim | Source
title: Human-readable title
aliases: []
summary: Short verified summary
sources:
  - source_id
confidence: 0.0
status: proposed | verified | rejected
```

### Relationship Proposal

```yaml
from: entity-id
to: entity-id
type: supports | contradicts | implements | cites | depends_on | refines | belongs_to
summary: Why this relationship exists
sources:
  - source_id
confidence: 0.0
```

### Source Manifest

```json
{
  "source_id": "reddit-20260428-example",
  "kind": "reddit_thread",
  "path_or_url": "raw/longchat/example.md",
  "captured_at": "2026-04-28T00:00:00Z",
  "network_mode": "live|offline-cache",
  "hash": "sha256..."
}
```

### Evidence

```json
{
  "position_id": "pos-01",
  "build_passed": true,
  "test_passed": true,
  "lint_passed": true,
  "schema_passed": true,
  "source_trace_passed": true,
  "score": 0.85,
  "product_path": "./proposal"
}
```

## Shared Cache Contract

Existing cache kinds remain valid:

- `constraint`
- `dependency_note`
- `failed_attempt`
- `rate_limit_signal`
- `score_signal`
- `verified_recipe`

Add these Tolaria-specific cache conventions:

| Kind | Example Producer | Purpose |
|---|---|---|
| `constraint` | orchestrator, schema verifier | Required vault schema, no direct pushes, no unsourced claims |
| `dependency_note` | ingest node | Source parser quirks, local crawler notes, model route behavior |
| `failed_attempt` | any node | Bad source path, invalid relationship type, failed model route |
| `score_signal` | auditor, verifier | Why a proposal improved or failed |
| `verified_recipe` | successful node | Working conversion recipe or validation command |

Do not cache full sibling products. Cache only verified facts, negative lessons, constraints, and small recipes.

## Build Order

### Advisory: Agent Of Empires Patterns

Grok and local review agree that `agent-of-empires` helps as a reference for session hygiene, not as a replacement for QSM.

Useful patterns to borrow:

- optional git worktree isolation for software-build rooms
- a harness or agent capability registry instead of scattered per-harness conditionals
- per-room status files, such as `.rooms/pos-N/.qsm_status/status.json`, for live node state and build-health
- repo-local config and small lifecycle hooks for setup/teardown and cache policy

Patterns to avoid for now:

- tmux as a core QSM runtime dependency
- TUI/web dashboard work before the collapse engine is stronger
- Docker sandboxing as a default path
- broad multi-CLI support beyond OpenCode and DeepAgents/LangChain

Impact on the plan:

- route-health should evolve into route-health plus build-health by consuming per-room status and run outcomes
- OpenCode live-cache consumption should use a status-file/supervisor protocol rather than terminal parsing
- cache pruning should use lifecycle status and hooks after the basic cache quality gates are implemented

Council attachment:

`/Users/nexus/Downloads/NemoClaw/Council/attachments/qsm-agent-of-empires-advisory-20260429.md`

### Phase 1: Route-Health Selection

Goal: avoid assigning real nodes to model routes that are listed but unusable.

Implementation status: first slice implemented.

Current observed behavior:

- `oc/ling-2.6-flash-free` produced normal chat completion and passed real 2-node validation.
- `or/*` routes may list models but fail with missing credentials.
- some routes spend short completions on reasoning-only output.
- some routes return SSE-style chunks where a non-stream JSON route is expected.

Implementation:

- Add `qsm route-health` or `qsm doctor --route-health`.
- Probe candidate models through 9Router with a tiny chat request.
- Record latency, response shape, content presence, and error class.
- Save results to `.state/route_health.json`.
- Write verified `route_health` cache items into `.lake/cache`.
- Filter default real-harness agents to healthy routes, unless the user explicitly overrides a model.

Implemented:

- `qsm route-health`
- `.state/route_health.json`
- `route_health` cache items
- `qsm run -route-health` optional gate
- LangChain live-cache visibility for `route_health`
- `route-health -models free` discovery from 9Router `/v1/models`
- `qsm run -route-health -route-health-models free` replacement agents from healthy discovered routes

Route-health record:

```json
{
  "model": "oc/ling-2.6-flash-free",
  "ok": true,
  "latency_ms": 570,
  "response_shape": "openai_chat_json",
  "content_ok": true,
  "tool_like_ok": false,
  "error": ""
}
```

Acceptance:

- `./qsm route-health -root . -harness langchain` writes `.state/route_health.json`. Done.
- `.lake/cache` contains `route_health` items. Done.
- Unhealthy routes are excluded from auto agent selection when `qsm run -route-health` is used. Done.
- A real 2-node LangChain run still succeeds with the selected healthy route. Done.

Latest live route-health result:

| Model | Result | Reason |
|---|---|---|
| `oc/ling-2.6-flash-free` | passed | healthy OpenAI-style JSON chat response |
| `oc/minimax-m2.5-free` | failed | returned reasoning with empty chat content |
| `or/inclusionai/ling-2.6-1t:free` | failed | no active `or` provider credentials |

Latest route-health gated run:

- objective: `obj-1777347848`
- route health: `1/3` healthy
- selected agents: `1` (`alpha`, `oc/ling-2.6-flash-free`)
- nodes: `2 requested / 2 started / 2 succeeded / 0 failed`
- cache summary: `constraint:1 route_health:3 verified_recipe:6`
- delivery: `deliveries/obj-1777347848`

Latest free-pool replacement run:

- route-health pool: `free`
- route health: `2/5` healthy at launch (`wombo`, `oc/nemotron-3-super-free`)
- selected agents: `2` synthesized replacement agents
- nodes: `3 requested / 3 started / 1 succeeded / 2 failed`
- approved winner: `pos-02` using `oc/nemotron-3-super-free`
- delivery: `deliveries/obj-1777349311`
- lesson: chat-health alone is not enough. `wombo` can pass a tiny chat probe but still fail the agent-build loop. Next route-health refinement should add a build-health/reliability score from actual node outcomes.

### Phase 2: Tolaria Vault Connector

Goal: stage collapsed output into Tolaria safely.

Implementation:

- Add config for `QSM_TOLARIA_VAULT` or `config.yaml`.
- Add a Tolaria delivery mode that writes to a staging area first:

```text
.state/tolaria_stage/
├── raw/
├── wiki/
├── graph/
└── manifest.json
```

- Validate stage before touching `vault/`.
- Copy staged files into `vault/` only after validation passes.
- Commit only if `git diff --quiet` reports meaningful changes.

Safety gates:

- no overlapping runs: `.state/orchestrator.lock`
- vault must be a Git repo
- no dirty vault unless `QSM_TOLARIA_ALLOW_DIRTY=1`
- schema validation passes
- relationship types are known
- entity IDs are stable and duplicate-free
- all claims have source references
- no generated room paths leak into vault content
- `git diff --check` passes

Acceptance:

- A simulated run produces a staged Tolaria patch but does not touch vault on failure.
- A passing run commits one safe update to a test vault.
- Re-running with no changes does not create an empty commit.

### Phase 3: OpenCode Live Cache Consumption

Goal: make OpenCode nodes consume live verified lessons like LangChain nodes do.

Implementation status: first slice implemented.

Implementation options:

1. Supervisor refresh:
   - Start OpenCode as usual.
   - In the Go harness, poll `.lake/cache` every few seconds while OpenCode runs.
   - Rewrite `.rooms/pos-N/.qsm_memory/CACHE.md` atomically.

2. Re-prompt loop:
   - Run OpenCode in smaller bounded steps.
   - After each step, refresh cache and continue with updated context.

Preferred first slice: supervisor refresh, because it is smaller and uses existing `Lake.WriteCacheMarkdown`.

Implemented:

- per-room status protocol at `.rooms/pos-N/.qsm_status/status.json`
- room event log at `.rooms/pos-N/.qsm_status/events.jsonl`
- executor status writes for queued/running/final states
- LangChain status updates through `QSM_STATUS_PATH`
- OpenCode supervisor refresh loop when `-shared-cache` is enabled
- `qsm status` room-health summary

Acceptance:

- OpenCode run with `-shared-cache` updates `.qsm_memory/CACHE.md` during execution. Unit covered with an OpenCode-mode slow harness.
- Cache refresh events are written to a room log. Done.
- No sibling product files appear in the cache.

### Phase 4: Cache Pruning And Quality Gates

Goal: keep `.lake/cache` useful during 24/7 operation.

Implementation:

- Add content hash dedupe.
- Reject tiny low-value content except explicit status signals.
- Keep highest-confidence duplicate by kind/objective/content hash.
- Add max items per objective and per kind.
- Add optional max age.

Suggested defaults:

- keep last 100 items per objective
- keep last 30 items per kind
- keep 7 days by default
- never prune current objective while running

Acceptance:

- Repeated failed attempts do not create exact duplicate cache spam.
- `qsm status` shows cache summary plus pruned count.
- Pruning never deletes current-run objective constraint before collapse.

### Phase 5: Tolaria 24/7 Orchestrator

Goal: run safe cycles on a schedule.

`orchestrator.sh` should:

```bash
#!/bin/bash
set -euo pipefail

ROOT="$HOME/tolaria-swarm/qsm"
VAULT="$HOME/tolaria-swarm/vault"
LOCK="$ROOT/.state/orchestrator.lock"

cd "$ROOT"
mkdir -p .state

if ! mkdir "$LOCK" 2>/dev/null; then
  echo "QSM already running; exiting"
  exit 0
fi
trap 'rmdir "$LOCK"' EXIT

./qsm doctor -root . -harness opencode
./qsm route-health -root . -harness opencode

./qsm run \
  -root . \
  -request "Auto-feed verified knowledge into Tolaria vault" \
  -positions auto \
  -parallel auto \
  -harness opencode \
  -shared-cache

./qsm tolaria-stage -root . -vault "$VAULT"
./qsm tolaria-validate -root . -vault "$VAULT"
./qsm tolaria-commit -root . -vault "$VAULT"
```

Launchd should start with a conservative interval, such as 30 minutes. The job should skip if the lock exists.

Acceptance:

- two scheduled cycles cannot overlap
- failed validation creates no Git commit
- successful validation creates exactly one commit with manifest
- push is optional and disabled until local commit flow is proven

## 24/7 Safety Policy

The system must prefer no update over a bad update.

Rules:

- nodes propose, collapse commits
- no room writes directly to vault
- no unsourced claims
- no unknown relationship types
- no push when vault is dirty
- no launchd overlap
- no network crawling unless network mode is explicitly enabled
- all vault changes must be reproducible from `.state/run_report.json`, `.state/verdict.json`, and `.state/tolaria_stage/manifest.json`

## Grok Review Summary

Grok agreed with this order:

1. route-health selection first
2. OpenCode live-cache consumption second
3. cache pruning and quality gates third

For the Tolaria brain route, this plan inserts the Tolaria vault connector before 24/7 automation. That is the main safety correction: automation should run only after route-health and vault validation exist.

Full Grok response is stored at:

`/Users/nexus/Downloads/NemoClaw/Council/grok_latest.txt`

## Immediate Next Task

Build route build-health from room status, then cache pruning and quality gates.

Do not start with launchd. Grok reviewed the status-protocol slice and recommended using the new `.qsm_status/status.json` contract before pruning:

- consume room status in route/build-health scoring
- track real node outcomes by model, not only chat-health probes
- surface build-health in `qsm status`
- use build-health to decide which cache lessons are high-quality
- then add content hash dedupe, max items per objective/kind, and prune reports
