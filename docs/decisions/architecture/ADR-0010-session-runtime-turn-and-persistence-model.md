# ADR-0010: Session Runtime Turn Architecture and Persistence Model

Status: ACCEPTED
Created: 2026-09-07
Last status change: 2026-09-07
Supersedes: None
Superseded by: None

## Context

ADR-0007 accepted the LOBBY lifecycle contract. Start transitions a Session to `RUNNING` and processes the engine's required initial signal. The engine contract (`game/language/v1/engine/README.md`) already establishes that one `Step` call applies exactly one `Signal` and returns one atomic `Commit`, and that a transition's own operations may produce further `InternalSignals` that require additional `Step` calls without the engine internally chaining them.

RUNNING-phase design needs an accepted answer to what one externally triggered runtime transaction actually is, what gets persisted as historical/observable Session state, and how pending interactions (questions) and timer obligations relate to that history. Without this, Session Runtime cannot durably persist runtime execution, recover after a crash, or expose a coherent Session-state sequence to callers.

## Decision

### RuntimeTurn is the historical/transactional unit

One `RuntimeTurn` begins from one external/runtime cause (an accepted interaction response, a timer expiration, or a future platform signal). It may execute 1..N internal `engine.Step` calls while draining `InternalSignals`. All Steps belonging to one Turn execute within the same Session Runtime transaction. Only the final `Snapshot` after the whole Turn completes becomes an observable/authoritative Session state. Intermediate Step snapshots are not persisted as separate historical Session states. One successfully committed RuntimeTurn corresponds to exactly one monotonically increasing Session runtime `sequence` and one historical Snapshot.

`RuntimeStep` is technical execution history only: engine debugging, technical audit, and inspection of the internal chain of Steps inside a Turn. It is not the user-facing/session-history unit and does not own Session-state sequence numbers. Do not assign Session-state sequence numbers to individual Steps.

### Persistence model

Session Runtime persists the following conceptual tables. Column names, relationships, and the full entity-relationship diagram are recorded in `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`; this ADR does not duplicate the exhaustive column list.

- `sessions` - lobby/runtime/terminal phase and lifecycle timestamps; `game_definition_uuid` is a logical cross-domain reference to Game Management with no database FK.
- `session_actors` - Session-owned actor identity correlated to `Identity.User` through `user_uuid` (logical reference, no FK); `SessionActorID` remains internal-only.
- `session_participants` - gameplay participation state for a SessionActor; remains relationally stored after runtime-history archival.
- `join_codes` - authoritative JoinCode state, unchanged in shape by this decision.
- `session_requests` - durable idempotency records for lobby/lifecycle requests (`CREATE`, `JOIN`, `LEAVE`, `START`). This replaces the earlier proposed name `session_command_receipts`. It does not persist a `request_hash`; a retry with the same idempotency key must be semantically equivalent to the originally stored `request_payload` - same key and same request reproduces the same logical result, same key with a different request is a conflict. The exact JSON canonicalization/comparison algorithm is not decided here.
- `session_runtime_turns` - the historical center of runtime execution: `session_id`, `sequence` (unique per session), `source_kind` (at least `INTERACTION`, `TIMER`, and future `PLATFORM_EVENT`-style sources; the source-kind enum is not exhaustively frozen), `source_interaction_id`/`source_timer_obligation_id` (nullable, mutually situational depending on `source_kind`), `actor_id` (nullable), the final `snapshot_payload` for the whole Turn, `snapshot_format_version`, and `created_at`. Not every Turn originates from an Interaction.
- `session_runtime_steps` - technical history inside one RuntimeTurn: `runtime_turn_id`, `step_index` (unique per turn), `commit_payload`, `created_at`. Carries no Session-state sequence.
- `session_runtime_state` - only the current authoritative runtime position: `session_id`, `current_turn_id`, `updated_at`. It does not duplicate the Snapshot payload; current Session runtime state is defined as the snapshot stored on `current_turn_id`. When Turn N+1 commits, Session Runtime atomically inserts Turn N+1 and its Snapshot, persists its Steps and other mutations, and moves `current_turn_id` to Turn N+1.
- `session_interactions` - durable externally-visible/current interaction state for stale-message safety and recovery/reconnection support: interaction UUID as the public handle, `interaction_payload` (the exposed question), `response_payload` (nullable, the accepted response), `state`, `engine_path`/`engine_slot` (internal Session/engine integration metadata, not public transport identifiers - `engine_path` identifies the nested engine/runtime instance owning the interaction, `engine_slot` identifies the specific pending slot inside it), `opened_by_turn_id`, and `closed_by_turn_id` (nullable). Historical relationships are Turn-level, not sequence/step-level.
- `session_timer_obligations` - durable logical timer obligations: a crossing-boundary `uuid`, `engine_path`/`engine_slot` with the same internal meaning as for interactions, `delay_ms`, `state` (`ACTIVE`/`CANCELLED`/`CONSUMED`), `created_by_turn_id`, and `closed_by_turn_id` (nullable). No absolute `due_at`/deadline is owned by Session Runtime (see ADR-0011).

### Turn / Interaction / Timer relationships

A Turn that opens an interaction is referenced by that interaction's `opened_by_turn_id`. The response that later drives the causing Turn is referenced by that Turn's `source_interaction_id`. The Turn that closes the interaction (accepts or supersedes it) sets `closed_by_turn_id` on the interaction.

