# Architecture Decisions

Status: DECISION INDEX

ADRs preserve architecture decision rationale. Current architecture remains owned by canonical architecture/domain docs.

| ID | Title | Status | Created | Canonical Impact |
| --- | --- | --- | --- | --- |
| [ADR-0001](ADR-0001-intra-domain-responsibility-boundary.md) | Intra-Domain Responsibility Boundary (Application Coordinates, Domain Decides, Persistence Stores) | ACCEPTED | 2026-09-06 | `ARCHITECTURE.md`, `docs/engineering/standards/domain-logic-placement.md` |
| [ADR-0002](ADR-0002-game-capability-persistence-transaction-boundary.md) | Game Capability Persistence and Transaction Boundary (Game Management / Session Runtime) | ACCEPTED | 2026-09-06 | `game/README.md`, `ARCHITECTURE.md` |
| [ADR-0003](ADR-0003-session-runtime-durable-boundary.md) | Session Runtime Durable Boundary, Live Coordinator, and V1 Scaling | ACCEPTED | 2026-09-06 | `game/README.md` |
| [ADR-0004](ADR-0004-session-runtime-actor-and-lifecycle-foundations.md) | Session Runtime Actor and Lifecycle Foundations | ACCEPTED | 2026-09-06 | `game/README.md` |
| [ADR-0005](ADR-0005-cross-domain-public-entity-references.md) | Cross-Domain Public Entity References | ACCEPTED | 2026-09-06 | `ARCHITECTURE.md`, `docs/engineering/standards/cross-domain-reference-naming.md` |

Whenever an ADR is created or its lifecycle status changes, update this index.
