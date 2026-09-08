# GAME-ADR-0009: Session Runtime History Archival Direction and Verified Hard-Delete

Status: ACCEPTED
Created: 2026-09-07
Last status change: 2026-09-07
Supersedes: None
Superseded by: None
Legacy ID: ADR-0012

## Context

GAME-ADR-0007 introduces heavy runtime-history tables (`session_runtime_turns`, `session_runtime_steps`, `session_interactions`, `session_timer_obligations`) plus the thin `session_runtime_state` pointer. Left in PostgreSQL indefinitely, this history grows without bound for every completed Session, even though most of it stops being operationally relevant once a Session reaches a terminal/archiveable condition.

A direction is needed for long-term retention that does not require keeping all runtime history hot forever, without freezing the exact archive artifact schema or a concrete object-storage integration now.

## Decision

PostgreSQL remains the hot/runtime store. After a Session reaches an appropriate terminal/archiveable condition, heavy runtime history may eventually be serialized into a versioned JSON artifact and written to long-term object storage such as Google Cloud Storage. The exact final JSON archive schema is not frozen by this record, but the artifact must eventually be capable of preserving the historical Session/runtime information needed after hot-history deletion.

Session Runtime persists a 1:1 archive-metadata entity, `session_history_archives`, conceptually:

- `id`;
- `session_id` (unique);
- `status`: `PENDING` | `READY` | `FAILED` or an equivalent enum;
- `storage_provider`;
- `storage_key`;
- `format_version`;
- `checksum`;
- `archived_at` (nullable);
- `created_at`;
- `updated_at`.

A provider/key/object identifier is preferred over persisting an expiring or public URL, so the reference remains stable and does not depend on a time-limited signed link.

Heavy runtime/history tables may become an explicit hard-delete exception after successful verified archival. Candidate removable hot runtime data includes:

- `session_runtime_state`;
- `session_runtime_turns`;
- `session_runtime_steps`;
- `session_interactions`;
- `session_timer_obligations`.

Deletion may occur only after: the long-term archive has been successfully written; integrity/checksum verification succeeds; and archive metadata is `READY`. If archival or verification fails, there is no hard delete.

`session_actors` and `session_participants` are explicitly excluded from this deletion policy. They remain relationally stored indefinitely because they are lightweight and useful for product queries such as which Sessions a User participated in, who hosted a Session, and which Users participated. Retention of `session_requests`, `join_codes`, and other lightweight lifecycle metadata is not changed by this decision unless another accepted rule already defines it.

GCS implementation, the concrete archival/verification worker, and the final JSON schema are deferred and not authorized by this record.

## Rationale

Bounding hot-store growth to actively relevant Sessions keeps PostgreSQL sized for operational/runtime concerns rather than indefinite historical storage, while a versioned JSON artifact in object storage is a low-cost way to preserve the historical record for the rare cases it is still needed (support, dispute resolution, analytics).

Requiring verified archival (not merely "archival attempted") before hard-delete is a fail-closed safety property: a failed or unverified archive must never result in silently losing the only copy of a Session's runtime history.

Excluding `session_actors`/`session_participants` from deletion preserves cheap, always-available relational answers to common product questions (a User's session history, who hosted what) without needing to rehydrate an archive for routine queries.

Preferring a provider/key identifier over a persisted URL avoids depending on a link that can expire or be regenerated, keeping the archive metadata itself the stable source of truth for locating the artifact.

## Alternatives Considered

### Keep all runtime history in PostgreSQL indefinitely

Rejected. Unbounded growth of `session_runtime_turns`/`session_runtime_steps`/`session_interactions`/`session_timer_obligations` in a hot OLTP-style store has an ongoing cost with no corresponding operational benefit once a Session is no longer active.

### Soft-delete or TTL-expire runtime history without a long-term archive

Rejected. It destroys audit/debugging/history value with no recovery path, unlike an archive-then-delete approach.

### Allow hard-delete once archival is attempted, before verification succeeds

Rejected. An unverified or failed archive write must not be treated as equivalent to a successfully preserved copy; this would risk silent data loss.

### Persist a public/expiring URL instead of a provider/key identifier

Rejected. An expiring or regenerable URL is a worse long-term stable reference than a provider-scoped storage key.

### Include `session_actors`/`session_participants` in the hard-delete-eligible set

Rejected. These tables are lightweight and support ongoing product queries; deleting them would remove cheap, commonly needed relational answers without a corresponding storage-cost justification.

## Consequences

- Future implementation must add an archival/verification workflow before any hard-delete of the listed hot runtime-history tables becomes safe to run.
- `session_history_archives` status must reach `READY` with a verified checksum before deletion is permitted; `FAILED` or `PENDING` blocks deletion.
- `session_actors` and `session_participants` remain queryable regardless of archival/deletion state.
- The concrete JSON archive schema, GCS writer, and verification mechanism remain deferred design/implementation work.

## Canonical Knowledge Impact

- `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` (new) - records `session_history_archives` and the archival/hard-delete relationship to the hot runtime tables.
- `game/README.md` - references this ADR for the accepted archival direction and hard-delete exception.

## Implementation Impact

Future implementation must build the archival/verification workflow and the hard-delete path guarded by verified `READY` status. No GCS integration, migration, production code, or WORK is authorized by this record.
