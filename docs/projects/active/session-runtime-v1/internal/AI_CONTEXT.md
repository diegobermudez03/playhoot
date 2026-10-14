Process: Project execution (session-runtime-v1), human-directed implementation
Topic: Real-time Session Runtime per plans/session_runtime architecture plan
Current stage: tracking set up; waiting for the human's first implementation instruction
Current execution surface: CODEBASE AGENT
Related durable artifacts: docs/projects/active/session-runtime-v1/PROJECT.md, docs/projects/active/session-runtime-v1/CHECKLIST.md, plans/session_runtime/Playhoot_Session_Runtime_Architecture_Plan_with_TOC.docx, play/docs/decisions/SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md
Blocked by: nothing
Next action: wait for the human's next instruction; map it to checklist IDs
Last durable checkpoint: monitoring/metrics entry-point API implemented and tested (2026-10-13); emission to a backend still pending
Last updated: 2026-10-13

## Working agreement (human decisions)

- The human directs what to implement and in what order. Do not plan a task order on your own.
- The agent debates, proposes, and flags where an instruction is under-specified or does not satisfy the plan's requirements; the human decides.
- The architecture plan is the skeleton. Concrete implementation is decided together, step by step.
- CHECKLIST.md is the end-state verification list. Tick items only when implemented AND verified. When the human's instructions skip a checklist item, surface it.
- Monitoring and metrics are mandatory for completion even though the plan lists them as deferred implementation detail (plan section 31).

## Progress (not duplicating git)

- monitoring/metrics entry-point API done (emission to a backend pending).
- Create Session, /api edge written without tests: `POST /sessions`, `api/internal/auth` middleware (unexported Identity port `authorizer`), unexported `sessionCreator` port in `api/session`; public contract types in `identity/identity.go` and `play/session.go`. Identity is wired to a placeholder (`identity/usecases/authorize`: random UserUUID, always allowed; real behavior must make CREATE_SESSION require an account, not a guest). The Orchestrator stand-in in `unwired.go` at repo root must be removed once CreateSession exists.
- Unit tests (table of callbacks + gomock, per the user's test style) exist for the `/api` edge: create-session handler, `auth.Require`/`bearerToken`/`UserUUID`, REST request metric, status recorder, `restObservability`. Mocks regenerate with `go generate` in `api/internal/auth` and `api/session`.
- Still open for create session: Orchestrator CreateSession, /game playable check, /play/session durable creation, idempotency ownership, join code generation, tests, metrics emission.
- `play/session/internal/...` does not compile (imports still point at the old `session/` path); pre-existing, unrelated.

## Context notes

- The plan lives only as a .docx. To read it: copy it to the scratchpad, unzip, and extract `word/document.xml` text. Promotion to durable owners is pending a human decision (see PROJECT.md).
- Domain directory is `play/` in the checkout although docs say `session/`.
- Existing helpers: `play/runtime/sandbox` (QuickJS/WASM worker), `play/session/internal/objectstore`, empty migrations registry. `monitoring/` has only `alerts.go` (no metrics yet). Logging standard: `docs/engineering/standards/logging.md`.
