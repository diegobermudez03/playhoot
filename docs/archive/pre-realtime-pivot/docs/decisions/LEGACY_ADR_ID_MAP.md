# Legacy ADR ID Map

Status: HISTORICAL MIGRATION RECORD

On 2026-09-07, Playhoot moved from a single centralized global architecture-decision family (`docs/decisions/architecture/`) to scope-based decision families: global/cross-domain ADRs remain centralized, and bounded-context/domain-scoped ADRs moved to their owning domain (`<domain>/docs/decisions/`). See `docs/decisions/README.md` for the current routing model and `docs/ai/CHANGELOG.md` for the applied workflow-change entry.

This map resolves every legacy `ADR-NNNN` identifier that existed before the migration to its current canonical identifier and location. It exists so old chat logs, commits, and documentation references remain resolvable. Status, rationale, alternatives, consequences, and created dates were preserved unchanged during migration; only identifier, location, and internal cross-reference wiring changed.

| Legacy ID | New Canonical ID | New Location | Scope |
| --- | --- | --- | --- |
| ADR-0001 | ADR-0001 (unchanged) | `docs/decisions/architecture/ADR-0001-intra-domain-responsibility-boundary.md` | Global (not moved) |
| ADR-0002 | GAME-ADR-0001 | `game/docs/decisions/GAME-ADR-0001-game-capability-persistence-transaction-boundary.md` | Game |
| ADR-0003 | GAME-ADR-0002 | `game/docs/decisions/GAME-ADR-0002-session-runtime-durable-boundary.md` | Game |
| ADR-0004 | GAME-ADR-0003 | `game/docs/decisions/GAME-ADR-0003-session-runtime-actor-and-lifecycle-foundations.md` | Game |
| ADR-0005 | ADR-0005 (unchanged) | `docs/decisions/architecture/ADR-0005-cross-domain-public-entity-references.md` | Global (not moved) |
| ADR-0006 | IDENTITY-ADR-0001 | `identity/docs/decisions/IDENTITY-ADR-0001-identity-user-public-identity-boundary.md` | Identity |
| ADR-0007 | GAME-ADR-0004 | `game/docs/decisions/GAME-ADR-0004-session-lobby-lifecycle-contract.md` | Game |
| ADR-0008 | GAME-ADR-0005 | `game/docs/decisions/GAME-ADR-0005-session-public-and-internal-identity-boundary.md` | Game |
| ADR-0009 | GAME-ADR-0006 | `game/docs/decisions/GAME-ADR-0006-game-language-root-player-roster-contract.md` | Game |
| ADR-0010 | GAME-ADR-0007 | `game/docs/decisions/GAME-ADR-0007-session-runtime-turn-and-persistence-model.md` | Game |
| ADR-0011 | GAME-ADR-0008 | `game/docs/decisions/GAME-ADR-0008-session-runtime-v1-timer-recovery-simplification.md` | Game |
| ADR-0012 | GAME-ADR-0009 | `game/docs/decisions/GAME-ADR-0009-session-runtime-history-archival-and-hard-delete.md` | Game |
| ADR-0013 | GAME-ADR-0010 | `game/docs/decisions/GAME-ADR-0010-session-disconnect-reconnect-resync-boundary.md` | Game |

## 2026-09-28 Migration: Game Management / Session Runtime Domain Split

On 2026-09-28, following `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`'s domain split, the `GAME-ADR-*` records whose rationale is Session-Runtime-lifecycle-specific moved to their own independent-sequence family, `session/docs/decisions/SESSION-ADR-*`. Status, rationale, alternatives, consequences, and created dates were preserved unchanged; only identifier, location, and internal cross-reference wiring changed. A handful of `GAME-ADR-*` records stayed at `game/docs/decisions/` (foundational/shared, the retirement decision itself, or about Game Language's own internal engine/sandbox mechanics) — see `game/docs/decisions/INDEX.md`'s own note.

