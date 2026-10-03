# Global Architecture Decisions

Status: DECISION INDEX (GLOBAL / CROSS-DOMAIN)

ADRs preserve architecture decision rationale. Current architecture remains owned by canonical architecture/domain docs.

This directory holds only global/cross-domain architecture decisions: decisions whose authority spans bounded contexts or the whole system. Bounded-context/domain-scoped decisions live in that domain's own decision family — see the root registry at `docs/decisions/INDEX.md` (Game: `game/docs/decisions/INDEX.md`; Identity: `identity/docs/decisions/INDEX.md`).

| ID | Title | Status | Created | Canonical Impact |
| --- | --- | --- | --- | --- |
| [ADR-0001](ADR-0001-intra-domain-responsibility-boundary.md) | Intra-Domain Responsibility Boundary (Application Coordinates, Domain Decides, Persistence Stores) | ACCEPTED | 2026-09-06 | `ARCHITECTURE.md`, `docs/engineering/standards/domain-logic-placement.md` |
| [ADR-0005](ADR-0005-cross-domain-public-entity-references.md) | Cross-Domain Public Entity References | ACCEPTED | 2026-09-06 | `ARCHITECTURE.md`, `docs/engineering/standards/cross-domain-reference-naming.md` |
| [ADR-0014](ADR-0014-management-session-domain-split.md) | Game Management / Session Runtime Domain Split, Game Language Internalized Into Session Runtime | ACCEPTED | 2026-09-27 | `ARCHITECTURE.md`, `docs/engineering/standards/domain-logic-placement.md`, `game/README.md`, `game/docs/decisions/GAME-ADR-0001-game-capability-persistence-transaction-boundary.md` |

ADR-0015 and ADR-0016 (JavaScript rule execution, and the executor as a separately deployed service) were superseded on 2026-10-03 by `session/docs/decisions/SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md` and archived in `docs/archive/pre-realtime-pivot/docs/decisions/architecture/` (history only). ADR-0014's domain-split, independent-bounded-context, and Composer/Orchestrator composition-flow decisions remain valid.


## Migrated Records

ADR-0002, ADR-0003, ADR-0004, ADR-0006, ADR-0007, ADR-0008, ADR-0009, ADR-0010, ADR-0011, ADR-0012, and ADR-0013 previously lived in this directory. On 2026-09-07 they were migrated to their owning domain's decision family (Game or Identity) under the scope-based ADR ownership model, because their decision authority was contained within one bounded context. See `docs/archive/pre-realtime-pivot/docs/decisions/LEGACY_ADR_ID_MAP.md` for the full old-ID -> new-ID -> new-location mapping, and `docs/decisions/README.md` for the routing rule.

## Next Global ID

Next newly allocated global ADR: `ADR-0017`. Legacy global-style IDs existed through at least `ADR-0013` before migration (see `docs/archive/pre-realtime-pivot/docs/decisions/LEGACY_ADR_ID_MAP.md`); never reuse a legacy ID, including one that migrated to a domain family. Gaps in this directory's numbering (ADR-0002 through ADR-0004, ADR-0006 through ADR-0013, ADR-0015, ADR-0016) are intentional and expected; they are not missing records.

Whenever a global ADR is created or its lifecycle status changes, update this index.
