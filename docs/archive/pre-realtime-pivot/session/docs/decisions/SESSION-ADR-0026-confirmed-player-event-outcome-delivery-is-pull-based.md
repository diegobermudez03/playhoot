# SESSION-ADR-0026: Confirmed PLAYER_EVENT Outcome Delivery Is Pull-Based, No Durable Outbox

Status: ACCEPTED
Created: 2026-09-29
Last status change: 2026-09-29
Supersedes: None
Superseded by: None

## Context

`docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md` extends `SESSION-ADR-0019`'s best-effort/no-outbox stance for one specific class of output: the confirmed outcome (accepted/rejected/failed) of a submitted `PLAYER_EVENT` must not be silently lost to a transient delivery failure the way a cosmetic `SEND_EVENT` may be. `SESSION-ADR-0019` itself already anticipated this, explicitly reserving that a future capability needing durable/irreversible delivery "must explicitly design its own delivery/idempotency/retry semantics as a separate decision" rather than inherit its own "no outbox" conclusion. This record is that decision, tracked as `docs/projects/active/js-runtime-migration/PROJECT.md`'s own Material Decision #4.

The open question was whether the durable mechanism `ADR-0015` requires means a dedicated push-style outbox (a new table plus a background dispatcher retrying delivery until acknowledged), or something lighter.

Ground truth, checked directly against the current implementation: `SubmitPlayerEvent` already durably records its own outcome in `session_requests` (keyed by `(user_uuid, operation, idempotency_key)`, `SESSION-ADR-0020`'s own idempotency-token mechanism), in the exact same transaction that commits the RuntimeTurn. A same-key retry already replays that exact outcome without re-executing (`interpretExistingSubmitPlayerEventClaim`). The outcome is therefore never actually lost today - what remained undecided was purely how a client that never received the original response (for example, a dropped live connection between commit and delivery) learns it afterward.

## Decision

Confirmed `PLAYER_EVENT` outcome delivery is **pull-based**, not push-based. A new pure, unlocked, non-transactional read capability - `Manager.GetSubmitPlayerEventOutcome`, mirroring `Manager.GetClientState`'s own precedent - looks up the already-durable `session_requests` row for a given `(user, idempotency_key)` and returns its recorded outcome, with no re-execution and no side effect. No new durable table, schema, or background dispatcher is introduced. A live-connection layer (`session-runtime-v1`'s `WORK-0020`) is free to call this on reconnect; a client may equivalently resubmit the original request with the same idempotency key (already-existing replay behavior). Both paths return the identical, already-committed outcome.

## Rationale

The actual correctness requirement `ADR-0015` states is "not silently lost," not "the server proactively guarantees delivery." Durability is already satisfied by the existing idempotency mechanism, which persists the outcome in the same transaction as the Turn it resulted from - there is no window where the outcome exists only in memory, waiting to be pushed. What remained undecided was purely a retrieval question, and a pull-based read closes it without introducing a second durability mechanism alongside the one that already exists.

This follows the same reasoning `SESSION-ADR-0019` already used to reject a generic delivery outbox for presentation, and the same shape `Manager.GetClientState` already established for per-player views: recompute/re-read current durable truth on demand, rather than building a push pipeline that replays history. A dedicated outbox table plus dispatcher would duplicate a recovery path the existing idempotency model already provides, at the ongoing cost of retry-policy/ack-tracking/compaction complexity, for a guarantee (proactive server-initiated push) nothing about the actual requirement demands.

## Alternatives Considered

### Dedicated durable outbox table plus background dispatcher

Rejected. Would duplicate the recovery path `session_requests` already provides, at real ongoing complexity cost, for a guarantee the actual requirement does not ask for - "the client can always determine what happened" is satisfied by pull; "the server chases the client down with a retry loop" is not required by anything accepted so far.

### Extend `SESSION-ADR-0019`'s best-effort stance to this case too

Rejected. `ADR-0015` explicitly extends, not merely inherits, the best-effort rule for this specific case - a confirmed action's own accept/reject/fail distinction has materially different consequences than a missed cosmetic effect, which is exactly the exception `SESSION-ADR-0019`'s own text already anticipated and reserved.

## Consequences

- `Manager.GetSubmitPlayerEventOutcome` becomes a required new capability, implemented by `docs/projects/active/js-runtime-migration/works/WORK-0043-durable-confirmed-turn-result-delivery-outbox.md`.
- No `session_delivery_outbox`, `delivery_attempts`, or per-client ACK table is introduced - `SESSION-ADR-0019`'s own constraint that no such schema be added "without a new, separately justified decision" is satisfied by this record precisely because no such schema is being introduced.
- `session-runtime-v1`'s `WORK-0020` (or any future live-connection layer) may call this new read capability on reconnect; this record does not itself design that call site, only the backend capability it calls.
- If a genuine push-delivery requirement emerges later (for example, a product requirement for server-initiated notification independent of any client request), that remains open for a future, separately justified decision - this record does not foreclose it, following `SESSION-ADR-0019`'s own precedent of not foreclosing future reconsideration.

## Canonical Knowledge Impact

- `session/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - gains a short cross-reference to this record's pull-based answer for confirmed `PLAYER_EVENT` outcome delivery, if/when `WORK-0043` touches that document during implementation.

## Implementation Impact

Routed to `docs/projects/active/js-runtime-migration/works/WORK-0043-durable-confirmed-turn-result-delivery-outbox.md`.
