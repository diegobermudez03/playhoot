# WORK-0001: Create Game Draft With Initial Definition

Status: READY
Created: 2026-09-06
Last status change: 2026-09-06

Related decisions:
- None.

Canonical context:
- `game/README.md`
- `game/CURRENT_STATE.md`
- `game/docs/DATA_MODEL.md`
- `game/docs/FLOWS.md`
- `docs/engineering/standards/repositories.md`
- `docs/engineering/standards/error-handling.md`
- `docs/engineering/standards/data-integrity.md`
- `docs/engineering/standards/testing.md`

## Outcome

Add the Game Management operation needed to persist a newly authored game draft together with its initial definition/version.

This lets Playhoot create the first durable authored-game record for a pilot creation flow while keeping publication, playability validation, transport/API, identity, AI generation, and session execution outside this work.

## Context

Game owns authored games, their definitions/versions, metadata, and visibility/publication state. Current implementation has Game Management storage, migrations, and retrieval of a playable game with its current version, but create/edit/publish lifecycle operations were not found implemented.

The current Game Management schema already contains:

- `games` with `uuid`, `name`, `description`, `owner_uuid`, nullable `current_definition_id`, `logo_image_url`, and `visibility`.
- `game_definitions` with `uuid`, `game_id`, `version_number`, `script`, nullable `published_at`, and nullable `disabled_at`.
- A logical current-definition reference from `games.current_definition_id` to `game_definitions.id`.
- A uniqueness constraint on `game_definitions (game_id, version_number)`.
- History tables for games and game definitions, with no accepted creation-time history rule.

Existing Game code exposes `game.Draft`, `game.Private`, `game.Hidden`, and `game.Public` visibility values, with only `Hidden` and `Public` considered playable by `businessservice.IsPlayableVisibility`.

## Scope

### In Scope

- Add a Game Management creation capability for a new game draft plus its initial definition.
- Persist the `games` row as `draft`.
- Persist the initial `game_definitions` row for the new game as version `1`.
- Store the initial definition script using the existing Game Language v1 definition JSON representation.
- Establish the new game's `current_definition_id` reference to the initial definition.
- Return a created-game result containing the identifiers and authored data needed by existing Game Management conventions.
- Add focused service/repository tests and persisted-state verification for the new behavior.

### Out of Scope

- HTTP/API endpoint.
- Google authentication.
- Identity/Profile implementation or boundary decisions.
- AI game generation.
- Creation wizard.
- Template selection.
- Image/file upload.
- Publishing.
- Public discovery/search.
- Session/room creation.
- Multiplayer execution.
- Editing an existing Game.
- Creating later Game versions.
- Billing/limits.
- Analytics.
- Schema changes, unless implementation later discovers the approved behavior cannot be implemented safely with the current schema and the work returns to DRAFT for human decision.
- ADR or PDR creation.

## Approved Design

- Creating the game and its initial definition is one atomic logical/database operation.
- On success, persistence contains a complete new game, a complete initial definition, and the game points at that definition as current.
- On failure of any required persistence step, no partial game creation may remain.
- The newly created game starts with `game.Draft` / stored visibility `draft`.
- The draft is not created as `public`, `private`, or `hidden`.
- Publishing remains a separate future operation and is not implemented here.
- The initial definition version is `1`.
- The initial definition becomes the game's current definition after successful creation.
- The initial definition is not published merely because it exists; `published_at` remains unset/null under the current schema.
- Creating a draft does not require proving the definition can execute successfully in the Game Language Engine.
- The operation must not compile or execute the definition as a prerequisite for draft creation.
- This work does not introduce creation-time writes to `game_histories` or `game_definition_histories` solely because those tables exist.
- The operation may accept an owner identifier required by the existing Game model, but this does not resolve Identity/Profile ownership or authentication boundaries.

### Public Create-Draft Contract

The human-approved public Game Management contract is:

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

Caller-provided authored content:

- `Name`
- `Description`
- `OwnerUUID`
- `LogoImageURL`
- initial `program.Definition`

Game Management-generated or controlled values:

- Game UUID
- initial Definition UUID
- visibility, always `game.Draft` / stored `draft`
- initial definition version number, always `1`
- publication state, with `published_at` unset/null
- current-definition linkage from the created Game to the created initial definition

The caller must not provide or control `Visibility`, `VersionNumber`, `PublishedAt`, or `CurrentDefinitionID`.

Do not add new product validation rules for `Name`, `Description`, or `LogoImageURL` in this WORK unless an existing accepted contract already requires them.

## Constraints and Invariants

