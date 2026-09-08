# GAME-ADR-0012: Game Language Keyed Timer Slots

Status: ACCEPTED
Created: 2026-09-07
Last status change: 2026-09-07
Supersedes: None
Superseded by: None

## Context

GAME-ADR-0011 established that gameplay progress while a targeted actor is offline is ordinary authored Game Language policy, typically expressed as a timer/timeout transition. The existing accepted ordinary `TimerSlotDeclaration` (`game/language/v1/program/timer.go`) holds at most one pending timer per statically named slot per workflow instance; scheduling into an already-occupied slot is an execution error, and there is no implicit reset, replacement, extension, or coalescing. This makes an authored root-level policy such as "start an independent disconnect timeout for each disconnected player" (`timer[P1]`, `timer[P2]`, ...) awkward or impossible with one static slot, since every player's timeout would contend for the same single pending-timer location. This record accepts a general Game Language capability to address this, without solving it as a Session Runtime-specific workaround and without inventing a disconnect-specific primitive.

## Decision

### General capability, not a disconnect-specific primitive

Game Language will support a `KeyedTimerSlot<Key>` concept: an independently addressable family of pending timers keyed by an authored key, distinct from the existing ordinary single-pending-timer `TimerSlotDeclaration`. Naming, API shape, and Go struct/type names are not frozen by this record - only the semantic capability is accepted. A dedicated `DisconnectTimer` primitive is explicitly rejected; the disconnect use case motivated this capability but does not own it. Other anticipated uses include player cooldowns, team timers, per-object timers, and keyed negotiations/processes.

### Identity and cardinality

Conceptual identity is `(workflow instance/path, slot, key)`. At most one timer may be pending for one exact tuple. Different keys under the same slot have fully independent timers, so `disconnect_timeout[P1]` and `disconnect_timeout[P2]` may both be pending simultaneously.

### Scheduling semantics

Conceptually `schedule(slot, key, delay)`. If `(slot, key)` already has a pending timer, this is an execution error and the whole enclosing transition remains atomic - it fails entirely, mirroring the existing ordinary-`TimerSlot` occupied-slot rule. There is no implicit reset, replacement, extension, or coalescing. An authored workflow must explicitly cancel first if replacement is desired.

### Cancellation semantics

Conceptually `cancel(slot, key)`, affecting only that key; idempotent no-op if no timer is currently pending for that exact tuple. Cancelling one key's timer must not affect any other key's timer under the same slot.

### Expiration semantics

When a keyed timer expires, the authored workflow must be able to identify which key expired: conceptually `KeyedTimerExpired(slot)` carrying `key: KeyType`, or an equivalent syntax - exact source/Go type names are not frozen here. The key is authored-language information and must be exposed, unlike the internal timer-obligation UUID, wall-clock scheduling data, Coordinator physical timer identity, or internal database identifiers, none of which are ever exposed.

### Persistence consequence

The durable `session_timer_obligations` representation (GAME-ADR-0007) must be able to persist a keyed-timer discriminator alongside the existing `engine_path`/`engine_slot`/`delay_ms`/`state`/Turn relationships, so a keyed timer's expiration can be reconstructed with the correct authored key on recovery. This is addressed as an update to the accepted persistence model (`game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`), not by a Session Runtime-specific workaround table.

### Archive implication

The long-term runtime archive (GAME-ADR-0009) must eventually preserve whatever timer-key metadata is necessary to understand/replay archived keyed-timer history. The concrete archive JSON format is not frozen by this record.

## Rationale

The existing ordinary `TimerSlot`'s static, one-pending-timer-per-slot design is intentional and correct for the common case of one named timeout per workflow instance; changing it to implicitly hold multiple concurrent timers would change existing semantics and complicate every current/future caller for a need only some workflows have. A separate keyed concept preserves the existing simple slot's semantics unchanged while giving authors who need independently addressable concurrent timers (per-player, per-team, per-object) a first-class way to express that, without resorting to workarounds such as dynamically generated slot names (already explicitly disallowed - slot names are static, source-level strings, never runtime expressions) or encoding multiple timers into one slot's payload by hand.

