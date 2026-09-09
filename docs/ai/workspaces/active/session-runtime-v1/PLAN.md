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

### Slice 1 — Session Lobby Foundation

**Delivers**: Create/Join/Leave working correctly under the accepted persistence model and LOBBY per-Session DB-locking serialization (GAME-ADR-0004): `session_requests` idempotency, `session_actors`/`session_participants` replacing raw owner/player UUID fields, `lobby_expires_at`, JoinCode issuance/revocation, and Session Runtime itself resolving/pinning/compiling the Game definition through the existing `getgame` read capability instead of accepting an external `engine.Program`.

**Major accepted dependencies**: GAME-ADR-0001 (persistence/transaction boundary), GAME-ADR-0002/0003 (durable boundary, actor/lifecycle foundations), GAME-ADR-0004 (lobby contract, LOBBY serialization), GAME-ADR-0005 (public/internal identity boundary), ADR-0005/IDENTITY-ADR-0001 (cross-domain UUID reference), `getgame.GetPlayableGameWithCurrentVersion` + `engineservice.Compile` (already implemented).

**Why first**: The existing scaffolding does not compile and directly contradicts accepted architecture (external `Program`, raw UUID fields, wrong schema) - this is the most urgent correctness gap, not an enhancement. Every later slice needs a correctly modeled, serialized Session to attach to. It also proves out the DB-locking serialization mechanism (the foundation GAME-ADR-0018 later extends to RUNNING) at the lowest-risk point, before any engine execution is involved.

**What later slices depend on it**: every slice below.

**Known implementation drift/risk**: replaces the current schema/migrations outright rather than extending them; must resolve the Identity/`UserUUID` gap noted above; `CreateRoom`/`JoinRoom` public signatures must change (no external `engine.Program` parameter).

### Slice 2 — Start & First RuntimeTurn (Engine Wiring)

**Delivers**: Start transitions `LOBBY -> RUNNING` by calling `engineservice.NewSnapshot`, draining `Commit.InternalSignals` through repeated `Step` calls up to `MAX_STEPS_PER_RUNTIME_TURN` (GAME-ADR-0019), and persisting the resulting Turn 1 / `session_runtime_state` / `session_runtime_turns` / `session_runtime_steps` atomically with `phase = RUNNING`, `started_at`, and JoinCode revocation. Includes the pre-first-Turn fatal-failure path (GAME-ADR-0019): a deterministic initialization failure terminalizes directly from `LOBBY` with `base_turn_id = NULL`, no Turn 1, `started_at` left null - distinguished from transient infrastructure failure, which leaves the Session retryable in `LOBBY`.

**Major accepted dependencies**: GAME-ADR-0007 (RuntimeTurn model), GAME-ADR-0019 (Step bound, pre-first-Turn fatal Start, RUNNING-only-after-commit), GAME-ADR-0006 (`players: list<user>` root roster).

**Why at this point**: This is the highest-risk, most novel piece of the whole architecture - the RuntimeTurn transactional model has never been exercised against real persistence. Proving it out right after the lobby foundation (rather than after building more surface area on top of an unproven mechanism) reduces the risk of costly rework later.

**What later slices depend on it**: Slices 3-9 all execute RuntimeTurns using the same mechanism this slice establishes.

**Known implementation drift/risk**: none of the accepted RuntimeTurn tables exist yet (see drift above); this slice creates them from scratch, not by migrating existing data.

### Slice 3 — RUNNING Interaction Response Processing

**Delivers**: A player can answer an open interaction. Session Runtime obtains per-Session serialization (GAME-ADR-0018), reloads current `current_turn_id`/Snapshot, calls `Step` with the response signal, persists the resulting RuntimeTurn, and updates `session_interactions` (`opened_by_turn_id`/`closed_by_turn_id`). This is the first exercise of GAME-ADR-0018's RUNNING serialization and authoritative-ordering rule with two genuinely concurrent causes possible (see Slice 4).

**Major accepted dependencies**: GAME-ADR-0018 (RUNNING serialization, reload-after-lock, ordering-by-commit), GAME-ADR-0007 (Interaction persistence via Turn references).

**Why at this point**: Needs Slice 2's engine wiring to exist; is the simplest RUNNING-phase mutation (no physical scheduling involved, unlike timers) so it validates the RUNNING serialization mechanism with the least additional moving parts.

**What later slices depend on it**: Slice 4 (timers competing with interactions for the same serialization boundary), Slice 6 (disconnect/reconnect interacting with open interactions).

### Slice 4 — Timer Obligations & Expiration Processing

**Delivers**: `session_timer_obligations` persistence, ordinary (non-keyed) `TimerSlot` scheduling/cancellation/expiration wired through RuntimeTurn processing, and the first real Coordinator-owned ephemeral mechanism: physical timer scheduling plus recovery/reconciliation after process restart using the full-configured-delay tradeoff (GAME-ADR-0008, GAME-ADR-0013).

**Major accepted dependencies**: GAME-ADR-0008 (no durable due-at, full-delay reschedule), GAME-ADR-0013 (process-agnostic recovery), GAME-ADR-0018 (timer expiration contends for the same per-Session serialization as interaction responses - this is the concrete GAME-ADR-0018 worked example, Q5 vs. T1).

