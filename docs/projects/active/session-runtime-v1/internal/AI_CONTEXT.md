Process: Project execution (session-runtime-v1), human-directed implementation
Topic: Real-time Session Runtime per plans/session_runtime architecture plan
Current stage: logical transient errors and events implemented; timers dropped by SESSION-ADR-0031 and tested (2026-10-18); waiting for the human's review
Current execution surface: CODEBASE AGENT
Related durable artifacts: docs/projects/active/session-runtime-v1/PROJECT.md, docs/projects/active/session-runtime-v1/CHECKLIST.md, plans/session_runtime/Playhoot_Session_Runtime_Architecture_Plan_with_TOC.docx, play/docs/decisions/SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md
Blocked by: nothing
Next action: wait for the human's next instruction
Last durable checkpoint: logical transient errors and events (`errors: [{participant, error}]`, `events: [{event, participants?}]` from samples/command/update, delivered inside player_view, offered to replay) implemented, CHECKLIST L/V/R, CURRENT_STATE and PROJECT updated (2026-10-18); the progress notes below stop at 2026-10-15, CHECKLIST.md and PROJECT.md hold everything since
Last updated: 2026-10-18

## Working agreement (human decisions)

- The human directs what to implement and in what order. Do not plan a task order on your own.
- The agent debates, proposes, and flags where an instruction is under-specified or does not satisfy the plan's requirements; the human decides.
- The architecture plan is the skeleton. Concrete implementation is decided together, step by step.
- CHECKLIST.md is the end-state verification list. Tick items only when implemented AND verified. When the human's instructions skip a checklist item, surface it.
- Monitoring and metrics are mandatory for completion even though the plan lists them as deferred implementation detail (plan section 31).

## Progress (not duplicating git)

- monitoring/metrics entry-point API done (emission to a backend pending).
- Create Session, /api edge written without tests: `POST /sessions`, `api/internal/auth` middleware (unexported Identity port `authorizer`), unexported `sessionCreator` port in `api/play`; contract types in `identity/identity.go` (only the Operation and AccessDecision enums) and `play/session/session.go` (outcome enum, result, conflict error). Identity is wired to a placeholder (`identity/usecases/authorize`: random UserUUID, always allowed; real behavior must make CREATE_SESSION require an account, not a guest). The Orchestrator stand-in in `unwired.go` at repo root must be removed once CreateSession exists.
- Unit tests (table of callbacks + gomock, per the user's test style) exist for the `/api` edge: create-session handler, `auth.Require`/`bearerToken`/`UserUUID`, REST request metric, status recorder, `restObservability`. Mocks regenerate with `go generate` in `api/internal/auth` and `api/play`.
- Metrics ownership is fixed by `docs/engineering/standards/metrics.md` (written from the human's explicit rule, no separate review checkpoint): the Session domain must count sessions created; the handler records no metrics. The `api_authorization_*` metrics in `api/internal/auth` stay: the human decided they are the auth middleware's own metrics, not Identity's.
- Orchestrator `CreateSession` implemented and unit-tested (`orchestrator/playlifecycle/create_session.go`; orchestrator is organized by behavior group, see its README): game playability check (port returns bool) then Session creation; identifiers and the idempotency key are plain strings, no named types. Idempotency is passed through to the Session domain. Game side is wired to the `game/usecases/checkplayability` placeholder (always true); Session side to the always-failing stand-in in `unwired.go`. All placeholders are tracked in CHECKLIST.md "Placeholders left behind" (keep it current: every new stub/stand-in gets an item there and in the owning domain's CURRENT_STATE.md).
- Known ordering caveat (human-directed order): the playability check runs before the Session domain claims the idempotency key, so a retry after the Game stopped being playable returns `game_unavailable` instead of replaying the original created result.
- Session domain `CreateSession` implemented (`play/session/workflows/sessionlifecycle`, schema in `play/docs/DATA_MODEL.md`, tables prefixed `play_`, no FKs by human decision) and wired in `main`; `unwired.go` deleted. Unit tests pass; repo tests passed when the human ran them against Postgres (2026-10-15). Integration tests pass (run by the human). The agent environment has no Postgres, so DB tests skip there.
- Create Session is complete. Remaining cross-cutting items it touched: emission to a metrics backend (Y), a flow that registers games and EXECUTABLE versions (placeholder, B), Identity and Game placeholders (out of scope).
- `play/session/internal/...` does not compile (imports still point at the old `session/` path); pre-existing, unrelated.

## Context notes

- The plan lives only as a .docx. To read it: copy it to the scratchpad, unzip, and extract `word/document.xml` text. Promotion to durable owners is pending a human decision (see PROJECT.md).
- Domain directory is `play/` in the checkout although docs say `session/`.
- Existing helpers: `play/runtime/sandbox` (QuickJS/WASM worker), `play/session/internal/objectstore`, empty migrations registry. `monitoring/` has only `alerts.go` (no metrics yet). Logging standard: `docs/engineering/standards/logging.md`.
- Found 2026-10-18, not acted on: the Space Drift seed script fails `scriptcheck.Validate` (18 violations, mutations of its copied world in start/samples, and now command); the validator is conservative and nothing calls it yet, but a real publishing flow would reject this seed as written.
