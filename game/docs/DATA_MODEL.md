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

Status: this replaces the pre-Slice-1 `sessions`/`session_players`/`session_states`/`join_codes` shape - see WORK-0001's Data/Migration Impact. Slice 2 (WORK-0003) added `session_runtime_turns`/`session_runtime_steps` and `sessions.current_turn_id` (GAME-ADR-0023 - a logical, non-DB-enforced pointer to the current authoritative RuntimeTurn, colocated on `sessions` rather than a separate `session_runtime_state` table). Slice 3 (WORK-0004) added `session_interactions`. The full accepted design (including the not-yet-implemented timer/failure tables) remains recorded in `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`.

```mermaid
classDiagram
    class sessions {
        id
        uuid
        game_definition_uuid
        host_actor_id
        phase
        lobby_expires_at
        started_at
        current_turn_id
        terminal_at
        terminal_reason
        created_at
        updated_at
    }
    class session_actors {
        id
        session_id
        user_uuid
        semantic_presence
        created_at
    }
    class session_participants {
        id
        session_actor_id
        display_name
        active
        joined_at
        left_at
    }
    class join_codes {
        id
        session_id
        code
        created_at
        revoked_at
    }
    class session_requests {
        id
        operation
        idempotency_key
        user_uuid
        session_id
        request_payload
        outcome
        response_payload
        status
        created_at
    }
    class session_runtime_turns {
        id
        session_id
        sequence
        source_kind
        source_interaction_id
        source_timer_obligation_id
        actor_id
        snapshot_payload
        snapshot_format_version
        created_at
    }
    class session_runtime_steps {
        id
        runtime_turn_id
        step_index
        commit_payload
        created_at
    }
    class session_interactions {
        id
        uuid
        session_id
        session_actor_id
        kind
        engine_path
        engine_slot
        interaction_payload
        response_payload
        state
        opened_by_turn_id
        closed_by_turn_id
        closure_reason
        created_at
    }

    sessions "1" --> "*" session_actors : "session_actors.session_id -> sessions.id"
    sessions "0..1 host" --> "1" session_actors : "sessions.host_actor_id -> session_actors.id"
    session_actors "1" --> "0..1" session_participants : "session_participants.session_actor_id -> session_actors.id"
    sessions "1" --> "*" join_codes : "join_codes.session_id -> sessions.id"
    sessions "0..1" --> "*" session_requests : "session_requests.session_id -> sessions.id"
    sessions "1" --> "*" session_runtime_turns : "session_runtime_turns.session_id -> sessions.id"
    session_runtime_turns "0..1" --> "*" sessions : "sessions.current_turn_id -> session_runtime_turns.id (logical, non-DB-enforced)"
    session_runtime_turns "1" --> "*" session_runtime_steps : "session_runtime_steps.runtime_turn_id -> session_runtime_turns.id"
    sessions "1" --> "*" session_interactions : "session_interactions.session_id -> sessions.id"
    session_actors "1" --> "*" session_interactions : "session_interactions.session_actor_id -> session_actors.id"
    session_runtime_turns "1 opens" --> "*" session_interactions : "session_interactions.opened_by_turn_id -> session_runtime_turns.id"
    session_runtime_turns "0..1 closes" --> "*" session_interactions : "session_interactions.closed_by_turn_id -> session_runtime_turns.id"
    session_interactions "0..1 causes" --> "*" session_runtime_turns : "session_runtime_turns.source_interaction_id -> session_interactions.id"
```

`phase` is `LOBBY | RUNNING | TERMINAL` in the currently implemented behavior (Create/Join/Leave/Start/AnswerInteraction). `started_at` is set only once Start commits a Session's first RuntimeTurn; it remains `NULL` for a Session still in `LOBBY` or one that fatally terminalized before ever running (`terminal_reason` = `RUNTIME_STATE_INVALID` or `RUNTIME_EXECUTION_FAILED`). `session_runtime_turns.source_interaction_id`/`actor_id` are populated by AnswerInteraction's own caused Turn; `source_timer_obligation_id` is always `NULL` in the currently implemented behavior (Slice 5 populates it for its own cause). `sequence` currently reaches 2 for a Session with one answered interaction (Start's first Turn, then AnswerInteraction's); no later slice that would advance it further is implemented yet. `session_interactions.kind` is `QUESTION | ASK_GROUP`; `state` is `ACTIVE | CLOSED | TERMINATED` in this codebase's own chosen vocabulary (`CLOSED` for ordinary Turn-produced closure, `TERMINATED` for this slice's own narrow terminal-cleanup closure - `closed_by_turn_id` `NULL` + `closure_reason = SESSION_TERMINATED`); GAME-ADR-0019 leaves the exact enum naming an implementation-planning detail. `engine_path`/`interaction_payload`/`response_payload` are JSONB, encoding `engine.Value`-typed data through `engineservice.EncodeValue`/`DecodeValue`, never plain `encoding/json`.

## Relationship Types

No database-enforced foreign key constraints were found in the current Game migration SQL.

Logical persisted references:

- `game_definitions.game_id -> games.id`
- `game_images.game_id -> games.id`
- `game_histories.game_id -> games.id`
- `game_definition_histories.game_definition_id -> game_definitions.id`
- `games.current_definition_id -> game_definitions.id`
- `session_actors.session_id -> sessions.id`
- `sessions.host_actor_id -> session_actors.id`
- `session_participants.session_actor_id -> session_actors.id`
- `join_codes.session_id -> sessions.id`
- `session_requests.session_id -> sessions.id`
- `session_runtime_turns.session_id -> sessions.id`
- `session_runtime_steps.runtime_turn_id -> session_runtime_turns.id`
- `sessions.current_turn_id -> session_runtime_turns.id` (GAME-ADR-0023)
- `session_interactions.session_id -> sessions.id`
- `session_interactions.session_actor_id -> session_actors.id`
- `session_interactions.opened_by_turn_id -> session_runtime_turns.id`
- `session_interactions.closed_by_turn_id -> session_runtime_turns.id` (nullable)
- `session_runtime_turns.source_interaction_id -> session_interactions.id` (nullable)

Logical cross-capability references within Game (no database FK, same bounded context, independent persistence/transaction ownership per GAME-ADR-0001):

- `sessions.game_definition_uuid -> game_definitions.uuid` (Game Management capability, read via the narrow `getgamedefinition`/`getgame` read capabilities, never queried directly from Session Runtime persistence)

Logical cross-domain references (no database FK, different bounded context):

- `session_actors.user_uuid -> Identity.User` (Identity)
- `session_requests.user_uuid -> Identity.User` (Identity)

External logical identifiers:

- `games.owner_uuid`
