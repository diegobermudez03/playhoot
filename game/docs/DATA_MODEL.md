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
```

## Relationship Types

No database-enforced foreign key constraints were found in the current Game Management migration SQL.

Logical persisted references:

- `game_images.game_id -> games.id`
- `game_histories.game_id -> games.id`

External logical identifiers:

- `games.owner_uuid`
