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

## Global High-Water Mark

Legacy global-style IDs existed through at least `ADR-0013` before migration (including IDs that migrated to domain families). No new decision record may reuse any legacy `ADR-NNNN` identifier in this table, including ones that moved to a domain family. The next newly allocated GLOBAL architecture ID must be at least `ADR-0014` unless a later global ADR already exists at the time of allocation — check `docs/decisions/architecture/INDEX.md` for the current high-water mark before allocating.

## Notes

- A bare `ADR-000X` reference in historical prose (chat logs, commit messages, older docs) that refers to one of the migrated IDs above should be read as the corresponding new canonical ID.
- `ADR-0001` and `ADR-0005` were not moved: they were already global/cross-domain in scope and keep their original identifier and location.
- This table is append-only historical record; do not delete rows for legacy IDs even if a domain record is later superseded or deprecated.
