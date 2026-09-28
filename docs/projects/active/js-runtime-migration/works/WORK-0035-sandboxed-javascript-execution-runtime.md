# WORK-0035: Sandboxed JavaScript Execution Runtime (Pure Function Contract)

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`

Canonical context:
- `game/language/v1/engine/LOGICAL_CONTRACT.md` (the contract this replaces, for shape/precedent only — its DSL-specific content does not carry over)

## Outcome

Build the execution boundary `GAME-ADR-0028` establishes: `execute(previousState, event, context) -> { newState, requestedCommands }`, implemented against a real, selected sandbox runtime. This is the foundational capability nearly every other WORK in this Project integrates against (persistence, commands/timers/views, artifact validation, authoring simulation) — without it, no other execution-adjacent capability can be built for real rather than designed on paper.

## Context

Not yet designed. Depends on the sandbox-runtime-technology decision (`PROJECT.md` Material Decisions #1) — a dedicated technical exploration comparing at least an embedded pure-Go interpreter, a V8-isolate-based runtime, a WASM-compiled engine, and a subprocess-per-execution model against determinism, performance, isolation strength, and operational complexity.

## Scope

Not yet designed.

## Approved Design

Not yet designed. Must preserve, at minimum, the guarantees `GAME-ADR-0028` enumerates as carried forward (one instance per Session, caller-owned concurrency, Playhoot-owned timers, an enforced execution bound, immutable version pinning) and treat sandboxing/isolation (`WORK-0036`) as a separate, coordinated concern rather than folding it in here.

## Constraints and Invariants

- The runtime must be stateless as a library — every invocation is a pure function of its explicit inputs; no module-level globals/closures may carry state between invocations.
- Authored code must never call back into Go/host capabilities mid-execution, persist state directly, or return anything other than serializable state and declarative commands.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

### Accepted / Canonical Knowledge

- A new package-local contract document (successor to `game/language/v1/engine/LOGICAL_CONTRACT.md`) recording the execution boundary's exact semantics/invariants.

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md` (or its split successor) — Game Language row replaced by the new execution runtime's status.

## Blockers

- Sandbox runtime technology selection (`PROJECT.md` Material Decisions #1) — blocks DRAFT.
- Exact command/state wire schema (`PROJECT.md` Material Decisions #3) — depends on the above.

## Completion Record

Not yet started.
