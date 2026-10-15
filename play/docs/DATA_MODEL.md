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
    play_sessions |o--o{ play_create_session_requests : "play_create_session_requests.session_id = play_sessions.id"
```

Notes:

- A Game's `current_version_id` must point to an `EXECUTABLE` version (immutable, scripts ready to run; whether the Game is published is Game Management's concern, not this domain's); this is checked by the application, and a violation is treated as broken data.
- `play_create_session_requests` rows are claimed and completed in the same transaction that creates the Session, so a committed row always has an `outcome`.
- A join code is active while `released_at` is NULL. An expired code is released lazily when a new Session needs that same code.
