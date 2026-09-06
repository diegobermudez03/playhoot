# AI Context - Create Game Draft

Status: TEMPORARY / NON-CANONICAL / NON-AUTHORITATIVE

## Process

- Process: FEATURE_DEVELOPMENT
- Topic: create-game-draft
- Human review path: `docs/ai/workspaces/active/create-game-draft/HUMAN_REVIEW.md`
- Persistent WORK path: `docs/work/active/WORK-0001-create-game-draft.md`
- Persistent WORK status: DRAFT
- Current human checkpoint: decide whether to APPROVE, MODIFY, or REJECT the proposed public Game Management create-draft contract.

The public contract is PROPOSED, NOT HUMAN-APPROVED.

## Already-Approved Feature Constraints

- Game plus initial definition creation is atomic.
- A new Game is created as Draft.
- The initial definition is version `1` and becomes the Game's current definition.
- The initial definition is not published merely because it exists.
- Draft creation does not require proving Engine playability.
- Scope ends at Game Management.
- Excluded: API/HTTP, authentication, Identity/Profile implementation, AI generation, creation wizard, templates, publishing, discovery, sessions/rooms, multiplayer, editing, later versions, billing, analytics.
- Do not create game/game-definition history records merely because history tables exist.

## Relevant Canonical Sources

- `AGENTS.md`
- `docs/ai/OPERATING_MODEL.md`
- `docs/ai/KNOWLEDGE_MAP.md`
- `docs/ai/processes/FEATURE_DEVELOPMENT.md`
- `docs/ai/protocols/FEATURE_DEVELOPMENT.md`
- `docs/ai/workspaces/README.md`
- `docs/work/active/WORK-0001-create-game-draft.md`
- `game/README.md`
- `game/CURRENT_STATE.md`
- `game/docs/DATA_MODEL.md`
- `game/docs/FLOWS.md`
- `docs/engineering/standards/repositories.md`
- `docs/engineering/standards/error-handling.md`
- `docs/engineering/standards/data-integrity.md`
- `docs/engineering/standards/testing.md`

## Repository Evidence Inspected

- `game/game/models.go`: public `game.Game` model includes `UUID`, metadata, `OwnerUUID`, `LogoImageURL`, `Visibility`, `VersionUUID`, and `program.Definition`; visibility constants include `Draft`.
- `game/game/usecases/getgame/service.go`: existing Game Management use case convention uses `package getgame`, `type UseCase`, `New(db *gorm.DB) *UseCase`, and public method returning `*game.Game`.
- `game/game/usecases/getgame/repo.go`: repository reads current game plus current definition by joining `games.current_definition_id` to `game_definitions.id`.
- `game/game/internal/storage/tables.go`: storage structs include `games.current_definition_id`, `game_definitions.uuid`, `game_definitions.game_id`, `game_definitions.version_number`, `game_definitions.script`, and nullable `published_at`.
- `game/game/internal/storage/migrations/20260822000000_games.go`: `games` table has UUID, metadata, owner UUID, nullable current definition, logo URL, visibility.
- `game/game/internal/storage/migrations/20260822000001_game_definitions.go`: `game_definitions` table has UUID, game ID, version number, JSONB script, nullable published time, and unique `(game_id, version_number)`.
- `game/language/v1/program/definition.go`: `program.Definition` is the authored source-level game representation.
- `game/language/v1/program/gameservice/codec.go`: `EncodeJSON` encodes `program.Definition` to the existing compact JSON representation without semantic validation; `DecodeJSON` decodes persisted JSON back to `program.Definition`.
- `game/CURRENT_STATE.md` and `game/docs/FLOWS.md`: Game Management retrieval exists; create/edit/publish lifecycle operations were not found implemented.

## Current Public-Contract Proposal

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

Semantics:

- Caller provides authored content only: name, description, owner UUID, logo image URL, and initial definition.
- Game Management generates Game UUID.
- Game Management generates initial Definition UUID.
- Caller does not provide visibility, version number, published time, or current-definition ID.
- Game Management persists visibility as draft, definition version as `1`, `published_at` as null, and links the game's current definition to the created initial definition.
- Returned result should reuse `*game.Game` unless implementation later reveals a material reason for a separate DTO.
- Do not introduce new product validation rules for name, description, or logo URL unless an existing accepted contract already requires them.

## Rationale And Conclusions

- `Params` is the clearest public input shape for a multi-field creation operation and leaves room for future local expansion without positional churn.
- `CreateGameDraft` makes the approved Draft-only behavior explicit in the public method name.
- The contract prevents callers from injecting lifecycle/version/publication state owned by Game Management.
- Reusing `game.Game` matches current Game Management retrieval conventions and avoids creating a new public DTO without a demonstrated need.
- No repository evidence was found that requires changing the material proposal.

## Unresolved Material Decision

The public Game Management create-draft contract remains unresolved and must be decided by the human in `HUMAN_REVIEW.md`.

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

- No canonical product, architecture, domain, data-model, or engineering-standard documentation should change during the current design checkpoint.
- After implementation exists, expected current-state documentation updates are `game/CURRENT_STATE.md` and `game/docs/FLOWS.md`, as already listed in the DRAFT WORK.
- `docs/work/active/WORK-0001-create-game-draft.md` must remain DRAFT until the human approves READY.

## Next Protocol Action

After the human responds:

- If APPROVE: persist the contract decision into the DRAFT WORK, validate Definition of Ready, and prepare a READY REVIEW for human approval.
- If MODIFY: update the human review and DRAFT WORK only as authorized, then re-check for material decisions.
- If REJECT: record the rejection path in the workspace and ask for the replacement direction needed to continue.

Implementation must not begin until the human explicitly approves the WORK as READY.