Making this a general Game Language primitive rather than a `DisconnectTimer` special case follows the same reasoning GAME-ADR-0011 used against an implicit disconnect broadcast mechanism: solving a narrow problem with a narrowly named primitive forecloses reuse for cooldowns, per-team timers, or other keyed processes that plausibly need identical semantics, and avoids Session Runtime special-casing disconnect timers outside the ordinary Game Language timer model.

Mirroring the ordinary `TimerSlot`'s atomic-failure-on-occupied-tuple and explicit-cancel-before-reschedule rules keeps the language's timer semantics uniform: an author who already understands ordinary `TimerSlot` scheduling does not need to learn a second, different concurrency-control rule for keyed timers.

## Alternatives Considered

### Solve multi-player disconnect timeout inside Session Runtime

Rejected. Session Runtime does not own authored gameplay timing policy. Special-casing "one timer per disconnected player" outside Game Language would create a parallel, non-authored timer mechanism the language itself cannot express, observe, or override, and would not generalize to any other keyed-timer need.

### Introduce a dedicated `DisconnectTimer` primitive

Rejected. It would be a narrow, single-purpose mechanism duplicating the general keyed-timer need already illustrated by cooldowns, team timers, and other per-object timers; a general primitive covers the disconnect case without foreclosing reuse.

### Allow dynamically generated/runtime-expression slot names on the existing `TimerSlot`

Rejected. Slot names are deliberately static, source-level identifiers, never runtime expressions (see `game/language/v1/program/timer.go`). Allowing dynamic names would undermine the compiler's ability to statically validate slot references and would blur the line between a slot (a compile-time structural concept) and ordinary keyed runtime data.

### Implicitly reset/replace a pending keyed timer on a second schedule for the same key

Rejected. This mirrors the already-accepted ordinary `TimerSlot` rule that an occupied slot must be explicitly cancelled before rescheduling; implicit replacement would silently discard an authored timer the workflow may still be relying on and would make timer behavior depend on scheduling order rather than explicit authored intent.

### Persist an absolute due-at/deadline for keyed timers

Rejected as out of scope for this record. GAME-ADR-0008 already accepted the V1 tradeoff of not persisting absolute deadlines for ordinary timers, rescheduling from the full configured delay on recovery instead; keyed timers do not need a different persistence tradeoff than ordinary timers for the same reason.

## Consequences

- Game Language gains a second timer-slot family (`KeyedTimerSlot<Key>`) alongside the existing static `TimerSlotDeclaration`; the compiler, engine, and authoring documentation will need new declarations/operations/signal source(s) when implemented (not by this record).
- `session_timer_obligations` gains a nullable internal key-discriminator column (`engine_key` or an equivalent typed representation), null for ordinary timers and populated for keyed timers; this is internal Session/engine routing metadata and must never be exposed directly to Coordinator/frontend merely because it is persisted.
- The long-term archive format must eventually account for keyed-timer metadata; the concrete schema remains deferred (GAME-ADR-0009).
- No `due_at`, no durable `live_timer_schedules`, and no change to the accepted full-delay recovery simplification (GAME-ADR-0008) is introduced by this record.

## Canonical Knowledge Impact

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - `session_timer_obligations` gains a nullable `engine_key` column (or equivalent) in the accepted design; semantics documented (null for ordinary timers, populated for keyed timers).
- `game/README.md` - Session Runtime Turn And Persistence Model section notes the accepted keyed-timer persistence consequence.
- `game/language/v1/program/README.md` - notes the accepted, not-yet-implemented keyed-timer-slot concept alongside the existing static `TimerSlot`.
- `game/language/v1/engine/README.md` and `game/language/v1/engine/LOGICAL_CONTRACT.md` - note the accepted, not-yet-implemented keyed-timer semantic capability.

## Implementation Impact

Future implementation must design the concrete `KeyedTimerSlot<Key>` declaration/operations/signal source in `program`, corresponding compiler/engine support, the `session_timer_obligations.engine_key` migration, and Session Runtime's construction of the correct keyed expiration signal on recovery. No compiler, engine, program, migration, or Session Runtime code, and no WORK, is authorized by this record.