Example: Turn 10 opens interaction Q1 (`Q1.opened_by_turn_id = 10`). Q1's later response causes Turn 12 (`Turn12.source_interaction_id = Q1`), which closes Q1 (`Q1.closed_by_turn_id = 12`).

The same pattern applies to timers: the Turn that creates a timer sets `created_by_turn_id`. When Coordinator reports the timer elapsed, the resulting Turn references it through `source_timer_obligation_id`, and the Turn that consumes or cancels it sets `closed_by_turn_id` with `state` becoming `CONSUMED` or `CANCELLED` respectively.

Example: Turn 20 creates timer T1 (`T1.created_by_turn_id = 20`). Coordinator later reports T1 elapsed, causing Turn 25 (`Turn25.source_timer_obligation_id = T1`), which closes T1 (`T1.closed_by_turn_id = 25`, `T1.state = CONSUMED`).

### Interaction response semantics

The first valid response to an `ACTIVE` interaction may create the causing RuntimeTurn and store the accepted response. Retrying an already-accepted equivalent response may behave idempotently. Attempting to submit a conflicting different response to an already-resolved interaction must not cause another engine effect. A `SessionRequest` idempotency record is not required for every gameplay interaction unless a later accepted design requires it; the interaction UUID itself provides the natural external identity for response deduplication.

## Rationale

Tying the observable Session-state sequence to RuntimeTurn instead of every internal engine Step keeps the user-facing/history model aligned with what actually changed session meaning: one external cause, one authoritative before/after Snapshot. Persisting every Step as its own authoritative state would expose engine-internal chaining (draining InternalSignals) as if it were independently significant Session history, which it is not - only the final Snapshot after the whole Turn is meaningful outside the engine.

Keeping RuntimeStep as technical-only history preserves engine debuggability and audit without inflating the historical/session-state model with intermediate states nothing external ever observed.

Modeling Interaction and Timer as durable entities with Turn-level `opened_by`/`created_by`/`closed_by` references (rather than sequence-number bookkeeping) keeps stale-message safety and recovery tied to the same historical unit that owns Session-state sequencing, and avoids a second, redundant sequence concept living on Steps.

Naming the idempotency table `session_requests` instead of `session_command_receipts` better reflects that it stores durable records of accepted lifecycle requests, not merely a receipt/acknowledgement. Avoiding `request_hash` keeps the comparison mechanism as an implementation concern rather than freezing a specific hashing/canonicalization algorithm now.

## Alternatives Considered

### Persist every engine Step as an authoritative Session-state entry

Rejected. It conflates engine-internal execution mechanics with the externally observable Session-history model and would require deciding which intermediate Steps "count," which the engine contract does not distinguish.

### Assign Session-state sequence numbers to RuntimeStep

Rejected. Sequence belongs to the externally observable unit (the Turn). A Step-level sequence would create two competing sequence concepts.

### Give RuntimeTurn a `first_sequence`/`last_sequence` range instead of one `sequence`

Rejected. A Turn produces exactly one authoritative Snapshot and one Session-state sequence value; a range implies Steps themselves carry Session-state significance, which they do not.

### Track Interaction/Timer open and close by Step-level sequence (`opened_sequence`/`closed_sequence`, `opened_by_step_id`/`closed_by_step_id`)

Rejected. Historical relationships for Interaction and Timer are Turn-level, consistent with RuntimeTurn owning the Session-state sequence. Step-level tracking would leak internal execution detail into durable cross-entity references.

### Duplicate the Snapshot payload in `session_runtime_state`

Rejected. `session_runtime_state` exists only to point at the current authoritative position; duplicating the Snapshot would create two sources of truth for current state that could drift.

### Store the idempotency table as `session_command_receipts` with a `request_hash`

Rejected. `session_requests` better reflects durable request storage, and comparison against the stored `request_payload` avoids freezing a specific hash/canonicalization algorithm before it is designed.

### Create a `SessionRequest` idempotency record for every gameplay interaction response

Rejected for now. Interaction UUID already provides a natural external identity for response deduplication; requiring a SessionRequest for every gameplay interaction would duplicate that mechanism without an accepted need.

## Consequences

- Future Session Runtime implementation must persist RuntimeTurn as the Session-state historical unit and RuntimeStep as technical-only history beneath it.
- Interaction and Timer durability, recovery, and stale-message safety must be designed against Turn-level references (`opened_by_turn_id`, `created_by_turn_id`, `closed_by_turn_id`, `source_interaction_id`, `source_timer_obligation_id`), not Step-level or sequence-range bookkeeping.
- `session_runtime_state` must remain a thin pointer to `current_turn_id`; it must not become a second Snapshot store.
- The idempotency table for lobby/lifecycle requests is named `session_requests` and does not use `request_hash`.
- RUNNING-phase concurrent-input serialization, bounded-loop protection while draining InternalSignals, and post-commit delivery-failure handling remain future architecture/implementation work not decided by this record.

## Canonical Knowledge Impact

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (new) - records the accepted full table/column/relationship diagram for this model.
- `game/README.md` - adds the accepted RuntimeTurn/RuntimeStep historical model and references this ADR and the new persistence-model document.
- `docs/ai/KNOWLEDGE_MAP.md` - lists the new accepted-design document under Specialized Documentation.

## Implementation Impact

Future implementation work must align Session Runtime migrations, persistence, and RUNNING-phase execution with this accepted model. No migration, production code, or WORK is authorized by this record.
