# WORK-0010: User Intent Runtime Path

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20

Related decisions:
- GAME-ADR-0018 (RUNNING serialization boundary, reload-after-lock, ordering-by-commit)
- GAME-ADR-0002 (Live Session Coordinator responsibility boundary)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/language/v1/program/signal.go` (`UserIntentSignalSource`), `game/language/v1/program/ui.go` (`EmitUserIntentAction`)
- `game/language/v1/engine/signal.go` (`SignalKindIntent`)
- `docs/projects/active/session-runtime-v1/works/WORK-0004-interaction-response-processing.md` (the existing RuntimeTurn/serialization pattern this WORK's runtime path reuses)

## Outcome

A player can trigger an authored user intent (an unsolicited player action, not an answer to something Session Runtime already opened) and have it flow end-to-end: UI action -> transport command -> Session Runtime authorization/validation -> the correct runtime/workflow instance -> a RuntimeTurn -> persisted resulting state -> Presentation/Effect consequences.

`UserIntentSignalSource`/`EmitUserIntentAction`/`SignalKindIntent` are fully implemented, compiled, and engine-tested in Game Language today - but `grep "Intent"` across `game/session/`, `play/`, and `api/` returns zero matches. There is currently no way for a player to submit an intent through any live path; only answering an already-open interaction is wired (WORK-0004/WORK-0005).

## Context

This is a distinct runtime path from `AnswerInteraction`, not a variant of it: `AnswerInteraction` always targets a specific already-open `session_interactions` row Session Runtime itself opened. A user intent is unsolicited - the player initiates it, not Session Runtime - so there is no existing open-interaction row to correlate against. The open design question this WORK must resolve is routing context: an intent may need to target a particular workflow/runtime instance, and the client action needs enough context to route it correctly without exposing engine-internal slot/path identities (the same non-leakage principle WORK-0005/WORK-0006 already apply to Output translation).

## Scope

### In Scope (known required outcome; design not yet started)

- A `Manager`-level capability (parallel to `Start`/`AnswerInteraction`) that accepts a client-submitted intent, resolves it to the correct runtime/workflow instance, and drives a RuntimeTurn through the existing Step-draining/bound execution mechanism.
- A live-transport command and Coordinator-side translation, reusing WORK-0005's existing translation/fan-out shape.
- Whatever Presentation/Effect consequences result, reusing WORK-0006's fan-out mechanism once that WORK exists.

### Out of Scope

- Any change to Game Language's own intent compiler/engine support - already implemented and out of this WORK's scope.
- Manual Session cancellation (WORK-0011) - a distinct host-only operation, not a player intent.

## Approved Design

Not yet designed. The routing-context representation (how a client identifies which workflow/runtime instance an intent targets, without exposing engine internals) is the central open question, deliberately left open per the reconciliation prompt that created this WORK.

## Constraints and Invariants

- Must reuse the existing per-Session RUNNING serialization boundary (GAME-ADR-0018) - an intent-driven RuntimeTurn is subject to the same reload-after-lock/ordering-by-commit/Step-bound rules as `AnswerInteraction`.
- Must not expose engine-internal slot/path identities to the client.

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- Routing-context representation (see Approved Design) is the material open question for the DRAFT phase.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/CURRENT_STATE.md`, `game/docs/FLOWS.md`, `play/README.md` once designed.

## Completion Record

Not started. PLANNED.
