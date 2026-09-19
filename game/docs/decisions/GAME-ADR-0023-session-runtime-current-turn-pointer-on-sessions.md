# GAME-ADR-0023: Session Runtime Current-Turn Pointer Lives On `sessions`, Not A Separate Table

Status: ACCEPTED
Created: 2026-09-19
Last status change: 2026-09-19
Supersedes: None
Superseded by: None

Refines - does not supersede - GAME-ADR-0007's persistence-table list: every other decision in GAME-ADR-0007 (RuntimeTurn as the historical/transactional unit, RuntimeStep as technical-only history, the Turn/Interaction/Timer reference model) is unchanged. Only where the "current authoritative RuntimeTurn" pointer is physically stored changes.

## Context

GAME-ADR-0007 introduced `session_runtime_state` as a separate, thin single-row-per-session table holding only `session_id`/`current_turn_id`/`updated_at`, explicitly to avoid duplicating the Snapshot payload as a second source of truth.

Implementing Slice 2 (Start, `docs/work/active/WORK-0003-session-start-first-runtimeturn.md`) exposed a concrete cost of keeping that pointer in its own table: every Session Runtime mutation that needs to know "what is the current Turn" (Start today; interaction responses and timer expirations in Slices 3/5) already loads and row-locks the owning `sessions` row first, for the per-Session serialization GAME-ADR-0004/0018 already require. `session_runtime_state` therefore added a second row, a second query/upsert, and a second table to reason about, purely to hold a value that was already available for free from a row every caller already has locked in hand.

The reason this was not simply "move it to `sessions`" from the start is GAME-ADR-0009's archival/hard-delete policy: `session_runtime_state` (along with `session_runtime_turns`/`session_runtime_steps`/`session_interactions`/`session_timer_obligations`) is explicitly hard-delete-eligible once a Session's heavy runtime history is archived, while `sessions` itself is explicitly excluded from that policy and retained indefinitely. Putting `current_turn_id` directly on the permanent `sessions` row appeared to risk a dangling reference once the row it pointed to is hard-deleted.

## Decision

`sessions` gains a `current_turn_id` column (nullable BIGINT, no database-enforced FK, consistent with every other reference in this schema - see `SESSION_RUNTIME_PERSISTENCE_MODEL.md`'s Relationship Types). `session_runtime_state` is removed; it never needed to be its own table.

The archival concern above is resolved by reframing, not by adding a cleanup obligation: `sessions.current_turn_id` is a logical reference to "the Turn most recently authoritative for this Session," identified by the same value (the Turn's id, or an equally stable identifier) regardless of where that Turn's data currently lives. While the Session's heavy runtime history is still hot in PostgreSQL, that value resolves against a live `session_runtime_turns` row. Once the Session is archived and its heavy history hard-deleted (GAME-ADR-0009), the same value still identifies which Turn was current at archival time - it now resolves against the corresponding entry inside the archived JSON artifact instead of a live row. This is not a broken reference: it is a reference whose backing storage tier changed, exactly the same nature of "logical, not database-enforced" reference this schema already uses everywhere (cross-domain `user_uuid` references, the cross-capability `game_definition_uuid` reference). No column needs to be nulled out at archival-deletion time, and no consequence of GAME-ADR-0009's hard-delete policy changes.

`current_turn_id` is set once, atomically, in the same transaction that commits a Session's first RuntimeTurn (Start) or advances to a later one (a future Slice 3/5 RuntimeTurn commit) - identical transactional placement to what `session_runtime_state`'s upsert already provided, just as a plain `UPDATE sessions` instead of a second table's upsert.

## Rationale

The pointer's only consumers are callers that already hold the `sessions` row locked for serialization; there was never a genuine access path that needed "current Turn" without the `sessions` row already in hand. Colocating removes a table, a query, and a write per Turn, with no loss of the "no duplicated Snapshot payload" property GAME-ADR-0007 actually cared about - `current_turn_id` is still just an id, never the Snapshot itself.

The archival objection dissolves once the reference is understood as logical rather than DB-enforced, which this schema already commits to everywhere else (`SESSION_RUNTIME_PERSISTENCE_MODEL.md`'s Relationship Types: "No database-enforced foreign key constraints are assumed by this accepted design"). `sessions` surviving hard-delete of `session_runtime_turns` while still holding a value that identifies a since-archived Turn is the same shape of fact as `session_actors.user_uuid` surviving whatever Identity does to that `User` later - Session Runtime was never relying on Postgres to enforce or resolve that reference for correctness.

## Alternatives Considered

### Keep `session_runtime_state` as its own table (status quo)

Rejected. Costs a second table, a second per-Turn write, and a second row to keep consistent, purely to hold one value every caller already has free access to via the `sessions` row it already locks.

### Move `current_turn_id` onto `sessions`, and null it out when heavy history is hard-deleted after archival

Considered as a middle option. Rejected as unnecessary complexity: it adds a new obligation to the archival-deletion transaction (remember to null this specific column on the permanent row) purely to avoid a "dangling" reference that is not actually dangling once understood as a logical, storage-tier-crossing reference - the same kind this schema already tolerates everywhere.

### Give `sessions.current_turn_id` a database foreign-key constraint

Rejected, consistent with this schema's existing no-enforced-FK convention: a real FK would break the moment the referenced `session_runtime_turns` row is hard-deleted after archival, which is exactly the accepted GAME-ADR-0009 outcome for an archived Session.

## Consequences

- `session_runtime_state` is removed from the accepted persistence model; `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` and `game/docs/DATA_MODEL.md` are updated to show `current_turn_id` as a `sessions` column instead.
- Any future reader resolving `sessions.current_turn_id` for an already-`TERMINAL`-and-archived Session must resolve it against the archive artifact (once GAME-ADR-0009's archive format is actually designed - still deferred), not assume a live `session_runtime_turns` row. This is a documentation/consumer-contract nuance, not a new mechanism to build now.
- Slice 3/5 (interaction responses, timer expirations) advance `sessions.current_turn_id` directly instead of upserting a separate `session_runtime_state` row, when they materialize.
- No change to RuntimeTurn/RuntimeStep semantics, the Turn/Interaction/Timer reference model, or any other GAME-ADR-0007 decision.

## Canonical Knowledge Impact

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - removes `session_runtime_state` as its own entity; documents `sessions.current_turn_id` and the archival storage-tier-crossing note above.
- `game/README.md` - Session Runtime Turn And Persistence Model section's `session_runtime_state` mention updated to describe a `sessions` column instead.
- `game/docs/DATA_MODEL.md` / `game/docs/FLOWS.md` - current-implementation schema/flow updated to match (Slice 2/WORK-0003 implementation).
- `game/docs/decisions/INDEX.md` - adds this record; next Game ADR becomes `GAME-ADR-0024`.

## Implementation Impact

`docs/work/active/WORK-0003-session-start-first-runtimeturn.md` (Slice 2, already IMPLEMENTING) is revised to drop `session_runtime_state`/`SetRuntimeState`/its migration and instead set `sessions.current_turn_id` directly from `step_start.go`, per this record - see that WORK's own revision record for the concrete migration/code changes.
