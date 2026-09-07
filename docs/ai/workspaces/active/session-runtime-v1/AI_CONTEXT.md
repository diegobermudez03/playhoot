Process: Domain Design
Topic: Session Runtime lifecycle/runtime design; minimum public Identity-domain concept needed by Session Runtime
Current stage: architecture foundations canonically promoted; temporarily entering minimal Identity Domain Design
Current execution surface: CONVERSATIONAL AI
Parent process: Architecture Discussion
Return to: Session Runtime architecture discussion after the minimum public Identity concept needed for SessionActor correlation is accepted or deferred
Related durable artifacts: `ARCHITECTURE.md`, `game/README.md`, `docs/decisions/architecture/ADR-0002-game-capability-persistence-transaction-boundary.md`, `docs/decisions/architecture/ADR-0003-session-runtime-durable-boundary.md`, `docs/decisions/architecture/ADR-0004-session-runtime-actor-and-lifecycle-foundations.md`, `docs/decisions/architecture/ADR-0005-cross-domain-public-entity-references.md`, `docs/engineering/standards/cross-domain-reference-naming.md`
Blocked by: human decisions on the minimal public Identity entity concept that Session Runtime may reference
Next action: Conversational AI should run a minimal Domain Design discussion using `HUMAN_REVIEW.md`; do not design auth/login/profile beyond what is needed for the SessionActor cross-domain reference boundary
Last durable checkpoint: accepted Session Runtime runtime boundary, Coordinator boundary, timer durability, V1 scaling stance, actor/lifecycle foundations, and cross-domain entity-reference architecture rule were promoted to canonical docs and ADRs; no WORK created
Last updated: 2026-09-06

# Resume Context

This workspace preserves the active initiative `session-runtime-v1`.

Goal: design and then incrementally implement the complete Session Runtime lifecycle. This is prioritized because playable multiplayer sessions are a core initial product capability.

The initiative was in Architecture Discussion. It is now temporarily entering Domain Design to define only the minimum public Identity-domain concept needed by Session Runtime for SessionActor correlation. Return to Session Runtime architecture after that boundary question is accepted or deferred.

This workspace records process continuity only. Accepted architecture/domain facts were promoted to canonical owners and ADRs where required. No WORK or implementation authority exists yet.

## Source References Loaded

