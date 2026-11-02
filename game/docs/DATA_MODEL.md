# Game Management - Data Model

Status: CURRENT IMPLEMENTATION

Game Management owns no script/rule-versioning table: `games.current_definition_id`/`game_definitions`/`game_definition_histories` were retired outright. No database transaction spans Game Management-owned and Session-Runtime-owned tables (`ARCHITECTURE.md -> State and Transaction Boundaries`).


```mermaid
classDiagram
    class games {
        id
        uuid
        name
        description
        owner_uuid
        logo_image_url
        visibility
        created_at
        updated_at
        deleted_at
        published_at
    }
    class game_editions {
        id
        uuid
        game_id
        status
        created_at
        published_at
    }
    class game_edition_status_changes {
        id
        edition_id
        from_status
        to_status
        changed_at
    }
    class game_create_requests {
        id
        owner_uuid
        idempotency_key
        game_id
        created_at
    }
    class game_images {
        id
        game_id
        image_url
        created_at
        removed_at
    }
    class game_histories {
        id
        game_id
        name
        description
        logo_image_url
        visibility
        is_published
        created_at
    }

    games "1" --> "*" game_images : "logical: game_images.game_id -> games.id"
    games "1" --> "*" game_histories : "logical: game_histories.game_id -> games.id"
    games "1" --> "*" game_editions : "logical: game_editions.game_id -> games.id"
    game_editions "1" --> "*" game_edition_status_changes : "logical: game_edition_status_changes.edition_id -> game_editions.id"
    games "1" --> "0..1" game_create_requests : "logical: game_create_requests.game_id -> games.id"
```

Added by the game creation project (2026-11-01, `docs/projects/active/game-creation-v1`):

- `games.published_at` is written once, when the Game is first published (data-history rule 1); null means unpublished. A Game is created private, with an empty card (`name`, `description`, `logo_image_url` empty) and its creation recorded in `game_histories` with every field (`is_published` false).
- `game_editions` are the versions of a Game being built or published. `uuid` is also the uuid of the Play version the edition is built in (Play owns the version, its sources and its readiness; Game Management only references it). `status` is `EDITING` or `PUBLISHED` (`game/internal/business/edition`); a Game has at most one `EDITING` edition (partial unique index `game_editions_one_editing_idx`). Every status change is appended to `game_edition_status_changes` in the same transaction (data-history rule 2); `published_at` is written once.
- `game_create_requests` are the idempotency requests of Game creation, unique per (`owner_uuid`, `idempotency_key`); `game_id` is written once, in the transaction that created the Game.

## Relationship Types

No database-enforced foreign key constraints were found in the current Game Management migration SQL.

Logical persisted references:

- `game_images.game_id -> games.id`
- `game_histories.game_id -> games.id`

External logical identifiers:

- `games.owner_uuid`