**Why at this point**: Needs Slice 3's RuntimeTurn-from-external-cause pattern already working; introduces the first genuinely concurrent-cause scenario the RUNNING serialization model must handle correctly.

**What later slices depend on it**: Slice 6 (disconnect timeouts are timers), Slice 8 (keyed timers extend this same mechanism), Slice 9 (Coordinator's physical-timer piece grows from here).

### Slice 5 — RuntimeTurn Execution Bound & Runtime Failure Diagnostics

**Delivers**: `MAX_STEPS_PER_RUNTIME_TURN` enforcement across all RuntimeTurn-producing paths (Start, interaction response, timer expiration), the four-class failure taxonomy (GAME-ADR-0017), `session_runtime_failures` persistence, atomic fatal materialization, and the terminal-cleanup invariant (GAME-ADR-0019): a `TERMINAL` Session retains no `ACTIVE` interaction/timer obligation, closed atomically with `closed_by_turn_id = NULL` + `SESSION_TERMINATED`-equivalent closure reason when not Turn-produced.

**Major accepted dependencies**: GAME-ADR-0017 (failure taxonomy, diagnostic entity), GAME-ADR-0019 (Step bound, terminal cleanup, diagnostic error code).

**Why at this point**: The Step bound is technically already required by Slice 2 (Start is subject to it too) and could be implemented alongside Slices 2-4; it is called out as its own slice because the *diagnostic persistence* and *terminal-cleanup atomicity* are cross-cutting concerns most efficiently retrofitted once the three RuntimeTurn-producing paths (Start, interaction, timer) already exist, rather than designed three times. Treat Slices 2/5 as tightly coupled in practice - a Feature Development pass may reasonably fold the basic Step-counting mechanism into Slice 2 and defer only the full diagnostic/terminal-cleanup surface to this slice.

**What later slices depend on it**: Slice 7 (inactivity expiration reuses the same terminal-cleanup invariant), Slice 9 (Coordinator must know a Session became `TERMINAL` via this path to stop delivering to it).

### Slice 6 — Disconnect, Reconnect, Semantic Presence & Resync

**Delivers**: `session_actors.semantic_presence`, phase-dependent LOBBY/RUNNING consequences (GAME-ADR-0015), the atomic presence-transition-plus-Game-Language-processing rule (GAME-ADR-0016), and the resync read capability (GAME-ADR-0010) returning a player-facing projection instead of the raw Snapshot.

**Major accepted dependencies (including a Game Language prerequisite)**: GAME-ADR-0010, GAME-ADR-0011, GAME-ADR-0015, GAME-ADR-0016. **Game Language prerequisite**: `UserDisconnected`'s placeholder schema must be given its accepted `{user: user}` shape and `UserReconnected` must be added to the compiler's `namedLifecycleSignals` catalog (`game/language/v1/engine/internal/compiler/compile_signals.go`) *before* Session Runtime can deliver either signal - this is Game Language work, not Session Runtime work, and must land first.

**Why at this point**: Needs Slices 2-4 (a running engine and open interactions/timers to interact with disconnect state) and a completed Game Language prerequisite that has its own lead time - sequencing it after the core RuntimeTurn loop is proven avoids blocking that core loop on a Game Language change.

**What later slices depend on it**: Slice 8 (per-player disconnect timeout is the primary motivating use case for keyed timers), Slice 9 (Coordinator's transport-grace mechanism reports semantic disconnect into this slice's capability).

### Slice 7 — Durable Inactivity Expiration & Session Reaper

**Delivers**: `sessions.activity_expires_at`, renewal on meaningful activity, lazy materialization on a stale operation, and a background Reaper (GAME-ADR-0014), reusing Slice 5's terminal-cleanup invariant.

**Major accepted dependencies**: GAME-ADR-0014, GAME-ADR-0019 (terminal cleanup).

**Why at this point**: Purely additive once RUNNING operations (Slices 2-4) exist to renew/validate against; lower architectural risk than Slices 1-6, so it can follow rather than gate them.

**What later slices depend on it**: Slice 9 (Coordinator should stop attempting delivery to an inactivity-terminalized Session).

### Slice 8 — Keyed Timer Slots

**Delivers**: The general `KeyedTimerSlot<Key>` Game Language capability (compiler + engine) and the `session_timer_obligations.engine_key` persistence consequence (GAME-ADR-0012).

**Major accepted dependencies (Game Language prerequisite)**: This slice *is* the Game Language prerequisite work itself - compiler/engine changes in `game/language/v1/program`/`game/language/v1/engine/internal/compiler` - consumed by Session Runtime's Slice 4 timer mechanism.

**Why at this point**: Not required for a minimally playable end-to-end game (a single ordinary `TimerSlot` per disconnect policy is a viable interim), so it can follow the core loop; but it should precede any game definition that actually needs independent per-player timers.

**What later slices depend on it**: any future game definition/authored policy needing simultaneous independent timers (for example, independent per-player disconnect timeouts).

### Slice 9 — Live Session Coordinator & Post-Commit Delivery

**Delivers**: The actual transport/WebSocket edge - live connection binding, Output fan-out to connected clients, the transport grace/debounce mechanism feeding Slice 6's semantic-disconnect path, physical timer scheduling/rescheduling feeding Slice 4, and resync delivery on reconnect - all under the best-effort delivery contract (GAME-ADR-0020): durable commit is authoritative regardless of delivery success, no generic delivery outbox, presentation-only effects may be lost.

**Major accepted dependencies**: GAME-ADR-0002 (Coordinator boundary), GAME-ADR-0020 (post-commit delivery semantics), and effectively every earlier slice (Coordinator calls Session Runtime operations; it owns no business logic of its own).

**Why at this point**: Architecturally this is mostly infrastructure wiring rather than domain-logic risk - the domain correctness questions (serialization, RuntimeTurn atomicity, failure handling, terminal cleanup) are better proven against a direct/test harness first. However, this slice is genuinely required before a human can *play* a real multiplayer Session end-to-end, so it should not be pushed out indefinitely once Slices 1-6 are stable; a thin/minimal version could reasonably be pulled forward earlier for an internal demo if useful.

**What later slices depend on it**: none - this is the outward-facing capability the rest of the plan exists to support.

### Slice 10 — Long-Term Runtime History Archival

**Delivers**: `session_history_archives`, JSON archive serialization to object storage (for example GCS), and verified hard-delete of heavy runtime-history tables after successful archival (GAME-ADR-0009).

**Major accepted dependencies**: GAME-ADR-0009.

**Why last**: Lowest architectural risk, purely additive, and explicitly not required for an initial end-to-end playable Session - archival only matters once real Sessions have actually produced history worth archiving/deleting.

**What later slices depend on it**: none.

## Dependencies Summary

- Slice 1 gates everything.
- Slice 2 gates Slices 3-9 (all RuntimeTurn-producing work reuses its mechanism).
- Slice 6 has a hard Game Language prerequisite (`UserDisconnected`/`UserReconnected` compiler support) that must land before Session Runtime code in that slice can deliver either signal - this is the clearest case of Game Language work gating a Session Runtime behavior.
- Slice 8 (keyed timers) is itself Game Language work consumed by Slice 4's timer mechanism, but is not required before Slice 4 - only before a game definition needs simultaneous independent timers.
- Slice 9 (Coordinator) depends on nearly everything else but nothing depends on it, so it can float later in the sequence or be pulled forward for a thin demo without blocking domain-logic slices.
- Slice 5's diagnostic/terminal-cleanup work is tightly coupled to Slices 2-4 in practice; treat the Step-bound mechanism as effectively part of Slice 2 and revisit the full diagnostic/terminal-cleanup scope once Slices 2-4 exist.

## Deferred / Out Of Scope For This Plan

- Post-launch "Session re-entry after complete client-state loss" UX, recorded non-authoritatively in `docs/product/IDEAS.md` - not designed, not part of any slice above.
- Post-Start dynamic player admission / late admission into an already-running Session - explicitly deferred by GAME-ADR-0015/GAME-ADR-0016.
- The exact durable archival JSON schema and GCS implementation details beyond what Slice 10 minimally needs (GAME-ADR-0009 leaves this deferred).
- Future external irreversible Game Language side-effect delivery semantics (payment, external API mutation, durable notification, cross-domain command) - explicitly excluded from GAME-ADR-0020's best-effort delivery rule and requiring its own future architecture decision if/when such a capability is proposed.
- Host transfer, exhaustive `source_kind`/interaction-kind enums, platform abuse/resource limits and per-user/session rate boundaries, mutable global display/profile ownership, Identity reconciliation/merge/alias semantics, Session Configuration/arbitrary external game-specific root params - all remain deferred design topics per `AI_CONTEXT.md`'s Deferred Design Topics list and are not slices in this plan.

## WORK References

None yet. No slice above has been materialized into Feature Development / `docs/work/active/`. Per the just-in-time WORK model, only the recommended first slice should be considered for materialization next, after human review of this plan.

## Recommended First Slice

**Slice 1 — Session Lobby Foundation.**

Rationale: (a) dependency order - every other slice needs a correctly modeled, serialized Session; (b) current repository state - the existing scaffolding does not compile (`step_create_room.go`'s `CreateRoom` has no function body) and actively contradicts accepted architecture (external `engine.Program` parameter, raw owner/player UUID fields, an entirely superseded schema), so this is an urgent correctness gap, not a nice-to-have; (c) useful vertical progress - delivers an actually working Create/Join/Leave flow; (d) risk reduction - proves out the DB-locking per-Session serialization mechanism (which GAME-ADR-0018 later extends to RUNNING) at the lowest-risk point, before any engine execution is layered on top; (e) validates architecture early - exercises the "Session Runtime resolves/pins/compiles the Game definition itself" pattern this whole initiative depends on.

This plan does not create Slice 1's WORK specification. Per the Orchestrator's Feature Development graduation model, the next Conversational AI step should review this decomposition with the human, then - if agreed - materialize Slice 1 into a DRAFT WORK specification.
