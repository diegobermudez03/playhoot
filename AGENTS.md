# AI Agent Bootstrap

This file is the entry point for AI agents working on Playhoot.

Before performing non-trivial work:

1. Read `docs/ai/OPERATING_MODEL.md`.
2. Use `docs/ai/KNOWLEDGE_MAP.md` to identify and load only the context
   relevant to the task.
3. Determine what type of work is being performed.
4. If a process runbook exists for that type of work, use
   `docs/ai/processes/*` as the human-facing guide and follow the matching
   `docs/ai/protocols/*` execution protocol.
5. Inspect actual code, tests, and migrations whenever current implementation
   behavior matters.
6. Do not silently resolve contradictions between canonical documentation and implementation. Report them as drift.
7. Do not treat ideas, proposals, examples, TODOs, or future-looking
   documentation as accepted decisions.
8. Understand the product/business reason for material changes before implementing them.
9. Escalate material product, architecture, domain, persistence, consistency,
   security, infrastructure, or engineering-standard decisions instead of
   inventing them during implementation.
10. Do not scan every Markdown document by default. Load context through the
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
