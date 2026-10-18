# Session Runtime - Data Model

Status: CURRENT IMPLEMENTATION

Schema owned by `play/session`, created by `play/session/internal/migrations`. Every table is prefixed with `play_`. There are no foreign keys: the relationships below are indexed columns kept correct by the application layer.

```mermaid
erDiagram
    play_games {
        BIGSERIAL id PK
        UUID game_uuid "unique; same identity as Game Management's Game"
        BIGINT current_version_id "nullable; play_game_versions.id"
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }
    play_game_versions {
        BIGSERIAL id PK
        UUID uuid "unique"
        BIGINT game_id "play_games.id"
        TEXT status "DRAFT | EXECUTABLE"
        JSONB manifest "versioned runtime manifest"
        TEXT script_object_key "nullable; backend script in object storage"
        TEXT script_sha256 "nullable"
        BIGINT script_size "nullable"
        TEXT frontend_object_key "nullable; frontend package in object storage"
        TEXT frontend_sha256 "nullable"
        BIGINT frontend_size "nullable"
        TIMESTAMPTZ executable_at "nullable; when it became executable"
        TIMESTAMPTZ created_at
    }
    play_sessions {
        BIGSERIAL id PK
        UUID uuid "unique"
        BIGINT game_version_id "play_game_versions.id"
        UUID created_by_user_uuid "Identity User"
        TEXT status "LOBBY | RUNNING | FINISHED | CANCELLED | EXPIRED | ABORTED"
        TEXT end_cause "nullable"
        TIMESTAMPTZ lobby_expires_at
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }
    play_session_join_codes {
        BIGSERIAL id PK
        BIGINT session_id "play_sessions.id"
        TEXT code "unique among rows with released_at NULL"
        TIMESTAMPTZ expires_at
        TIMESTAMPTZ released_at "nullable; set when no longer active"
        TIMESTAMPTZ created_at
    }
    play_session_participants {
        BIGSERIAL id PK
        BIGINT session_id "play_sessions.id; unique with participant_id and with user_uuid"
        UUID user_uuid "Identity User"
        TEXT participant_id "P1..Pn, what the game knows the player as"
        TIMESTAMPTZ created_at
    }
    play_session_checkpoints {
        BIGSERIAL id PK
        BIGINT session_id "unique; play_sessions.id"
        BIGINT context_revision
        BYTEA content "opaque; encoded by the live Runtime"
        TIMESTAMPTZ created_at
        TIMESTAMPTZ updated_at
    }
    play_create_session_requests {
        BIGSERIAL id PK
        UUID user_uuid "unique with idempotency_key"
        TEXT idempotency_key
        UUID game_uuid
        TEXT outcome "nullable until completed; created | game_unavailable"
        BIGINT session_id "nullable; play_sessions.id"
        TIMESTAMPTZ created_at
    }

    play_games ||--o{ play_game_versions : "play_game_versions.game_id = play_games.id"
    play_games }o--o| play_game_versions : "play_games.current_version_id = play_game_versions.id"
    play_game_versions ||--o{ play_sessions : "play_sessions.game_version_id = play_game_versions.id"
    play_sessions ||--o{ play_session_join_codes : "play_session_join_codes.session_id = play_sessions.id"
    play_sessions ||--o{ play_session_participants : "play_session_participants.session_id = play_sessions.id"
    play_sessions ||--o| play_session_checkpoints : "play_session_checkpoints.session_id = play_sessions.id"
    play_sessions |o--o{ play_create_session_requests : "play_create_session_requests.session_id = play_sessions.id"
```

Notes:

- A Game's `current_version_id` must point to an `EXECUTABLE` version (immutable, scripts ready to run; whether the Game is published is Game Management's concern, not this domain's); this is checked by the application, and a violation is treated as broken data.
- `play_create_session_requests` rows are claimed and completed in the same transaction that creates the Session, so a committed row always has an `outcome`.
- `play_game_versions.manifest` is the version's runtime manifest: the structural facts the live Runtime needs, as `{"schema_version":1,"players":{"min":N,"max":M}}` (1 <= min <= max). It grows by adding fields; a pinned (EXECUTABLE) version must hold a valid one, and an invalid one is treated as broken data.
- A join code is active while `released_at` is NULL. An expired code is released lazily when a new Session needs that same code.
- An EXECUTABLE version must have both locators (`script_*`, `frontend_*`: object key, SHA-256 as lowercase hex, size in bytes); this is checked by the application, and a missing one is treated as broken data. Keys are content-addressed under `game-versions/backend-scripts/` and `game-versions/frontend-packages/`. The frontend package is one self-contained HTML file (decided by the human 2026-10-17 for v1); a game's assets will be resolved separately by Playhoot.
- `play_session_participants` and the first `play_session_checkpoints` row are written in the same transaction that makes a Session RUNNING, so a RUNNING Session always has its roster and a recoverable checkpoint. A checkpoint row is only replaced by one of a higher `context_revision`; there is no history (replay is a separate mechanism).
- `end_cause` is set with a terminal status; the only value written today is `GAME_START_FAILURE` (with ABORTED).
- Starting or aborting a Session releases its join codes.
