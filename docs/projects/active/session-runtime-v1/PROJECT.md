# Project: Session Runtime v1

Status: ACTIVE
Created: 2026-10-13
Last updated: 2026-10-18

## Goal

Implement the real-time Session Runtime described in the architecture plan `plans/session_runtime/Playhoot_Session_Runtime_Architecture_Plan_with_TOC.docx`: durable Session lifecycle (`/play/session`), live in-memory runtime (`/play/runtime`), JavaScript sandbox execution, recovery checkpoints, and Replay recording (producer side), plus the monitoring and metrics required to operate it.

The plan is the architectural skeleton; concrete implementation is decided step by step between the human (who directs what is implemented and in what order) and the agent (who challenges, proposes, and flags gaps).

## Explicitly Out Of Scope

- Replay playback (service, API, UI). Only the producer side (data contract + recording) is in scope (plan section 22).
- Frontend SDK / helper libraries and the generated-game frontend host (plan sections 13, 31). Only the backend protocol they depend on is in scope.
- Game generation / authoring tooling (Composer).
- Anything the plan lists as intentionally deferred (plan section 31) beyond the choice that implementation has to make to deliver the checklist.

## Current Work

Done so far, in the order the human directed: metrics entry points (`monitoring/metrics`), and Create Session end to end (API endpoint and auth middleware, play-lifecycle orchestration, Session domain creation with schema, test data and tests), completed 2026-10-15; see CHECKLIST.md section D. Admin join (connection and admission), completed 2026-10-17: live Session ownership and owner routing decided (`play/docs/decisions/SESSION-ADR-0029-live-session-ownership-and-owner-routing.md`); `GET /sessions/{sessionUUID}/admin` implemented end to end (WebSocket auth, owner routing, upgrade, admin wire messages, live connection controller with graceful shutdown and metrics, Runtime admission with a real durable eligibility check and a first, minimal Session controller), verified by tests and by hand. Left for later, on purpose: the admin operations (perspective selection and views, cancellation) answer "not implemented", ownership is a single-instance placeholder, and controllers are never destroyed; see CHECKLIST.md sections A1, A2, E and H. Player join (connection and lobby admission), completed 2026-10-17: `GET /sessions/{sessionUUID}/player` and `GET /join/{joinCode}` implemented end to end (real join code resolution, one lane per message type, the full player connection contract declared, and the admin's extended with `StartSession` and lobby updates), `POST /v1/guests` for clients without an account, Runtime lobby admission with a real durable eligibility check, and the first iteration of the runtime manifest (player limits); verified by tests, an end-to-end run and by hand. Left for later, on purpose: Identity's guest creation and authorization are placeholders, the operations of a running game wait for Start, capabilities wait for the manifest to declare them, and rate limiting is pending; see CHECKLIST.md sections A3, A4, B, F, G and W. Start Session, in progress since 2026-10-17: the API side (`start_session`, and an explicit `session_started` with the roster for admins and players) and the Runtime orchestration are implemented and tested, with the durable Session and the Sandbox behind declared interfaces whose stand-ins always fail; see CHECKLIST.md section G. COMMAND protocol, completed 2026-10-18: manifest `commands` with payloads declared like inputs, pre-JS validation, per-participant token idempotency, an immediate boundary that runs SAMPLEs and COMMANDs as one atomic sandbox batch in timeline order (`command(state, cmd, context)` -> `{ state, response }`), correlated response, metrics, and a `teleport` command in Space Drift; verified by tests and an end-to-end run; the human accepted it as complete. Storing processed tokens in checkpoints and recording responses in replay close with periodic checkpoints (P) and the Replay Recorder (R); see CHECKLIST.md section J. update(), completed 2026-10-18: declared with `entrypoints: ["update"]`, run as the last operation of every boundary at the boundary's time with `context.elapsed` since the previous operation (so transitions before another operation now end at their last one), never queued; Space Drift uses it; verified by tests and an end-to-end run, and accepted by the human as complete: one batch, one projection and one view per boundary, and flush boundaries asked by COMMANDs end with update() too; see CHECKLIST.md section K. Until inactivity (U) exists, an update() game runs every tick even with nobody playing. Logical transient errors and events, implemented 2026-10-18: `samples()`, `command()` and `update()` may return `errors: [{participant, error}]` and `events: [{event, participants?}]` (broadcast without `participants`) with their state; once the batch commits they travel to their players inside that boundary's `player_view` and are offered to replay (best effort), never persisted, kept by the controller or checkpointed; verified by tests; see CHECKLIST.md sections L, V and R. Timers, decided 2026-10-18 by the human: no platform timers; time-driven behavior goes through update(), with deadlines in the game state (`play/docs/decisions/SESSION-ADR-0031-game-time-is-handled-by-update-without-platform-timers.md`), so the plan's timer items are out of scope. Transport recovery, implemented 2026-10-18: a lost player connection keeps its participant playing for a 3 s transport grace (inputs reset at once); reattaching within it, or switching devices, gets a FULL view without the game being told, with SAMPLEs numbered per connection; when the grace runs out `playerDisconnected()` runs in the timeline and the participant is disconnected (rejoin, O4, not built); verified by tests; see CHECKLIST.md sections O1, O2 and O5. Leave and abandoned Sessions, implemented 2026-10-18: leaving a running game is an immediate disconnection running the same `playerDisconnected()` (decided by the human, `play/docs/decisions/SESSION-ADR-0032-leaving-is-an-immediate-disconnection.md`: no `playerLeft()`, no permanent LEFT, rejoin will apply to both); once every participant is disconnected the Session is cancelled durably with `ALL_PLAYERS_LEFT`; verified by unit, controller and Postgres integration tests; see CHECKLIST.md sections O2 and O3. Rejoin, implemented 2026-10-18: a disconnected participant (by leaving or by the grace) of a game declaring `playerRejoined` in its manifest comes back through the same endpoint; `playerRejoined(state, participant, context)` returns `{ state, accepted }` in the timeline while the attach waits; accepted, they get a FULL view of the state the rejoin left; rejected, `rejoin_rejected`; a Session with every participant disconnected ends even if the game supports rejoining (decided by the human); verified by tests; see CHECKLIST.md section O4. The next step is the human's call.

No WORK files exist: the human directs each step in conversation (see "Material Decisions Needing Human Input").

## Work

| Order | Work | Status |
|------:|------|--------|
| - | none yet | - |

## Ordering / Dependencies

Intentionally not planned up front. The human gives the implementation order; this file will record real dependencies as WORK items are created.

## Capability Coverage

The required end state is tracked in [CHECKLIST.md](CHECKLIST.md): every capability the plan requires (plus monitoring/metrics, which the plan treats as implementation detail but this project treats as mandatory), each with verification checkpoints. A checkpoint is marked only when the behavior is implemented and verified (test or other evidence), not when code merely exists.

When a WORK is created, the capabilities it owns are referenced from the checklist by checklist ID. A capability that is required but owned by no WORK is reported to the human as a gap, not silently dropped.

## Material Decisions Needing Human Input

1. **WORK materialization vs. Invariant 1.** `docs/projects/README.md` Invariant 1 says every known required outcome must have at least a PLANNED WORK. The human asked for a checklist instead of an up-front task breakdown. Until told otherwise, the checklist acts as the capability coverage matrix and WORK items are created when the human directs each step. Confirm this is the intended exception, or ask for PLANNED WORK per checklist section.
2. **Durability of the plan document.** The accepted architecture currently exists only as a `.docx` under `plans/`. Under the repo's one-fact-one-owner rule it should eventually be promoted to its durable owners (SESSION-ADR(s), `play/README.md`, `docs/architecture/CROSS_DOMAIN_FLOWS.md`, `play/docs/FLOWS.md`, etc.) and/or converted to Markdown. Not done yet; needs the human's decision on form and timing.
3. **Directory-name drift.** `AGENTS.md`, `docs/ai/KNOWLEDGE_MAP.md`, and the Session docs refer to `session/`, but the checkout has the domain at `play/` (`play/session`, `play/runtime/sandbox`). Reported, not changed.
4. **Create Session retry after the Game stops being playable.** The orchestration asks Game Management before the Session domain looks at the idempotency key, so such a retry answers `game_unavailable` instead of replaying the created Session. Kept as is for now; revisit if it matters (option: let the Session domain replay a known key before the playability check).

## Completion Criteria

The project is complete when every item in [CHECKLIST.md](CHECKLIST.md) is checked or explicitly moved out of scope by the human, including the Observability section and the Invariants verification section, and the durable knowledge has been promoted to its owners (canonical domain docs, `CURRENT_STATE.md`, ADRs).
