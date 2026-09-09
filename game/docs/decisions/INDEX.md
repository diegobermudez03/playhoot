# Game Architecture Decisions

Status: DECISION INDEX (DOMAIN-SCOPED)

These records preserve the historical architecture rationale for the Game bounded context (Game Management, Session Runtime, Game Language). They are not current accepted truth.

Current accepted truth for Game is owned by `game/README.md` (and, where applicable, `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, `game/docs/FLOWS.md`, and package-local docs referenced from the Knowledge Map). Agents should not read every Game ADR by default; load an individual record only when the rationale/history behind a current Game rule is actually needed.

This index is the Game decision family under the repository-wide routing model described in `docs/decisions/README.md` and `docs/decisions/INDEX.md`. Naming: `GAME-ADR-NNNN`, with an independent sequence from the global architecture family and from other domains.

| ID | Title | Status | Created | Legacy ID | Canonical Impact |
| --- | --- | --- | --- | --- | --- |
| [GAME-ADR-0001](GAME-ADR-0001-game-capability-persistence-transaction-boundary.md) | Game Capability Persistence and Transaction Boundary (Game Management / Session Runtime) | ACCEPTED | 2026-09-06 | ADR-0002 | `game/README.md`, `ARCHITECTURE.md` |
| [GAME-ADR-0002](GAME-ADR-0002-session-runtime-durable-boundary.md) | Session Runtime Durable Boundary, Live Coordinator, and V1 Scaling | ACCEPTED | 2026-09-06 | ADR-0003 | `game/README.md` |
| [GAME-ADR-0003](GAME-ADR-0003-session-runtime-actor-and-lifecycle-foundations.md) | Session Runtime Actor and Lifecycle Foundations | ACCEPTED | 2026-09-06 | ADR-0004 | `game/README.md` |
| [GAME-ADR-0004](GAME-ADR-0004-session-lobby-lifecycle-contract.md) | Session Lobby Lifecycle Contract | ACCEPTED | 2026-09-06 | ADR-0007 | `game/README.md` |
| [GAME-ADR-0005](GAME-ADR-0005-session-public-and-internal-identity-boundary.md) | Session Public and Internal Identity Boundary | ACCEPTED | 2026-09-06 | ADR-0008 | `game/README.md`, `ARCHITECTURE.md` |
| [GAME-ADR-0006](GAME-ADR-0006-game-language-root-player-roster-contract.md) | Game Language Root Player Roster Contract | ACCEPTED | 2026-09-06 | ADR-0009 | `game/language/v1/program/README.md`, `game/language/v1/engine/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md`, `game/README.md` |
| [GAME-ADR-0007](GAME-ADR-0007-session-runtime-turn-and-persistence-model.md) | Session Runtime Turn Architecture and Persistence Model | ACCEPTED | 2026-09-07 | ADR-0010 | `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`, `game/README.md`, `docs/ai/KNOWLEDGE_MAP.md` |
| [GAME-ADR-0008](GAME-ADR-0008-session-runtime-v1-timer-recovery-simplification.md) | V1 Game Language Timer Schedule Recovery Simplification | ACCEPTED | 2026-09-07 | ADR-0011 | `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`, `game/README.md` |
| [GAME-ADR-0009](GAME-ADR-0009-session-runtime-history-archival-and-hard-delete.md) | Session Runtime History Archival Direction and Verified Hard-Delete | ACCEPTED | 2026-09-07 | ADR-0012 | `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`, `game/README.md` |
| [GAME-ADR-0010](GAME-ADR-0010-session-disconnect-reconnect-resync-boundary.md) | Session Disconnect, Reconnect, and Resynchronization Boundary | ACCEPTED | 2026-09-07 | ADR-0013 | `game/README.md` |
| [GAME-ADR-0011](GAME-ADR-0011-game-language-disconnect-reconnect-authored-semantics.md) | Game Language Disconnect/Reconnect Authored Semantics | ACCEPTED | 2026-09-07 | None | `game/README.md`, `game/language/v1/program/README.md`, `game/language/v1/engine/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md` |
| [GAME-ADR-0012](GAME-ADR-0012-game-language-keyed-timer-slots.md) | Game Language Keyed Timer Slots | ACCEPTED | 2026-09-07 | None | `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`, `game/README.md`, `game/language/v1/program/README.md`, `game/language/v1/engine/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md` |
| [GAME-ADR-0013](GAME-ADR-0013-session-runtime-process-agnostic-recovery.md) | Session Runtime Process-Agnostic Recovery And RuntimeTurn Crash Semantics | ACCEPTED | 2026-09-07 | None | `game/README.md` |
| [GAME-ADR-0014](GAME-ADR-0014-session-runtime-durable-inactivity-expiration.md) | Session Runtime Durable Inactivity Expiration | ACCEPTED | 2026-09-07 | None | `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md`, `game/README.md` |
| [GAME-ADR-0015](GAME-ADR-0015-session-actor-semantic-presence-and-lobby-membership.md) | Session Actor Semantic Presence and Phase-Dependent Participation Consequences | ACCEPTED | 2026-09-07 | None | `game/README.md`, `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` |
| [GAME-ADR-0016](GAME-ADR-0016-session-semantic-presence-recovery-after-total-coordinator-state-loss.md) | Session Semantic Presence Recovery After Total Coordinator State Loss | ACCEPTED | 2026-09-07 | None | `game/README.md`, `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` |
| [GAME-ADR-0017](GAME-ADR-0017-session-runtime-failure-classification-and-diagnostic-persistence.md) | Session Runtime Failure Classification and Fatal Diagnostic Persistence | ACCEPTED | 2026-09-08 | None | `game/README.md`, `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` |
| [GAME-ADR-0018](GAME-ADR-0018-session-running-mutation-serialization.md) | Session RUNNING Mutation Serialization | ACCEPTED | 2026-09-08 | None | `game/README.md`, `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` |
| [GAME-ADR-0019](GAME-ADR-0019-runtimeturn-execution-bound-and-terminal-cleanup.md) | RuntimeTurn Execution Bound and Terminal Cleanup | ACCEPTED | 2026-09-08 | None | `game/README.md`, `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` |

Next Game ADR: `GAME-ADR-0020`.

These IDs were migrated from the previously centralized global architecture family (`docs/decisions/architecture/`) on 2026-09-07. Historical status, rationale, alternatives, consequences, and created dates were preserved unchanged; only the identifier/location/cross-reference wiring changed. See `docs/decisions/LEGACY_ADR_ID_MAP.md` for the full old-ID -> new-ID mapping.

Whenever a Game ADR is created or its lifecycle status changes, update this index.
