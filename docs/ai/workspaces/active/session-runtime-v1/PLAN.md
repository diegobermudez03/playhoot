# session-runtime-v1 — Implementation Plan

Status: TEMPORARY / NON-CANONICAL / NON-AUTHORITATIVE. Initiative-level decomposition only. Does not own accepted architecture/domain/product truth, detailed implementation contracts, or WORK status/progress - see `AI_CONTEXT.md` for canonical-reference links and `docs/work/` for actual WORK once slices graduate into Feature Development.

Architecture Discussion for this initiative is CLOSED (see `AI_CONTEXT.md` resume header and `HUMAN_REVIEW.md`). This plan decomposes the accepted Session Runtime V1 direction (`game/README.md`, `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`, `game/docs/decisions/GAME-ADR-0001` through `GAME-ADR-0020`) into implementable slices. No slice below is READY WORK; materialization is just-in-time per `docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md`'s Feature Development graduation model.

## Initiative Implementation Goal

Implement the accepted Session Runtime V1 architecture incrementally until a real multiplayer Session can execute end-to-end according to the accepted contracts: a host creates a Session, players join a lobby, the host starts it, the authored Game Language program executes turn by turn through accepted interactions/timers, players may disconnect/reconnect, and the Session reaches a terminal state with no dangling runtime obligations - all under the accepted persistence, serialization, and failure-handling model.

## Actual Implementation Drift Found (Inspected Before Planning)

- **Game Language (engine) is implemented and tested** (`game/language/v1/engine/`, `game/language/v1/program/`): `engineservice.Compile`, `NewSnapshot`, `Step`, `Evaluate` all exist and work; `Commit.InternalSignals` drains only when the caller re-invokes `Step` - Session Runtime owns that draining loop today and it does not exist yet. This is the reusable foundation every RUNNING-phase slice below builds on.
- **Game Management read capability is implemented**: `game/game/usecases/getgame.GetPlayableGameWithCurrentVersion` already returns a `game.Game` with a decoded `program.Definition`, ready to feed `engineservice.Compile`. The "resolve/pin the immutable Game definition through a narrow Game Management read capability" architecture requirement (GAME-ADR-0001, GAME-ADR-0004) already has its building block; Session Runtime just doesn't call it yet.
- **Session Runtime is scaffolding only, and does not currently build.** `game/session/workflows/sessionlifecycle/internal/repo/step_create_room.go` declares `func (r *Repo) CreateRoom(ctx context.Context)` with no body - this does not compile. `CreateRoom` (public) and `JoinRoom` are no-op stubs returning zero values.
- **Existing Session Runtime scaffolding directly contradicts accepted architecture**, not just "incomplete":
  - `CreateRoom(ctx, program engine.Program, gameVersionUUID string, ownerUUID string)` accepts an externally-supplied, already-compiled `engine.Program` - GAME-ADR-0004 requires Session Runtime itself to resolve/pin/compile the definition.
  - `session`/`sessionPlayer` use raw `OwnerUUID`/`PlayerUUID` string fields - not the accepted `SessionActorID`-internal / `UserUUID`-public boundary (GAME-ADR-0005).
  - The existing schema (`sessions`, `session_states`, `session_players`, `join_codes` in `game/session/internal/storage/tables.go`, mirrored in `game/docs/DATA_MODEL.md`) has none of the accepted tables: no `session_requests`, `session_runtime_turns`, `session_runtime_steps`, `session_runtime_state`, `session_actors`, `session_participants`, `session_interactions`, `session_timer_obligations`, `session_runtime_failures`, `session_history_archives`; no `lobby_expires_at`/`activity_expires_at`/`semantic_presence`.