| Legacy ID | New Canonical ID | New Location | Scope |
| --- | --- | --- | --- |
| GAME-ADR-0002 | SESSION-ADR-0001 | `session/docs/decisions/SESSION-ADR-0001-session-runtime-durable-boundary.md` | Session Runtime |
| GAME-ADR-0003 | SESSION-ADR-0002 | `session/docs/decisions/SESSION-ADR-0002-session-runtime-actor-and-lifecycle-foundations.md` | Session Runtime |
| GAME-ADR-0004 | SESSION-ADR-0003 | `session/docs/decisions/SESSION-ADR-0003-session-lobby-lifecycle-contract.md` | Session Runtime |
| GAME-ADR-0005 | SESSION-ADR-0004 | `session/docs/decisions/SESSION-ADR-0004-session-public-and-internal-identity-boundary.md` | Session Runtime |
| GAME-ADR-0006 | SESSION-ADR-0005 | `session/docs/decisions/SESSION-ADR-0005-game-language-root-player-roster-contract.md` | Session Runtime |
| GAME-ADR-0007 | SESSION-ADR-0006 | `session/docs/decisions/SESSION-ADR-0006-session-runtime-turn-and-persistence-model.md` | Session Runtime |
| GAME-ADR-0008 | SESSION-ADR-0007 | `session/docs/decisions/SESSION-ADR-0007-session-runtime-v1-timer-recovery-simplification.md` | Session Runtime |
| GAME-ADR-0009 | SESSION-ADR-0008 | `session/docs/decisions/SESSION-ADR-0008-session-runtime-history-archival-and-hard-delete.md` | Session Runtime |
| GAME-ADR-0010 | SESSION-ADR-0009 | `session/docs/decisions/SESSION-ADR-0009-session-disconnect-reconnect-resync-boundary.md` | Session Runtime |
| GAME-ADR-0011 | SESSION-ADR-0010 | `session/docs/decisions/SESSION-ADR-0010-game-language-disconnect-reconnect-authored-semantics.md` | Session Runtime |
| GAME-ADR-0012 | SESSION-ADR-0011 | `session/docs/decisions/SESSION-ADR-0011-game-language-keyed-timer-slots.md` | Session Runtime |
| GAME-ADR-0013 | SESSION-ADR-0012 | `session/docs/decisions/SESSION-ADR-0012-session-runtime-process-agnostic-recovery.md` | Session Runtime |
| GAME-ADR-0014 | SESSION-ADR-0013 | `session/docs/decisions/SESSION-ADR-0013-session-runtime-durable-inactivity-expiration.md` | Session Runtime |
| GAME-ADR-0015 | SESSION-ADR-0014 | `session/docs/decisions/SESSION-ADR-0014-session-actor-semantic-presence-and-lobby-membership.md` | Session Runtime |
| GAME-ADR-0016 | SESSION-ADR-0015 | `session/docs/decisions/SESSION-ADR-0015-session-semantic-presence-recovery-after-total-coordinator-state-loss.md` | Session Runtime |
| GAME-ADR-0017 | SESSION-ADR-0016 | `session/docs/decisions/SESSION-ADR-0016-session-runtime-failure-classification-and-diagnostic-persistence.md` | Session Runtime |
| GAME-ADR-0018 | SESSION-ADR-0017 | `session/docs/decisions/SESSION-ADR-0017-session-running-mutation-serialization.md` | Session Runtime |
| GAME-ADR-0019 | SESSION-ADR-0018 | `session/docs/decisions/SESSION-ADR-0018-runtimeturn-execution-bound-and-terminal-cleanup.md` | Session Runtime |
| GAME-ADR-0020 | SESSION-ADR-0019 | `session/docs/decisions/SESSION-ADR-0019-session-runtime-post-commit-client-delivery-semantics.md` | Session Runtime |
| GAME-ADR-0021 | SESSION-ADR-0020 | `session/docs/decisions/SESSION-ADR-0020-session-lobby-command-idempotency-token-semantics.md` | Session Runtime |
| GAME-ADR-0022 | SESSION-ADR-0021 | `session/docs/decisions/SESSION-ADR-0021-session-lobby-business-declines-as-workflow-outcomes.md` | Session Runtime |
| GAME-ADR-0023 | SESSION-ADR-0022 | `session/docs/decisions/SESSION-ADR-0022-session-runtime-current-turn-pointer-on-sessions.md` | Session Runtime |
| GAME-ADR-0024 | SESSION-ADR-0023 | `session/docs/decisions/SESSION-ADR-0023-replay-first-session-runtime-persistence.md` | Session Runtime |
| GAME-ADR-0025 | SESSION-ADR-0024 | `session/docs/decisions/SESSION-ADR-0024-role-aware-live-connections.md` | Session Runtime |
| GAME-ADR-0029 | SESSION-ADR-0025 | `session/docs/decisions/SESSION-ADR-0025-snapshot-based-session-runtime-persistence.md` | Session Runtime |

`GAME-ADR-0001`, `GAME-ADR-0026`, `GAME-ADR-0027`, `GAME-ADR-0028`, and `GAME-ADR-0030` were not moved by this migration: they remain foundational/shared, the retirement decision itself, or about Game Language's own internal mechanics, and keep their `game/docs/decisions/` identifier and location. A bare `GAME-ADR-000X` reference in historical prose that refers to one of the migrated IDs above should be read as the corresponding new `SESSION-ADR` ID; for a record that also carried a legacy `ADR-NNNN` identifier from the 2026-09-07 migration (see the table above), both hops chain to the same final `SESSION-ADR-NNNN`.

## Global High-Water Mark

Legacy global-style IDs existed through at least `ADR-0013` before migration (including IDs that migrated to domain families). No new decision record may reuse any legacy `ADR-NNNN` identifier in this table, including ones that moved to a domain family. The next newly allocated GLOBAL architecture ID must be at least `ADR-0014` unless a later global ADR already exists at the time of allocation — check `docs/decisions/architecture/INDEX.md` for the current high-water mark before allocating.

## Notes

- A bare `ADR-000X` reference in historical prose (chat logs, commit messages, older docs) that refers to one of the migrated IDs above should be read as the corresponding new canonical ID.
- `ADR-0001` and `ADR-0005` were not moved: they were already global/cross-domain in scope and keep their original identifier and location.
- This table is append-only historical record; do not delete rows for legacy IDs even if a domain record is later superseded or deprecated.
