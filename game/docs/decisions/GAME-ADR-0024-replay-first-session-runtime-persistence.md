# GAME-ADR-0024: Replay-First Session Runtime Persistence

Status: ACCEPTED
Created: 2026-09-20
Last status change: 2026-09-20
Supersedes: GAME-ADR-0007 (the `session_runtime_turns.snapshot_payload`/`snapshot_format_version` and `session_runtime_state` Snapshot-persistence decision only — RuntimeTurn as the historical/transactional unit, RuntimeStep as technical-only history, and the Turn/Interaction/Timer relationship rules are unaffected and are restated here), GAME-ADR-0009 (the object-storage/GCS archive-location decision only — the archive-metadata-entity concept, the verified-archival-before-hard-delete policy, and the exclusion of `session_actors`/`session_participants`/`session_runtime_failures` from deletion are unaffected and are restated here)
Superseded by: None
Legacy ID: None

## Context

GAME-ADR-0007 accepted a model where every committed RuntimeTurn persists the engine's full `Snapshot` (`snapshot_payload`/`snapshot_format_version`), and GAME-ADR-0009 accepted eventually moving that accumulated per-Turn history to GCS as a JSON artifact once a Session terminalizes. Both were reasonable when written, but two things are now clearer:

1. Game Language execution is confirmed fully deterministic given `(compiled Program, InitializationInput.Seed, InitializationInput.RootParameters, an ordered sequence of driving Signals)` — no wall-clock or OS-randomness dependency anywhere in `game/language/v1/engine` (`RandomState` is seeded once from `InitializationInput.Seed` and thereafter advanced only by a committed `DrawRandomOperation`; WORK-0006 already relied on and confirmed this for its own Seed-persistence addition). A full Snapshot is therefore *derived* state, recomputable from durably-captured causes, not itself an independent fact.
2. A durable full-Snapshot column on every RuntimeTurn grows without bound for a long-running Session and duplicates information already recoverable by replaying the ordered causes — the opposite of what GAME-ADR-0009's own archival direction was trying to bound in the first place.

Separately, keeping every completed Session's history in a second storage system (GCS) adds an integration surface (provider client, checksum verification, an object key/provider identifier) that is no longer wanted for Session Runtime V1; the same PostgreSQL database is an acceptable long-term archive destination once heavy per-Turn history is bounded by not persisting Snapshots in the first place.

## Decision

### Persist only what is required for live correctness, durable input ordering, and deterministic replay

Session Runtime persists:

