# GAME-ADR-0010: Session Disconnect, Reconnect, and Resynchronization Boundary

Status: ACCEPTED
Created: 2026-09-07
Last status change: 2026-09-07
Supersedes: None
Superseded by: None
Legacy ID: ADR-0013

## Context

GAME-ADR-0002 already assigns detection of physical disconnects and physical timer/scheduling mechanisms to the Live Session Coordinator, while Session Runtime owns durable authoritative session/runtime state. GAME-ADR-0003 and `game/README.md` left disconnect/reconnect semantics explicitly deferred: physical connection presence is not Participant state, but no accepted design existed for what happens when a connection is lost, when that loss becomes meaningful to the game, how a returning User resumes their Session identity, or how a client resynchronizes after reconnecting.

Without an accepted boundary, disconnect handling risks collapsing three distinct responsibilities into one place: the transport fact that a socket disappeared, the platform-level decision that a User is now considered disconnected/reconnected, and the gameplay consequence of that event. This ADR accepts the transport/platform boundary and the resynchronization contract shape. It does not design how authored Game Language observes or reacts to these events - that remains the next architecture milestone.

## Decision

### Three distinct responsibilities

- Transport fact (Coordinator): a physical connection disappeared or returned.
- Platform/runtime semantic event (Session Runtime boundary): this Session User is now considered disconnected or reconnected.
- Gameplay consequence (Session Runtime + Game Language): what the game does because of that event.

These responsibilities must not be collapsed into one owner.

### Physical disconnection does not change logical participation

Loss of a physical WebSocket/transport connection is owned by the Live Session Coordinator. A physical disconnect does not automatically deactivate `session_participants`, remove a Participant, free a gameplay slot, delete a SessionActor, or terminate the Session. Connection state and logical participation remain distinct concepts. Session Runtime does not add authoritative fields such as `is_connected`, `connection_id`, `websocket_id`, or other physical connection state.

### Coordinator transport grace period

The Coordinator may apply a short, in-memory transport grace/debounce period when a connection disappears, to absorb transient transport failures such as momentary Wi-Fi loss, browser/network reconnection, or short-lived socket replacement. If the User reconnects within this grace period, the Coordinator rebinds/replaces the physical connection and cancels the pending disconnect escalation; Session Runtime/Game Language does not observe a semantic disconnect. The exact grace duration is not decided here and remains configurable/implementation-level unless later promoted as product policy. This grace period is not gameplay policy and is not the amount of time a game gives a disconnected player before forfeiting/removing them.

### Semantic disconnect is elevated only after transport grace

If the transport grace expires without successful reconnection, the Coordinator notifies Session Runtime that the User/SessionActor has semantically disconnected. Session Runtime remains responsible for translating this platform event into the appropriate runtime/Game Language input. The Coordinator must not decide gameplay consequences such as removing a player, forfeiting a player, pausing the game, terminating the game, or continuing without the player - those consequences belong to Session Runtime/Game Language semantics. The exact Game Language contract/name for disconnect is not approved by this record; `PlayerDisconnected` (or any other name) must not be treated as frozen syntax/API before that contract is designed.

### Reconnect reuses the existing SessionActor/Participant

Reconnect is not Join. A reconnect uses the Session UUID and the authenticated `UserUUID` from the trusted application/auth boundary. Session Runtime resolves `(SessionUUID, UserUUID) -> existing SessionActor` and verifies the existing logical participation/session relationship. A reconnect must not create another SessionActor, create another Participant slot, consume another player slot, require a JoinCode, or rerun normal lobby Join admission. The same User resumes the same Session identity.

### Transport reconnection does not imply gameplay reinstatement

Physical connectivity and gameplay eligibility are separate. If the game had already observed the semantic disconnect, the authored game/session policy may already have removed, forfeited, disabled, or otherwise changed the gameplay state of that SessionActor. A User may successfully reconnect at the transport level while the current Game state still prevents them from resuming active gameplay: transport reconnect does not equal automatic gameplay reinstatement. If the semantic disconnect had already been delivered to the game, a corresponding reconnect semantic event may need to be delivered to Session Runtime/Game Language; the exact Game Language reconnect contract remains the next design topic.

