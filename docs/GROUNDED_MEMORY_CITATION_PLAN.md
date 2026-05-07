# QSM V3 Grounded Memory and Citation Plan

Date: 2026-05-02

This plan is the result of a Grok Council debate plus independent web/source verification. It is intentionally scoped to the current Quantum Swarm V3 repo rather than a generic RAG architecture.

## Goal

Add a NotebookLM-style grounded citation layer to Quantum Swarm V3 so agent products and evidence can point back to exact source text from `.lake`, room memory, and shared cache.

The purpose is not to turn QSM into NotebookLM. The purpose is to make QSM's build loop more auditable:

```text
request -> lake/context -> room harness -> product/evidence -> citations -> collapse -> delivery
```

Human review should be able to check:

- Which source text supported an agent claim.
- Whether the source quote actually matches the generated product/evidence.
- Whether the branch is grounded or merely plausible.

## Verified External Facts

These are the facts accepted for this plan.

1. NotebookLM cites source material directly.

   Google states that NotebookLM uses direct quotes, text, and images from uploaded sources as citations; hovering/selecting a citation reveals the quote and opens it in source context.

   Source: [Use chat in NotebookLM](https://support.google.com/notebooklm/answer/16179559?hl=en)

2. NotebookLM answers from uploaded sources and may fail when the query is unclear or the answer is not in the sources.

   Google states that unclear phrases can prevent retrieval of relevant information, and that if an answer is not in the source material, NotebookLM will not provide a response.

   Source: [NotebookLM FAQ](https://support.google.com/notebooklm/answer/16269187?hl=en)

3. NotebookLM sources are static copies with explicit limits.

   Google documents supported source types and says each source can contain up to 500,000 words or 200 MB, with up to 50 sources.

   Source: [Add or discover new sources](https://support.google.com/notebooklm/answer/16215270?hl=en)

4. Hybrid retrieval with RRF is real, but not required for QSM Phase 1.

   Elastic documents RRF as a way to combine independent retrievers such as kNN/vector search and standard keyword retrieval into a combined ranking.

   Source: [Elasticsearch Reciprocal Rank Fusion](https://www.elastic.co/docs/reference/elasticsearch/rest-apis/reciprocal-rank-fusion/)

5. Agentic RAG and query rewriting are useful but have control tradeoffs.

   LangChain documents retrieval tools that can return source documents/metadata, contextual search query generation, relevance grading, and query rewriting. It also notes agentic retrieval can skip needed search or issue unnecessary searches.

   Source: [LangChain RAG docs](https://docs.langchain.com/oss/python/langchain/rag)

6. Persistent agent loops can reduce overhead, but this is not the next QSM slice.

   OpenAI describes Codex-style agent loops as many tool/API round trips and shows WebSocket mode can model a rollout as a long-running Response for latency improvement.

   Source: [OpenAI WebSocket agent workflow article](https://openai.com/index/speeding-up-agentic-workflows-with-websockets/)

## What Grok Got Wrong or We Rejected

During debate, Grok initially made unsupported claims about NotebookLM paid-tier limits and a specific Gemini source-blindness bug. Those claims are rejected because no public URL was provided.

Grok also named a non-existent local file, `internal/livecache/middleware.go`. In this repo, the live cache logic lives primarily in:

- `harness/langchain_runner.py`
- `internal/lake/cache.go`
- `internal/swarm/harness.go`
- `internal/swarm/types.go`
- `internal/wiki/wiki.go`

## Current QSM Insertion Points

QSM already has the right skeleton:

| Existing part | Current role | Grounding extension |
|---|---|---|
| `.lake/artifacts/*.json` | synthesis, research, positions, collapse artifacts | source corpus for deterministic quote mapping |
| `.lake/cache/*.json` | verified live cache facts | citation-bearing cache items |
| `.rooms/pos-N/.qsm_memory/CACHE.md` | room-local shared cache view | expose grounded quotes to active agents |
| `harness/langchain_runner.py` | DeepAgents loop, product/evidence normalization | add grounding prompt and evidence citations |
| `internal/swarm/types.go` | `BranchResult` and `RunReport` | add citations to branch evidence/result |
| `internal/wiki/wiki.go` | compiled memory | add Grounded Citations section |
| `internal/collapse/collapse.go` | deterministic scoring | later: optional citation-quality scoring |

## Phase 1: Deterministic Citation Layer

Scope: add exact quote citations without vector search, query rewriting, reranking, or new heavy dependencies.

### Data Types

Add a small package:

```text
internal/grounding/citation.go
```

Proposed structs:

```go
type Citation struct {
    ID         string  `json:"id"`
    Source     string  `json:"source"`
    SourceType string  `json:"source_type,omitempty"` // lake_artifact, cache_item, room_memory
    SentenceID int     `json:"sentence_id,omitempty"`
    Quote      string  `json:"quote"`
    Score      float64 `json:"score"`
}

type CitationReport struct {
    Citations []Citation `json:"citations,omitempty"`
    Coverage  float64    `json:"coverage,omitempty"`
    Missing   []string   `json:"missing,omitempty"`
}
```

Add `Citations []grounding.Citation` or equivalent JSON-compatible field to `swarm.BranchResult`.

### Citation Mapper

Implement deterministic mapping first:

1. Load candidate source text from:
   - `.lake/artifacts/*.json`: `claim`, `content`
   - `.lake/cache/*.json`: `content`
   - room memory: `.rooms/pos-N/.qsm_memory/CACHE.md`

2. Split source text into small passages:
   - sentence-like split for prose
   - paragraph fallback for Markdown/code blocks
   - retain `source`, `sentence_id`, and exact quote

3. Extract claim candidates from generated evidence/product notes:
   - `evidence.metadata.final_message`
   - `product/README.md` if present
   - `evidence.warning` and other agent-written summary fields

4. Score overlap deterministically:
   - normalized lowercase tokens
   - stopword-light filtering
   - exact phrase bonus
   - score threshold, for example `>= 0.35`

5. Return top citations with exact source quotes.

No LLM should be trusted to invent citations. The mapper should only cite text that exists locally.

### LangChain Runner Changes

Update `harness/langchain_runner.py`:

- Add grounding instructions to `system_prompt()` and `live_cache_prompt()`:
  - claims should cite source snippets when available
  - if no source supports a claim, say no grounded evidence
  - product-building must still continue; lack of citation is a warning, not a hard stop

- After `normalize_evidence()` and `fallback_verified_evidence()`, attach citations by calling a deterministic local mapper or by writing enough fields for the Go harness to map after execution.

Preferred Phase 1 design:

```text
Python runner writes normal evidence -> Go harness reads evidence -> Go citation mapper enriches BranchResult/evidence.json
```

This keeps grounding deterministic and testable in Go.

### Go Harness Changes

Update `internal/swarm/harness.go`:

- After `readAgentEvidence()` and product verification, call the citation mapper.
- Merge citations into `BranchResult`.
- Rewrite `evidence.json` with the enriched result.

Update `internal/swarm/types.go`:

- Add `Citations` to `BranchResult`.

Update `internal/lake/cache.go`:

- Optional Phase 1.1: add `Citations []Citation` to `CacheItem`.
- If importing `internal/grounding` would cause package layering issues, keep cache citations in `Metadata` for Phase 1 and type it later.

Update `internal/wiki/wiki.go`:

- Append `## Grounded Citations` section.
- Include citations already present in lake artifacts/cache/evidence.

### Collapse Behavior

Do not make citations required for approval in Phase 1.

For now:

- build/test/lint/product gates remain authoritative
- missing citations are warnings
- citation coverage is printed in status/report

Only after stable data:

- add citation quality as a small scoring factor, maximum 10-20 percent
- never let citation score override failed build/test/lint gates

## Acceptance Criteria

1. `go test ./...` passes.

2. Python runner compiles:

```bash
.venv/bin/python -m py_compile harness/langchain_runner.py
```

3. A LangChain fallback smoke produces evidence with a `citations` field:

```bash
QSM_LANGCHAIN_FALLBACK=1 ./qsm run \
  -root . \
  -harness langchain \
  -request "Grounded smoke: produce a README that cites local QSM wiki context." \
  -positions 1 \
  -parallel 1 \
  -shared-cache
```

4. A real harness smoke remains delivery-capable:

```bash
./scripts/ensure_9router.sh
./qsm run \
  -root . \
  -harness langchain \
  -request "Grounded go-live smoke: produce a short readiness note and cite QSM source context if available." \
  -positions 1 \
  -parallel 1 \
  -route-health \
  -route-health-models free \
  -deepseek-fallback \
  -shared-cache \
  -timeout 6m
```

5. `qsm status -json` includes citation data for each branch result.

6. `internal/wiki/wiki.md` contains a `Grounded Citations` section after `qsm wiki -root .`.

## Rejected or Deferred

### Deferred: Query Rewriting

Reason: useful, but it can drift the original intent. Add only after deterministic citations are stable.

Future scope:

- generate 3-5 query variants during Phase A
- store variants as `.lake` artifacts
- use variants to hydrate more evidence, not to replace the original objective

### Deferred: Hybrid Retrieval and RRF

Reason: real and useful at scale, but unnecessary for the current small per-objective lake.

Trigger:

- `.lake/artifacts` exceeds about 100-200 relevant source chunks
- Tolaria/Obsidian vault ingestion becomes active
- deterministic overlap misses obvious citations

Future scope:

- BM25/indexed sparse retrieval first
- vector search second
- RRF to merge rankings
- reranker only at collapse/audit time

### Deferred: WebSocket/Persistent Session Runtime

Reason: valuable for latency, but QSM's immediate quality bottleneck is grounded evidence, not transport overhead.

Future scope:

- persistent route/session cache for agent loops
- avoid resending full room memory
- only after harness reliability and citations are green

### Rejected: Citation-As-Hard-Gate in Phase 1

Reason: would block valid build tasks that do not require source claims, such as simple games or generated utilities.

Use citation as audit signal first, gate later only for research/legal/document-answering objectives.

## Phase 2: Query Decomposition and Grounded Research

Add after Phase 1 passes for several runs.

1. Add `grounding.QueryPlan`:
   - original request
   - extracted entities
   - required evidence types
   - optional query variants

2. Store query plans in `.lake/artifacts` as `grounding_query_plan`.

3. Hydrator uses query plan to pull more targeted local evidence.

4. Runner sees query plan in room memory but cannot mutate it.

## Phase 3: Tolaria/Obsidian Notebook Mode

This is the real NotebookLM-like mode.

1. Ingest Markdown vault notes as static source snapshots.

2. Build local index:
   - document path
   - headings
   - paragraph offsets
   - exact quote spans

3. Add sparse search first.

4. Add vector/RRF only when needed.

5. Return answers with exact quote citations and source paths.

## Final Decision

Implement Phase 1 next.

The smallest useful plan is:

```text
grounding.Citation type
        -> deterministic citation mapper
        -> enrich BranchResult/evidence.json
        -> add grounding prompt to LangChain runner
        -> render citations in wiki/status
```

This gives QSM the NotebookLM trust mechanic without overbuilding a RAG platform too early.
