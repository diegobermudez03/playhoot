# GAME-ADR-0011: Game Language Disconnect/Reconnect Authored Semantics

Status: ACCEPTED
Created: 2026-09-07
Last status change: 2026-09-07
Supersedes: None
Superseded by: None

## Context

GAME-ADR-0010 accepted the transport-and-platform disconnect/reconnect/resync boundary but explicitly deferred how authored Game Language observes and reacts to a disconnect/reconnect event, declining to freeze any signal name (including `PlayerDisconnected` or the pre-existing `UserDisconnected` placeholder) or default policy. That deferred question is the next Operational Lifecycle milestone this record closes: how Session Runtime represents disconnect/reconnect to the engine, whether they are standardized platform signals, how an authored game opts in, what happens when a game defines no handler, and how offline actors interact with normal interactions/progress.

`game/language/v1/program/signal.go`'s `NamedSignalSource` doc comment and `game/language/v1/engine/internal/compiler/compile_signals.go`'s `namedLifecycleSignals` catalog already contain a placeholder named signal `UserDisconnected` with an empty, unvalidated schema, predating this checkpoint and GAME-ADR-0010. `UserReconnected` does not exist anywhere in the current implementation. Neither is compiler/engine/program code changed by this record.

## Decision

### Standard authored lifecycle signals

`UserDisconnected` and `UserReconnected` are `NamedSignalSource` platform/lifecycle signals (`program.NamedSignalSource`), the same closed mechanism already used for `WorkflowStarted`, `SessionCancelled`, and `ParentCancelled` - not new `SignalKind`/`SignalSource` variants. Each exposes exactly one authored field: `user: user`, representing Session-local runtime identity derived from `SessionActorID`.

Never exposed as part of this schema: `Identity.UserUUID`, connection IDs, socket IDs, IP address, disconnect timestamp, network/transport reason, transport-grace information, or any other Coordinator-internal detail.

The pre-existing empty-schema `UserDisconnected` placeholder is treated as compatible scaffolding whose intended schema is now accepted as `user: user`. `UserReconnected` is newly accepted conceptually, as the same shape. This record does not implement either signal's compiler schema.

### Root delivery only, no implicit broadcast

Session Runtime delivers `UserDisconnected`/`UserReconnected` to the root workflow instance only. There is no implicit broadcast to every active nested child-workflow, task-group, or ask-group instance. The current engine `Step` contract addresses one workflow instance path per call and does not internally chain or fan out to other instances (see `game/language/v1/engine/README.md`); an implicit broadcast mechanism would need to invent hidden multi-instance ordering, fan-out, and multi-`Step` semantics the engine does not currently have. Nested workflows may later be coordinated explicitly by authored root-level logic, or by a future language capability - neither is designed here.

### Handling is optional; there is no default gameplay consequence

An authored game is not required to declare a transition reacting to `UserDisconnected` or `UserReconnected`. If the root workflow has no matching transition, the engine rejects the signal exactly as it already does for any other unmatched signal (`engineservice.ErrSignalRejected`) - this is a valid, ignored lifecycle event, not an error condition. Session Runtime must not infer remove, forfeit, pause, skip, or end from an unhandled lifecycle signal. "The game ignores disconnect" means only "disconnect itself does not mutate gameplay" - it does not mean the platform guarantees continued gameplay progress.

### No RuntimeTurn solely to record a rejected lifecycle event

A rejected/unhandled `UserDisconnected`/`UserReconnected` delivery that produces no `Commit` does not create a `RuntimeTurn` merely to persist a historical no-op, consistent with `RuntimeTurn` already meaning one committed authoritative state transition (GAME-ADR-0007). Telemetry/logging of the raw platform event may exist later at a different layer; it is not Session-state.

### Offline actors still receive interactions normally

If authored execution reaches an interaction/question targeted at a SessionActor whose physical transport connection is currently absent, Session Runtime still creates the normal durable `SessionInteraction`. It must not suppress it, auto-answer it, skip it, deactivate the Participant, redirect it, or invent a default response merely because the actor is offline. The interaction remains logically ACTIVE; Coordinator may be unable to deliver it live, but logical interaction existence is independent of successful live delivery (`logical interaction existence != successful live delivery`). If the User reconnects later, the already-accepted resync capability (GAME-ADR-0010) can surface the still-active interaction.

### Progress while an actor is offline is authored policy, not a Session Runtime inference

Whether/how gameplay proceeds while a targeted actor is offline is ordinary authored Game Language policy - for example, an authored timer/timeout transition deciding skip/default/forfeit - using normal Game Language mechanisms. Disconnect is not itself required to implement ordinary turn-timeout behavior. If the game opens/waits for an interaction, there is no applicable timeout, and the player never reconnects, the execution may legitimately remain waiting; Session Runtime must not "intelligently" invent progress. A future platform-level max-runtime/inactivity/runaway protection may eventually terminate an abandoned Session, but must not fabricate gameplay responses or silently perform authored actions; that topic remains deferred. GAME-ADR-0012 separately accepts a general Game Language capability (keyed timer slots) that makes expressing independent per-user offline-timeout policy possible, since it was found awkward with the existing single-pending-timer `TimerSlotDeclaration`.

## Rationale

