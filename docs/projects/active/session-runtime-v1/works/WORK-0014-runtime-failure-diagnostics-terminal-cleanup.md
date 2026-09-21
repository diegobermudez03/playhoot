# WORK-0014: Runtime Failure Diagnostics + Terminal Cleanup

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20 (Part B reconciliation, same day: Snapshot-availability assumption reconciled - see "Scope Clarification (Part B Reconciliation, 2026-09-20)" below)

Related decisions:
- GAME-ADR-0017 (failure taxonomy, diagnostic entity - HUMAN-APPROVED and canonically promoted, not yet implemented)
- GAME-ADR-0019 (RuntimeTurn execution bound and terminal cleanup)
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - no durable Snapshot exists for this WORK's diagnostics to reference)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/README.md` (states GAME-ADR-0017 is "HUMAN-APPROVED and canonically promoted; not yet implemented")
- `game/session/workflows/sessionlifecycle/step_start.go` (`terminalizeStartFatal`), `step_answer_interaction.go` (`terminalizeAnswerInteractionFatal`) - the existing narrow fatal paths this WORK generalizes/enriches

## Outcome

Every deterministic runtime failure across every RuntimeTurn-producing path (Start, interaction response, and future timer expiration/user-intent/cancellation paths) produces a durable, queryable diagnostic record with a stable error code, distinguished from transient infrastructure failure, and every terminal Session - regardless of cause - retains no dangling `ACTIVE` interaction or timer obligation.

Today, two narrow fatal paths exist (`terminalizeStartFatal`/`terminalizeAnswerInteractionFatal`), each closing only the interactions its own path knows about. GAME-ADR-0017's fuller diagnostic entity (`session_runtime_failures`, stable error codes, queryability) has no implementation anywhere.

## Context

This WORK retrofits diagnostics/cleanup across all RuntimeTurn-producing paths that exist by the time it is designed (Start, AnswerInteraction, and whichever of WORK-0010/0011/0012 have landed by then), rather than each new path reinventing its own narrow version, as Start and AnswerInteraction currently each do independently.

## Scope

### In Scope (known required outcome; design not yet started)

- `session_runtime_failures` persistence (GAME-ADR-0017).
- Stable error codes, including at minimum `runtime_turn_step_limit_exceeded`.
- Atomic fatal materialization, generalized across every RuntimeTurn-producing path.
- The terminal-cleanup invariant (GAME-ADR-0019): a `TERMINAL` Session retains no `ACTIVE` interaction/timer obligation, closed atomically with `closed_by_turn_id = NULL` when not Turn-produced.

### Out of Scope

- Any new failure-causing path itself - this WORK enriches how existing/future paths report failure, it does not add new ways to fail.

## Approved Design

Not yet designed. Whether this becomes a shared internal mechanism package (parallel to the existing RuntimeTurn Step-draining mechanism) or is retrofitted per-path is left open.

## Constraints and Invariants

- Must not weaken the existing Step-bound guard (already enforced since WORK-0003) - this WORK adds richer diagnostics/cleanup around failures that guard already stops, it does not change when execution stops.
- Terminal cleanup must be atomic with the terminal transition itself.

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- None yet beyond the shared-mechanism-vs-per-path design question above.

## Scope Clarification (Part B Reconciliation, 2026-09-20)

`game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md` removes durable per-Turn Snapshot persistence. This WORK's `session_runtime_failures.diagnostic_payload` (already accepted by GAME-ADR-0017 as "a versioned internal diagnostic structure... that may preserve the attempted cause, the failed signal, and traces from successful in-memory Steps before the fatal Step") must not assume a durable Snapshot is available to reference or embed - any diagnostic content it captures must be self-contained (the in-memory state actually available at failure time, captured directly into the payload) rather than a pointer to a Snapshot row that no longer exists under the replay-first model. This WORK must not reintroduce Snapshot persistence under another name (for example, a "failure snapshot" column) merely to make diagnostics easier - if richer diagnostic context is genuinely needed, it is captured directly in `diagnostic_payload` at the moment of failure, consistent with GAME-ADR-0017's own already-accepted shape. No implementation was performed; this WORK's Status remains PLANNED.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/README.md` (GAME-ADR-0017 status line), `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md` once designed.

## Completion Record

Not started. PLANNED.