### No new durable connection-state table for V1

The accepted Session Runtime persistence model (GAME-ADR-0007) does not gain a connection-presence table solely for disconnect/reconnect. Live physical connection bindings and transport-grace state remain ephemeral Coordinator state; no table is introduced merely for grace expiration, socket binding, last-connection timestamp, or temporary disconnect detection, and a full process restart may lose an in-flight grace period. This is acceptable for V1 unless a later accepted requirement says otherwise. Existing durable state - SessionActor, Participant, the current RuntimeTurn/Snapshot, active SessionInteractions, and active TimerObligations - remains sufficient for Session Runtime's authoritative business/runtime state.

### Session Runtime provides a resync capability

After the Coordinator has authenticated/rebound a User connection, it may request a Session resynchronization view using the SessionUUID and the authenticated UserUUID. This is a read/reconstruction capability over existing authoritative state; resync itself is a read and does not mutate the game. The exact transport endpoint/DTO/API signature is not frozen here.

### Never expose the raw engine Snapshot as client resync state

Session Runtime must not return its raw internal `engine.Snapshot` directly to Coordinator/frontend as the public resync contract. Instead, it produces a player-facing Session projection appropriate for the resolved SessionActor, conceptually capable of including Session lifecycle/phase, current player-visible game state, currently active SessionInteractions for that User, and other accepted player-visible state derived from the current authoritative runtime. Internal engine identities, paths, slots, SessionActorIDs, or raw Snapshot representation must not leak merely because resync exists. The final frontend DTO is not designed here.

### Resync carries the current monotonic runtime version

The resync representation includes a monotonic identifier of the authoritative runtime position, using the already-accepted RuntimeTurn `sequence` concept as the natural version unless canonical constraints require a different representation. This lets Coordinator/client know which authoritative runtime version the resync represents and reason about stale prior deliveries. A second, independent Session runtime version counter is not introduced merely for this purpose.

### Reconnect during transport grace creates no RuntimeTurn

If physical reconnection occurs before semantic disconnect escalation: the Coordinator rebinds the connection; the pending transport grace is cancelled; the Coordinator requests/resends current Session resync state as appropriate; no semantic disconnect/reconnect occurred from the Session/Game perspective; and no RuntimeTurn is created solely because the socket was replaced.

### Reconnect after semantic disconnect processes gameplay/runtime semantics first

If the Coordinator had already escalated the disconnect to Session Runtime/Game Language: the new connection is authenticated/rebound; Session Runtime processes the semantic reconnect according to the future accepted Game Language/runtime contract; that semantic event may produce a RuntimeTurn and resulting authoritative state; and resync is then built from the resulting/current authoritative state. The client is synchronized to what the game currently believes, rather than blindly restored to the state that existed before disconnect.

### Game-defined disconnect/reconnect policy remains unresolved

Different games may reasonably require different behavior (for example: continue immediately, wait for the player, start a gameplay timer, forfeit after timeout, remove the player, pause part of the workflow, or end the game) - these are illustrative examples only, not accepted syntax or policy. The exact mechanism by which authored Game Language observes and reacts to disconnect/reconnect is deliberately deferred to the next architecture discussion. This record does not invent signal names, language syntax, required handlers, default game policy, a reconnect timeout, or automatic removal semantics.

## Rationale

Keeping the transport fact, the platform semantic event, and the gameplay consequence as three distinct responsibilities prevents the Coordinator from silently becoming a gameplay-policy owner and prevents Session Runtime from having to reimplement transport-failure absorption. A short Coordinator-local grace period is standard practice for absorbing transient network blips without paying the cost of a full semantic disconnect/reconnect cycle for every brief hiccup.

Treating reconnect as identity resumption rather than Join preserves the accepted SessionActor/Participant model (GAME-ADR-0003, GAME-ADR-0004) and avoids creating parallel or duplicate participation records for the same User. Separating transport reconnection from gameplay reinstatement is necessary because a game may have already reacted to the semantic disconnect (forfeiting a slot, for example) before the socket physically returns; pretending otherwise would let a lucky/fast reconnect silently bypass game-defined consequences.

