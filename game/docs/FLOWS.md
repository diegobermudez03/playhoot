# Game - Flows

Status: CURRENT IMPLEMENTATION

## Create Draft Game With Initial Definition

```mermaid
sequenceDiagram
    participant Caller
    participant UseCase as creategame.UseCase
    participant Repo as creategame.Repo
    participant DB as Game tables
    participant Language as gameservice

    Caller->>UseCase: CreateGameDraft(params)
    UseCase->>Language: EncodeJSON(params.Definition)
    UseCase->>Repo: createGameDraft(generated UUIDs, authored data, script)
    Repo->>DB: BEGIN
    Repo->>DB: INSERT games as draft
    Repo->>DB: INSERT game_definitions as version 1
    Repo->>DB: UPDATE games.current_definition_id
    Repo->>DB: COMMIT
    Repo-->>UseCase: created game/version identifiers
    UseCase-->>Caller: *game.Game
```

Implemented behavior:

- Game and initial definition creation is atomic.
- The created game is stored with `draft` visibility.
- The initial definition is stored as version `1`, remains unpublished, and becomes the game's current definition.
- Creation does not compile or execute the definition as an Engine playability check.
- Creation does not write game or game-definition history rows.

Evidence:

- `game/game/usecases/creategame/service.go`
- `game/game/usecases/creategame/repo.go`
- `game/game/usecases/creategame/service_test.go`
- `game/game/usecases/creategame/repo_test.go`

## Retrieve Playable Game With Current Version

```mermaid
sequenceDiagram
    participant Caller
    participant UseCase as getgame.Service
    participant Repo as getgame.Repo
    participant DB as Game tables
    participant Rules as businessservice
    participant Language as gameservice

    Caller->>UseCase: GetPlayableGameWithCurrentVersion(gameUUID)
    UseCase->>Repo: GetGameCurrentVersion(gameUUID)
    Repo->>DB: SELECT games + current game_definitions
    DB-->>Repo: Game row and current version script
    Repo-->>UseCase: game.Game
    UseCase->>Rules: ValidateVisibility / IsPlayableVisibility
    UseCase->>Language: DecodeJSON(script)
    Language-->>UseCase: program.Definition
    UseCase-->>Caller: playable game with current definition
```

Implemented behavior:

- Missing games return no game.
- Non-playable visibility is rejected.
- Invalid visibility and invalid definition scripts are treated as broken game data.

Evidence:

- `game/game/usecases/getgame/service.go`
- `game/game/usecases/getgame/repo.go`
- `game/game/usecases/getgame/service_test.go`
- `game/game/usecases/getgame/repo_test.go`

## Not Documented As Implemented

- `CreateRoom` and `JoinRoom` are stubs in the inspected Session Runtime lifecycle code.
