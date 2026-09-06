# Feature Development Protocol

Status: AGENT EXECUTION PROTOCOL

Use with the human-facing guide at `docs/ai/processes/FEATURE_DEVELOPMENT.md`.

This protocol owns design-to-READY execution, primarily performed by a CONVERSATIONAL AI with repository persistence delegated through a CODEBASE AGENT HANDOFF when needed. Concrete implementation (CODEBASE AGENT) and independent review (INDEPENDENT REVIEWER) are governed by `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`.

## Scope

Use for concrete product or technical capabilities that may become approved WORK.

Do not implement until a persistent WORK specification is READY. Do not use Feature Development to silently make product, architecture, domain, or engineering-standard decisions that require their owning processes.

## HUMAN SURFACE

Use `docs/ai/workspaces/active/<feature-topic>/HUMAN_REVIEW.md` for non-trivial checkpoints:

- Feature Design Review: scope challenge, behavior, options, tradeoffs, material decisions, documentation impact, and recommendation.
- READY Review: concise human explanation of the DRAFT WORK, unresolved blockers, acceptance criteria, verification, documentation impact, and what approval authorizes.

The human should be able to authorize READY from the human review without reading machine-oriented WORK internals line by line.

## SYSTEM EFFECTS

- Load context through the Knowledge Map.
- Inspect code, tests, migrations, and current-state docs when implementation reality matters.
- Ensure `AI_CONTEXT.md` is persisted, via a CODEBASE AGENT HANDOFF, for fresh-session continuation, never as hidden implementation authority.
- Create DRAFT WORK under `docs/work/active/` only when concrete implementation work is being prepared.
- Use `docs/work/templates/WORK_SPEC.template.md`.
- Validate Definition of Ready from `docs/work/README.md`.
- Synchronize accepted/canonical knowledge only through the owning product, architecture, domain, or standard process.
- Update current-state documentation only after implementation actually changes system state or behavior.
- Remove the temporary workspace once WORK and relevant canonical context are durable enough for handoff.

No material decision may exist only in `AI_CONTEXT.md`.

## Scope Challenge Gate

Before optimizing inside the requested domain/package, determine whether:

- the feature belongs here;
- product behavior is unresolved;
- a domain boundary is missing or wrong;
- a global architecture decision is needed;
- a reusable engineering standard is needed.

Pause and route material blockers to the correct process. Do not force a separate process for trivial/local choices.

## Design And WORK

Cover only relevant behavior, public contracts, domain model, data/persistence, consistency, concurrency, errors, security, observability, failure modes, and tests.

The WORK specification must distinguish approved decisions, local implementation freedom, out of scope, unresolved blockers, verification, and documentation impact.

Documentation Impact must distinguish:

- accepted/canonical knowledge affected by approved decisions;
- current-state documentation expected to change after implementation;
- documents intentionally unchanged when useful.

Current-state documentation must not be updated to future state.

## READY

DRAFT has no implementation authority.

Only the human may authorize DRAFT -> READY. A Codebase Agent must not self-approve it.

READY means material uncertainty is resolved and local implementation choices are inside the Operating Model autonomy boundary.

## Handoff

After READY, use the minimal implementation handoff from `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`.

The Codebase Agent must independently load AGENTS, the Operating Model, Knowledge Map, WORK, and relevant implementation context.

## Completion

Feature Development is complete only after the implementation/review protocol reaches closure:

- Definition of Ready was met before implementation.
- Implementation and tests are complete.
- Verification ran or limitations are reported.
- Independent review verdict is APPROVED.
- No REQUIRED_FIX or DECISION_REQUIRED finding remains unresolved.
- Any human-accepted exceptional limitation/deviation has been resolved through the implementation/review protocol and recorded where appropriate.
- Required documentation synchronization is complete.
- No unresolved material drift was introduced.
- DONE or CANCELLED WORK has moved from `docs/work/active/` to `docs/work/completed/`.
