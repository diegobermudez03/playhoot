# Domain Design Protocol

Status: AGENT EXECUTION PROTOCOL

Use with the human-facing guide at `docs/ai/processes/DOMAIN_DESIGN.md`. This protocol is primarily executed by a CONVERSATIONAL AI; repository persistence/synchronization it requires may be delegated through a CODEBASE AGENT HANDOFF.

## Scope

This protocol creates, removes, splits, merges, or materially redefines Playhoot business/domain boundaries.

Domain does not mean package, folder, deployment, or service.

## HUMAN SURFACE

Show the human the business responsibility, ubiquitous concepts, owned state, invariants, lifecycle, public capabilities, explicit non-ownership, alternatives, recommendation, and migration impact.

For non-trivial or multi-session work, maintain `docs/ai/workspaces/active/<domain-topic>/HUMAN_REVIEW.md`.

## SYSTEM EFFECTS

- Load product, architecture, domain, decision, and current-state context through the Knowledge Map.
- Inspect code, tests, migrations, and current-state documentation when actual ownership or data matters.
- Challenge artificial separation when almost every operation would require orchestration only to cross the boundary.
- Challenge oversized domains when responsibilities differ even if deployment is shared.
- Persist material accepted/rejected boundary rationale as an ADR when it meets `docs/decisions/README.md` thresholds.
- Synchronize accepted boundary decisions to canonical architecture/domain documentation.
- Use `docs/ai/templates/domain/` only after the boundary decision is accepted.
- Route implementation/migration to Feature Development and WORK.
- For non-trivial or multi-session workspaces, maintain the paired
  `docs/ai/workspaces/active/<domain-topic>/AI_CONTEXT.md` alongside
  `HUMAN_REVIEW.md` and follow `docs/ai/workspaces/README.md`.
- Before asking for a material decision, ensure the workspace has enough
  persisted state for a fresh agent to resume. When the human answers, update
  the active process state and/or durable owner before future continuation
  depends on it, keeping the human-facing checkpoint current where practical.

No material domain decision may exist only in `AI_CONTEXT.md`.

## Execution

1. State the boundary question.
2. Identify business responsibility, concepts, owned data, and invariants.
3. Compare lifecycles and public capabilities.
4. Define explicit non-ownership.
5. Inspect dependency pressure and cross-domain coordination consequences.
6. Explore alternatives, including no domain change.
7. Recommend and request a human decision.
8. Synchronize accepted documentation and route migration work.

## Completion

Complete when ownership is accepted, rejected, or deferred; migration impact is identified if current code differs; and next documentation or implementation work is routed.
