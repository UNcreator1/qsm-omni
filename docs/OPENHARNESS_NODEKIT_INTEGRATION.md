# OpenHarness Node Kit Integration

Date: 2026-05-04

OpenHarness was cloned to:

```text
/Users/nexus/Downloads/NemoClaw/OpenHarness
```

Pinned inspected commit:

```text
7873f0d
```

## Decision

QSM should not vendor OpenHarness internals into the orchestrator. OpenHarness is a fast-moving personal-agent harness with channel integrations, auth, permissions, hooks, tools, and plugins. QSM is a deterministic multi-position product factory with planning, lake hydration, isolated rooms, auto-test, collapse, and force scoring.

The integration pattern is therefore:

```text
QSM orchestrator
  -> creates room / position / lake / cache / force requirements
  -> writes OpenHarness-inspired node kit into each room
  -> runs OpenCode or LangChain node
  -> QSM verifies, scores, collapses, and delivers
```

## Implemented

Each room now receives:

```text
.qsm_harness/
├── manifest.json
├── hooks.json
├── PERMISSIONS.md
├── MEMORY.md
└── skills/
    ├── force/SKILL.md
    ├── lake/SKILL.md
    ├── plan/SKILL.md
    ├── review/SKILL.md
    └── test/SKILL.md
```

This borrows OpenHarness concepts:

- Markdown skills.
- Lifecycle hook expectations.
- Path and command governance.
- Persistent memory entry points.
- Cost/token awareness remains handled by QSM `cost_report`.

## Why This Is Safer

QSM gets the useful node discipline without inheriting OpenHarness remote chat/channel security surface. Nodes can read the kit as local instructions, while QSM still owns:

- room creation,
- shared cache,
- QSM verifier,
- force checklist,
- deterministic collapse,
- delivery,
- benchmark reports.

## Next

1. Convert `.qsm_harness/hooks.json` from expectations into executed hooks.
2. Add command/path permission checks around OpenCode and LangChain shell calls.
3. Add a plugin loader for QSM-specific skills from `.qsm_plugins/`.
4. Add `qsm nodekit` command to inspect and validate room kits.
5. Use OpenHarness as an optional node harness backend only after its auth/channel/security settings are isolated from QSM production runs.
