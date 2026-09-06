# Product Discussion Protocol

Status: AGENT EXECUTION PROTOCOL

Use with the human-facing guide at `docs/ai/processes/PRODUCT_DISCUSSION.md`.

## Scope

This protocol decides product direction. It must not implement code, silently create architecture, or turn proposals into accepted decisions without human approval.

## HUMAN SURFACE

Show the human the product problem, user/persona/use case, options, tradeoffs, MVP vs future distinction, recommendation, material decisions needed, and next process.

For non-trivial or multi-session work, maintain `docs/ai/workspaces/active/<product-topic>/HUMAN_REVIEW.md`.

## SYSTEM EFFECTS

- Load context through `docs/ai/KNOWLEDGE_MAP.md`.
- Distinguish EXISTING, PROPOSED, ACCEPTED, and IMPLEMENTED facts.
- Report drift between canonical documentation and implementation when found.
- Persist material human-decided product rationale as a PDR when it meets `docs/decisions/README.md` thresholds.
- Synchronize accepted product decisions to canonical product owners.
- Put deferred ideas in non-authoritative idea/radar locations only when useful.
- Route accepted implementation work to Feature Development and WORK.
- For non-trivial or multi-session workspaces, maintain the paired
  `docs/ai/workspaces/active/<product-topic>/AI_CONTEXT.md` alongside
  `HUMAN_REVIEW.md` and follow `docs/ai/workspaces/README.md`.
- Before asking for a material decision, ensure the workspace has enough
  persisted state for a fresh agent to resume. When the human answers, update
  the active process state and/or durable owner before future continuation
  depends on it, keeping the human-facing checkpoint current where practical.

No material product decision may exist only in `AI_CONTEXT.md`.

## Execution

1. Clarify the product question and user/business problem.
2. Challenge the proposed solution and timing.
3. Compare alternatives, including no change.
4. Separate MVP/current target from future possibilities.
5. Recommend accept, reject, defer, experiment, or route.
6. Obtain explicit human decision before updating accepted product truth.

## Completion

Complete when the product question is answered or deferred, accepted facts are separated from proposals, durable owners are synchronized where required, and the next process is identified.
