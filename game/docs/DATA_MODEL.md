# Game Management - Data Model

Status: CURRENT IMPLEMENTATION

Session Runtime's own persisted copy of the executable Game Version Artifact (`session_games`/`session_game_version_artifacts`) is documented in `session/docs/DATA_MODEL.md`, not here - no database transaction spans Game Management-owned and Session-Runtime-owned tables (`ARCHITECTURE.md -> State and Transaction Boundaries`).

```mermaid
classDiagram
    class games {
        id
        uuid
        name
        description
        owner_uuid
        current_definition_id
        logo_image_url
        visibility
        created_at
        updated_at
        deleted_at
    }
    class game_definitions {
        id
        uuid
        game_id
        version_number
        script
        published_at
        created_at
        updated_at
        disabled_at
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
    class game_definition_histories {
        id
        game_definition_id
        script
        published_at
        disabled_at
        created_at
    }

    games "1" --> "*" game_definitions : "logical: game_definitions.game_id -> games.id"
    games "1" --> "*" game_images : "logical: game_images.game_id -> games.id"
    games "1" --> "*" game_histories : "logical: game_histories.game_id -> games.id"
    game_definitions "1" --> "*" game_definition_histories : "logical: game_definition_histories.game_definition_id -> game_definitions.id"
    games "0..1 current" --> "1" game_definitions : "logical: games.current_definition_id -> game_definitions.id"
```

## Relationship Types

No database-enforced foreign key constraints were found in the current Game Management migration SQL.

Logical persisted references:

- `game_definitions.game_id -> games.id`
- `game_images.game_id -> games.id`
- `game_histories.game_id -> games.id`
- `game_definition_histories.game_definition_id -> game_definitions.id`
- `games.current_definition_id -> game_definitions.id`

External logical identifiers:

- `games.owner_uuid`
