# Game - Data Model

Status: CURRENT IMPLEMENTATION

## Game Management Tables

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

## Session Runtime Tables

```mermaid
classDiagram
    class sessions {
        id
        uuid
        game_definition_uuid
        owner_uuid
        started_at
        ended_at
        created_at
    }
    class session_players {
        id
        session_id
        player_uuid
        joined_at
        left_at
        created_at
        updated_at
    }
    class join_codes {
        id
        code
        session_id
        created_at
        deleted_at
    }
    class session_states {
        id
        state_number
        session_id
        json_state
        created_at
    }
    class game_definitions {
        uuid
    }

    sessions "1" --> "*" session_states : "logical: session_states.session_id -> sessions.id"
    sessions "1" --> "*" session_players : "logical: session_players.session_id -> sessions.uuid"
    sessions "1" --> "*" join_codes : "logical: join_codes.session_id -> sessions.uuid"
    sessions "*" --> "1" game_definitions : "logical: sessions.game_definition_uuid -> game_definitions.uuid"
```

## Relationship Types

No database-enforced foreign key constraints were found in the current Game migration SQL.

Logical persisted references:

- `game_definitions.game_id -> games.id`
- `game_images.game_id -> games.id`
- `game_histories.game_id -> games.id`
- `game_definition_histories.game_definition_id -> game_definitions.id`
- `games.current_definition_id -> game_definitions.id`
- `session_states.session_id -> sessions.id`
- `session_players.session_id -> sessions.uuid`
- `join_codes.session_id -> sessions.uuid`
- `sessions.game_definition_uuid -> game_definitions.uuid`

External logical identifiers:

- `games.owner_uuid`
- `sessions.owner_uuid`
- `session_players.player_uuid`
