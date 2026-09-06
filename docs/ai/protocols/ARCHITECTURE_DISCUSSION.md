# Architecture Discussion Protocol

Status: AGENT EXECUTION PROTOCOL

Use with the human-facing guide at `docs/ai/processes/ARCHITECTURE_DISCUSSION.md`.

## Scope

This protocol decides system-level technical structure, ownership, coupling, persistence, infrastructure, public contracts, and similar architecture concerns.

It must not implement code or treat an accepted architecture decision as implemented current state.

## HUMAN SURFACE

Show the human the underlying problem, current accepted constraints, relevant implementation facts, alternatives, tradeoffs, recommendation, confidence/caveats, and material decisions needed.

For non-trivial or multi-session work, maintain `docs/ai/workspaces/active/<architecture-topic>/HUMAN_REVIEW.md`.

## SYSTEM EFFECTS

- Load relevant product, architecture, domain, decision, and current-state context through the Knowledge Map.
- Inspect code, tests, migrations, and current-state documentation when actual behavior matters.
- Apply the Principal Engineer Contract and anti-overengineering questions.
- Persist significant human-decided architecture rationale as an ADR when it meets `docs/decisions/README.md` thresholds.
- Synchronize accepted architecture/domain documentation in the same change when practical.
- Keep future or unimplemented designs out of current-state diagrams.
- Route implementation impact to Feature Development and WORK.
- For non-trivial or multi-session workspaces, maintain the paired
  `docs/ai/workspaces/active/<architecture-topic>/AI_CONTEXT.md` alongside
  `HUMAN_REVIEW.md` and follow `docs/ai/workspaces/README.md`.
- Before asking for a material decision, ensure the workspace has enough
  persisted state for a fresh agent to resume. When the human answers, update
  the active process state and/or durable owner before future continuation
  depends on it, keeping the human-facing checkpoint current where practical.

No material architecture decision may exist only in `AI_CONTEXT.md`.

## Execution

1. State the architecture question and real pressure behind it.
2. Establish current accepted and implemented state.
3. Identify product, domain, data, operational, security, and evolution constraints where relevant.
4. Challenge the implied boundary, technology, or solution.
5. Explore alternatives, including no change.
6. Compare tradeoffs proportionally.
7. Recommend and request a human decision.
8. Persist accepted/rejected rationale and route downstream work when required.

## Completion

Complete when the architecture question has a recommendation and human decision or clear deferral, accepted decisions are not confused with implementation state, and downstream work is routed.
