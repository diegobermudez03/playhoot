# Game - Current State

Status: CURRENT IMPLEMENTATION

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| Game Management | PARTIAL | Persisted authored game/version/image/history schema exists; retrieving a playable game with its current version, and loading an immutable Game Definition directly by its own Definition/Version UUID, are both implemented and tested. Create/edit/publish lifecycle operations were not found in the inspected code. |
| Session Runtime | PARTIAL | Create/Join/Leave are implemented and tested against the accepted Session/SessionActor/Participant/JoinCode/session_requests persistence model, with per-Session DB-locking serialization and `(user_uuid, operation, idempotency_key)` idempotency. No Game execution/RuntimeTurn, WebSocket/runtime execution, disconnect/reconnect, or inactivity expiration exist yet (Slice 2+). |
| Game Language | IMPLEMENTED | Current v1 source model, JSON codec/validation, compiler, runtime step/evaluate behavior, snapshot codec, and tests exist. |

## Current Gaps

- Game Management lifecycle operations beyond retrieving a playable game (by current version or by a pinned Definition/Version UUID) were not found implemented.
- Session Runtime Start/RuntimeTurn/engine execution, the Live Session Coordinator/WebSocket transport, disconnect/reconnect, and `activity_expires_at` inactivity expiration are not yet implemented (see `docs/ai/workspaces/active/session-runtime-v1/PLAN.md` Slices 2+).

## Known Drift

- None currently identified.

## Evidence

- Game Management storage and migrations: `game/management/internal/storage/`.
- Game Management playable-game retrieval: `game/management/usecases/getgame/`.
- Game Management pinned-definition retrieval: `game/management/usecases/getgamedefinition/`.
- Session Runtime storage and migrations: `game/session/internal/storage/`.
- Session Runtime Create/Join/Leave lifecycle operations: `game/session/workflows/sessionlifecycle/` - one `Manager` workflow controller (`manager.go`) exposing `Create`/`Join`/`Leave` as its steps (`step_create.go`/`step_join.go`/`step_leave.go`), with a narrow `internal/repo/` persistence layer.
- Session Runtime shared lobby mechanics: `game/session/internal/sessionlock/` (locked-row fact reporting only - expiration policy itself is Manager-owned, see `expiration.go`), `game/session/internal/idempotency/` (claim/replay mechanics - replay/conflict/new-command policy itself is Manager-owned, see each step's own `step_create.go`/`step_join.go`/`step_leave.go`). Both mechanism packages are called directly by the Manager's steps, not through repository forwarding methods. Actor/Participant persistence lives in `internal/repo/` and is consumed by all three steps through step-local narrow interfaces (not a shared horizontal entity package).
- Game Language v1 implementation and tests: `game/language/v1/program/` and `game/language/v1/engine/`.
