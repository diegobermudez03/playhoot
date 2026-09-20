# Game - Current State

Status: CURRENT IMPLEMENTATION

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| Game Management | PARTIAL | Persisted authored game/version/image/history schema exists; retrieving a playable game with its current version, and loading an immutable Game Definition directly by its own Definition/Version UUID, are both implemented and tested. Create/edit/publish lifecycle operations were not found in the inspected code. |
| Session Runtime | PARTIAL | Create/Join/Leave/Start/AnswerInteraction are implemented against the accepted Session/SessionActor/Participant/JoinCode/session_requests/RuntimeTurn/RuntimeStep/session_interactions persistence model plus `sessions.current_turn_id` (GAME-ADR-0023 - a logical, non-DB-enforced pointer colocated on `sessions` rather than a separate table), with per-Session DB-locking serialization extended unchanged to RUNNING (GAME-ADR-0018) and `(user_uuid, operation, idempotency_key)` idempotency for the LOBBY operations (AnswerInteraction uses no idempotency record - the interaction row's own persisted state/response_payload is its dedup identity). Start executes the Game Language engine's first RuntimeTurn and transitions `LOBBY -> RUNNING`; AnswerInteraction lets a player answer an open `Question`/`AskGroup` interaction against a `RUNNING` Session, reloading current authoritative state and committing a further RuntimeTurn. Both steps share one RuntimeTurn Step-draining/bound-execution mechanism (`internal/runtimeturn`, `MAX_STEPS_PER_RUNTIME_TURN = 20`) and one interaction-capture mechanism recording every `OpenQuestionOutput`/`CloseQuestionOutput` a committed RuntimeTurn produces (including Start's own first Turn) as `session_interactions` rows; this slice's own fatal path also closes any interaction it leaves `ACTIVE` (the narrow terminal-cleanup case - the fully general sweep remains Slice 6). `go build`/`go vet`/`go test ./... -count=1` clean, including real-Postgres repository-integration/concurrency tests (`docs/work/active/WORK-0004-interaction-response-processing.md`). No timer obligations, the `session_runtime_failures` diagnostic entity, WebSocket/Coordinator transport, disconnect/reconnect, or inactivity expiration exist yet (Slice 5+); independent review of WORK-0004 has not yet run. |
| Game Language | IMPLEMENTED | Current v1 source model, JSON codec/validation, compiler, runtime step/evaluate behavior, snapshot codec, and tests exist. |

## Current Gaps

- Game Management lifecycle operations beyond retrieving a playable game (by current version or by a pinned Definition/Version UUID) were not found implemented.
- Session Runtime timer obligations, the `session_runtime_failures` diagnostic entity, the Live Session Coordinator/WebSocket transport, disconnect/reconnect, and `activity_expires_at` inactivity expiration are not yet implemented (see `docs/ai/workspaces/active/session-runtime-v1/PLAN.md` Slices 5+). `activity_expires_at` in particular has no schema/enforcement anywhere yet despite being already-accepted architecture (`game/README.md`'s Durable Inactivity Expiration section) - AnswerInteraction is the first RUNNING-phase mutation this could apply to, but WORK-0004 did not add it, consistent with Start never having added it either; flagged here as a pre-existing gap, not a regression introduced by this slice.
- WORK-0004 (Slice 3, Interaction Response Processing) has been implemented and verified (`go build`/`go vet`/`go test ./... -count=1`, real-Postgres) but has not yet gone through independent review - see the WORK's own Completion Record.

## Known Drift

- None currently identified.

## Evidence

- Game Management storage and migrations: `game/management/internal/storage/`.
- Game Management playable-game retrieval: `game/management/usecases/getgame/`.
- Game Management pinned-definition retrieval: `game/management/usecases/getgamedefinition/`.
- Session Runtime storage and migrations: `game/session/internal/storage/`.
- Session Runtime Create/Join/Leave/Start/AnswerInteraction lifecycle/execution operations: `game/session/workflows/sessionlifecycle/` - one `Manager` workflow controller (`manager.go`) exposing `Create`/`Join`/`Leave`/`Start`/`AnswerInteraction` as its steps (`step_create.go`/`step_join.go`/`step_leave.go`/`step_start.go`/`step_answer_interaction.go`), with a narrow `internal/repo/` persistence layer. The shared RuntimeTurn Step-draining/bound-execution mechanism Start and AnswerInteraction both call lives in `internal/runtimeturn/` (`Drain`), scoped to this workflow package rather than at `game/session/internal` - see `docs/work/active/WORK-0004-interaction-response-processing.md`. The shared interaction-capture mechanism (`interaction_capture.go`) records every committed RuntimeTurn's `OpenQuestionOutput`/`CloseQuestionOutput` as `session_interactions` rows, called by both `step_start.go` and `step_answer_interaction.go`.
- Session Runtime shared lobby mechanics: `game/session/internal/sessionlock/` (locked-row fact reporting only - expiration policy itself is Manager-owned, see `expiration.go`), `game/session/internal/idempotency/` (claim/replay mechanics - replay/conflict/new-command policy itself is Manager-owned, see each step's own `step_create.go`/`step_join.go`/`step_leave.go`/`step_start.go`). Both mechanism packages are called directly by the Manager's steps, not through repository forwarding methods. Actor/Participant persistence lives in `internal/repo/` and is consumed by all four steps through step-local narrow interfaces (not a shared horizontal entity package).
- Game Language v1 implementation and tests: `game/language/v1/program/` and `game/language/v1/engine/`.
