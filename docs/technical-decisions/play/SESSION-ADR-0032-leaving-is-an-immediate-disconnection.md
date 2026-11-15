# SESSION-ADR-0032: Leaving Is An Immediate Disconnection

Status: ACCEPTED
Created: 2026-10-18
Last status change: 2026-10-18
Supersedes: None
Superseded by: None

## Context

The architecture plan (`plans/session_runtime/...`) treats a player losing their network and a player leaving on purpose as two flows with two outcomes:

- §15: when the transport grace runs out, the Runtime inserts `playerDisconnected(P)` into the timeline and the participant becomes DISCONNECTED. A disconnected participant may rejoin through `playerRejoined()` if the game supports it.
- §16 and invariant 9: an explicit LEAVE skips the grace, inserts `playerLeft(P)`, and the participant becomes LEFT. LEFT is permanent: Playhoot never asks the script whether someone who left may return.

That gives game scripts two entrypoints for the same game question: a participant stopped playing, so what happens to their part of the game? A game must already handle a disconnected participant who never comes back, so a separate `playerLeft()` adds nothing it can act on.

## Decision

1. **One entrypoint.** When a participant stops playing, by leaving or because their transport grace ran out, the game runs `playerDisconnected(state, participant, context)`. It is not told which one happened. There is no `playerLeft()` entrypoint.
2. **Leaving is an immediate disconnection.** An explicit `leave` in a running game resets the participant's inputs, puts `playerDisconnected(P)` on the timeline at once, without the transport grace, and asks for a boundary right away. The connection ends as `left`.
3. **No separate LEFT status.** A participant who left is disconnected like any other. Rejoining (§17), once built, applies to both. Whether someone may come back is the game's decision, made in `playerRejoined()`.
4. **A Session nobody plays ends.** Once every participant is disconnected and the game was told about each of them, the Runtime cancels the Session durably (CANCELLED, end cause `ALL_PLAYERS_LEFT`, by the platform) and ends it live, whether the game supports rejoining or not. A rejoin still waiting for the game's answer keeps it running until decided. (Clarified by the human 2026-10-18, when rejoin was built.)

## Rationale

- One entrypoint and one status keep generated scripts simpler and remove a distinction games could not use.
- The game, not the platform, owns policy about who may return. That matches how the plan already leaves rejoin policy to `playerRejoined()`, without a static `rejoinWindow` in metadata.

## Alternatives Considered

### Two entrypoints and a permanent LEFT, as in the plan

Rejected by the human: the script cannot do anything different with the two events, and making leave permanent takes a decision away from the game.

### One entrypoint that receives the cause

Rejected by the human: it would let games react differently, but a game must handle a participant who never returns either way.

## Consequences

- Invariant 9 of the plan ("explicit leave is permanent") no longer holds. A participant who left can come back only through rejoin, and only if the game accepts it.
- Unlike the plan (§15), a game that implements `playerRejoined()` does not keep its Session running while every participant is away. Rejoining is only possible while someone is still playing.
- The `ALL_PLAYERS_LEFT` cause covers every participant being disconnected, whether they left or lost their connection.

## Canonical Knowledge Impact

- `play/CURRENT_STATE.md`: leave and disconnection in a running game, and abandoned Sessions.
- `play/docs/DATA_MODEL.md`: the `ALL_PLAYERS_LEFT` end cause.

## Implementation Impact

Implemented together with this decision: `leave` in `play/runtime/workflows/liveconnection`, and `CancelAbandonedSession` in `play/session/workflows/sessionlifecycle`.
