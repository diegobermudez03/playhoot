# Game Architecture Decisions

Status: DECISION INDEX (DOMAIN-SCOPED)

These records preserve historical architecture rationale scoped to Game Management and to Game Language (`game/language/v1/...`, currently parked inside `game/` pending its own eventual retirement). They are not current accepted truth.

**Split (2026-09-28):** this family originally held every decision from the old shared "Game" bounded context (Game Management, Session Runtime, Game Language together). Now that Game Management and Session Runtime are independent bounded contexts (`docs/decisions/architecture/ADR-0014-management-session-domain-split.md`), the 25 records whose rationale is Session-Runtime-lifecycle-specific moved to their own family, `session/docs/decisions/` (`SESSION-ADR-NNNN`, independent sequence) — see `session/docs/decisions/INDEX.md`. Each moved record keeps a `Legacy ID: GAME-ADR-NNNN` header line, and `docs/decisions/LEGACY_ADR_ID_MAP.md` records the full mapping so old references remain resolvable. The 5 records below stayed here because they are foundational/shared across both domains (`GAME-ADR-0001`), the retirement decision itself (`GAME-ADR-0028`), or about Game Language's own internal engine/sandbox mechanics (`GAME-ADR-0026`, `GAME-ADR-0027`, `GAME-ADR-0030`), which currently live physically inside `game/` alongside this index.

Current accepted truth for Game Management is owned by `game/README.md` (and, where applicable, `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, `game/docs/FLOWS.md`, and package-local docs referenced from the Knowledge Map). Agents should not read every Game ADR by default; load an individual record only when the rationale/history behind a current rule is actually needed.

This index is the Game decision family under the repository-wide routing model described in `docs/decisions/README.md` and `docs/decisions/INDEX.md`. Naming: `GAME-ADR-NNNN`, with an independent sequence from the global architecture family and from other domains (including the newer `SESSION-ADR-NNNN` family this index's own records partly seeded).

| ID | Title | Status | Created | Legacy ID | Canonical Impact |
| --- | --- | --- | --- | --- | --- |
| [GAME-ADR-0001](GAME-ADR-0001-game-capability-persistence-transaction-boundary.md) | Game Capability Persistence and Transaction Boundary (Game Management / Session Runtime) | ACCEPTED | 2026-09-06 | ADR-0002 | `game/README.md`, `session/README.md`, `ARCHITECTURE.md` |
| [GAME-ADR-0026](GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md) | Flat Workflow Execution Model, Keyed Interaction Slots, and Engine-Owned Interaction Addressing | ACCEPTED | 2026-09-24 | None | `game/language/v1/engine/LOGICAL_CONTRACT.md`, `game/language/v1/engine/README.md`, `game/language/v1/program/README.md`, `session/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` |
| [GAME-ADR-0027](GAME-ADR-0027-engine-owned-turn-execution-and-replay.md) | Engine-Owned Turn Execution — Replay and Step-Chain Draining Move Inside `engineservice` | ACCEPTED | 2026-09-24 | None | `game/language/v1/engine/LOGICAL_CONTRACT.md`, `game/language/v1/engine/README.md`, `session/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` |
| [GAME-ADR-0028](GAME-ADR-0028-javascript-execution-replaces-game-language.md) | JavaScript Execution Replaces Game Language (program/engine v1) | ACCEPTED | 2026-09-27 | None | `game/README.md`, `game/CURRENT_STATE.md`, `session/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md`, `game/language/v1/program/README.md` |
| [GAME-ADR-0030](GAME-ADR-0030-sandbox-runtime-quickjs-wasm-wazero-process-isolated-workers.md) | Sandbox Runtime — QuickJS-on-WASM (wazero) In OS-Process-Isolated Workers | ACCEPTED | 2026-09-27 | None | `docs/projects/active/js-runtime-migration/PROJECT.md`, `docs/projects/active/js-runtime-migration/works/WORK-0035-sandboxed-javascript-execution-runtime.md`, `docs/projects/active/js-runtime-migration/works/WORK-0036-execution-resource-limits-and-isolation-boundary.md` |

Next Game ADR: `GAME-ADR-0031`.

`docs/decisions/architecture/ADR-0014-management-session-domain-split.md` (global) supersedes GAME-ADR-0001's bounded-context-grouping and cross-domain-read-dependency portions **in part only** — see GAME-ADR-0001's own `Superseded by:` header for the precise scope; its independent persistence/transaction-boundary rationale remains valid and is restated by ADR-0014.

GAME-ADR-0026 generalizes (does not supersede) `session/docs/decisions/SESSION-ADR-0011`'s keyed-slot concept (formerly GAME-ADR-0012). GAME-ADR-0027 refines `session/docs/decisions/SESSION-ADR-0018` (formerly GAME-ADR-0019; Step-chain-bound enforcement location only) and generalizes `session/docs/decisions/SESSION-ADR-0023` (formerly GAME-ADR-0024; replay ownership, not the no-persisted-Snapshot principle).

`docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md` (global) supersedes GAME-ADR-0030's deployment-topology portion **only** — see ADR-0016's own header for the precise scope; GAME-ADR-0030's sandbox-technology choice and process-isolated-worker layering remain valid and are restated by ADR-0016.

GAME-ADR-0028 restates (does not supersede) GAME-ADR-0001, GAME-ADR-0026, GAME-ADR-0027, and several now-`SESSION-ADR-*` records (formerly GAME-ADR-0006, 0008, 0010, 0011, 0012, 0018, 0019 — see `session/docs/decisions/INDEX.md`) at the architecture-principle level while retiring the compiled-DSL mechanism (`program`/`engine` v1) each was originally written against; see `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`'s Consequences for the full list.

These IDs were migrated from the previously centralized global architecture family (`docs/decisions/architecture/`) on 2026-09-07. Historical status, rationale, alternatives, consequences, and created dates were preserved unchanged; only the identifier/location/cross-reference wiring changed. See `docs/decisions/LEGACY_ADR_ID_MAP.md` for the full old-ID -> new-ID mapping, including the 2026-09-28 GAME-ADR -> SESSION-ADR split.

Whenever a Game ADR is created or its lifecycle status changes, update this index.