- **No Live Session Coordinator/WebSocket/transport code exists anywhere in the repository.** `play/` (the most plausible future home given `api/README.md`'s "external transport edge" description and the empty `play/` directory) contains no files at all - not even a README. Coordinator is not merely unimplemented; it has no current placeholder to extend.
- **No DB-locking/per-Session serialization mechanism, `MAX_STEPS_PER_RUNTIME_TURN` bound, terminal-cleanup transaction, or runtime-failure diagnostic persistence exists anywhere** - GAME-ADR-0018/0019/0017/0020 are accepted design with zero implementation surface today.
- **Identity has an accepted domain model but no implementation** (`identity/CURRENT_STATE.md`). Session Runtime's public boundary requires an authenticated `UserUUID` from a trusted layer (GAME-ADR-0005), but there is currently nothing that authenticates a caller into a `UserUUID`. Slice 1 below must decide (as an implementation-planning/Feature-Development question, not decided here) how early slices obtain a `UserUUID` before Identity exists - most likely a temporary trusted-caller assumption for internal development - without inventing a permanent authentication bypass.
- **`game/language/v1/program/signal.go`'s `NamedSignalSource` catalog and `compile_signals.go`'s `namedLifecycleSignals`** still contain only a placeholder empty-schema `UserDisconnected`; `UserReconnected` and any `KeyedTimerSlot` capability do not exist in `program`/`engine` at all (GAME-ADR-0011/GAME-ADR-0012 accept the target design, not the implementation).

## Ordered Implementation Slices

Status: APPROVED SEQUENCE, human-approved 2026-09-08. The most important change from the originally proposed order is that a thin Live Coordinator/WebSocket slice now moves to position 4, immediately after Interaction Response Processing, instead of near the end. Reason: after Slices 1-3, Playhoot can create/join/start a Session and process one interaction entirely at the Go-API level, but nobody has ever exercised the real live path - moving a deliberately thin Coordinator here lets Playhoot exercise a real multiplayer Session from connected clients instead of implementing almost all remaining backend/runtime architecture (timers, failure diagnostics, disconnect/reconnect, inactivity, keyed timers, archival) before ever validating that live path. This is a sequencing/risk-reduction choice, not an architecture change.

### Slice 1 — Session Lobby Foundation

Status: graduated to DRAFT WORK. See `docs/work/active/WORK-0001-session-lobby-foundation.md`.

**Delivers**: Create/Join/Leave working correctly under the accepted persistence model and LOBBY per-Session DB-locking serialization (GAME-ADR-0004): `session_requests` idempotency, `session_actors`/`session_participants` replacing raw owner/player UUID fields, `lobby_expires_at`, JoinCode issuance/revocation, and Session Runtime itself resolving/pinning/compiling the Game definition through the existing `getgame` read capability instead of accepting an external `engine.Program`. A Session can be created, joined, left, and reconstructed from durable storage. No Game execution and no WebSocket/runtime execution happen in this slice.

**Major accepted dependencies**: GAME-ADR-0001 (persistence/transaction boundary), GAME-ADR-0002/0003 (durable boundary, actor/lifecycle foundations), GAME-ADR-0004 (lobby contract, LOBBY serialization), GAME-ADR-0005 (public/internal identity boundary), ADR-0005/IDENTITY-ADR-0001 (cross-domain UUID reference), `getgame.GetPlayableGameWithCurrentVersion` + `engineservice.Compile` (already implemented).

**Why first**: The existing scaffolding does not compile and directly contradicts accepted architecture (external `Program`, raw UUID fields, wrong schema) - this is the most urgent correctness gap, not an enhancement. Every later slice needs a correctly modeled, serialized Session to attach to. It also proves out the DB-locking serialization mechanism (the foundation GAME-ADR-0018 later extends to RUNNING) at the lowest-risk point, before any engine execution is involved.

**What later slices depend on it**: every slice below.

**Known implementation drift/risk**: replaces the current schema/migrations outright rather than extending them; Identity/`UserUUID` handled per the approved boundary below (trusted caller-supplied `UserUUID`, no fake-auth); `CreateRoom`/`JoinRoom` public signatures must change (no external `engine.Program` parameter).

**Identity/Auth boundary (approved)**: Session application/domain APIs receive an already-trusted `UserUUID` per the accepted architecture (GAME-ADR-0005) - credential-to-`UserUUID` resolution belongs outside Session Runtime and is integrated later when a real transport/auth boundary exists (Slice 4+). Slice 1 does not implement Identity/Auth, and does not add a temporary fake-auth mechanism that would later need removal; test/internal callers supply `UserUUID` directly, exactly as the accepted public contract already expects them to.

### Slice 2 — Start + First RuntimeTurn

**Delivers**: Start transitions `LOBBY -> RUNNING` by calling `engineservice.NewSnapshot`, draining `Commit.InternalSignals` through repeated `Step` calls, and persisting the resulting Turn 1 / `session_runtime_state` / `session_runtime_turns` / `session_runtime_steps` atomically with `phase = RUNNING`, `started_at`, and JoinCode revocation. **This slice introduces the common RuntimeTurn executor that every later runtime cause (interaction response in Slice 3, timer expiration in Slice 5) reuses, and that executor must enforce `MAX_STEPS_PER_RUNTIME_TURN = 20` from the moment it exists** - there must be no intermediate implementation where Start (or any subsequent caller of the same executor) can drain `InternalSignals` indefinitely. Includes the pre-first-Turn fatal-failure path (GAME-ADR-0019): a deterministic initialization failure (including exceeding the Step bound) terminalizes directly from `LOBBY` with `base_turn_id = NULL`, no Turn 1, `started_at` left null - distinguished from transient infrastructure failure, which leaves the Session retryable in `LOBBY`. The richer persistent `SessionRuntimeFailure` diagnostics surface (stable error codes, `diagnostic_payload`, querying failures by cause) is intentionally deferred to Slice 6 - this slice only needs the guard to actually stop execution and terminalize correctly, not the full diagnostic entity.

**Major accepted dependencies**: GAME-ADR-0007 (RuntimeTurn model), GAME-ADR-0019 (Step bound, pre-first-Turn fatal Start, RUNNING-only-after-commit), GAME-ADR-0006 (`players: list<user>` root roster).

**Why at this point**: This is the highest-risk, most novel piece of the whole architecture - the RuntimeTurn transactional model has never been exercised against real persistence. Proving it out right after the lobby foundation reduces the risk of costly rework later, and establishes the one common executor every later runtime-cause slice reuses.

**What later slices depend on it**: Slices 3, 5, and 6 all execute RuntimeTurns using the same executor/Step-bound guard this slice establishes.

**Known implementation drift/risk**: none of the accepted RuntimeTurn tables exist yet (see drift above); this slice creates them from scratch, not by migrating existing data.

### Slice 3 — Interaction Response Processing

**Delivers**: A player can answer an open interaction. Session Runtime obtains per-Session serialization (GAME-ADR-0018), reloads current `current_turn_id`/Snapshot, calls `Step` with the response signal through Slice 2's common RuntimeTurn executor (so the same 20-Step guard applies automatically), persists the resulting RuntimeTurn, and updates `session_interactions` (`opened_by_turn_id`/`closed_by_turn_id`).

**Major accepted dependencies**: GAME-ADR-0018 (RUNNING serialization, reload-after-lock, ordering-by-commit), GAME-ADR-0007 (Interaction persistence via Turn references).

**Why at this point**: Needs Slice 2's engine wiring/executor to exist; is the simplest RUNNING-phase mutation (no physical scheduling involved, unlike timers) so it validates the RUNNING serialization mechanism with the least additional moving parts, and completes the minimal vertical slice (create -> join -> start -> answer) that Slice 4's thin Coordinator needs to expose over a real transport.

**What later slices depend on it**: Slice 4 (the thin Coordinator delivers exactly this operation set live), Slice 5 (timers competing with interactions for the same serialization boundary), Slice 7 (disconnect/reconnect interacting with open interactions).

### Slice 4 — Thin Live Coordinator / WebSocket

**Delivers**: A deliberately minimal live transport binding sufficient to exercise Create -> Join -> Start -> answer-one-interaction end-to-end from real connected clients: connection authentication/binding to `(SessionUUID, UserUUID)`, translating Slice 1-3's existing Session application operations into transport requests, and fanning out the resulting engine `Output`s (at minimum whatever is needed to see an opened interaction and its resolution) to connected clients - all under the already-accepted best-effort delivery contract (GAME-ADR-0020: durable commit is authoritative, no generic delivery outbox).

This slice intentionally EXCLUDES the later operational complexity that belongs to subsequent slices - it must not silently grow to include: disconnect grace/debounce, semantic presence tracking/recovery, timer physical scheduling/reconciliation, an inactivity reaper, full process-loss recovery, or advanced reconnect semantics. A client that disconnects simply stops receiving further messages until Slice 7 exists; this slice does not need to handle that gracefully yet.

**Major accepted dependencies**: GAME-ADR-0002 (Coordinator boundary - ephemeral, no authoritative truth), GAME-ADR-0020 (post-commit delivery is best-effort, no outbox). Explicitly NOT yet needed here: GAME-ADR-0010 (disconnect/reconnect/resync boundary), GAME-ADR-0008/0013 (timer recovery), GAME-ADR-0014 (inactivity), GAME-ADR-0015/0016 (semantic presence) - all deferred to Slice 7 (or Slice 5 for timer scheduling).

**Why at this point**: After Slices 1-3, Playhoot can exercise the full create/join/start/answer loop only at the Go-API level - nobody has proven the actual live path works. Validating that early, with a thin slice, catches transport/integration problems before nearly all remaining backend/runtime architecture is built on top of an unproven live path.

**What later slices depend on it**: Slice 5 (physical timer scheduling extends this same Coordinator), Slice 7 (disconnect grace/reconnect/resync extends this same Coordinator with exactly the operational complexity excluded here).

**Known implementation drift/risk**: no Coordinator/WebSocket code or even a placeholder package exists anywhere in the repository yet (the most plausible future home, `play/`, is completely empty - not even a README). This slice creates the first such code and must actively resist scope creep into the excluded concerns above.

### Slice 5 — Timer Obligations

**Delivers**: `session_timer_obligations` persistence, ordinary (non-keyed) `TimerSlot` scheduling/cancellation/expiration wired through Slice 2's common RuntimeTurn executor, and physical timer scheduling added to Slice 4's already-existing thin Coordinator (not a new Coordinator mechanism - it extends the one already built) using the full-configured-delay recovery tradeoff (GAME-ADR-0008, GAME-ADR-0013).

**Major accepted dependencies**: GAME-ADR-0008 (no durable due-at, full-delay reschedule), GAME-ADR-0013 (process-agnostic recovery), GAME-ADR-0018 (timer expiration contends for the same per-Session serialization as interaction responses - the concrete GAME-ADR-0018 worked example, Q5 vs. T1).

**Why at this point**: Needs Slice 3's RuntimeTurn-from-external-cause pattern and Slice 4's live Coordinator (to actually schedule/fire a physical timer) already working; introduces the first genuinely concurrent-cause scenario the RUNNING serialization model must handle correctly.

**What later slices depend on it**: Slice 7 (disconnect timeouts are timers), Slice 9 (keyed timers extend this same mechanism).

### Slice 6 — Failure Diagnostics + Terminal Cleanup

**Delivers**: The richer persistent `SessionRuntimeFailure` diagnostics surface (GAME-ADR-0017) around the fatal failures Slice 2's Step-bound guard already stops - `session_runtime_failures` persistence, stable error codes (including `runtime_turn_step_limit_exceeded`), atomic fatal materialization - plus the terminal-cleanup invariant (GAME-ADR-0019): a `TERMINAL` Session retains no `ACTIVE` interaction/timer obligation, closed atomically with `closed_by_turn_id = NULL` + `SESSION_TERMINATED`-equivalent closure reason when not Turn-produced.

**Major accepted dependencies**: GAME-ADR-0017 (failure taxonomy, diagnostic entity), GAME-ADR-0019 (terminal cleanup, diagnostic error code).

**Why at this point**: Slice 2 already stops execution correctly on Step-bound overflow (that guard is not optional and is not deferred here); what this slice adds is the *richer diagnostic persistence and queryability* plus the *terminal-cleanup atomicity* across the RuntimeTurn-producing paths that now exist (Start, interaction, timer) - most efficiently built once those three paths and Slice 4's live Coordinator already exist, rather than designed four times.

**What later slices depend on it**: Slice 8 (inactivity expiration reuses the same terminal-cleanup invariant).

### Slice 7 — Disconnect / Reconnect + Resync

**Delivers**: `session_actors.semantic_presence`, phase-dependent LOBBY/RUNNING consequences (GAME-ADR-0015), the atomic presence-transition-plus-Game-Language-processing rule (GAME-ADR-0016), the resync read capability (GAME-ADR-0010) returning a player-facing projection instead of the raw Snapshot, and - critically - extending Slice 4's thin Coordinator with exactly the operational complexity that slice intentionally excluded: transport grace/debounce, semantic-disconnect reporting, and reconnect handling.

**Major accepted dependencies (including a Game Language prerequisite)**: GAME-ADR-0010, GAME-ADR-0011, GAME-ADR-0015, GAME-ADR-0016. **Game Language prerequisite**: `UserDisconnected`'s placeholder schema must be given its accepted `{user: user}` shape and `UserReconnected` must be added to the compiler's `namedLifecycleSignals` catalog (`game/language/v1/engine/internal/compiler/compile_signals.go`) *before* Session Runtime can deliver either signal - this is Game Language work, not Session Runtime work, and must land first.

**Why at this point**: Needs Slice 4's live Coordinator (there is no transport connection to disconnect/reconnect without it) and Slices 2-5 (a running engine and open interactions/timers to interact with disconnect state), plus a completed Game Language prerequisite with its own lead time.

**What later slices depend on it**: Slice 9 (per-player disconnect timeout is the primary motivating use case for keyed timers).

### Slice 8 — Inactivity Expiration / Reaper

**Delivers**: `sessions.activity_expires_at`, renewal on meaningful activity, lazy materialization on a stale operation, and a background Reaper (GAME-ADR-0014), reusing Slice 6's terminal-cleanup invariant.

**Major accepted dependencies**: GAME-ADR-0014, GAME-ADR-0019 (terminal cleanup).

**Why at this point**: Purely additive once RUNNING operations (Slices 2-3, 5) exist to renew/validate against; lower architectural risk than Slices 1-7, so it can follow rather than gate them.

**What later slices depend on it**: none beyond general Coordinator hygiene (stop delivering to an inactivity-terminalized Session, already covered by Slice 4/7's Coordinator).

### Slice 9 — Keyed Timers

**Delivers**: The general `KeyedTimerSlot<Key>` Game Language capability (compiler + engine) and the `session_timer_obligations.engine_key` persistence consequence (GAME-ADR-0012).

**Major accepted dependencies (Game Language prerequisite)**: This slice *is* the Game Language prerequisite work itself - compiler/engine changes in `game/language/v1/program`/`game/language/v1/engine/internal/compiler` - consumed by Session Runtime's Slice 5 timer mechanism.

**Why at this point**: Not required for a minimally playable end-to-end game (a single ordinary `TimerSlot` per disconnect policy is a viable interim), so it can follow the core loop; but it should precede any game definition that actually needs independent per-player timers.

**What later slices depend on it**: any future game definition/authored policy needing simultaneous independent timers (for example, independent per-player disconnect timeouts).

### Slice 10 — Archival

**Delivers**: `session_history_archives`, JSON archive serialization to object storage (for example GCS), and verified hard-delete of heavy runtime-history tables after successful archival (GAME-ADR-0009).

**Major accepted dependencies**: GAME-ADR-0009.

**Why last**: Lowest architectural risk, purely additive, and explicitly not required for an initial end-to-end playable Session - archival only matters once real Sessions have actually produced history worth archiving/deleting.

**What later slices depend on it**: none.

## Dependencies Summary

- Slice 1 gates everything.
- Slice 2 gates Slices 3, 5, and 6 (all RuntimeTurn-producing work reuses its common executor and 20-Step guard - there must be no window where any of them can drain `InternalSignals` unbounded).
- Slice 3 gates Slice 4 (the thin Coordinator has nothing live to expose without Create/Join/Start/answer already working at the Go-API level).
- Slice 4 gates Slice 5 (physical timer scheduling extends it) and Slice 7 (disconnect grace/reconnect extends it) - both reuse the same Coordinator rather than inventing a second one.
- Slice 7 has a hard Game Language prerequisite (`UserDisconnected`/`UserReconnected` compiler support) that must land before Session Runtime code in that slice can deliver either signal.
- Slice 9 (keyed timers) is itself Game Language work consumed by Slice 5's timer mechanism, but is not required before Slice 5 - only before a game definition needs simultaneous independent timers.
- Slice 6's diagnostic/terminal-cleanup work depends on Slices 2, 3, and 5 already existing (it retrofits diagnostics/cleanup across all three RuntimeTurn-producing paths); the Step-bound guard itself is not deferred and lands in Slice 2.

## Deferred / Out Of Scope For This Plan

- Post-launch "Session re-entry after complete client-state loss" UX, recorded non-authoritatively in `docs/product/IDEAS.md` - not designed, not part of any slice above.
- Post-Start dynamic player admission / late admission into an already-running Session - explicitly deferred by GAME-ADR-0015/GAME-ADR-0016.
- The exact durable archival JSON schema and GCS implementation details beyond what Slice 10 minimally needs (GAME-ADR-0009 leaves this deferred).
- Future external irreversible Game Language side-effect delivery semantics (payment, external API mutation, durable notification, cross-domain command) - explicitly excluded from GAME-ADR-0020's best-effort delivery rule and requiring its own future architecture decision if/when such a capability is proposed.
- Host transfer, exhaustive `source_kind`/interaction-kind enums, platform abuse/resource limits and per-user/session rate boundaries, mutable global display/profile ownership, Identity reconciliation/merge/alias semantics, Session Configuration/arbitrary external game-specific root params - all remain deferred design topics per `AI_CONTEXT.md`'s Deferred Design Topics list and are not slices in this plan.

## WORK References

- **WORK-0001** - `docs/work/active/WORK-0001-session-lobby-foundation.md` - Slice 1, Session Lobby Foundation. Status: DRAFT (not READY; no implementation authority yet).

Per the just-in-time WORK model, no other slice has been materialized into Feature Development yet. Slice 2 (Start + First RuntimeTurn) is the next candidate once WORK-0001 reaches DONE.

## Recommended First Slice

**Slice 1 — Session Lobby Foundation**, now graduated into `WORK-0001-session-lobby-foundation.md` (DRAFT, not READY).

Rationale: (a) dependency order - every other slice needs a correctly modeled, serialized Session; (b) current repository state - the existing scaffolding does not compile (`step_create_room.go`'s `CreateRoom` has no function body) and actively contradicts accepted architecture (external `engine.Program` parameter, raw owner/player UUID fields, an entirely superseded schema), so this is an urgent correctness gap, not a nice-to-have; (c) useful vertical progress - delivers an actually working Create/Join/Leave flow; (d) risk reduction - proves out the DB-locking per-Session serialization mechanism (which GAME-ADR-0018 later extends to RUNNING) at the lowest-risk point, before any engine execution is layered on top; (e) validates architecture early - exercises the "Session Runtime resolves/pins/compiles the Game definition itself" pattern this whole initiative depends on.

The human must still explicitly approve WORK-0001 as READY before any implementation begins.
