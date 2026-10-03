# Game Architecture Decisions

Status: DECISION INDEX (DOMAIN-SCOPED)

These records preserve architecture rationale scoped to Game Management. They are not current accepted truth: that is owned by `game/README.md` (and, where applicable, `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, `game/docs/FLOWS.md`). Load an individual record only when the rationale behind a current rule is needed.

Naming: `GAME-ADR-NNNN`, with an independent sequence from the global architecture family and from other domains.

| ID | Title | Status | Created | Legacy ID | Canonical Impact |
| --- | --- | --- | --- | --- | --- |
| [GAME-ADR-0001](GAME-ADR-0001-game-capability-persistence-transaction-boundary.md) | Game Capability Persistence and Transaction Boundary (Game Management / Session Runtime) | ACCEPTED | 2026-09-06 | ADR-0002 | `game/README.md`, `session/README.md`, `ARCHITECTURE.md` |

`GAME-ADR-0026`, `GAME-ADR-0027`, `GAME-ADR-0028` and `GAME-ADR-0030` concern the retired Game Language and the previous JavaScript execution design. They are archived in `docs/archive/pre-realtime-pivot/game/docs/decisions/` and are history only (see `docs/archive/pre-realtime-pivot/README.md`). Their numbers are not reused.

`docs/decisions/architecture/ADR-0014-management-session-domain-split.md` (global) supersedes GAME-ADR-0001's bounded-context-grouping and cross-domain-read-dependency portions **in part only** — see GAME-ADR-0001's own `Superseded by:` header for the precise scope; its independent persistence/transaction-boundary rationale remains valid and is restated by ADR-0014.

Next Game ADR: `GAME-ADR-0031`.

Whenever a Game ADR is created or its lifecycle status changes, update this index.
