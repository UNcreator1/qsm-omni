# DeepAgents Node Loop Mapping

This note maps the current DeepAgents/LangChain architecture into a Quantum Swarm node.

Source inspected: `/Users/nexus/Downloads/NemoClaw/deepagents_study` at commit `95f845d2`.

## DeepAgents Shape

DeepAgents does not expose a single hand-written `while plan/code/test` loop. The loop is a LangGraph agent produced by `create_deep_agent`, with behavior assembled through middleware:

- `TodoListMiddleware`: explicit planning and progress state.
- `FilesystemMiddleware`: `ls`, `read_file`, `write_file`, `edit_file`, `glob`, `grep`.
- `SubAgentMiddleware`: `task` tool for isolated short-lived subagents.
- `create_summarization_middleware`: context management for long runs.
- `PatchToolCallsMiddleware`: tool-call compatibility cleanup.
- provider profile middleware: model/provider-specific fixes.
- `MemoryMiddleware`: load persistent memory files into the prompt.
- `_PermissionMiddleware`: final filesystem/tool permission gate.

The CLI layer builds on this with:

- `LocalShellBackend`: filesystem plus `execute` shell tool.
- `CompositeBackend`: routes large tool results and conversation history away from the main workspace.
- `AskUserMiddleware`: disabled for QSM non-interactive rooms.
- HITL/approval config: should be disabled or replaced by QSM room policy for autonomous nodes.

## QSM Node Contract

Each QSM position should become one DeepAgent graph invocation:

1. Create `.rooms/pos-N`.
2. Instantiate a room-scoped backend with `root_dir=.rooms/pos-N`.
3. Load Wiki memory from `internal/wiki/wiki.md`.
4. Load the position plan from `.rooms/pos-N/PLAN.md`.
5. Invoke the graph with a single objective prompt.
6. Require the graph to write:
   - `./product/`
   - `./evidence.json`
7. Let the Go executor collect `BranchResult`.
8. Collapse only after every node is accounted for.

## Recommended Backend

Use `LocalShellBackend(root_dir=room, virtual_mode=True, inherit_env=True, env=...)` for local development.

Reasoning:

- `root_dir=room` gives each node its own working directory.
- `virtual_mode=True` blocks path traversal for filesystem tools.
- Shell execution is still not a true sandbox, so QSM must keep room prompts strict and eventually move real production execution into Docker/VM/sandbox backends.

## Recommended DeepAgent Settings

For a QSM node:

- `interactive=False`
- `auto_approve=True`
- `enable_ask_user=False`
- `enable_memory=True`
- `enable_skills=False` initially
- `enable_shell=True`
- `cwd=room`
- `assistant_id=qsm-{objective_id}-{position_id}-{agent_id}`

The system prompt should be QSM-specific:

- work only inside current room and `./product`
- use todos for `Plan -> Code -> Test -> Verify`
- do not ask the user questions
- do not spawn external/background subagents unless QSM grants that capability
- write machine-readable evidence before final response

## Model Routing

DeepAgents model strings use `provider:model` syntax through LangChain `init_chat_model`.

For QSM with 9Router, the clean target is an OpenAI-compatible chat model wrapper pointed at:

- `base_url = QSM_9ROUTER_URL`
- `api_key = QSM_9ROUTER_API_KEY`
- `model = provider/model` from the QSM agent spec

If using OpenRouter-native LangChain packages instead, install and configure `langchain-openrouter`; otherwise prefer a small local OpenAI-compatible adapter so QSM can keep using 9Router as the socket layer.

## Node Evidence

DeepAgents does not automatically produce QSM `BranchResult`. The runner must validate and normalize evidence after graph completion:

```json
{
  "position_id": "pos-01",
  "room": ".rooms/pos-01",
  "build_passed": true,
  "test_passed": true,
  "lint_passed": true,
  "score": 0.0,
  "product_path": "./product"
}
```

If an agent reports `./product/README.md`, QSM normalizes it to `./product`.

## Implementation Plan

1. Add a Python dependency environment for DeepAgents/LangChain.
2. Replace `harness/langchain_runner.py` fallback with:
   - import `create_cli_agent` or direct `create_deep_agent`
   - create room-scoped backend
   - invoke the graph
   - validate `product` and `evidence`
3. Keep deterministic fallback behind an explicit env flag, not as the default for `-harness langchain`.
4. Add `qsm doctor -harness langchain` checks for Python packages.
5. Add a 1-node `langchain` smoke test, then allow multi-node concurrency.
