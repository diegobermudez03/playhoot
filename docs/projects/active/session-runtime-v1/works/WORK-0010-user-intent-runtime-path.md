# WORK-0010: User Intent Runtime Path

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20 (Part B/J reconciliation, same day: replay-input durability and PARTICIPANT-only command surface made explicit - see "Scope Addition (Part B/J Reconciliation, 2026-09-20)" below)

Related decisions:
- GAME-ADR-0018 (RUNNING serialization boundary, reload-after-lock, ordering-by-commit)
- GAME-ADR-0002 (Live Session Coordinator responsibility boundary)
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - this WORK's UserIntent input must be durably, order-preservingly representable)
- GAME-ADR-0025 (Role-Aware Live Connections - UserIntent is a PARTICIPANT-only command)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/language/v1/program/signal.go` (`UserIntentSignalSource`), `game/language/v1/program/ui.go` (`EmitUserIntentAction`)
- `game/language/v1/engine/signal.go` (`SignalKindIntent`)
- `docs/projects/active/session-runtime-v1/works/WORK-0004-interaction-response-processing.md` (the existing RuntimeTurn/serialization pattern this WORK's runtime path reuses)
- `docs/projects/active/session-runtime-v1/works/WORK-0019-replay-first-session-runtime-persistence-migration.md` (owns the general replay-input model; this WORK is responsible for satisfying it for UserIntent specifically)
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (the PARTICIPANT connection this WORK's live command rides)

## Outcome

A player can trigger an authored user intent (an unsolicited player action, not an answer to something Session Runtime already opened) and have it flow end-to-end: UI action -> transport command -> Session Runtime authorization/validation -> the correct runtime/workflow instance -> a RuntimeTurn -> persisted resulting state -> Presentation/Effect consequences.

`UserIntentSignalSource`/`EmitUserIntentAction`/`SignalKindIntent` are fully implemented, compiled, and engine-tested in Game Language today - but `grep "Intent"` across `game/session/`, `play/`, and `api/` returns zero matches. There is currently no way for a player to submit an intent through any live path; only answering an already-open interaction is wired (WORK-0004/WORK-0005).

## Context

This is a distinct runtime path from `AnswerInteraction`, not a variant of it: `AnswerInteraction` always targets a specific already-open `session_interactions` row Session Runtime itself opened. A user intent is unsolicited - the player initiates it, not Session Runtime - so there is no existing open-interaction row to correlate against. The open design question this WORK must resolve is routing context: an intent may need to target a particular workflow/runtime instance, and the client action needs enough context to route it correctly without exposing engine-internal slot/path identities (the same non-leakage principle WORK-0005/WORK-0006 already apply to Output translation).

## Scope

### In Scope (known required outcome; design not yet started)

- A `Manager`-level capability (parallel to `Start`/`AnswerInteraction`) that accepts a client-submitted intent, resolves it to the correct runtime/workflow instance, and drives a RuntimeTurn through the existing Step-draining/bound execution mechanism. This part needs only `sessionlifecycle.Manager` and no `play`/transport code.
- A live-transport command and Coordinator-side translation. **(2026-09-23 correction)** `play`/`play/sessionruntime` do not exist today - deleted in full by WORK-0005's Blocker 11 (2026-09-21) and not yet rebuilt; this part reuses whatever translation/fan-out shape `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (re)builds, once it lands, not WORK-0005's original (deleted) implementation.
- Whatever Presentation/Effect consequences result, reusing WORK-0006's play-half fan-out mechanism (not yet drafted - see WORK-0006's own "Scope Correction (Domain/Play Split, 2026-09-23)") once it exists.

### Out of Scope

- Any change to Game Language's own intent compiler/engine support - already implemented and out of this WORK's scope.
- Manual Session cancellation (WORK-0011) - a distinct host-only operation, not a player intent.

## Approved Design

Not yet designed. The routing-context representation (how a client identifies which workflow/runtime instance an intent targets, without exposing engine internals) is the central open question, deliberately left open per the reconciliation prompt that created this WORK.

## Constraints and Invariants

- Must reuse the existing per-Session RUNNING serialization boundary (GAME-ADR-0018) - an intent-driven RuntimeTurn is subject to the same reload-after-lock/ordering-by-commit/Step-bound rules as `AnswerInteraction`.
- Must not expose engine-internal slot/path identities to the client.
- **(Part B reconciliation)** A submitted UserIntent's content (intent name, submitted arguments, actor, routing context) must be durably captured, in commit order, before or atomically with the RuntimeTurn it drives - per GAME-ADR-0024/WORK-0019's replay-input model, this WORK gives that content a durable home (a new narrow satellite entity, or a field on the RuntimeTurn envelope itself), since no existing normalized entity already captures it the way `session_interactions` does for answers. This WORK does not persist the resulting Presentation/Effect consequences for replay - those remain derived, per WORK-0006.
- **(Part J reconciliation)** UserIntent is a PARTICIPANT-connection command, reached only from the PARTICIPANT connection WORK-0020 establishes - never from an ADMIN connection, and never duplicating Auth/Identity beyond WORK-0005's already-established trusted-`UserUUID` stance.

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- Routing-context representation (see Approved Design) is the material open question for the DRAFT phase.
- Depends on WORK-0020 (PARTICIPANT connection) and, for its durable replay-input shape, coordination with WORK-0019's model.

## Scope Addition (Part B/J Reconciliation, 2026-09-20)

A broader reconciliation session accepted GAME-ADR-0024 (replay-first persistence) and GAME-ADR-0025 (role-aware connections). Both bear directly on this WORK, which was PLANNED without either constraint spelled out: (1) a submitted UserIntent is a RuntimeTurn-driving cause with no existing durable home, so this WORK must give it one, consistent with every other cause in the replay-input model, rather than only persisting whatever `sessionlifecycle.Manager` already needed for its own immediate correctness; (2) UserIntent is unambiguously a PARTICIPANT-connection command, never an ADMIN one, now that the connection-role distinction exists as accepted architecture. Neither changes this WORK's central open question (routing-context representation), which remains for DRAFT. No implementation was performed; this WORK's Status remains PLANNED.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/CURRENT_STATE.md`, `game/docs/FLOWS.md`, `play/README.md` once designed.

## Completion Record

Not started. PLANNED.
