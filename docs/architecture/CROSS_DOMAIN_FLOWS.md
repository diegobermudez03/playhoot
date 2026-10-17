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

## Admin Join

```mermaid
sequenceDiagram
    participant Client
    participant API as api (GET /sessions/{sessionUUID}/admin)
    participant Identity as identity authorize
    participant Owner as play/runtime ownership
    participant Live as play/runtime liveconnection

    Client->>API: WebSocket handshake, subprotocols playhoot.v1 + bearer token
    API->>Identity: Authorize(token, JOIN_SESSION_AS_ADMIN)
    Identity-->>API: userUUID, allowed / unauthenticated (401) / forbidden (403)
    API->>Owner: LocateOrClaimOwner(sessionUUID)
    Owner-->>API: owned here (epoch) / session unavailable (404) / owned elsewhere (503, no forwarding yet)
    API->>API: upgrade (playhoot.v1, allowed origin)
    API->>Live: AttachAdmin(sessionUUID, userUUID, epoch)
    Live->>Live: EligibleToJoinAsAdmin (play/session), then find or create the Session's controller
    alt rejected
        Live-->>API: rejected + reason
        API->>Client: rejected message, close 1008
    else accepted
        Live-->>API: AdminConnection operations
        par client requests
            Client->>API: select_perspective / cancel_session
            API->>Live: SelectPerspective / CancelSession
            API->>Client: result message
        and views
            API->>Live: NextView (blocks)
            API->>Client: player_view message
        and end
            API->>Live: Ended (blocks)
            API->>Client: close with the end reason
        end
        API->>Live: Disconnect
    end
```

Implemented behavior:

- Identity and owner routing both happen before the upgrade, in that order, so an unauthorized caller never makes a Session owned. Logical admission happens after the upgrade, in the Runtime.
- The Runtime defines each kind of connection's operations and types and never sees the socket; each API endpoint maps them to and from its own JSON messages (`api/play/admin_join.go`).
- Ownership (`LocateOrClaimOwner`) is a placeholder today (every Session is owned by this instance under epoch 1). Admission is real on the Runtime side (eligibility, controller find-or-create, ADMIN connection registered), but the durable admin eligibility check answers eligible for everyone, and the admin operations answer "not implemented" (`play/CURRENT_STATE.md`). Identity's authorization is the same placeholder as for Create Session.
