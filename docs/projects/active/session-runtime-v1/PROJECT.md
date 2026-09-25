# Project: Session Runtime V1

Status: ACTIVE (Phase 1 resumed - WORK-0006 DONE, see "Unblocked (2026-09-24)" below)
Created: 2026-09-20 (migrated from the earlier `docs/ai/workspaces/active/session-runtime-v1/` initiative workspace and its numbered-Slice `PLAN.md`, which together tracked this initiative from 2026-09-08)
Last updated: 2026-09-24 (Phase 1 actually resumed: human-directed, with WORK-0028 already DONE per this Project's own recommendation - WORK-0006 (domain half: `Manager` returning Effect/Presentation Outputs in memory) implemented and independently reviewed APPROVED - see "Current Work"/its own WORK file. Prior update, same day: Phase 1 unblocked: GAME-ADR-0026 is implemented, including the `sessionlifecycle` rework the pause below anticipated - see "Unblocked (2026-09-24)" below. Prior update, same day: Project-wide pause: WORK-0006 returned READY -> DRAFT and its implementation reverted, pending `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md` (PROPOSED) - see "Paused (2026-09-24)" below. Prior update, same day: WORK-0006 DRAFT -> READY, narrowed to its domain half only; WORK-0020 corrected to explicitly own rebuilding `play` from scratch, since it was still drafted as if extending an implementation that had already been deleted - see "Drift Correction (2026-09-23): WORK-0006/0007/0010/0012/0020 Assumed `play` Still Existed" below. Prior update, same day: WORK-0019 DONE - independent review APPROVED after two fix/re-review passes, real-Postgres verification completed; see "Current Work" below. Prior update: 2026-09-21 inside-out sequencing restructuring - see "Restructuring (2026-09-21): Inside-Out Sequencing" below)

## Paused (2026-09-24): Game Language Engine Redesign

A human design review of how Session Runtime addresses interactions (`Path`/`Slot`, exposed all the way from the engine into `sessionlifecycle`'s own code) escalated into questioning whether Game Language's nested-workflow execution model (Child Workflows, Task Groups) is needed at all. Neither construct is used by any completed WORK in this Project, by any authored example, or by any prior GAME-ADR's acceptance - and the already-accepted `game/README.md` itself already deferred how the outside world would ever interact with a nested instance. An audit against the product's target game range found no game needing a genuinely separate execution instance; every case is expressible on one flat workflow instance with keyed interaction slots (generalizing GAME-ADR-0012) and engine-assigned interaction IDs. This is now `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md` (PROPOSED, pending human acceptance).

Because `sessionlifecycle` is built directly against the engine `Output`/`Signal` shapes this record changes, **Phase 1 of this Project is paused** until GAME-ADR-0026 is accepted and at least its engine/compiler/program side is implemented (tracked as a separate, engine-scoped effort - not yet filed as a Project/WORK of its own). WORK-0006's already-completed implementation pass was reverted (not because it was wrong, but to avoid building/reviewing further against a surface about to change) and its Status returned to DRAFT - see its own "Paused (2026-09-24)" section. No other WORK in this Project changes as a result of this pause; WORK-0005/WORK-0019 (DONE) and WORK-0001/0003/0004 (DONE, historical) are unaffected, confirming the audit's finding that nested execution was never actually exercised by anything already built.

(Superseded the same day - see "Unblocked (2026-09-24)" immediately below.)

## Unblocked (2026-09-24): Game Language Engine Redesign Implemented

GAME-ADR-0026 is accepted, and both its engine/compiler/program-side implementation (tracked as `docs/projects/completed/game-language-flat-execution-model/works/WORK-0026-engine-owned-interaction-addressing.md`) and the `sessionlifecycle` rework the pause above anticipated (`docs/projects/completed/game-language-flat-execution-model/works/WORK-0027-session-runtime-interaction-addressing-rework.md`, implemented together with WORK-0026 in one combined pass) are implemented: `sessionlifecycle` now consumes the engine's `InteractionID`-addressed `Output`/`Signal` shapes directly, and `session_interactions` is keyed by `engine_interaction_id` instead of `(engine_path, engine_slot)`. **Phase 1 of this Project is therefore no longer paused.** Whether/when Phase 1 actually resumes (WORK-0006 onward), and any resequencing that requires, remains this Project's own decision - not made by this note.

**Flag (2026-09-24), same day, before WORK-0006 actually resumed:** a follow-up audit of `game-language-flat-execution-model` found `sessionlifecycle` still reimplements part of the engine's own execution model on top of `engineservice.Step` - its own step-chaining loop (`internal/runtimeturn.Drain`, GAME-ADR-0019's bound enforced caller-side) and its own replay loop (`replay.go`'s `reconstructCurrentSnapshot`/`replayTurn`, implementing GAME-ADR-0024). This is now `game/docs/decisions/GAME-ADR-0027-engine-owned-turn-execution-and-replay.md` (PROPOSED) and `docs/projects/completed/game-language-flat-execution-model/works/WORK-0028-engine-owned-turn-execution.md` (DRAFT), which moves both loops inside `engineservice` and changes `interaction_capture.go`'s `captureInteractions` signature (`[]runtimeturn.StepTrace` -> `[]engine.Output`) - the exact file WORK-0006's own Approved Design touches to add Effect/Presentation Output handling. **Recommendation, mirroring the reasoning that paused WORK-0006 for GAME-ADR-0026 above: let WORK-0028 land first**, so WORK-0006 is implemented once against a settled `captureInteractions`/Output-handling surface rather than twice. This is a recommendation for whoever resumes Phase 1, not a decision made by this note - WORK-0006's own Status is left at its current DRAFT rather than unilaterally re-paused here. **Followed**: WORK-0028 landed DONE before WORK-0006 actually resumed; WORK-0006 is now DONE, implemented once against WORK-0028's settled surface.

## Goal

Implement the complete Session backend required to execute the supported Game Definition semantics end-to-end, and provide the server-side surfaces necessary to build and run the Session frontend: a host creates a Session, players join a lobby, the host starts it, the authored Game Language program executes turn by turn through interactions/user intents/timers, players may disconnect/reconnect, a host may cancel, and the Session reaches a terminal state with no dangling runtime obligations - all under the accepted persistence, serialization, and failure-handling model.

## Explicitly Out Of Scope

- **Authentication / authorization / full Identity integration.** The backend uses the current simplified identity assumption (a trusted caller supplies an already-authenticated `UserUUID` directly; no credential verification exists anywhere yet) for the duration of this Project. Real Identity/Auth integration is a separate future initiative.
- Host "kick a participant" / "transfer host" - not currently known to be required for a V1 frontend; not owned by any WORK. See Material Decisions below.
- A personalized/configurable end-of-game results screen - deferred (`docs/product/IDEAS.md`), WORK-0007 delivers only a generic termination signal.
- Multi-instance/distributed live-transport routing (Redis, sticky routing) - excluded from all of V1 by GAME-ADR-0002, not a gap.
- Building an actual replay/rewatch feature - WORK-0019 only keeps replay theoretically possible (the durable replay-input model, per GAME-ADR-0024); no replay feature is built by this Project.

## Restructuring (2026-09-21): Inside-Out Sequencing

WORK-0005 built a stateful live-session Coordinator (`play`) and its bridge to `sessionlifecycle.Manager` (`play/sessionruntime`) before Session Runtime's own domain layer (`game/session`) could actually return the Game Language engine's `Output` values - `Manager.Start`/`AnswerInteraction` only ever persist and expose 2 of the engine's 9 committed `Output` variants (Open/CloseQuestion), and never return any of them to a caller as a Go value. `play/sessionruntime` worked around this by issuing its own raw SQL against Session Runtime's tables instead. WORK-0006 (still DRAFT, no code written) already designed the correct fix for the Presentation/Effect-shaped Outputs it covers - `Manager` additively returning them in memory - but it was never implemented before `play` shipped its own workaround. Continuing to build `play`'s connection-role/lobby/spectator/timer logic on top of that incomplete, already-known-to-change foundation would mean writing more stateful logic that a near-term WORK already knew it would have to rebuild.

**Decision**: `play`/`play/sessionruntime` are removed (see WORK-0005's Blocker 11); WORK-0005 is reduced to an HTTP/WebSocket transport skeleton with no domain coupling. This Project's remaining WORK is resequenced into three explicit phases, inside-out: finish Session Runtime's own engine-Output handling first (a `game/session` domain concern), then rebuild the stateful live Coordinator (`play`) once, correctly, on top of it, then the transport layer that exposes it (`api`, largely co-implemented with each Phase 2 WORK). Nothing beyond WORK-0005 has any implemented code yet, so this resequencing is free - no completed or reviewed WORK is reopened.

Several currently-DRAFT WORKs (0006, 0007, 0010, 0011, 0012) mix a Session-Runtime-domain concern with a `play`/`api` concern in one document. They are **not** split into separate files yet - each is annotated below with which phase its still-undesigned domain/play halves belong to; the actual split happens when each WORK is next drafted for real (just-in-time, matching this Project's existing practice - see e.g. WORK-0002's own historical "next just-in-time candidate" framing).

## Current Work

- **WORK-0005** (Thin Live Coordinator / WebSocket) - **DONE (2026-09-22)**. Reduced to an HTTP/WebSocket transport skeleton (Blocker 11) - `play`/`play/sessionruntime` are deleted; `POST /sessions` and `GET /ws` work as real transport (a real 501, a real connection upgrade, centralized observability + trace/span IDs, Blockers 12-13) but call no domain package. Independent review APPROVED after one REQUIRED_FIX (stale doc comments) was fixed and re-reviewed.
- **WORK-0006** (Broaden Live Fan-Out: Effects + Presentations, Domain Half) - **DONE (2026-09-24)**. `sessionlifecycle.Manager.Start`/`AnswerInteraction` additively return the committed Turn's Effect/Presentation Outputs in memory (`StartResult`/`AnswerInteractionResult.Outputs`), zero new durable persistence, no client-delivery wire code (that remains the not-yet-drafted play-half following WORK-0020). Independent review APPROVED after one NON_BLOCKING finding (this WORK's own status-history wording), fixed same-session.
- **WORK-0019** (Replay-First Session Runtime Persistence Migration) - **DONE (2026-09-23)**. Implements GAME-ADR-0024. `session_runtime_turns`'s Snapshot columns and `session_runtime_steps` removed; `session_runtime_starts`/`session_cause_events` added; replay-based reconstruction (`reconstructCurrentSnapshot`) implemented and proven against real Postgres by a test comparing reconstructed state to values live execution actually produced (not merely re-derived from the same durable rows). Independent review APPROVED after two fix/re-review passes (a circular test and missing data-integrity alerts, both fixed). Moved to the front of Phase 1 - it defines the durable representation every future RuntimeTurn cause (UserIntent, SessionCancelled, TimerExpired) must satisfy, now including the shared `session_cause_events` table Blocker 3 fixed for them, and a `PROJECT.md`-documented requirement that each such WORK also extend `sessionlifecycle.replay.go`'s `loadReplaySignal`, not only add a table.
- **Phase 1 (Session Runtime domain completion) resumed and its lead item (WORK-0006) is DONE** - see "Unblocked (2026-09-24)" above and WORK-0006's own Completion Record.
- **WORK-0007** (Game-Completion Termination Detection, domain half) - **DONE (2026-09-24)**. `sessionlifecycle.Manager.Start`/`AnswerInteraction` detect a `RunCompletedOutput` (the game's own instance reaching Completed/Failed/Cancelled - engine-renamed from `WorkflowCompletedOutput` as part of this WORK, see its own "Scope Addition") and terminalize the Session under one of three new `TerminalReasonGame*` values, closing any other still-`ACTIVE` interaction; no broadcast/transport code (that remains the not-yet-drafted play-half following WORK-0020). Independent review APPROVED, three NON_BLOCKING findings, two fixed same-session (an idempotent-replay gap; this PROJECT.md's own synchronization).
- **WORK-0020** (Role-Aware Live Connections / Host Administration Channel) - DRAFT. Implements GAME-ADR-0025; Blockers 1-3 need human approval before READY. First WORK of Phase 2, and now explicitly the WORK that rebuilds the entire `play` Coordinator from scratch (both registries, both dispatch paths, `Deliver`) - its own file previously still described adding a registry to an implementation that had already been deleted; corrected 2026-09-23, see "Drift Correction" below.

## Drift Correction (2026-09-23): WORK-0006/0007/0010/0012/0020 Assumed `play` Still Existed

WORK-0005's Blocker 11 (2026-09-21, one day after WORK-0006/0007/0010/0011/0012/0020 were drafted) deleted `play`/`play/sessionruntime` in full and restructured this Project into the inside-out phases described above - specifically so the live Coordinator would be rebuilt once, correctly, on a complete Session Runtime domain layer, rather than patched incrementally. This restructuring's own narrative (above) already says WORK-0006/0007/0010/0011/0012 "mix a Session-Runtime-domain concern with a `play`/`api` concern in one document" and that WORK-0020 is Phase 2's lead item - but the individual WORK files themselves were never actually edited to match: WORK-0006 still listed `play/sessionruntime` translation and wire-message types as "In Scope" with Acceptance Criteria requiring a client to receive a message; WORK-0007/WORK-0010/WORK-0012 still described `play`/`play/coordinator.go`/"the existing live Coordinator" as implementations they extend; WORK-0020 described itself as adding a second registry "alongside the existing participant registry" and treated WORK-0005's Create/Join/AnswerInteraction/Deliver path as an unchanged foundation - all factually false once `play` was deleted. This was a genuine documentation drift (the restructuring decision was already made and accepted; the individual files simply weren't synchronized to it), not a new architectural decision. Per explicit human direction (2026-09-23):

- **WORK-0006** was split for real (it is the next WORK to implement): trimmed to its domain half only, DRAFT -> READY.
- **WORK-0020** was corrected to explicitly be the WORK that (re)introduces the `play` layer from scratch - its Scope/Approved Design/Acceptance Criteria/Documentation Impact now describe building both registries, both dispatch paths, and `Deliver` as new code, not extending deleted code. Still DRAFT; its own Blockers 1-3 are unaffected and still need resolution before READY.
- **WORK-0007/WORK-0010/WORK-0012** received lighter corrections (stale `play`-exists references fixed, pointed at WORK-0020 instead) without a full Scope/Acceptance-Criteria rewrite, since none of them are being drafted for real yet - the full split for each happens when it is, per this Project's existing just-in-time practice (unchanged by this correction).
- **WORK-0011** needed no correction - it already referred to WORK-0020's future ADMIN connection correctly, without assuming `play` currently exists.

No production code was touched by this correction.

## 2026-09-20 Reconciliation Pass

A dedicated reconciliation session accepted several human architecture decisions that materially change this Project's persistence and live-connection direction without reopening any completed WORK's historical accuracy:

- **Replay-first Session Runtime persistence** (no durable per-Turn Snapshot; durable replay-input causes plus deterministic replay reconstruct current/historical Runtime state) - `game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md`, implemented by new **WORK-0019**.
- **Role-aware live connections** (ADMIN and PARTICIPANT as structurally independent connection roles; a host may hold both) - `game/docs/decisions/GAME-ADR-0025-role-aware-live-connections.md`, implemented by new **WORK-0020**, which formally supersedes WORK-0005's own "Host Connection Design Revision" proposal (Blocker 8).
- **Host Participant spectator view** during RUNNING, read-only visual mirroring only - new **WORK-0021** (PLANNED).
- **Archival moves from GCS to same-database PostgreSQL JSONB compaction** - WORK-0017 rewritten accordingly.
- WORK-0006, WORK-0007, WORK-0008, WORK-0010, WORK-0011, WORK-0012, WORK-0013, WORK-0014, WORK-0015, WORK-0016 each received a targeted scope reconciliation against the two decisions above - see each WORK's own "Scope Addition/Revision/Clarification (... Reconciliation, 2026-09-20)" section for exactly what changed.
- WORK-0001, WORK-0003, WORK-0004 (DONE) and WORK-0018 (PLANNED, untouched - Game-Language-only signal-schema work, not implicated by either decision) were deliberately left unchanged.

## Work

Reorganized 2026-09-21 into three inside-out phases (see Restructuring above). WORK-0006/0007/0010/0011/0012 each appear once, in the phase their still-undesigned domain half belongs to; each also needs a play-half counterpart in Phase 2 once it is actually drafted (not yet a separate document - see Restructuring).

**Foundational (before Phase 1):**

| Order | Work | Status |
|------:|------|--------|
| 1 | WORK-0001 — Session Lobby Foundation | DONE |
| 2 | WORK-0003 — Start + First RuntimeTurn | DONE |
| 3 | WORK-0004 — Interaction Response Processing | DONE |
| — | WORK-0002 — Rename `game/game` → `game/management` (out-of-band structural rename, not sequenced) | DONE |
| 4 | WORK-0005 — Thin Live Coordinator / WebSocket (reduced to a transport skeleton, Blocker 11) | DONE |

**Phase 1 — Session Runtime Domain Completion (`game/session`).** Goal: `Manager` (and every RuntimeTurn-producing method) actually handles all 9 engine `Output` kinds, with the right persistence shape per kind - some derived-on-demand and never persisted (Presentations/Effects), some genuinely durable because they drive a future cause (Timers, UserIntents, Cancellation).

| Order | Work | Status |
|------:|------|--------|
| 5 | WORK-0019 — Replay-First Session Runtime Persistence Migration (moved first - defines the durable model every cause below must satisfy) | DONE |
| 6 | WORK-0006 — domain half only: `Manager` additively returns Effect/Presentation Outputs in memory | DONE |
| 7 | WORK-0007 — domain half only: `RunCompletedOutput` detection + termination | DONE |
| 8 | WORK-0012 — domain half: Timer persistence + `TimerExpired`-as-cause | PLANNED |
| 9 | WORK-0013 — Keyed Timers (WORK-0012's compiler prerequisite) | PLANNED |
| 10 | WORK-0018 — Game Language Disconnect/Reconnect Signal Support (unrelated parallel prerequisite, gates Phase 2's WORK-0015) | PLANNED |
| 11 | WORK-0010 — domain half: UserIntent as a new RuntimeTurn cause | PLANNED |
| 12 | WORK-0011 — domain half: SessionCancelled as a new cause | PLANNED |
| 13 | WORK-0014 — Runtime Failure Diagnostics + Terminal Cleanup | PLANNED |
| 14 | WORK-0016 — Inactivity Expiration / Reaper (schema half) | PLANNED |
| 15 | WORK-0017 — Archival | PLANNED |
| 16 | WORK-0023 — Session Runtime Observability Metrics (needs WORK-0019's replay mechanism to exist first; feeds WORK-0019's own deferred cache decision, Blocker 4) | PLANNED |
| — | WORK-0022 — Session/Platform Abuse and Resource-Rate Limits (identified 2026-09-22 while resolving WORK-0019's Blocker 2; not yet sequenced - see Blockers) | PLANNED |

**Phase 2 — Live Coordinator (`play`, rebuilt once Phase 1 is real).**

| Order | Work | Status |
|------:|------|--------|
| 17 | WORK-0020 — Role-Aware Live Connections / Host Administration Channel (registry foundation, first in this phase) | DRAFT |
| — | WORK-0006/0007/0010/0011/0012 — play halves (not yet split into documents) | not started |
| — | WORK-0029 — Session-Owned Output/Value Schema (decouple `game/session` from `engine`; the actual mapping a play-half consumer reads from) | PLANNED |
| 18 | WORK-0008 — Live Lobby / Session Bootstrap | PLANNED |
| 19 | WORK-0021 — Host Participant Spectator View | PLANNED |
| — | WORK-0015 — Disconnect/Reconnect/Full Resync, play half | PLANNED |

**Phase 3 — Transport (`api`).** Rebuilding `api/session`'s real dispatch happens as part of whichever Phase 2 WORK needs it, not as a separate pass.

| Order | Work | Status |
|------:|------|--------|
| 20 | WORK-0009 — Client-Safe Game UI Manifest (independent read endpoint) | PLANNED |

24 WORK total: 8 DONE, 0 IMPLEMENTING, 0 READY, 1 DRAFT, 15 PLANNED.

## 2026-09-22 Addition: WORK-0022 (Abuse/Resource-Rate Limits)

While resolving WORK-0019's Blocker 2 (removing `session_runtime_steps`), a human review identified that no WORK in this Project owns platform abuse/resource-rate limits above the single-execution level GAME-ADR-0019 already bounds (`engine.Limits`, `runtimeturn.Drain`'s `MaxSteps`) - a real gap given Session Runtime executes user-authored Game Language programs. This was already listed as a deferred design topic in `internal/AI_CONTEXT.md` with no owning WORK, which is itself a process gap (Invariant 1 of `docs/projects/README.md` requires every known-required future outcome to have a WORK, even PLANNED). **WORK-0022** now tracks it. It is not yet sequenced into a phase - it needs human/security decisions on enforcement shape and thresholds (see its own Blockers) before it can be placed and move to DRAFT.

## Ordering / Dependencies

- **Foundational (1-4)**: unchanged from before this restructuring - every later WORK needs a correctly modeled, serialized Session with a working RuntimeTurn executor (1-3), and a working HTTP/WS entry point to eventually expose things through (4, now a transport skeleton per Blocker 11).
- **Phase 1 is now this Project's actual next work**, not WORK-0006/0019/0020 running in parallel with a live `play` as before. WORK-0019 leads it: it defines the durable representation every later cause in this phase (UserIntent, SessionCancelled, TimerExpired) must satisfy, so it can no longer sit in parallel with a transport WORK the way the pre-restructuring plan had it.
- WORK-0006's domain half, WORK-0007's domain half, WORK-0012 (+ its WORK-0013 compiler prerequisite), WORK-0018 (Game-Language-only, no Session Runtime dependency of its own, but gates Phase 2's WORK-0015), WORK-0010's domain half, and WORK-0011's domain half each need only WORK-0019's durable model (where they touch persistence) or nothing beyond the Foundational tier - none of them need a live `play` to exist, since they change `Manager`'s own return values/persistence, not anything transport-facing.
- **Satisfying WORK-0019's durable replay-input model is two separate implementation steps, not one.** (1) Persisting the new cause's content durably and in order - into `session_cause_events` for UserIntent/SessionCancelled/UserDisconnected/UserReconnected, or into `session_timer_obligations` for TimerExpired. (2) Teaching replay itself to turn that durable record back into an `engine.Signal` - a new `case` on `game/session/workflows/sessionlifecycle/replay.go`'s `loadReplaySignal`, alongside its existing interaction-response case. Today `loadReplaySignal` recognizes only that one cause and returns a hard error for any other `source_kind`, by design - so a WORK that adds step (1) without also adding step (2) does not silently misbehave, but it does make every Session that ever used the new cause unreconstructable, which is exactly the correctness bar GAME-ADR-0024 sets. WORK-0010/WORK-0011/WORK-0012/WORK-0013/WORK-0015 must each budget for step (2) when they are drafted, not only for their own new table/column.
- WORK-0014 (diagnostics/cleanup) ideally lands after the other Phase 1 causes exist, so it retrofits once rather than repeatedly, but only strictly needs at least one RuntimeTurn-producing path plus WORK-0019 (no durable Snapshot to reference otherwise).
- WORK-0016's schema half (activity_expires_at) and WORK-0017 (archival) are purely additive, lowest-risk, and trail the rest of Phase 1.
- **Phase 2 cannot start until Phase 1 actually returns/handles the Output data it fans out** - this is the entire point of the restructuring. WORK-0020 leads it (the ADMIN/PARTICIPANT registry foundation everything else in this phase binds into), followed by WORK-0006/0007/0010/0011/0012's play halves (now consuming real in-memory Output values instead of a DB read-back hack), WORK-0008 (needs WORK-0020's role model), WORK-0021 (needs WORK-0020 + WORK-0006's play half), and WORK-0015's play half (needs WORK-0018 from Phase 1, WORK-0019's replay reconstruction, and WORK-0020's per-role reconnect design).
- **Phase 3 (`api`)** is mostly co-implemented with whichever Phase 2 WORK needs a real dispatch layer again - not a separate pass. WORK-0009 is the one independent read endpoint, buildable once Phase 1's UI-facing declarations are stable.

## Capability Coverage

Required to run a Session frontend end-to-end against the supported Game Language semantics. No capability below is in the "required but unowned" state - each is DONE, owned by an active WORK, owned by a PLANNED WORK, or explicitly out of this Project's scope. Re-audited 2026-09-20 (reconciliation pass) against the replay-first persistence and role-aware live-connection decisions.

| Capability | Status | Owner |
|---|---|---|
| Lobby lifecycle (Create/Join/Leave, expiration) | DONE | WORK-0001 |
| Participant lifecycle (join/leave; kick/transfer-host not yet decided in scope) | DONE (join/leave); open scope question otherwise | WORK-0001; see Material Decisions |
| Host control (Start authorization) | DONE; extended by cancellation | WORK-0003; WORK-0011 |
| Start | DONE | WORK-0003 |
| Runtime turns (Step-drain/bound execution) | DONE | WORK-0003 |
| Interactions/questions | DONE | WORK-0004 |
| User intents | Not implemented | WORK-0010 (PLANNED) |
| Presentation state | **DONE** - returned by `Manager.Start`/`AnswerInteraction` in memory; not yet fanned out to clients | WORK-0006 (DONE); client delivery owned by a not-yet-drafted play-half WORK following WORK-0020 |
| UI effects | **DONE** - returned by `Manager.Start`/`AnswerInteraction` in memory; not yet fanned out to clients | WORK-0006 (DONE); client delivery same as above |
| Client-safe UI/game definition | Does not exist | WORK-0009 (PLANNED) |
| Per-viewer output | Partial (Question durably persisted; Presentation/Effect returned in memory, none yet delivered to a client) | WORK-0004 (question) + WORK-0006 (presentation/effect, DONE) |
| Live transport (Create/Join/AnswerInteraction/Deliver) | **Reduced 2026-09-21 (Blocker 11), DONE as a skeleton**: real routes, real WS upgrade, centralized observability + trace/span IDs, no domain coupling - `play`/`play/sessionruntime` removed. Rebuilt by Phase 2's WORK-0020 onward, once Phase 1 completes | WORK-0005 (DONE, skeleton) |
| **Engine Output handling (Presentations/Effects/Timers/RunCompleted returned or persisted)** | Partial - Question Outputs are captured (persisted), Effect/Presentation Outputs are returned to the caller in memory, and a `RunCompletedOutput` terminalizes the Session under a matching `TerminalReasonGame*` value; only timers are still neither persisted nor returned | WORK-0006/0007's domain halves (DONE); WORK-0010/0011/0012's domain halves (Phase 1, PLANNED) |
| **Replay-input persistence** | DONE - `session_runtime_starts` durably persists Start's Seed/RootParameters; `session_runtime_turns` carries no Snapshot column | WORK-0019 |
| **Deterministic runtime reconstruction (process-loss recovery without a stored Snapshot)** | DONE - `reconstructCurrentSnapshot` replays durable state with no cache of any kind, verified against real Postgres | WORK-0019 |
| **Admin live connection** | Design accepted (GAME-ADR-0025); not implemented (no `play` exists at all today) | WORK-0020 (DRAFT, Phase 2) |
| **Participant live connection (role-formalized)** | Not implemented - the single-connection-kind implementation WORK-0005 originally built was removed (Blocker 11); rebuilt by WORK-0020 onward | WORK-0020 (DRAFT, Phase 2) |
| **Role-scoped command authorization** | Design accepted; not implemented (Manager's own per-command checks already exist and remain the actual enforcement) | WORK-0020 (DRAFT) |
| Live lobby / bootstrap / roster / leave, role-specific projection | Not implemented | WORK-0008 (PLANNED), depends on WORK-0020 |
| **Host-as-Participant dual connection** | Design accepted; not implemented | WORK-0020 (DRAFT) |
| **Host spectator view** | Design accepted (GAME-ADR-0025); not implemented | WORK-0021 (PLANNED) |
| **Participant-count lobby fan-out** | Not implemented | WORK-0008 (PLANNED) |
| Terminal notifications | Detection/reason DONE (domain half, WORK-0007); the client-visible broadcast/close still not implemented - must reach both connection roles | WORK-0007 (DONE, domain half); play-half WORK not yet drafted, depends on WORK-0020; reused by WORK-0011/0012/0016 |
| Manual cancellation | Not implemented | WORK-0011 (PLANNED) |
| Timers | Not implemented in Session Runtime (engine-only) | WORK-0012 (PLANNED) |
| Keyed timers | Does not exist in Game Language | WORK-0013 (PLANNED) |
| Disconnect/reconnect | Not implemented; signal schema incomplete; must be per connection role | WORK-0015 (PLANNED), depends on WORK-0018/WORK-0019/WORK-0020 |
| Full resync | Not implemented; must reconstruct via replay, not a stored Snapshot | WORK-0015 (PLANNED), depends on WORK-0019 |
| Inactivity expiration | Not implemented | WORK-0016 (PLANNED) |
| Runtime failure handling | Partial (two narrow fatal paths exist); must not assume a stored Snapshot | WORK-0003/WORK-0004 (existing paths); WORK-0014 (PLANNED, generalizes), depends on WORK-0019 |
| **Session archival/compaction** | Design accepted (GAME-ADR-0024, PostgreSQL JSONB, supersedes the original GCS direction) | WORK-0017 (PLANNED, rewritten), depends on WORK-0019 |
| Authentication / Identity | Not implemented, intentionally | Explicitly out of scope for this Project |

No required outcome from this reconciliation pass emerged ownerless - every bolded new row above already has an owning WORK created or revised by this pass.

## Material Decisions Needing Human Input

These were found during this Project's completeness audit and reconciliation (2026-09-20). None have been silently decided; each is recorded here rather than assumed.

1. **Host "kick a participant" / "transfer host"** - not clearly known to be required for a Session Runtime V1 frontend, so no WORK was created for it. If it is required, it needs its own PLANNED WORK; if not, it should be recorded as explicitly out of scope rather than left ambiguous.
2. **WORK-0007's `LOBBY_EXPIRED`-while-connected gap** - becomes concretely reachable once WORK-0008 (live lobby) lets a client hold a connection before `Start`. Worth resolving when WORK-0007 and WORK-0008 are jointly refined; not resolved by this migration.
3. **WORK-0018's domain placement** - it is Game Language/compiler work (not Session Runtime application logic), tracked under this Project because it directly gates WORK-0015, the same treatment the original plan already gave Keyed Timers (WORK-0013). Confirm this is the right call rather than spinning up a separate Game Language initiative for it.
4. **The recommended execution order above** is derived from actual code dependencies found during this audit, not merely inherited from the original plan (most notably: Keyed Timers now ordered before Disconnect/Reconnect, reversing the original order). It is a recommendation, not a commitment - nothing has been implemented against it yet.

5. **WORK-0005's original host/Participant Blocker (Blocker 8) has been transferred to WORK-0020, not resolved in place.** (2026-09-20 update, superseding this item's prior wording) A broader reconciliation session found the host-connection gap sits inside a larger required capability (role-aware ADMIN/PARTICIPANT live connections, independently needed by WORK-0007/WORK-0008/WORK-0011/WORK-0021) and created `game/docs/decisions/GAME-ADR-0025-role-aware-live-connections.md` (ACCEPTED - the architecture direction itself) and `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (DRAFT - the concrete implementation, adopting WORK-0005's own prior proposal as its starting point) to own it. What still needs explicit human input is WORK-0020's own Blockers 1-3 (confirming/broadening the inherited mechanism now that its scope is wider than WORK-0005's original narrow fix, the exact route shape, and whether the registry should anticipate WORK-0021's future spectator-subscription needs) before WORK-0020 can move DRAFT -> READY. WORK-0005 itself is no longer blocked by this and returned to IMPLEMENTING (see WORK-0005's own "Status Correction (Part E Reconciliation, 2026-09-20)"), pending only its own independent review.
6. **WORK-0019's Blockers 1-5 (replay-first persistence implementation shape)** - durable Seed/RootParameters column/table shape, `session_runtime_steps`'s final disposition (a genuine tradeoff between debuggability and the same unbounded-growth concern GAME-ADR-0024 otherwise resolves - worth explicit human input rather than a unilateral implementation choice), the durable representation for each future RuntimeTurn cause without an existing normalized home, whether an ephemeral current-state cache is actually needed yet (a NOW/SOON/LATER/NOT NEEDED call), and migration sequencing. See WORK-0019's own Blockers section.
7. **WORK-0020's Blockers 1-3 (role-aware connection implementation shape)** - see item 5 above and WORK-0020's own Blockers section.
8. **GAME-ADR-0024's still-open replay-compatibility question**: whether a future compiler/engine-build change (beyond the already-existing pinned `program.Metadata.LanguageVersion`) ever needs its own tracking mechanism. Not urgent (no such change exists today), but flagged rather than silently assumed away - see GAME-ADR-0024's "Replay compatibility" section.
9. **WORK-0021 (Host Participant Spectator View) was deliberately left PLANNED, not DRAFT**, specifically so its initial-Presentation-reconstruction mechanism can reuse WORK-0015's eventual generic resync/projection capability rather than duplicating that logic before WORK-0015 exists. A human may want to confirm this sequencing judgment (defer WORK-0021's actual design until closer to WORK-0015) rather than accelerate it, given WORK-0021's other two dependencies (WORK-0020, WORK-0006) land much earlier.

## Completion Criteria

Session Runtime V1 is complete when:

1. Every WORK in the table above is DONE, or explicitly moved to this Project's Out Of Scope with human confirmation.
2. Every row in the Capability Coverage matrix reads either DONE or "explicitly out of scope" - none reads "not implemented"/"partial" against a capability this Project's Goal requires.
3. Authentication/Identity integration remains the one deliberately excluded exception, tracked as a separate future initiative rather than blocking this Project's closure.

When satisfied, this Project moves from `docs/projects/active/` to `docs/projects/completed/`.
