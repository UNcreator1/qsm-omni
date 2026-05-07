# Quantum Swarm Team: Concept Specification (V3.0)

The framework transforms one high-level request into a tested, auditable delivered product through Recursive Superposition and Deterministic Collapse.

## Phase A: Strategic Reservoir

Stage 01 captures zero-shot hypotheses from multiple agents before external tools bias their reasoning.

Stage 02 hydrates those hypotheses with verified local, web, and document evidence. Verified knowledge is stored in `.lake/` and compiled into `internal/wiki/wiki.md`.

## Phase B: Execution Wave

Stage 03 chops the objective into divergent positions. Each position receives an isolated room under `.rooms/pos-N` and runs a Plan -> Code -> Test -> Verify harness loop.

## Phase C: High-Integrity Collapse

Stage 04 runs adversarial audit and scoring. Only branches with deterministic evidence can be approved for product delivery.

## Stack Boundaries

- 9Router is the model socket layer.
- OpenCode CLI is the actuation shell layer.
- Harness agents execute the autonomous loop.
- Wiki LLM is the persistent common truth layer.

## Hardware-Aware Node Count

The swarm does not assume a fixed number of nodes. Before Stage 03, it estimates how many concurrent nodes the hardware can sustain by budgeting:

- 9Router API sockets and response buffers
- OpenCode CLI process overhead per active node
- Harness agent loop memory and CPU weight
- Shared Wiki LLM memory plus per-node working context
- Reserved operating-system and safety memory

`qsm capacity` prints the current recommendation. `qsm run -positions auto` uses that recommendation as the default chop count.

## Runtime Truth Boundary

The framework has two execution classes:

- `simulated`: deterministic local harness for testing the orchestration pipeline.
- `opencode` / `langchain`: real agent harnesses that require 9Router credentials, an actuation runtime, and Wiki memory.

`qsm doctor -harness opencode` reports whether nodes can actually be active. If it is not green, the system cannot honestly claim real agents are running.
