# WORK-0018: Game Language Disconnect/Reconnect Signal Support

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20

Related decisions:
- GAME-ADR-0011 (Game Language disconnect/reconnect authored semantics - accepted target design)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/language/v1/engine/internal/compiler/compile_signals.go` (`namedLifecycleSignals` catalog - `UserDisconnected` currently a placeholder empty-schema entry; `UserReconnected` absent entirely)
- `game/language/v1/program/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md` (both already document this as the accepted target design, not yet implemented)
- `docs/projects/active/session-runtime-v1/works/WORK-0015-disconnect-reconnect-full-resync.md` (the Session Runtime capability this WORK gates)

## Outcome

`UserDisconnected` is given its accepted `{user: user}` shape (currently a placeholder empty schema), and `UserReconnected` is added to the compiler's `namedLifecycleSignals` catalog (currently absent entirely) - so an authored Game Language workflow can actually receive and react to either signal.

## Context

This is a hard prerequisite for WORK-0015 (Session Runtime Disconnect/Reconnect/Full Resync): Session Runtime cannot deliver a signal Game Language does not yet correctly support. It is Game Language/compiler domain work, not Session Runtime application logic, but it is tracked under this Project (the same way Keyed Timers, WORK-0013, is) because it directly and solely gates a required Session Runtime V1 capability, and both GAME-ADR-0011 (the accepted target semantics) and the current implementation gap already live inside `game/language/v1/`.

**Open scope note recorded for human review** (not decided here): whether Game Language-domain compiler work should be tracked under a Session-Runtime-named Project at all, versus as its own small Game Language initiative, is a judgment call - see `PROJECT.md`'s "Material Decisions Needing Human Input".

## Scope

### In Scope (known required outcome; design not yet started)

- `UserDisconnected`'s schema changed from placeholder-empty to `{user: user}`.
- `UserReconnected` added to `namedLifecycleSignals`, with an accepted schema.
- Whatever engine-side handling either signal's delivery requires beyond the compiler catalog entry itself.

### Out of Scope

- Session Runtime's own consumption of these signals (WORK-0015's scope, not this WORK's).
- Any other Game Language capability - this WORK is narrowly scoped to these two signals.

## Approved Design

Not yet designed beyond GAME-ADR-0011's already-accepted target shape (`{user: user}` for `UserDisconnected`); `UserReconnected`'s exact schema is not yet specified anywhere and remains open.

## Constraints and Invariants

- Must match GAME-ADR-0011's already-accepted semantics; this WORK implements an accepted design, it does not redesign it.

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- `UserReconnected`'s exact schema needs to be specified during DRAFT (GAME-ADR-0011 accepts the general direction but does not appear to fully pin this down - verify against the ADR text during design).

## Documentation Impact

Not yet assessed in detail; expected to touch `game/language/v1/program/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md` once designed.

## Completion Record

Not started. PLANNED.
