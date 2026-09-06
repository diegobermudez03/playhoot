# Engineering Standard Protocol

Status: AGENT EXECUTION PROTOCOL

Use with the human-facing guide at `docs/ai/processes/ENGINEERING_STANDARD.md`. This protocol is primarily executed by a CONVERSATIONAL AI; persisting an accepted standard, or executing approved migration work, is a CODEBASE AGENT step.

## Scope

This protocol creates or changes reusable Playhoot engineering, design, coding, testing, documentation, or error-handling standards.

It must not use a standard change as implementation authority for repository-wide migration.

## HUMAN SURFACE

Show the human the problem, evidence/current patterns, whether standardization is worthwhile, alternative rules, recommended wording, enforcement options, and migration strategy.

For non-trivial or multi-session work, maintain `docs/ai/workspaces/active/<standard-topic>/HUMAN_REVIEW.md`.

## SYSTEM EFFECTS

- Load relevant standards through `docs/engineering/standards/INDEX.md` and the Knowledge Map.
- Inspect implementation examples when current practice matters.
- Distinguish accepted standards from recurring implementation patterns.
- Route global architecture rules to Architecture Discussion instead of duplicating them as standards.
- Persist accepted reusable standards under `docs/engineering/standards/`.
- Update the standards index when standards are added, removed, or renamed.
- Separate enforcement/migration work from the standard decision.
- For non-trivial or multi-session workspaces, maintain the paired
  `docs/ai/workspaces/active/<standard-topic>/AI_CONTEXT.md` alongside
  `HUMAN_REVIEW.md` and follow `docs/ai/workspaces/README.md`.
- Before asking for a material decision, ensure the workspace has enough
  persisted state for a fresh agent to resume. When the human answers, update
  the active process state and/or durable owner before future continuation
  depends on it, keeping the human-facing checkpoint current where practical.

No material engineering-standard decision may exist only in `AI_CONTEXT.md`.

## Execution

1. State the inconsistency or problem.
2. Inspect current standard and representative code when relevant.
3. Decide whether a reusable standard is actually needed.
4. Compare alternatives and enforcement options.
5. Recommend standard wording.
6. Obtain human decision.
7. Record enforcement and migration strategy: FUTURE CODE ONLY, OPPORTUNISTIC MIGRATION, or MANDATORY MIGRATION.
8. Route implementation/migration to Feature Development and WORK when needed.

## Completion

Complete when the need for a standard is accepted or rejected, accepted wording and enforcement are clear, and migration work is routed separately.
