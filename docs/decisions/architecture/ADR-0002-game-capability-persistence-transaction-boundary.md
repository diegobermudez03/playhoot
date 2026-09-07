# ADR-0002: Game Capability Persistence and Transaction Boundary (Game Management / Session Runtime)

Status: ACCEPTED
Created: 2026-09-06
Last status change: 2026-09-06
Supersedes: None
Superseded by: None

## Context

`ARCHITECTURE.md` already accepts Game as one business bounded context containing Game Management and Session Runtime as internal capabilities, with Game Language as a supporting subsystem. That grouping was decided on semantic/business cohesion: a live session is the execution of an authored game, and Session Runtime fundamentally requires Game's executable definition/language semantics.

That grouping left open whether Game Management and Session Runtime, because they share one bounded context, also share one persistence/transaction boundary. Concretely, `game/session/workflows/sessionlifecycle/step_create_room.go` was a stub accepting an externally supplied `engine.Program` and game version UUID, with no accepted answer for how Session Runtime should obtain Game Management's definition/version data, and no accepted rule on whether creating a session may share a database transaction with Game Management persistence.

Both capabilities currently persist to the same physical PostgreSQL database (`game/game/internal/storage/` and `game/session/internal/storage/` already use separate migration packages, and the root migration runner applies both against the same `*gorm.DB`). `sessions.game_definition_uuid` already exists as a plain reference column with no database-enforced foreign key to Game Management tables. Session Runtime also has a plausible future reason to become independently deployable/scaled, given its real-time/concurrency profile differs from Game Management's authored-content CRUD/version/publication profile.

## Decision

Game remains one bounded context. Game Management and Session Runtime remain internal capabilities of Game, not separate business domains. Game Language remains a supporting subsystem of Game.

Within that one bounded context, Game Management and Session Runtime have independent persistence and transaction ownership:

- Game Management owns its persisted state: authored games, definitions/versions, publication/visibility state, images, and related authored-game history.
- Session Runtime owns its persisted state: sessions, session state, participants, join codes, and other live-runtime persistence.
- They may continue to share the same physical PostgreSQL database. Physical co-location does not authorize either capability to mutate the other's tables, and does not require separate PostgreSQL schemas/namespaces merely to simulate future separation.
- Their migrations and persistence implementations remain independently owned.
- No database transaction may span both Game Management-owned and Session Runtime-owned state. A transaction handle must not be propagated from one capability into the other.

Session Runtime is allowed to require Game Management information — in particular, the game definition/version that defines runtime behavior — but depends on a narrow Game Management definition/read capability contract, not on Game Management's repository as its architectural API. Concrete infrastructure may be reused where appropriate, but persistence-implementation sharing must not erase capability ownership. The exact Go interface/type design implementing that contract is downstream implementation work, not decided here.

A session must be pinned to a concrete execution definition/version, and the definition observed by an existing session must remain semantically stable for that session's lifetime. Version immutability for existing sessions is the preferred model; snapshot ownership by Session Runtime is not decided here and is deferred unless a later design establishes a reason for it.

Communication between the two capabilities stays in-process (direct function/interface call) for the current modular-monolith, single-database phase. No network, queue, or RPC machinery is introduced to simulate future service separation. Independent deployment of Session Runtime, with a different communication mechanism chosen for the operational problem that exists at that time, remains a possible future option and is not a current requirement.

## Rationale

Semantic/business cohesion (one Game bounded context) and persistence/transaction ownership are different concerns. Combining Game Management and Session Runtime into one domain is justified because a session is the execution of a Game and needs Game's definition/language semantics; splitting them into separate domains would force ordinary Game behavior through artificial cross-domain orchestration.

Separating their persistence and transactions is justified independently: no current invariant requires atomic mutation across Game Management and Session Runtime state, and using a shared transaction merely because both currently share one PostgreSQL instance would create infrastructure coupling that is not currently needed. Session Runtime has a plausible future reason for independent deployment/scaling (real-time, concurrency, connection, and availability profile likely diverging from Game Management's authored-content CRUD/version/publication profile), and preserving a narrow, non-transactional seam now keeps that extraction path open cheaply.

## Alternatives Considered

### Separate Game Management and Session Runtime business domains

Rejected. This was already rejected at the bounded-context level in the existing `ARCHITECTURE.md` acceptance of Game; it cuts across the core semantic lifecycle of a Game and causes ordinary runtime operations to appear as cross-domain orchestration.

### One Game bounded context with unrestricted shared persistence/transactions between the two capabilities

Rejected. It would provide atomicity that is not currently required by any known invariant and would couple the two capabilities to the current physical database topology.

### Session Runtime directly depends on the Game Management repository as its architectural API

Rejected. Repository interfaces should remain narrow and consumer-driven; treating another capability's persistence implementation as the contract erases capability ownership even though concrete infrastructure may still be reused underneath a narrow contract.

### Introduce RPC/events/separate databases now

Rejected. There is no current operational need sufficient to justify distributed-system complexity; the modular monolith with in-process calls remains sufficient for the current phase.

### Persist a complete definition snapshot into every session immediately

Deferred, not currently required. An immutable referenced execution version is sufficient for the current architecture. Snapshot/materialization may be reconsidered if independent Session Runtime availability or deployment later requires it.

## Consequences

- Because Game Management and Session Runtime do not share a transaction, this architecture does not promise a globally atomic guarantee such as "the Game remained playable at exactly the instant the Session transaction committed." Resolving a playable immutable definition version and then creating a session pinned to that version, outside any Session Runtime write transaction, is accepted as sufficient for current requirements. Stronger linearizable consistency around unpublish/disable vs. new-session creation is not currently promised and must be escalated as a new requirement if it becomes necessary.
- `game/session/workflows/sessionlifecycle/step_create_room.go` and other session lifecycle entry points can no longer treat an externally supplied `engine.Program` as the accepted contract; Session Runtime is expected to obtain the definition itself through the narrow Game Management read capability.
- A compiled-program cache keyed by an immutable definition UUID is a valid future optimization if runtime traffic or independent-deployment latency requirements make synchronous definition lookups costly, but it is not required by this decision.
- No repository-wide or cross-capability database migration is authorized by this decision.

## Canonical Knowledge Impact

- `game/README.md` — adds the accepted Game Management / Session Runtime persistence-ownership and transaction-boundary rule, the narrow definition/read capability-interaction rule, and the session definition-pinning/immutability invariant.
- `ARCHITECTURE.md` — adds a pointer from the existing Game bounded-context/capability summary to where this Game-specific persistence/transaction boundary is recorded, without duplicating the detail.

## Implementation Impact

Not authorized by this record. Routing this decision's implementation impact (replacing the externally supplied `engine.Program` requirement in session lifecycle APIs, introducing the narrow definition/read capability, ensuring Session Runtime transactions touch only Session Runtime-owned persistence, pinning sessions to stable execution definitions, and considering Game Language package encapsulation) to the normal approved implementation process (`docs/ai/processes/FEATURE_DEVELOPMENT.md` / `docs/work/README.md`) is expected as follow-up work, not performed here.
