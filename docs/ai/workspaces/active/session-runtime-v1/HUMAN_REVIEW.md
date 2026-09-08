# Session Actor Semantic Presence And Lobby-To-Runtime Membership Checkpoint

Process: Architecture Discussion

Status: RESOLVED - accepted 2026-09-07. No checkpoint is currently pending; this file is retained as the human-facing record of the resolved checkpoint until the next milestone (durable semantic presence recovery after total process loss) produces its own checkpoint.

## What Was Approved

Semantic presence and lobby-to-runtime membership - the Operational Lifecycle milestone opened after process-crash/recovery and durable inactivity expiration closed - were reviewed and accepted as GAME-ADR-0015.

### Durable SessionActor semantic presence (GAME-ADR-0015)

- `session_actors` gains a durable `semantic_presence` field (`CONNECTED | DISCONNECTED`) - conceptually owned by `session_actors`, not `session_participants` - answering only "has this SessionActor crossed the Coordinator -> Session semantic connected/disconnected boundary?" Named `semantic_presence` rather than `is_connected` to avoid implying physical socket presence.
- This is distinct from `session_participants.active` ("does this actor currently occupy/admit a participant position?"); the two may legitimately diverge, for example a reconnecting actor that is semantically `CONNECTED` while locked out of a full lobby.
- Physical connection state remains entirely ephemeral Coordinator state: no socket IDs, connection IDs, live WebSocket presence, Coordinator bindings, grace timers, or process IDs are persisted. GAME-ADR-0010's rejection of `is_connected`/`connection_id`/`websocket_id` persistence is unchanged - this is an additive refinement, not a reversal.
- After Coordinator grace expires, Session Runtime serializes the SessionActor `CONNECTED -> DISCONNECTED` transition per Session; repeated disconnect reports while already `DISCONNECTED` are idempotent and must not duplicate Game Language disconnect effects.

### LOBBY disconnect/reconnect semantics

- In `LOBBY`, semantic disconnect deactivates the active Participant and releases its slot immediately - the user no longer counts toward lobby capacity, and no `UserDisconnected` exists yet (no game runtime has started). This is intentionally stronger than RUNNING: a player still inside Coordinator transport grace never reaches Session Runtime as a disconnect and keeps their active lobby slot.
- Reconnect reuses the same SessionActor/existing Participant record (not a new Join identity). If the prior disconnect deactivated the Participant, re-admission is revalidated against *current* lobby constraints (still `LOBBY`, not expired, current capacity against `players.max`, other applicable constraints) exactly as Join would validate.
- Reconnect does not guarantee recovery of a previously released slot: if another actor occupied it while the first was disconnected, the reconnecting actor may remain semantically `CONNECTED` with an inactive Participant and no reserved slot.

### Start roster and permanent exclusion

- Start (already serialized per Session, GAME-ADR-0004) builds the `players: list<user>` roster strictly from Participants active at that serialized moment. A `DISCONNECTED` actor, or a `CONNECTED` actor not currently re-admitted as active, is excluded.
- The Coordinator-grace-vs-Start race is resolved entirely by the existing per-Session serialization, with no special-casing outside Session: whichever operation wins serialization determines inclusion.
- An actor excluded from the Start-time roster does not join the running game merely by reconnecting afterward - Session Runtime must not deliver `UserReconnected` toward the engine, or dynamically add, a SessionActor absent from that execution's roster. Post-Start late admission remains a distinct, separately deferred capability, not designed here.

### RUNNING disconnect semantics remain unchanged and are reaffirmed

- Once a Participant was included in the Start-time roster, `RUNNING` semantic disconnect does not deactivate/remove it from the runtime roster. Session Runtime may emit the already-accepted root `UserDisconnected(user)` (GAME-ADR-0011); authored Game Language - not Session Runtime - determines gameplay consequences. No auto-forfeit, auto-skip, or freed slot. Reconnect of such a member may produce `UserReconnected(user)` before resync.

## Where This Was Persisted

- `game/docs/decisions/GAME-ADR-0015-session-actor-semantic-presence-and-lobby-membership.md`
- `game/docs/decisions/INDEX.md` (GAME-ADR-0015 row added; next Game ADR is now `GAME-ADR-0016`)
- `game/README.md` (SessionActor lifecycle paragraph updated to introduce `semantic_presence` alongside the unchanged physical-connection-state rejection; Session Runtime Lobby Lifecycle Contract section updated with LOBBY disconnect/reconnect admission semantics and the Start active-Participants-only roster/permanent-exclusion rule; Session Runtime Disconnect, Reconnect, and Resynchronization Boundary section updated with the phase-dependent LOBBY-vs-RUNNING consequence)
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (`session_actors.semantic_presence` added to both ER diagrams; new "SessionActor Semantic Presence vs Participant Admission" section)
- `docs/ai/workspaces/active/session-runtime-v1/AI_CONTEXT.md` (new accepted-decision section, updated deferred topics, updated next-milestone question, updated drift/explicitly-not-done notes)

## Explicitly Not Authorized By This Checkpoint

No production code, migrations, lobby reconnect implementation, Coordinator grace implementation, compiler/engine/program Game Language changes, tests, or WORK were created or changed. No process ownership model was introduced. No dynamic runtime membership / post-Start late admission was designed - the roster remains immutable after Start under this checkpoint. No physical connection/socket/process-ID persistence was introduced anywhere; GAME-ADR-0010's rejection of physical connection-state fields stands unchanged.

## Next Milestone (Not Designed Here)

Durable semantic presence recovery after total process loss: a `RUNNING` Session may durably contain actors whose `semantic_presence` is `CONNECTED`, while a new Coordinator process has no physical socket bindings because all ephemeral state was lost after a crash/restart. Open questions: whether Coordinator gives those actors a fresh transport/recovery grace after restart; when absence of a new binding becomes a new semantic disconnect; whether actors already durably `DISCONNECTED` require no such grace; and how this interacts with process-agnostic Session Runtime (GAME-ADR-0013) and durable inactivity expiration (GAME-ADR-0014). Do not introduce process ownership to solve it. A new checkpoint should be opened here once that design work produces a proposal for human review.
