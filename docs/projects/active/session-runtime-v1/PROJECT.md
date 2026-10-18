# Project: Session Runtime v1

Status: ACTIVE
Created: 2026-10-13
Last updated: 2026-10-15

## Goal

Implement the real-time Session Runtime described in the architecture plan `plans/session_runtime/Playhoot_Session_Runtime_Architecture_Plan_with_TOC.docx`: durable Session lifecycle (`/play/session`), live in-memory runtime (`/play/runtime`), JavaScript sandbox execution, recovery checkpoints, and Replay recording (producer side), plus the monitoring and metrics required to operate it.

The plan is the architectural skeleton; concrete implementation is decided step by step between the human (who directs what is implemented and in what order) and the agent (who challenges, proposes, and flags gaps).

## Explicitly Out Of Scope

- Replay playback (service, API, UI). Only the producer side (data contract + recording) is in scope (plan section 22).
- Frontend SDK / helper libraries and the generated-game frontend host (plan sections 13, 31). Only the backend protocol they depend on is in scope.
- Game generation / authoring tooling (Composer).
- Anything the plan lists as intentionally deferred (plan section 31) beyond the choice that implementation has to make to deliver the checklist.

## Current Work

Done so far, in the order the human directed: metrics entry points (`monitoring/metrics`), and Create Session end to end (API endpoint and auth middleware, play-lifecycle orchestration, Session domain creation with schema, test data and tests), completed 2026-10-15; see CHECKLIST.md section D. Admin join (connection and admission), completed 2026-10-17: live Session ownership and owner routing decided (`play/docs/decisions/SESSION-ADR-0029-live-session-ownership-and-owner-routing.md`); `GET /sessions/{sessionUUID}/admin` implemented end to end (WebSocket auth, owner routing, upgrade, admin wire messages, live connection controller with graceful shutdown and metrics, Runtime admission with a real durable eligibility check and a first, minimal Session controller), verified by tests and by hand. Left for later, on purpose: the admin operations (perspective selection and views, cancellation) answer "not implemented", ownership is a single-instance placeholder, and controllers are never destroyed; see CHECKLIST.md sections A1, A2, E and H. Player join's API layer (2026-10-17): `GET /sessions/{sessionUUID}/player` and `GET /join/{joinCode}` implemented on the API side and tested, with the full player connection contract declared in `play/runtime` (and the admin's extended with `StartSession` and lobby updates); join code resolution is real (`play/session/usecases/resolvejoincode`), and so is the Runtime's lobby admission (`AttachPlayer`, with the first iteration of the runtime manifest: player limits), and guest creation is pending in Identity; see CHECKLIST.md sections A3, A4, E, F and G. Guest tokens (2026-10-17): `POST /v1/guests` implemented on the API side, with Identity's guest creation a placeholder; authorization failures carry a stable code (`token_required` tells a client without an account to create a guest). Nothing is in progress; the next step is the human's call.

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
