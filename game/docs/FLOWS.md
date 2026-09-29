# Game Management - Flows

Status: CURRENT IMPLEMENTATION

## Check Game Visibility

```mermaid
sequenceDiagram
    participant Caller as orchestrator.Orchestrator
    participant UseCase as checkvisibility.Service
    participant Repo as game/internal/storage.Repo
    participant DB as games table
    participant Rules as businessservice

    Caller->>UseCase: IsVisible(gameUUID)
    UseCase->>Repo: ResolveVisibility(gameUUID)
    Repo->>DB: SELECT visibility FROM games WHERE uuid = ? AND deleted_at IS NULL
    DB-->>Repo: visibility value, or no row
    Repo-->>UseCase: *string (nil if not found)
    UseCase->>Rules: IsPlayableVisibility(visibility)
    UseCase-->>Caller: visible, found
```

Implemented behavior:

- Resolves only `games.visibility` by `games.uuid` - no join, no script/definition decode of any kind (Game Management owns no version/definition concept - see `game/docs/DATA_MODEL.md`).
- A missing or soft-deleted Game reports `found = false`.
- `found = true, visible = false` for `draft`/`private`; `found = true, visible = true` for `hidden`/`public` (`businessservice.IsPlayableVisibility`).
- Called by `orchestrator.Orchestrator.CreateSession` (`docs/projects/active/js-runtime-migration/works/WORK-0032-composer-mediated-session-creation-visibility-check.md`) before it calls Session Runtime's `Create` - Game Management's only current cross-domain caller.

Evidence:

- `game/usecases/checkvisibility/service.go`
- `game/usecases/checkvisibility/service_test.go`
- `game/internal/storage/repo.go`
- `game/internal/storage/repo_test.go`