- `docs/ai/OPERATING_MODEL.md` - execution-surface authority, drift handling, decision boundaries.
- `docs/ai/KNOWLEDGE_MAP.md` - routing map used to load Game/domain and process context.
- `docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md` - initiative workspace model and process transition behavior.
- `docs/ai/workspaces/README.md` - required active workspace semantics and resume header.
- `docs/ai/processes/ARCHITECTURE_DISCUSSION.md` and `docs/ai/protocols/ARCHITECTURE_DISCUSSION.md` - architecture checkpoint and canonical-promotion behavior.
- `docs/ai/processes/DOMAIN_DESIGN.md` and `docs/ai/protocols/DOMAIN_DESIGN.md` - next process for the Identity boundary question.
- `docs/ai/processes/ENGINEERING_STANDARD.md` and `docs/ai/protocols/ENGINEERING_STANDARD.md` - accepted reusable standard persistence behavior.
- `docs/decisions/README.md` and `docs/decisions/templates/ARCHITECTURE_DECISION.template.md` - ADR creation and accepted-decision synchronization rules.
- `ARCHITECTURE.md` - global architecture owner.
- `game/README.md` - Game domain model owner.
- `docs/engineering/standards/INDEX.md` - standards index owner.
- `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, and `game/docs/FLOWS.md` - current implementation state references; not modified.
- `game/session/workflows/sessionlifecycle/step_create_room.go` - current `CreateRoom` stub accepts externally supplied `engine.Program`.
- `game/session/workflows/sessionlifecycle/step_join_room.go` - current `JoinRoom` stub.
- `game/session/workflows/sessionlifecycle/internal/repo/step_create_room.go` - incomplete repo scaffold.

## Canonically Promoted Architecture Decisions

### ADR-0003

`docs/decisions/architecture/ADR-0003-session-runtime-durable-boundary.md`

Status: ACCEPTED.

Accepted facts:

- Session Runtime owns all durable authoritative state that determines the meaning of a session and must be reconstructible after process loss/restart.
- "Stateless" means process-stateless/reconstructible, not domain-stateless.
- `Live Session Coordinator` is a conceptual responsibility boundary outside Session Runtime.
- The Coordinator owns ephemeral live connection bindings, delivery/fan-out, physical disconnect detection, and physical timer/scheduling mechanisms.
- The Coordinator does not own authoritative session/game truth or business consequences.
- V1 may keep the Coordinator in the same Go process.
- V1 does not introduce sticky-session correctness requirements, Redis, distributed session routing, distributed locks, or other multi-instance mechanisms.
- V1 may use local maps, channels, and Go timers for ephemeral mechanisms.
- Correctness must not depend exclusively on ephemeral objects.
- Timer obligations that can affect game semantics are durable Session Runtime state.
- Coordinator detects elapsed wall-clock time and calls Session Runtime with the expiration signal/event; Session Runtime decides the semantic consequence.
- Pending or overdue timer obligations must be reconstructible/recoverable from durable state.

Canonical owner synchronized: `game/README.md`.

### ADR-0004

`docs/decisions/architecture/ADR-0004-session-runtime-actor-and-lifecycle-foundations.md`

Status: ACCEPTED.

Accepted facts:

- Host and Participant are independent concepts.
- Creating a Session establishes a host but does not automatically make that host a gameplay participant.
- The same Session-owned actor may be both host and Participant, but those are separate relationships.
- V1 does not introduce separate mutable `creator` and `host` concepts unless another accepted decision requires them.
- Host transfer is not currently designed.
- Participant is session-scoped and belongs to one Session.
- Session Runtime does not own a cross-session Player/Profile master entity.
- Participant contains Session-owned participation/lifecycle state plus a display-name snapshot.
- V1 does not add a generic persisted `role` field merely to anticipate future spectators, judges, controllers, or gameplay roles.
- Gameplay roles belong primarily to game definition/runtime state unless Session Runtime later has a concrete cross-game reason to own such a distinction.
- An actor may occupy at most one active logical participation for the same Session.
- Game does not establish a platform-wide invariant that one User may belong to only one active Session.
- Session Runtime conceptually owns a session-scoped identity such as `SessionActorID`.
- Host and Participant relationships refer to this local identity.
- Runtime/domain operations should primarily operate using Session-owned identity rather than propagating external Identity-domain UUIDs throughout engine/runtime behavior.
- A SessionActor may persist a cross-domain reference to the stable public identity entity exported by the future Identity domain.
- The exact public Identity entity name/semantics are unresolved and must be decided through Domain Design before Session docs freeze a persisted field name such as `user_uuid`.
- Conceptual Session lifecycle is `LOBBY -> RUNNING -> TERMINAL`; `TERMINAL` is irreversible.
- Terminal cause/reason is modeled separately from lifecycle phase; exhaustive terminal-reason enum is not yet accepted.
- Current expiration concern is `lobby_expires_at`, not a generic whole-session lifetime.
- Join/Start correctness must enforce `lobby_expires_at` even if cleanup has not materialized the Session as terminal.
- Later max-runtime/runaway-session policy is a different concern.
- Connection state is not Participant state; physical connection presence belongs to the Live Session Coordinator.
- Disconnect/reconnect semantics remain deferred.

Canonical owner synchronized: `game/README.md`.

### ADR-0005

`docs/decisions/architecture/ADR-0005-cross-domain-public-entity-references.md`

Status: ACCEPTED.

Accepted facts:

- A domain may explicitly publish certain logical entities as cross-domain referenceable entities.
- A cross-domain referenceable entity has a stable public UUID.
- The UUID identifies the logical entity, not a physical database row/table contract.
- Internal persistence may change without changing the public identity.
- Consumers may persist that public UUID as a logical reference.
- Consumers must not create cross-domain database foreign keys, directly access producer persistence as domain behavior, or depend on producer table layout.
- The producer resolves the public identity to its internal representation.
- Public logical entity identifier is distinct from storage/table primary key contract.
- A cross-domain referenceable/public entity must have a stable public UUID.
- Internal-only entities should normally use local IDs unless another explicit reason requires globally stable identity.
- Do not infer the inverse rule: not every UUID means public/exported.
- Orchestrator may own workflow/saga state, idempotency, retries, and intermediate results, but not permanent business source-of-truth mappings such as `global identity <-> SessionActor`.

Canonical owners synchronized: `ARCHITECTURE.md`, `docs/engineering/standards/cross-domain-reference-naming.md`, and `docs/engineering/standards/INDEX.md`.

## Engineering Standard

`docs/engineering/standards/cross-domain-reference-naming.md`

Status: CANONICAL ENGINEERING STANDARD.

Accepted naming rule: for a persisted cross-domain reference, name the field/column after the public entity being referenced, not after the consumer's local role for it. Examples include `user_uuid`, `game_uuid`, `organization_uuid`, and role-qualified forms like `created_by_user_uuid`.

Avoid ambiguous names such as `external_id`, `reference_id`, or `actor_uuid` when the value actually references a published `User` entity.

Do not prescribe `user_uuid` for Session Runtime until Domain Design has accepted the public Identity entity name and semantics.

Migration strategy: FUTURE CODE ONLY. No repository-wide migration or renaming is authorized.

## Deferred Design Topics

### Disconnect / Reconnect Semantics

Status: DEFERRED DESIGN TOPIC, not a decided feature.

Physical connection state is not logical session participation state.

Future design must determine:

- under what conditions a disconnected participant may reconnect;
- whether there is a grace period;
- when a participant is considered inactive, forfeited, or removed;
- whether the game continues, waits, substitutes/removes the participant, or terminates;
- what part of this behavior is generic Session Runtime policy versus authored Game Language behavior;
- how reconnect is represented without making Session Runtime own TCP/WebSocket connections.

Current human preference/intuition, not an accepted design: some reconnect/inactivity semantics may need to be expressible by the game itself because different games can require different behavior.

Do not design the final reconnect contract yet. Preserve it for the later lifecycle-operational milestone.

### Session Runtime Topics

- Exhaustive terminal-reason enum is not yet accepted.
- Host transfer is not designed.
- Max-runtime/runaway-session policy is distinct from `lobby_expires_at` and remains deferred.
- Completed-session history/archive ownership remains unresolved and may require Domain Design later.

## Next Domain Design Boundary Question

Define only the minimum public Identity-domain concept needed by Session Runtime.

The next Domain Design discussion must determine, rather than assume:

- What is the stable public Identity entity: `User`, `Principal`, or another concept?
- What business thing does that entity represent?
- Does the same public identity exist for guests and registered people?
- If a guest later registers, does the same public UUID survive that transition?
- Which state belongs to that public entity versus Account/Profile/Auth details?
- What public capabilities are necessary to create/resolve it?
- What Identity explicitly does not own.
- Whether `display_name` is Identity-owned mutable profile data while Session keeps its own participation-time snapshot.

Keep this Domain Design deliberately minimal. Do not design login providers, OAuth, profile pages, account settings, full authentication architecture, or Identity persistence unless needed to answer the boundary question.

## Current Implementation Facts

- `game/CURRENT_STATE.md` records Session Runtime as partial: session, session-state, session-player, and join-code schema exists; lifecycle scaffolding exists; `CreateRoom` and `JoinRoom` are stubs; no session execution flow was found implemented.
- `game/docs/DATA_MODEL.md` records current Session Runtime tables: `sessions`, `session_players`, `join_codes`, and `session_states`.
- `game/docs/FLOWS.md` records `CreateRoom` and `JoinRoom` as stubs.
- `game/session/workflows/sessionlifecycle/step_create_room.go` currently declares `CreateRoom(ctx, program engine.Program, gameVersionUUID, ownerUUID)`.
- `game/session/workflows/sessionlifecycle/step_join_room.go` currently returns nil without behavior.
- `game/session/workflows/sessionlifecycle/internal/repo/step_create_room.go` declares `func (r *Repo) CreateRoom(ctx context.Context)` without a function body.

## Drift To Carry Forward

DRIFT DETECTED

Canonical model: ADR-0002 says Session Runtime should no longer treat an externally supplied `engine.Program` as the accepted contract. It should obtain definition/version information itself through a narrow Game Management read capability and pin the session to a stable execution definition/version.

Current implementation: `game/session/workflows/sessionlifecycle/step_create_room.go` still accepts an externally supplied `engine.Program`, and the lifecycle/repo code is scaffolding.

Likely explanation: Session Runtime existed as a pre-decision scaffold before ADR-0002 clarified the capability boundary and definition-read contract.

Recommended resolution: route it into later Feature Development/WORK after required lifecycle/runtime and minimal Identity-domain decisions are accepted. Do not fix it during documentation persistence.

Related documentation note: `game/CURRENT_STATE.md` currently says no known drift, while ADR-0002 and the inspected `CreateRoom` stub reveal this drift. This workspace records the drift but does not update current-state documentation because current-state docs were excluded from this handoff.

## Explicitly Not Done

- No WORK was created.
- No production code, tests, migrations, or current implementation-state docs were changed.
- No implementation is claimed to exist.
- No Identity public entity name or semantics were invented.
- No final reconnect contract was designed.
