# AI Agent Bootstrap

This file is the entry point for AI agents working on Playhoot.

Before performing non-trivial work:

1. Determine your execution surface: CONVERSATIONAL AI, CODEBASE AGENT, or
   INDEPENDENT REVIEWER. If you have effective access to this repository
   checkout and can inspect/modify files, run commands, and persist workflow
   artifacts, you are acting as a Codebase Agent (or, when reviewing
   read-only, an Independent Reviewer). Role definitions and authority live in
   `docs/ai/OPERATING_MODEL.md` — do not redefine them here.
2. Read `docs/ai/OPERATING_MODEL.md`.
3. Use `docs/ai/KNOWLEDGE_MAP.md` to identify and load only the context
   relevant to the task.
4. For a normal natural-language request, use
   `docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md` before forcing the
   human to choose a process: search for a matching open initiative or WORK
   first, then select the internal process that owns the concern. Skip
   straight to the matching process/protocol only when clearly resuming a
   known persisted process.
5. If a process runbook exists for that type of work, use
   `docs/ai/processes/*` as the human-facing guide and follow the matching
   `docs/ai/protocols/*` execution protocol. Human process guides remain
   human-facing; internal execution mechanics live in the protocols.
6. Inspect actual code, tests, and migrations whenever current implementation
   behavior matters.
7. Do not silently resolve contradictions between canonical documentation and implementation. Report them as drift.
8. Do not treat ideas, proposals, examples, TODOs, or future-looking
   documentation as accepted decisions.
9. Understand the product/business reason for material changes before implementing them.
10. Escalate material product, architecture, domain, persistence, consistency,
    security, infrastructure, or engineering-standard decisions instead of
    inventing them during implementation.
11. Do not scan every Markdown document by default. Load context through the
    Knowledge Map.

## Engineering Standards

Canonical reusable engineering standards live under:

`docs/engineering/standards/`

Use `docs/engineering/standards/INDEX.md` and the Knowledge Map to load only
the standards relevant to the task.

Do not infer new canonical standards from recurring implementation patterns.
Use the Engineering Standard process for new or changed rules.

## Human Review And Temporary Context

For non-trivial AI process checkpoints, `HUMAN_REVIEW.md` is the normal human
review surface.

`AI_CONTEXT.md` is temporary resumable agent context only. It is not canonical
truth, not implementation authority, and must not contain hidden material
decisions.

When acting as CODEBASE AGENT or INDEPENDENT REVIEWER, respect an existing
active workspace under `docs/ai/workspaces/active/`: do not overwrite its
design authority, and keep its `AI_CONTEXT.md` resume state current at
meaningful repository-side checkpoints so an interrupted execution can be
resumed. See `docs/ai/workspaces/README.md`.
