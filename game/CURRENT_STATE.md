# Game - Current State

Status: CURRENT IMPLEMENTATION

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| Game Management | PARTIAL | Persisted authored game/version/image/history schema exists, and retrieving a playable game with its current version is implemented and tested. Create/edit/publish lifecycle operations were not found in the inspected code. |
| Session Runtime | PARTIAL | Session, session-state, session-player, and join-code schema exists. Lifecycle scaffolding exists, but `CreateRoom` and `JoinRoom` are stubs and no session execution flow was found implemented. |
| Game Language | IMPLEMENTED | Current v1 source model, JSON codec/validation, compiler, runtime step/evaluate behavior, snapshot codec, and tests exist. |

## Current Gaps

- Game Management lifecycle operations beyond retrieving a playable game with its current version were not found implemented.
- Session Runtime create/join/start/execution behavior was not found implemented beyond persistence schema and scaffolding.
- Session Runtime lifecycle packages currently do not build because `game/session/workflows/sessionlifecycle/internal/repo/step_create_room.go` declares `CreateRoom` without a function body.

## Known Drift

- None currently identified.

## Evidence

- Game Management storage and migrations: `game/game/internal/storage/`.
- Game Management playable-game retrieval: `game/game/usecases/getgame/`.
- Session Runtime storage and migrations: `game/session/internal/storage/`.
- Session Runtime lifecycle scaffolding: `game/session/workflows/sessionlifecycle/`.
- Game Language v1 implementation and tests: `game/language/v1/program/` and `game/language/v1/engine/`.
