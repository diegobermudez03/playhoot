# AI Context - Create Game Draft

Status: TEMPORARY / NON-CANONICAL / NON-AUTHORITATIVE

## Process

- Process: FEATURE_DEVELOPMENT
- Topic: create-game-draft
- Human review path: `docs/ai/workspaces/active/create-game-draft/HUMAN_REVIEW.md`
- Human review status: READY REVIEW
- Persistent WORK path: `docs/work/active/WORK-0001-create-game-draft.md`
- Persistent WORK status: DRAFT
- Current human checkpoint: READY approval.
- Implementation has not begun.

## Human Decisions

The public Game Management create-draft contract is HUMAN-APPROVED.

The prior material decision checkpoint about the public create-draft contract is resolved.

Approved public contract:

```go
package creategame

import (
    "context"

    "github.com/diegobermudez03/playhoot/game/game"
    "github.com/diegobermudez03/playhoot/game/language/v1/program"
)

type Params struct {
    Name         string
    Description  string
    OwnerUUID    string
    LogoImageURL string
    Definition   program.Definition
}

func (c *UseCase) CreateGameDraft(
    ctx context.Context,
    params Params,
) (*game.Game, error)
```

Approved semantics:

- Caller provides `Name`, `Description`, `OwnerUUID`, `LogoImageURL`, and the initial `program.Definition`.
- Game Management generates the Game UUID and initial Definition UUID.
- Caller does not control `Visibility`, `VersionNumber`, `PublishedAt`, or `CurrentDefinitionID`.
- Game Management applies Draft visibility, definition version `1`, unpublished initial definition, and current-definition linkage.
- The operation returns `*game.Game`.
- Do not add new product validation rules for name, description, or logo URL unless an existing accepted contract already requires them.

## Already-Approved Feature Constraints

- Game plus initial definition creation is atomic.
- A new Game is created as Draft.
- The initial definition is version `1` and becomes the Game's current definition.
- The initial definition is not published merely because it exists.
- Draft creation does not require proving Engine playability.
- Scope ends at Game Management.
- Excluded: API/HTTP, authentication, Identity/Profile implementation, AI generation, creation wizard, templates, publishing, discovery, sessions/rooms, multiplayer, editing, later versions, billing, analytics.
- Do not create game/game-definition history records merely because history tables exist.

## Relevant Sources

- `AGENTS.md`
- `docs/ai/OPERATING_MODEL.md`
- `docs/ai/KNOWLEDGE_MAP.md`
- `docs/ai/processes/FEATURE_DEVELOPMENT.md`
- `docs/ai/protocols/FEATURE_DEVELOPMENT.md`
- `docs/ai/workspaces/README.md`
- `docs/work/README.md`
- `docs/work/active/WORK-0001-create-game-draft.md`
- `game/README.md`
- `game/CURRENT_STATE.md`
- `game/docs/DATA_MODEL.md`
- `game/docs/FLOWS.md`
- `docs/engineering/standards/repositories.md`
- `docs/engineering/standards/error-handling.md`
- `docs/engineering/standards/data-integrity.md`
- `docs/engineering/standards/testing.md`

## Repository Evidence Already Inspected

- `game/game/models.go`: public `game.Game` model includes `UUID`, metadata, `OwnerUUID`, `LogoImageURL`, `Visibility`, `VersionUUID`, and `program.Definition`; visibility constants include `Draft`.
- `game/game/usecases/getgame/service.go`: existing Game Management use case convention uses `package getgame`, `type UseCase`, `New(db *gorm.DB) *UseCase`, and a public method returning `*game.Game`.
- `game/game/usecases/getgame/repo.go`: repository reads current game plus current definition by joining `games.current_definition_id` to `game_definitions.id`.
- `game/game/internal/storage/tables.go`: storage structs include `games.current_definition_id`, `game_definitions.uuid`, `game_definitions.game_id`, `game_definitions.version_number`, `game_definitions.script`, and nullable `published_at`.
- `game/game/internal/storage/migrations/20260822000000_games.go`: `games` table has UUID, metadata, owner UUID, nullable current definition, logo URL, visibility.
- `game/game/internal/storage/migrations/20260822000001_game_definitions.go`: `game_definitions` table has UUID, game ID, version number, JSONB script, nullable published time, and unique `(game_id, version_number)`.
- `game/language/v1/program/definition.go`: `program.Definition` is the authored source-level game representation.
- `game/language/v1/program/gameservice/codec.go`: `EncodeJSON` encodes `program.Definition` to the existing compact JSON representation without semantic validation; `DecodeJSON` decodes persisted JSON back to `program.Definition`.
- `game/CURRENT_STATE.md` and `game/docs/FLOWS.md`: Game Management retrieval exists; create/edit/publish lifecycle operations were not found implemented.

## Definition Of Ready Result

PASS.

Validation against `docs/work/README.md`:

- Outcome is clear.
- In-scope and out-of-scope boundaries are clear.
- Required product decisions are resolved.
- Required architecture/domain decisions are resolved.
- Required engineering-standard decisions are resolved.
- Important task-specific material design decisions are approved.
- Public contract is resolved.
- Persistence and atomicity semantics are resolved.
- Constraints and invariants are explicit.
- Acceptance criteria are observable and testable.
- Required verification is identified.
- Documentation impact is identified.
- No unresolved MATERIAL blocker remains.
- Local implementation choices are distinguished from human decisions.

## Remaining Blockers

None.

No hidden material decision exists only in this file.

## Likely Implementation Touchpoints

- New use case package likely under `game/game/usecases/creategame/`.
- Service constructor likely follows `getgame.New(db *gorm.DB) *UseCase`.
- Repository implementation likely uses a transaction to create `games`, create `game_definitions`, and update `games.current_definition_id`.
- Definition persistence should likely use `gameservice.EncodeJSON(params.Definition)`.
- UUID generation mechanism must follow existing project conventions found during implementation.
- Repository tests should use `game/game/internal/testdb`.

## Expected Verification Considerations

- Service-level success and error-path tests.
- Repository-level tests with disposable Game test DB.
- Persisted-state verification for game row, definition row, current-definition linkage, draft visibility, version `1`, and null `published_at`.
- Atomic failure test proving no partial game/definition remains.
- Verification that draft creation does not compile or execute the definition as an Engine playability prerequisite.
- Relevant Game Management tests should run during implementation; broader package tests may remain blocked by unrelated Session Runtime scaffolding noted in `game/CURRENT_STATE.md`.

## Expected Documentation Synchronization

- No canonical product, architecture, domain, data-model, or engineering-standard documentation should change during this READY checkpoint.
- After implementation exists, expected current-state documentation updates are `game/CURRENT_STATE.md` and `game/docs/FLOWS.md`, as listed in the DRAFT WORK.
- `docs/work/active/WORK-0001-create-game-draft.md` must remain DRAFT until the human approves READY.

## Next Protocol Action

If the human replies READY:

- Persist `WORK-0001` status transition from DRAFT to READY.
- Perform any workspace cleanup allowed by the Feature Development protocol once the durable WORK is sufficient.
- Provide the minimal implementation handoff from `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`.

Implementation must not begin until the human explicitly approves the WORK as READY.
