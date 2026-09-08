# GAME-ADR-0014: Session Runtime Durable Inactivity Expiration

Status: ACCEPTED
Created: 2026-09-07
Last status change: 2026-09-07
Supersedes: None
Superseded by: None

## Context

GAME-ADR-0013 accepts that Session Runtime is process-agnostic and that a process crash by itself is not a Session-domain event. That leaves an eventual-cleanup gap: without process ownership as a cleanup trigger, what stops a `RUNNING` Session from remaining `RUNNING` forever when its process died, nobody ever returned, or no graceful completion event occurred? GAME-ADR-0004 already accepted an analogous durable, self-enforcing deadline for `LOBBY` (`lobby_expires_at`, lazily materialized by any operation or a sweeper, not owned by a background job's discovery). RUNNING has no equivalent bound.

## Decision

### `activity_expires_at`

Every `RUNNING` Session has a durable inactivity deadline, `sessions.activity_expires_at`. It is not a process ownership lease. It answers: until what instant does the platform still consider this `RUNNING` Session active, unless meaningful activity extends it? Its purpose includes letting abandoned Sessions eventually leave hot `RUNNING` state, ensuring process crashes do not leave Sessions `RUNNING` forever, enabling eventual terminalization and archival, and providing a simple V1 lifecycle bound without process ownership.

### Renewal

Meaningful Session operations that demonstrate continued active use extend `activity_expires_at = now + inactivity_ttl`. The V1 policy may use a configurable TTL around 10 minutes; the exact duration is configurable operational/product policy and must not be hard-coded into Game Language semantics. The accepted V1 assumption is that a valid live social-game Session should not normally remain `RUNNING` for that entire interval without any meaningful Session activity.

Only operations that constitute meaningful evidence that the Session is still actively being used/runtime-progressing may renew the deadline. Arbitrary passive reads/polling must not keep a Session alive indefinitely merely because they access it. Examples that may reasonably count conceptually (not an exhaustive/frozen list): successful RuntimeTurn-producing gameplay operations, accepted interaction processing, timer expiration processing, meaningful lifecycle/runtime events, and authenticated reconnection/resume activity where product semantics justify it. The exact enumeration of renewal-triggering operations remains deferred.

### `activity_expires_at` is the source of truth

Semantic expiration occurs because the deadline passed, not because a Reaper happened to discover the Session. `phase = TERMINAL` with `terminal_reason = RUNTIME_INACTIVITY_EXPIRED` is a materialized representation of an already-true condition, the same pattern already accepted for `lobby_expires_at` -> a `LOBBY_EXPIRED`-equivalent terminal reason (GAME-ADR-0004).

### Every active-dependent operation validates the deadline

Any Session Runtime operation that requires the Session still be active must, under the normal per-Session serialization mechanism (GAME-ADR-0004): load/lock the Session; validate lifecycle; compare current time with `activity_expires_at`; and only if the Session has not expired may it process the operation and potentially renew the deadline. An operation arriving after the deadline must not revive the Session merely because the persisted `phase` still says `RUNNING`, even if the operation arrives only moments after the deadline.

### Lazy materialization

An ordinary Session operation that discovers `phase = RUNNING` and `now >= activity_expires_at` may atomically materialize, in the same transaction: `phase = TERMINAL`, `terminal_reason = RUNTIME_INACTIVITY_EXPIRED`, `terminal_at = activity_expires_at`; then reject the attempted active operation. The operation must not renew the deadline, reopen the Session, fabricate gameplay, or create an engine RuntimeTurn merely to represent expiration. Expiration is Session lifecycle semantics, not authored gameplay.

### Reaper role, separate from Archive Worker

A background Session Reaper is useful but does not determine whether expiration occurred; it proactively finds Sessions where `phase = RUNNING` and `activity_expires_at <= now` but the persisted lifecycle representation has not yet been materialized, then - using the same Session serialization rules as normal mutations - re-reads/revalidates and materializes `TERMINAL`/`RUNTIME_INACTIVITY_EXPIRED` if still expired and `RUNNING`. If a legitimate operation already won the serialization race and renewed the deadline before the Reaper acted, the Reaper rechecks and does nothing.

The deadline, not Reaper scheduling latency, decides the outcome: an operation that reaches serialization at `activity_expires_at - 100ms` may process normally, extend the deadline, and commit, and a later Reaper pass sees the new future deadline and does nothing; an operation that reaches the check after the deadline cannot renew or revive it, regardless of how close it was.

`terminal_at` always equals `activity_expires_at`, never the Reaper's or the lazy operation's current wall-clock time. If Playhoot is unavailable for two days and the deadline was Monday 21:10 but the Reaper next runs Thursday, the semantic terminal instant remains Monday 21:10. This matters for historical correctness, retention windows, archival timing, and analytics.

`terminal_reason = RUNTIME_INACTIVITY_EXPIRED` (or a repository-conventional equivalent naming normalization; the exhaustive terminal-reason enum remains not yet accepted per GAME-ADR-0004/`game/README.md`) means only "the RUNNING Session exceeded its allowed inactivity period." It does not assert that a process crashed, that infrastructure killed a pod, that the host disconnected, that every participant left, or any other root cause; it is deliberately not named `PROCESS_CRASH` or `RUNTIME_ORPHANED`, since neither process ownership nor a specific root cause is what the platform actually observed.

The Archive Worker (GAME-ADR-0009) remains completely separate from this lifecycle mechanism. Its input contract is already-materialized `TERMINAL` Session state plus archival retention policy; it must not inspect process ownership, detect crashes, determine whether a `RUNNING` Session is inactive, or interpret `activity_expires_at` to mutate lifecycle.

### Interaction/timer cleanup on inactivity termination

No new detailed cleanup semantics are decided here beyond: once a Session is semantically `TERMINAL`, future gameplay timer/interactions are no longer executable as active gameplay. The exact persistence-state transitions/closure reasons for still-open `session_interactions` and `session_timer_obligations` at inactivity termination remain a later implementation/design detail. Materializing inactivity expiration must not fabricate engine responses or RuntimeTurns to close gameplay.

### Rejected: process ownership/heartbeat as the cleanup mechanism

A durable process-owned Session lease, `owner_instance_id`, periodic per-process heartbeat proving ownership, or takeover generations/fencing introduced to determine which process owns a Session are rejected, consistent with GAME-ADR-0013. `activity_expires_at` is a Session-lifecycle timeout describing continued use, not an infrastructure ownership lease.

## Rationale

An inactivity deadline gives Session Runtime an eventual-cleanup mechanism that does not depend on knowing anything about process health or ownership, consistent with the process-agnostic design accepted in GAME-ADR-0013. Making the deadline itself - not a Reaper's discovery - the source of truth mirrors the already-accepted `lobby_expires_at` pattern (GAME-ADR-0004) and keeps correctness independent of background-job scheduling latency, which is exactly the property needed so an outage does not retroactively change when a Session actually expired.

Restricting renewal to operations that demonstrate meaningful active use (rather than any read/access) prevents indefinite artificial liveness from passive polling, which would otherwise defeat the deadline's purpose of bounding abandoned `RUNNING` Sessions.

Keeping the Archive Worker ignorant of expiration mechanics preserves the separation already accepted in GAME-ADR-0009: lifecycle correctness and long-term archival are different concerns with different failure modes, and collapsing them would make each harder to reason about independently.

## Alternatives Considered

### Rely on process/pod health monitoring or heartbeat loss to terminalize abandoned Sessions

Rejected. Couples Session lifecycle correctness to infrastructure-level process monitoring, contradicts the process-agnostic design accepted in GAME-ADR-0013, and cannot handle the common case where no process ever crashed but participants simply stopped interacting.

### Terminalize purely via Reaper discovery, with `terminal_at` set to the Reaper's current time

Rejected. Makes historical/retention/archival timing dependent on Reaper scheduling latency rather than the actual semantic instant of expiration, which would misrepresent when a Session actually stopped being active (for example, after a multi-day platform outage).

### Allow any read/poll operation to renew `activity_expires_at`

Rejected. Would let idle observers or passive polling keep a Session `RUNNING` indefinitely without any real gameplay progress, defeating the deadline's purpose.

### Introduce a durable `RECOVERING` or intermediate phase while the Reaper processes a Session

Rejected, consistent with GAME-ADR-0013. `TERMINAL` is materialized atomically within the same serialized transaction; there is no meaningfully different intermediate state to represent.

### Let the Archive Worker itself detect and materialize inactivity expiration

Rejected. Would collapse a lifecycle-correctness concern into the archival pipeline, contradicting the separation already accepted in GAME-ADR-0009 between the hot-store/lifecycle boundary and the archive-then-delete boundary.

### Name the terminal reason `PROCESS_CRASH` or `RUNTIME_ORPHANED`

Rejected. Neither name matches what is actually observed (deadline passage); both would falsely imply a specific root cause (a crash, or process ownership loss) that Session Runtime does not and should not know.

## Consequences

- `sessions` gains `activity_expires_at`, a durable inactivity deadline for `RUNNING` Sessions only, distinct from `lobby_expires_at`.
- Every RUNNING-phase operation that requires the Session to still be active must validate `activity_expires_at` under per-Session serialization before proceeding, and may lazily materialize expiration in the same transaction.
- A Session Reaper must be designed as eventual/best-effort materialization support, not as the source of expiration correctness, and must re-validate under the same serialization rules before materializing.
- `terminal_at` must equal `activity_expires_at`, never the time the Reaper or a lazy operation happened to run.
- The Archive Worker remains unaffected by and unaware of this mechanism beyond consuming the `TERMINAL` Sessions it eventually produces.
- The exact renewal-triggering operation enumeration, TTL configuration surface, and interaction/timer closure semantics on inactivity termination remain deferred implementation/design detail.

## Canonical Knowledge Impact

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` - adds `activity_expires_at` to `sessions` and documents the source-of-truth, renewal, lazy-materialization, Reaper, and `terminal_at` rules.
- `game/README.md` - adds a Durable Inactivity Expiration section referencing this ADR.

## Implementation Impact

Future implementation must add a migration for `activity_expires_at`, per-operation validation/lazy-materialization logic, and a Reaper job design; TTL must be configurable rather than hard-coded. No migration, Reaper implementation, production code, or WORK is authorized by this record.