- Use the existing Game Management schema and model naming where possible.
- The operation belongs inside the accepted Game bounded context, under Game Management.
- The game row and initial definition row must be committed or rolled back together.
- `games.visibility` for created games must be `draft`.
- `game_definitions.version_number` for the initial definition must be `1`.
- `games.current_definition_id` must reference the newly created `game_definitions.id`.
- `game_definitions.game_id` must reference the newly created `games.id`.
- `game_definitions.published_at` must remain null for the initial draft definition.
- The stored definition script must satisfy the existing serialization/structural persistence contract used for `game_definitions.script`.
- Do not require Game Language Engine compile/step/playability validation for draft creation.
- Preserve the current error-handling, repository, data-integrity, and testing standards referenced above.

## Acceptance Criteria

- Creating a valid draft request persists exactly one new `games` row for that request.
- The persisted game has visibility `draft`.
- Creating a valid draft request persists exactly one initial `game_definitions` row for the new game.
- The persisted initial definition has `version_number = 1`.
- The persisted initial definition is linked to the created game by `game_id`.
- The created game points to the initial definition through `current_definition_id`.
- The persisted initial definition is not marked published under the current schema.
- The created result returns `*game.Game` and exposes the created game identity and current definition/version identity through that approved public contract.
- The persisted definition can be read back as the authored definition representation expected by current Game Language v1 storage conventions.
- If any required step in game creation, definition creation, or current-definition assignment fails, the operation returns an error and leaves no partially created game or initial definition for that request.
- Repository/service error behavior follows the accepted error-handling standard, including intentional sentinel wrapping only for deliberate public error contracts.
- No creation records are written to `game_histories` or `game_definition_histories` by this operation unless separate approved work later establishes that invariant.
- Existing playable-game retrieval behavior remains unchanged.

## Implementation Freedom

Codex retains the normal implementation autonomy defined by `docs/ai/OPERATING_MODEL.md`.

Unless a material repository constraint is discovered during implementation, local choices remain open, including file placement within the approved `creategame` package, private helper decomposition, private types, private/local method and helper names, test fixture organization, mock setup details, transaction helper use, and internal error-context wording consistent with accepted standards.

Codex may add straightforward package plumbing, such as a constructor following established Game Management conventions, when that does not materially change the approved public contract.

Codex may not rename or materially change the approved public package contract, `Params` fields/types, `CreateGameDraft` method, or `*game.Game` return contract without a DISCOVERY and material reapproval.

If implementation later reveals a genuinely material new public-contract choice, it must be surfaced before implementation proceeds beyond the valid local design space.

## Verification

- Add service-level tests for successful draft creation and relevant error paths, following the canonical service test style.
- Add repository-level tests using the shared disposable Game test database infrastructure.
- Repository write tests must verify both the returned operation result and actual persisted state.
- Verify atomic failure behavior by causing a required multi-step persistence failure and asserting no partial persisted game/definition remains for the attempted creation.
- Verify that the successful persisted state includes draft visibility, initial definition version `1`, current-definition linkage, and null/unset published state.
- Verify that draft creation does not call Game Language Engine compilation/execution as a playability prerequisite.
- Run the relevant Game Management test set during implementation. If broader package tests remain blocked by unrelated current Session Runtime scaffolding, report that limitation rather than hiding it.

## Documentation Impact

### Accepted / Canonical Knowledge

- None expected. This work implements already approved task-local behavior and does not require updating canonical product, architecture, domain, data-model, or engineering-standard knowledge.

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md` - update Game Management status/evidence to include draft creation after the behavior exists.
- `game/docs/FLOWS.md` - add the implemented create-draft flow after the behavior exists.

### Intentionally Unchanged

- `game/README.md` - domain ownership does not change.
- `game/docs/DATA_MODEL.md` - current schema is expected to support this work without change.
- `ARCHITECTURE.md` - no global architecture change is expected.
- `docs/architecture/SYSTEM_MAP.md` - no system wiring change is expected.
- `docs/architecture/CROSS_DOMAIN_FLOWS.md` - no cross-domain flow change is expected.

## Blockers

- None.

## Potential Discoveries

- If implementation finds that the current schema cannot safely provide atomic creation plus current-definition semantics, return this work to DRAFT and surface the schema issue for human decision.
- If implementation finds an existing explicit invariant requiring creation-time writes to `game_histories` or `game_definition_histories`, return this work to DRAFT with the evidence instead of silently adding history semantics.
- If existing Game Language contracts make structural definition validity versus Engine playability validation materially ambiguous for this operation, return this work to DRAFT with the specific contract evidence.
- If the only viable implementation requires a material public API/contract decision outside existing Game Management conventions, return this work to DRAFT for human decision.

## Completion Record

Unfilled while DRAFT.
