# Game - Current State

Status: CURRENT IMPLEMENTATION

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| Game Management | PARTIAL | Persisted authored game/version/image/history schema exists; retrieving a playable game with its current version, and loading an immutable Game Definition directly by its own Definition/Version UUID, are both implemented and tested. Create/edit/publish lifecycle operations were not found in the inspected code. |
| Session Runtime | PARTIAL | Create/Join/Leave/Start are implemented against the accepted Session/SessionActor/Participant/JoinCode/session_requests/RuntimeTurn/RuntimeStep/RuntimeState persistence model, with per-Session DB-locking serialization and `(user_uuid, operation, idempotency_key)` idempotency. Start executes the Game Language engine's first RuntimeTurn (Step-draining under a `MAX_STEPS_PER_RUNTIME_TURN = 20` bound) and transitions `LOBBY -> RUNNING`. Written and compile/unit-tested in this sandbox (`go build`/`go vet`/`go test ./... -count=1` all clean); real-Postgres repository-integration/concurrency tests were written but could not be executed here - no reachable Docker/Postgres engine in this sandbox (same known limitation recorded by every prior Session Runtime checkpoint) - and independent implementation review has not yet been performed. No interaction response processing, timer obligations, the `session_runtime_failures` diagnostic entity, WebSocket/Coordinator transport, disconnect/reconnect, or inactivity expiration exist yet (Slice 3+). |
| Game Language | IMPLEMENTED | Current v1 source model, JSON codec/validation, compiler, runtime step/evaluate behavior, snapshot codec, and tests exist. |

## Current Gaps

- Game Management lifecycle operations beyond retrieving a playable game (by current version or by a pinned Definition/Version UUID) were not found implemented.
- Session Runtime interaction response processing, timer obligations, the `session_runtime_failures` diagnostic entity, the Live Session Coordinator/WebSocket transport, disconnect/reconnect, and `activity_expires_at` inactivity expiration are not yet implemented (see `docs/ai/workspaces/active/session-runtime-v1/PLAN.md` Slices 3+).
- Start's real-Postgres repository-integration/concurrency tests are written but unexecuted - no reachable Docker/Postgres engine exists in any sandbox used so far for this WORK - and independent implementation review of Start has not yet been performed (`docs/work/active/WORK-0003-session-start-first-runtimeturn.md`, Status IMPLEMENTING).

## Known Drift

- None currently identified.

## Evidence

- Game Management storage and migrations: `game/management/internal/storage/`.
- Game Management playable-game retrieval: `game/management/usecases/getgame/`.
- Game Management pinned-definition retrieval: `game/management/usecases/getgamedefinition/`.
- Session Runtime storage and migrations: `game/session/internal/storage/`.
- Session Runtime Create/Join/Leave/Start lifecycle operations: `game/session/workflows/sessionlifecycle/` - one `Manager` workflow controller (`manager.go`) exposing `Create`/`Join`/`Leave`/`Start` as its steps (`step_create.go`/`step_join.go`/`step_leave.go`/`step_start.go`), with a narrow `internal/repo/` persistence layer. Start's own RuntimeTurn execution logic (Step-draining, the 20-Step bound, Turn/Step/State persistence) lives inline inside `step_start.go` (`drainRuntimeTurn`), not a separate shared package - see `docs/work/active/WORK-0003-session-start-first-runtimeturn.md`.
- Session Runtime shared lobby mechanics: `game/session/internal/sessionlock/` (locked-row fact reporting only - expiration policy itself is Manager-owned, see `expiration.go`), `game/session/internal/idempotency/` (claim/replay mechanics - replay/conflict/new-command policy itself is Manager-owned, see each step's own `step_create.go`/`step_join.go`/`step_leave.go`/`step_start.go`). Both mechanism packages are called directly by the Manager's steps, not through repository forwarding methods. Actor/Participant persistence lives in `internal/repo/` and is consumed by all four steps through step-local narrow interfaces (not a shared horizontal entity package).
- Game Language v1 implementation and tests: `game/language/v1/program/` and `game/language/v1/engine/`.