Standardizing `UserDisconnected`/`UserReconnected` as ordinary `NamedSignalSource` lifecycle signals keeps disconnect/reconnect inside the same signal-driven model every other platform event already uses, rather than inventing a parallel notification side-channel. Restricting the authored field to `user: user` preserves the internal/public identity boundary already accepted for `SessionActorID` (GAME-ADR-0005) and keeps Coordinator/transport internals out of authored code, consistent with GAME-ADR-0010's boundary between the transport fact, the platform semantic event, and the gameplay consequence.

Root-only delivery avoids inventing fan-out/ordering semantics the engine's `Step` contract does not currently support: `Step` resolves and applies exactly one signal against one instance path per call. Implicit broadcast to every nested instance would require deciding delivery order across an arbitrary, dynamically shaped child tree, is unnecessary for the concrete disconnect/reconnect need (a root workflow can already relay explicitly if a game needs nested reaction), and can be reconsidered later as an explicit authored/language capability without retrofitting hidden platform semantics now.

Making handling optional with no default gameplay consequence follows the same principle GAME-ADR-0010 already applied to reconnect: Session Runtime does not invent product/gameplay policy. Different games have legitimately different disconnect tolerance (a trivia quiz may not care; a turn-based strategy game may). A platform-imposed default (skip/forfeit/pause) would itself be an unreviewed product decision and would surprise authors who never opted into it.

Keeping offline interactions fully durable/ACTIVE - rather than skipping or auto-answering them - preserves the existing accepted invariant that interaction existence is authoritative Session Runtime state (GAME-ADR-0007) independent of live delivery capability, and avoids Session Runtime silently making a gameplay decision (fabricating an answer) that belongs to authored Game Language or the human player.

## Alternatives Considered

### Broadcast disconnect/reconnect to every active nested workflow instance

Rejected. The engine's `Step` contract addresses one instance path per call; implicit multi-instance broadcast would introduce hidden ordering, fan-out, and multi-`Step` semantics the engine does not currently have, for a capability a root-level authored relay can already express when actually needed.

### Require an explicit "supports disconnect" opt-in flag separate from declaring a transition

Rejected as unnecessary. Whether an authored transition exists for the signal already expresses opt-in; a separate flag would duplicate that information, and the engine already treats an unmatched signal as a normal, well-defined outcome (`ErrSignalRejected`) rather than an error condition requiring a flag to avoid.

### Auto-remove/forfeit/pause an offline actor by platform default

Rejected. This is authored gameplay policy, not a platform invariant. Different games plausibly want different behavior (GAME-ADR-0010's illustrative list: continue/wait/timer/forfeit/remove/pause/end), and a platform-imposed default would silently override author intent with no sound way to reverse it if wrong.

### Auto-answer or skip an interaction targeted at an offline actor

Rejected. Fabricating a response is an authoritative gameplay action Session Runtime has no authority to invent, and would make interaction outcome depend on transient Coordinator connectivity rather than authored logic, undermining the accepted logical-interaction-existence invariant.

### Persist a RuntimeTurn for every disconnect/reconnect delivery, handled or not

Rejected. A rejected signal producing no `Commit` has no new authoritative state to record; forcing a `RuntimeTurn` purely to log "nothing happened" would inflate Turn history with no-op entries and blur the accepted meaning of a Turn as one committed authoritative state transition (GAME-ADR-0007). Telemetry, if wanted later, belongs at a different layer.

## Consequences

- The Game Language compiler's named-signal catalog (`namedLifecycleSignals`) must eventually be extended, not by this record, with `UserDisconnected: {user: user}` and a new `UserReconnected: {user: user}` entry; the existing empty-schema `UserDisconnected` placeholder remains unimplemented drift until that implementation work happens.
- Session Runtime must deliver these signals addressed to the root workflow instance path only, and must not implement or expose a broadcast delivery path to nested instances for these signals.
- Session Runtime must not create a `RuntimeTurn` when the engine rejects a disconnect/reconnect signal because no transition matched.
- Session Runtime's interaction-opening path must remain unconditional on live transport connectivity: creating a `SessionInteraction` never becomes conditional on the target SessionActor being currently connected.
- Any future max-runtime/inactivity/runaway protection must be designed as a separate Session-lifecycle decision and must not fabricate gameplay responses.
- The independent-per-user-timeout capability this record's offline-progress discussion surfaces is addressed by GAME-ADR-0012, not invented here.

## Canonical Knowledge Impact

- `game/README.md` - Session Runtime Disconnect, Reconnect, and Resynchronization Boundary section updated to record the accepted `UserDisconnected`/`UserReconnected` authored contract, root-only delivery, optional handling with no default gameplay consequence, offline-interaction durability, and the no-invented-progress rule; the prior "next architecture milestone" framing is closed.
- `game/language/v1/program/README.md` - notes the accepted, not-yet-implemented `UserDisconnected`/`UserReconnected` `NamedSignalSource` contract alongside the existing accepted root roster contract.
- `game/language/v1/engine/README.md` and `game/language/v1/engine/LOGICAL_CONTRACT.md` - note the accepted root-only delivery constraint and the offline-interaction/no-invented-progress invariants as accepted-but-unimplemented.

## Implementation Impact

Future implementation must extend the compiler's named-signal catalog with `UserDisconnected`/`UserReconnected` schemas, wire Session Runtime's semantic disconnect/reconnect notification path (GAME-ADR-0010) to deliver these as root-addressed signals, and ensure interaction-opening logic never becomes conditional on live connectivity. No compiler, engine, program, Session Runtime, Coordinator, or migration code, and no WORK, is authorized by this record.