Not persisting a connection-state table keeps ephemeral transport concerns out of Session Runtime's durable model, consistent with the GAME-ADR-0002 Coordinator/Session Runtime boundary, and avoids adding durability guarantees (crash recovery of grace timers) that V1 does not need.

A dedicated resync capability that returns a player-facing projection - rather than the raw engine Snapshot - preserves the same internal/public boundary already accepted for SessionActorID and engine paths/slots (GAME-ADR-0005, GAME-ADR-0007): a client should never need to understand engine-internal representation merely to resynchronize. Reusing RuntimeTurn `sequence` as the resync version avoids inventing a second, redundant version concept.

## Alternatives Considered

### Treat any physical disconnect as an immediate semantic disconnect

Rejected. It would make normal transient network blips (a phone switching from Wi-Fi to cellular, a brief reconnect) trigger gameplay consequences, which is unnecessarily disruptive and couples transport flakiness directly to game state.

### Let the Coordinator decide gameplay consequences of a disconnect

Rejected. It would make a transport/ephemeral component own business/game decisions, violating the Coordinator/Session Runtime boundary already accepted in GAME-ADR-0002.

### Treat reconnect as a normal Join

Rejected. It would create a second SessionActor/Participant for the same logical User, consume another slot, and require a JoinCode for a User who is already part of the Session.

### Automatically reinstate gameplay eligibility on transport reconnect

Rejected. It would silently override game-defined consequences (forfeiture, removal) that may have already occurred in response to the semantic disconnect.

### Persist a durable connection-state/grace table

Rejected for V1. Ephemeral transport/grace bookkeeping does not need crash-recoverable durability; existing durable Session Runtime state (SessionActor, Participant, RuntimeTurn/Snapshot, Interactions, TimerObligations) is sufficient.

### Return the raw `engine.Snapshot` as the resync payload

Rejected. It would leak internal engine identities, paths, and slots to the client/Coordinator boundary, violating the internal/public identity boundary already accepted in GAME-ADR-0005 and GAME-ADR-0007.

### Introduce a second resync-specific version counter

Rejected. RuntimeTurn `sequence` already provides a monotonic authoritative-position identifier; a second counter would be redundant and could drift from it.

### Freeze `PlayerDisconnected` (or another specific name) as the Game Language disconnect signal now

Rejected. The Game Language disconnect/reconnect contract - including whether it is a standardized platform signal, its name, and its schema - has not yet been designed and is the next architecture milestone.

## Consequences

- Coordinator implementation must implement a transport grace/debounce mechanism as ephemeral, non-durable state.
- Session Runtime must expose a semantic disconnect/reconnect notification boundary from Coordinator, without accepting or persisting physical connection identifiers.
- Session Runtime must expose a resync read capability keyed by `(SessionUUID, UserUUID)` that returns a player-facing projection carrying the current RuntimeTurn `sequence`, never the raw engine Snapshot.
- Reconnect implementation must resolve `(SessionUUID, UserUUID) -> existing SessionActor` and must not reuse or extend the Join code path.
- Whether reconnect after semantic disconnect produces a RuntimeTurn depends on the not-yet-designed Game Language disconnect/reconnect contract; this record only establishes that gameplay/runtime semantics are processed before resync in that case.
- Game Language disconnect/reconnect semantics, default behavior when a game defines no handler, and any resulting timer/interaction interactions remain the next architecture milestone and are not decided here.
- Process crash/interruption semantics and runaway/abuse protections beyond what is already covered by GAME-ADR-0002/GAME-ADR-0008 remain deferred.

## Canonical Knowledge Impact

- `game/README.md` - adds the accepted disconnect/reconnect transport-and-platform boundary, the no-durable-connection-state rule, and the resync capability/projection/versioning contract; updates the previously deferred disconnect/reconnect paragraph to reflect what is now accepted versus what remains deferred to Game Language design.

## Implementation Impact

Future implementation must build the Coordinator transport-grace mechanism, the Session Runtime semantic disconnect/reconnect notification boundary, the reconnect resolution path, and the resync read capability and projection according to this accepted boundary. No WebSocket handler, Coordinator code, timer implementation, migration, authentication code, Game Language change, or WORK is authorized by this record.