1. pinned Game semantics — the already-immutable `game_definitions.uuid`/`script` a Session pinned at Create (GAME-ADR-0004), including that script's own embedded `program.Metadata.LanguageVersion`;
2. Start's deterministic initialization — `InitializationInput.Seed` (drawn once, at Start, from a legitimate external randomness source) and `InitializationInput.RootParameters` (the `players: list<user>` roster built from active Participants at the serialized Start moment, GAME-ADR-0006);
3. the ordered durable driving-input history — every external/runtime cause capable of producing a RuntimeTurn, in the order Session Runtime actually committed them (GAME-ADR-0018's Turn-sequence ordering already is this order);
4. current derived Runtime state (`session_actors`, `session_participants`, `session_interactions`, `session_timer_obligations`, and other already-accepted normalized operational entities) while a Session is live, for the reasons GAME-ADR-0002/GAME-ADR-0003/GAME-ADR-0007 already give them independent existence (open-interaction identity across requests, timer obligation tracking, Participant admission).

Session Runtime does **not** persist a full `engine.Snapshot` per RuntimeTurn merely because it is convenient to reload. Current and historical Runtime state, Presentations, and Effects are derived — reconstructible from (1)-(3) by deterministic replay — not independent sources of truth. This is consistent with, not a reversal of, WORK-0006's already-accepted "Presentations/Effects are always derived, never persisted" principle; this record extends the same reasoning to the Snapshot itself.

### `session_runtime_turns` remains the ordered replay-input envelope, without a Snapshot column

`RuntimeTurn` remains the historical/transactional unit exactly as GAME-ADR-0007 defined it — one external/runtime cause, 1..N internal `engine.Step` calls, one committed Turn per Session runtime `sequence`. What changes is only what a Turn row carries: `snapshot_payload`/`snapshot_format_version` are removed. `session_id`, `sequence`, `source_kind`, `source_interaction_id`, `source_timer_obligation_id`, `actor_id`, and `created_at` remain, since these already identify *which durable cause* this Turn was and in what order — they are the replay-input pointer, not derived state. Where a cause's own content is not already captured by an existing normalized entity (an `AnswerInteraction`'s response is already `session_interactions.response_payload`; a `TimerExpired`'s identity is already `session_timer_obligations.engine_path`/`engine_slot`/`engine_key`), the owning WORK for that cause (WORK-0010 for UserIntent, WORK-0011 for SessionCancelled, WORK-0015/WORK-0018 for UserDisconnected/UserReconnected) is responsible for giving that cause an equivalently durable, ordered representation — either a new narrow satellite entity or an additional field on the Turn itself — before that cause is authorized to produce a RuntimeTurn. This ADR does not freeze which shape each future cause takes; it only requires that every RuntimeTurn-producing cause end up durably reconstructible, not merely "logged."

`session_runtime_steps` (technical-only intra-Turn execution trace) is a strong removal candidate under this model: it exists today only for engine debugging/audit, has no independent live-correctness purpose, and is itself fully re-derivable by replaying the Turn's own driving cause through the same compiled Program. Whether to remove it entirely, keep a bounded/short-retention version for near-term debugging, or retain it unchanged is left to WORK-0019's DRAFT design rather than decided here — its removal is not forced by this record, but its continued existence must be justified under the same needed-for-live-correctness/needed-as-replay-input/needed-for-explicit-diagnostics test this record applies to every other table, not grandfathered in merely because GAME-ADR-0007 already introduced it.

### Complete replay-input categories (audited against actual Engine/Session code)

Every `Signal` capable of producing a RuntimeTurn, per `game/language/v1/engine/signal.go` and `game/language/v1/program/signal.go`, together with the Session Runtime cause that drives it:

- `WorkflowStarted` (Start's own initial signal) — driven by `Start`; replay-input content is Seed + RootParameters above, not a signal payload of its own.
- `SignalKindQuestionAnswered` (`QuestionAnsweredSignalSource`) — driven by `AnswerInteraction`; content already durable in `session_interactions` (`response_payload`, `engine_path`/`engine_slot`, `session_actor_id`).
- `SignalKindIntent` (`UserIntentSignalSource`) — driven by a future `UserIntent` capability (WORK-0010); content not yet durable anywhere.
- `SignalKindTimerExpired` (`TimerExpiredSignalSource`) — driven by a future timer-expiration capability (WORK-0012/WORK-0013); content already durable in `session_timer_obligations` (`engine_path`/`engine_slot`/`engine_key`).
- `NamedSignalSource{Name: "SessionCancelled"}` — driven by a future manual-cancellation capability (WORK-0011); content not yet durable anywhere (at minimum, the issuing host's actor identity).
- `NamedSignalSource{Name: "UserDisconnected"}` / a future `UserReconnected` — driven by semantic presence transitions (WORK-0015, gated by WORK-0018's Game-Language-side schema work); content not yet durable as an ordered event (today, `session_actors.semantic_presence` is current-value-only, not an append-only log) — WORK-0015/WORK-0019 must ensure the transition's occurrence, not merely its resulting current value, is reconstructible in Turn order (for example, via the same `source_kind`/`actor_id` Turn-envelope pattern already used for interactions/timers).

`SignalKindChildCompleted`, `SignalKindChildFailed`, `SignalKindChildCancelled`, `SignalKindAskGroupAnswered`, `SignalKindAskGroupCompleted`, and `SignalKindTaskGroupCompleted` are never externally driven — they are produced only as `InternalSignals` while draining a Turn already in progress, per the engine's own `Step` contract. Consistent with the principle above, these are deterministic consequences of whichever external cause started the Turn and are not persisted as independent replay inputs.

### Runtime state reconstruction after process loss

A process resuming a `RUNNING` Session no longer loads a durable Snapshot. It loads pinned Game semantics, Start's Seed/RootParameters, and the ordered durable replay-input history up to and including `sessions.current_turn_id`, and deterministically re-executes them through the same compiled Program to obtain the current authoritative `engine.Snapshot` in memory. This extends, rather than contradicts, GAME-ADR-0013's process-agnostic recovery stance (no process/instance ownership, reconstruction from durable state) — only what "durable state" contains changes.

An ephemeral, non-durable in-memory cache/checkpoint of the current Snapshot may be used for performance (avoiding replay on every read); it is never authoritative, requires no durable-recovery design of its own, and its loss causes replay, never data loss. This record does not specify that cache's mechanism — it belongs to WORK-0019's DRAFT design — and explicitly forbids reintroducing a durable checkpoint/snapshot table under another name as a substitute for it.

### Replay compatibility

Replay correctness depends on interpreting a pinned script under the same language semantics it was authored against. Session Runtime already pins an immutable `game_definitions.uuid`/`script` per Session (GAME-ADR-0004) and never re-resolves it. That immutable script already carries its own `program.Metadata.LanguageVersion` field (`game/language/v1/program/metadata.go`), so this record does not introduce a redundant version column — the existing pinned, immutable script is already the semantic-version anchor a replay needs.

What is not yet solved, and is explicitly left open rather than silently assumed: Session Runtime does not currently record which literal compiler/engine build (as opposed to the authored `LanguageVersion` string) actually compiled a given pinned script. If a future `game/language/v2` (or a corrective rebuild of `v1` with different interpretation) is ever introduced, an operational guarantee is still needed that a script's declared `LanguageVersion` continues to resolve to a compiler build that interprets it the way it always has, for as long as any live or archived Session might still need to replay it. This record accepts relying on the existing `LanguageVersion` pinning as sufficient identity for now; it does not claim every future multi-build compiler-evolution scenario is already solved, and flags this as a question WORK-0019's DRAFT design (or a later record, if it turns out to need its own decision) should revisit rather than silently assume away.

### Archive destination (see also the archival decision this record also supersedes in part)

A completed Session's archive record is a versioned JSON payload stored in a PostgreSQL `session_archives`-equivalent table (exact naming/schema is implementation-planning detail for WORK-0019/WORK-0017), not an object-storage artifact. The payload contains what is required to understand/replay the completed Session — pinned Game semantic/version identity, Start's Seed/RootParameters, the ordered replay-input history, interactions/responses, terminal metadata, and any timer/intent/disconnect/cancellation inputs that occurred — not a dump of derived Snapshots/Presentations/Effects, which remain excluded from the archive for the same reason they were never persisted live.

## Rationale

Persisting only durable causes, not derived state, keeps the persistence model's growth bounded by what actually happened (external inputs), not by how many internal engine Steps or intermediate Snapshots that input happened to require to process — the same principle GAME-ADR-0007 already applied to `session_runtime_steps` vs. `session_runtime_turns`, now applied one level higher to the Turn's own Snapshot. Confirmed engine determinism is what makes this safe rather than merely convenient: nothing about current or historical Runtime state, Presentation state, or Effects is lost by not persisting a Snapshot, because all of it is a pure function of already-durable inputs.

Keeping the archive inside PostgreSQL removes an entire integration surface (object-storage client, checksum verification, provider/key metadata) that existed specifically to manage the volume problem full-Snapshot persistence created. Once that volume problem is addressed at its source (by not persisting Snapshots), a same-database JSONB compaction is simpler, has one fewer failure domain, and needs no new external dependency for Session Runtime V1's actual scale.

## Alternatives Considered

### Keep full-Snapshot persistence, archive it to GCS as originally planned

Rejected. This does not remove the underlying volume/duplication problem, only relocates it after the fact, and requires the GCS integration surface this record specifically wants to avoid for V1.

### Persist Snapshots but stop archiving them to GCS (archive elsewhere, keep live Snapshot persistence)

Rejected. This keeps the actual source of unbounded per-Turn growth (a full Snapshot on every committed Turn) untouched; only the long-term destination would change, not the live-path cost this record is meant to address.

### Persist an event-sourced log of every internal engine `Step`/Commit (not just external causes) as the replay source

Rejected. `InternalSignals` produced while draining a Turn are deterministic consequences of that Turn's one external cause; persisting them independently would duplicate information already implied by the cause plus the compiled Program, contradicting the same "derived state is not truth" principle this record establishes for Snapshots.

### Introduce a durable checkpoint/snapshot table under a different name for recovery performance

Rejected as an authoritative mechanism. An ephemeral, non-durable, best-effort in-memory cache remains available for performance without reintroducing a second source of truth or a durable-recovery obligation.

## Consequences

- WORK-0019 (Replay-First Session Runtime Persistence Migration) owns designing and implementing this transition: removing `snapshot_payload`/`snapshot_format_version` from `session_runtime_turns`, resolving `session_runtime_steps`'s fate, persisting Start's Seed/RootParameters, giving every future RuntimeTurn-producing cause (UserIntent, SessionCancelled, UserDisconnected/UserReconnected) a durable ordered representation, and defining any ephemeral current-state cache.
- Future WORKs introducing a new RuntimeTurn cause (WORK-0010, WORK-0011, WORK-0012/WORK-0013, WORK-0015) must integrate with this replay-input model rather than persisting their own ad hoc Snapshot-shaped state.
- WORK-0017 (Archival) is rewritten to compact into a PostgreSQL `session_archives`-equivalent table rather than GCS.
- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` is updated to describe this model as the current accepted design.
- No migration, production code, or implementation is authorized by this record; it is design/architecture acceptance only.

## Canonical Knowledge Impact

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` — replaces the full-Snapshot/GCS-archival sections with this model.
- `game/README.md` — Session Runtime Turn And Persistence Model, Process-Agnostic Recovery, and History Archival Direction sections updated to reference this record alongside GAME-ADR-0007/GAME-ADR-0009/GAME-ADR-0013.
- `docs/projects/active/session-runtime-v1/PROJECT.md` — new WORK-0019 added to the roadmap.

## Implementation Impact

Future implementation work must align Session Runtime migrations and RUNNING-phase execution with this model, per WORK-0019 once it reaches READY. No migration, production code, or WORK beyond WORK-0019's own creation is authorized by this record.

### Implemented by

`docs/projects/active/game-language-flat-execution-model/works/WORK-0026-engine-owned-interaction-addressing.md`/`WORK-0027-session-runtime-interaction-addressing-rework.md` superseded this record's "Complete replay-input categories" audit entry for `SignalKindQuestionAnswered`: that signal kind no longer exists (collapsed into `SignalKindInteractionAnswered`), and its replay-input content is now durable in `session_interactions` as `response_payload`/`engine_interaction_id`/`session_actor_id`, not `engine_path`/`engine_slot`. The `SignalKindTimerExpired`/`session_timer_obligations` entry immediately below it is unaffected - Timer was out of scope for both WORKs and still uses `engine_path`/`engine_slot`/`engine_key` exactly as originally recorded above.
