# Data History Standard

Status: CANONICAL ENGINEERING STANDARD

Decided by the human on 2026-10-29, in conversation, while reviewing the account login (`identity-v1` WORK-0059).

This standard applies to every schema migration and to every piece of code that writes persisted rows.

## Principle

The database must show how every row got to its current state. A previous state is read from data, never inferred ("if it is in this state, it must have been in X before"). Every change to the database is visible afterwards.

## Rules

1. **A value written once is its own history.** A column filled once and never changed again (typically an `<event>_at` timestamp, such as `released_at`, `finalized_at`, `deactivated_at`) already tells when that happened. It needs no separate history, as long as rule 3 does not ask for a cause.
2. **A value that changes more than once needs a history table.** A status that moves through a lifecycle, or any column overwritten in place, keeps every change in its own table (for example `play_session_status_changes`, `identity_login_attempt_events`, `identity_display_name_changes`). The repository method that makes the change appends the history row in the same transaction. A history table records new rows only; nothing updates or deletes them. The current value may stay on the main row, for reads and for atomic checks, but the history table is the record of how it got there.
3. **Record a cause or reason only when it matters** for debugging or for business logic: why a Session ended (`end_cause`), why a login attempt was rejected (`reason`). Not every change needs one; when it does, rule 1 is no longer enough for that value and it goes in a history row (or in a column written together with the event, once).
4. **A new table or a new mutable column decides its history when it is created.** Its migration and its writing repository show which of rule 1 or rule 2 applies.

## Exceptions

- The live Session's high-frequency data (samples, commands, views, ticks) is not kept change by change: keeping all of it would be far too heavy. It is approximated by checkpoints and replay chunks. That data is not database row changes; every row the platform writes about a Session follows the rules above.
- `play_sessions.owner_lease_expires_at`, overwritten by every owner heartbeat (`play/session/workflows/sessionlifecycle/internal/repo/owner_lease.go`), has no history (accepted by the human 2026-10-29): it is purely infrastructure and periodic, and logging covers it. Infrastructure data is otherwise not exempt; only this periodic heartbeat is. A history of heartbeats would still help debug infrastructure: the human wants it revisited later.

## Current Deviations

- `play_game_faults` (`play/session/usecases/recordgamefault`, `recordlimithits`): an upsert overwrites `occurrences`, `detail`, `last_reported_at` and `participant_id` on every report, with no history. Found 2026-10-30; to be redone (also breaks the insert rule of `repositories.md`).

## Review

Reviewed 2026-10-29 (`UPDATE` statements; upserts found 2026-10-30, above): every other in-place update in the backend is either a write-once column (rule 1: `play_session_join_codes.released_at`, `play_create_session_requests.outcome/session_id`, `play_replays.final_session_time_ms/finalized_at`, `identity_sessions.deactivated_at/replaced_by_id`, `identity_login_attempts` timestamps) or already has a history table (`play_sessions.status`, login attempts, display names). The seed migrations that set `play_games.current_version_id` are test data, not application writes.
