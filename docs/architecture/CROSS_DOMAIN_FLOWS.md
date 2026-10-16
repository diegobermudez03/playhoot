# Cross-Domain Flows

Status: CURRENT IMPLEMENTATION

Accepted cross-domain coordination rules live in `../../ARCHITECTURE.md`. Add a flow here only once it is implemented.

## Create Session

```mermaid
sequenceDiagram
    participant Client
    participant API as api (POST /sessions)
    participant Identity as identity authorize
    participant Orch as orchestrator/playlifecycle
    participant Game as game checkplayability
    participant Session as play/session sessionlifecycle

    Client->>API: POST /sessions {game_uuid, idempotency_key} + Bearer token
    API->>Identity: Authorize(token, CREATE_SESSION)
    Identity-->>API: userUUID, allowed / unauthenticated (401) / forbidden (403)
    API->>Orch: CreateSession(userUUID, gameUUID, key)
    Orch->>Game: IsPlayable(gameUUID, userUUID)
    alt not playable
        Orch-->>API: game_unavailable
    else playable
        Orch->>Session: CreateSession(userUUID, gameUUID, key)
        Session-->>Orch: created / game_unavailable / ErrIdempotencyConflict
        Orch-->>API: Session's result as is
    end
    API-->>Client: 201 {session_uuid, join_code, lobby_expires_at} / 404 / 409 / 400
```

Implemented behavior:

- The API authenticates and checks coarse access through Identity, then forwards to the play-lifecycle orchestration; the creator is never part of the request body.
- The orchestration asks Game Management first, then the Session domain, which owns idempotency, the pinned version, the join code and the durable Session (see `play/docs/FLOWS.md`).
- Identity's authorization and Game Management's playability check are placeholders (fixed test tokens; always playable), recorded in each domain's `CURRENT_STATE.md`.
- Because the playability check runs before the Session domain looks at the idempotency key, a retry made after the Game stopped being playable answers `game_unavailable` instead of replaying the created Session.
