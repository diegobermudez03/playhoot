# Game Management - Data Model

Status: CURRENT IMPLEMENTATION

Game Management owns no script/rule-versioning table: the playable versions, their sources and builds are Play's. No database transaction spans Game Management-owned and Session-Runtime-owned tables (`ARCHITECTURE.md -> State and Transaction Boundaries`).


```mermaid
classDiagram
    class games {
        id
        uuid
        name
        description
        owner_uuid
        visibility
        created_at
        updated_at
        deleted_at
        published_at
        template_allowed
        template_featured
    }
    class game_editions {
        id
        uuid
        game_id
        status
        template_game_uuid
        template_version_uuid
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
        name
        game_id
        created_at
    }
    class game_histories {
        id
        game_id
        name
        description
        visibility
        is_published
        created_at
    }

    games "1" --> "*" game_histories : "logical: game_histories.game_id -> games.id"
    games "1" --> "*" game_editions : "logical: game_editions.game_id -> games.id"
    game_editions "1" --> "*" game_edition_status_changes : "logical: game_edition_status_changes.edition_id -> game_editions.id"
    games "1" --> "0..1" game_create_requests : "logical: game_create_requests.game_id -> games.id"
```

Added by the game creation project (2026-11-01, `docs/projects/active/game-creation-v1`):

- `games.published_at` is written once, when the Game is first published (data-history rule 1); null means unpublished. A Game is created private, named, with an empty description, and its creation recorded in `game_histories` with every field (`is_published` false).
- `game_editions` are the versions of a Game being built or published. `uuid` is also the uuid of the Play version the edition is built in (Play owns the version, its sources and its readiness; Game Management only references it). `status` is `EDITING` or `PUBLISHED` (`game/internal/business/edition`); a Game has at most one `EDITING` edition (partial unique index `game_editions_one_editing_idx`). Every status change is appended to `game_edition_status_changes` in the same transaction (data-history rule 2); `published_at` is written once.
- `game_create_requests` are the idempotency requests of Game creation, unique per (`owner_uuid`, `idempotency_key`); `game_id` is written once, in the transaction that created the Game.

## Relationship Types

No database-enforced foreign key constraints were found in the current Game Management migration SQL.

Logical persisted references:

- `game_histories.game_id -> games.id`

External logical identifiers:

- `games.owner_uuid`


## Card, publication and editions (2026-11-02)

Migration `20261103000000_game_publishing` (game creation project, item J):

- `games.name` and `games.description` are `TEXT`; their limits (80 and 2000 characters) are in `game/internal/business/card`. `game_histories.name` and `.description` are `TEXT` too. A Game is named when created (`game_create_requests.name` keeps the name a creation was asked with, to tell a retry from another request).
- `game_card_images` (`uuid`, `game_id`, `kind` logo|photo, `sha256`, `size`, `mime_type`, `object_key`, `created_at`, `removed_at` written once): the images a card shows; at most one logo and eight photos not removed. Being write-once (`created_at`, `removed_at`), its rows are also the history of the logo and photos (data-history rule 1), so `game_histories` records only the card texts, visibility and publication.
- `game_card_objects` (`sha256` unique, `object_key`, `size`, `mime_type`, `created_at`, `registered_at`, `deleted_at`): every card image object stored, registered before the bytes are stored (`docs/engineering/standards/object-storage.md`).
- `game_card_image_requests` (`game_id`, `idempotency_key` unique together, `kind`, `sha256`, `image_id` written once): idempotency of image uploads.
- `game_releases` (`uuid`, `game_id`, `edition_id`, `sequence` unique per game, `revision`, `visibility`, `owner_uuid`, `idempotency_key` unique per game, `created_at`, `completed_at` / `abandoned_at` + `abandon_reason` written once, `languages` written with `completed_at`: the JSON list of the languages the published version's game is written in, as Play froze it, 2026-11-02): each publication of an edition at a revision of its Play version; at most one with neither end per game (partial unique index). A release is "being published" until completed or abandoned.
- `game_edition_requests` (`game_id`, `idempotency_key` unique together, `edition_id` written once): idempotency of opening a new edition.
- Publishing completes a release by moving its edition `EDITING` → `PUBLISHED` (with `game_editions.published_at` and a status change), writing `games.published_at` once at the first publication, and setting `games.visibility` (history in `game_histories`).
