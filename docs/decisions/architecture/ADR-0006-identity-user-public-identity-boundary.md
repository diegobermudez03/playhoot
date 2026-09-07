# ADR-0006: Identity User Public Identity Boundary

Status: ACCEPTED
Created: 2026-09-06
Last status change: 2026-09-06
Supersedes: None
Superseded by: None

## Context

ADR-0004 established that Session Runtime owns a session-scoped `SessionActorID`, and that a SessionActor may persist a cross-domain reference to the stable public identity entity exported by the future Identity domain. It deliberately did not decide whether that public Identity concept was `User`, `Principal`, or another term.

ADR-0005 then established the global rule for cross-domain public entity references: a producer domain may publish logical entities with stable public UUIDs, and consumers may persist those UUIDs as logical references without coupling to producer storage.

Session Runtime now needs the minimum Identity-domain boundary required to design lobby operations without freezing an invented or ambiguous identity reference.

## Decision

Accept `Identity` as the bounded context responsible for the stable identity of people interacting with Playhoot.

Identity's minimum business responsibility for this checkpoint is to maintain a stable Playhoot identity for a person so that other domains may refer consistently to that person across interactions and over time.

`User` is the public cross-domain referenceable entity exported by Identity. `User` represents the stable Playhoot identity of a real person interacting with the platform.

`User` owns a public `UserUUID` following the accepted cross-domain public entity reference architecture:

- stable;
- immutable as identity;
- non-reusable;
- identifies the logical User, not a database-row contract;
- remains resolvable by Identity regardless of internal persistence changes.

A guest is already a `User`. Guest and registered person are not separate public entities such as `Guest` versus `RegisteredUser`. If an existing guest later registers/authenticates in the normal conversion path, the same `UserUUID` survives. Registration adds capabilities/bindings to an existing User; it does not replace the User identity.

The existence of a `User` does not require an authenticated account.

Conceptually distinguish:

- User: who this person is within Playhoot;
- authentication/account concerns: how this person can prove/recover/use that identity;
- profile/presentation concerns: mutable information about how the person is represented.

Only User ownership is accepted by this decision. Account aggregates, OAuth providers, login flows, passwords, tokens, account settings, Identity persistence, and profile systems are not designed here.

Session Runtime owns its local, session-scoped `SessionActorID`. A SessionActor persistently correlates to the global User through the public cross-domain reference:

```text
user_uuid -> Identity.User
```

Session Runtime uses `SessionActorID` as its primary local/runtime identity and does not propagate `UserUUID` throughout engine semantics unnecessarily.

The `user_uuid` reference is logical:

- no cross-domain database foreign key;
- no direct reads of Identity persistence;
- no dependency on Identity table layout.

A mutable global display name is not part of the stable identity contract represented by `UserUUID`. Session Runtime owns a participation-time display-name snapshot associated with its SessionActor/Participant representation.

The canonical long-term owner of mutable global display/profile data, whether Identity itself or a future Profile responsibility, remains unresolved.

Identity reconciliation is deferred. A future edge case exists where a person may have an already-established authenticated User U1, a separately-created guest User U2, and later prove both correspond to the same person. Reconciliation, merge, or alias semantics are an Identity concern and remain deferred.

The deferred reconciliation problem does not weaken Session Runtime's contract: Session persists the `UserUUID` it was given under the accepted workflow at the time.

## Rationale

`User` is the best public entity name for the minimum boundary because Session Runtime needs to refer to a real person interacting with Playhoot, not to a credential, account, role, profile, or authentication principal. `Principal` would bias the model toward authentication/security semantics that are not needed for guest play. `Profile` would bias the model toward mutable presentation state, which is explicitly not the stable identity contract.

Treating guests and registered people as the same public `User` preserves continuity for game/session references. A person can start as a guest and later gain authentication or recovery capabilities without invalidating existing session correlations.

Separating User from Account/Auth/Profile keeps this decision small. It gives Session Runtime the stable public identity reference it needs without prematurely designing login providers, account storage, profile pages, or authorization policy.

## Alternatives Considered

### `Principal` as the public entity

Rejected for this boundary. It suggests authentication/authorization semantics and may become appropriate for security policy, but Session Runtime needs a stable person identity, including guests who may not be authenticated.

### `Profile` as the public entity

Rejected. Profile/presentation data is mutable and may belong to Identity or a future Profile responsibility. Session Runtime needs a stable identity reference, not a mutable representation.

### Separate `Guest` and `RegisteredUser` public entities

Rejected. This would make normal guest-to-registered conversion a cross-entity identity replacement problem for Session Runtime and other domains. A guest is already a User; registration adds capabilities/bindings.

### Session Runtime stores only local actors with no global User correlation

Rejected for the accepted Session Runtime needs. Session can use local `SessionActorID` internally, but it still needs durable correlation to the platform identity that participated.

### Solve Identity reconciliation now

Rejected. Merge/alias semantics are real but not needed to unblock Session Runtime lobby design. They remain a deferred Identity concern.

## Consequences

- Identity is now an accepted bounded context with a minimum domain model.
- `User` is the accepted public cross-domain referenceable entity for stable Playhoot person identity.
- `UserUUID` is the stable public UUID contract for `Identity.User`.
- `user_uuid` is now the correct persisted cross-domain reference name when referencing `Identity.User`, subject to the FUTURE CODE ONLY migration strategy.
- Session Runtime may canonically describe `SessionActor -> Identity.User` correlation through `user_uuid`.
- Existing references in prior ADRs to an unresolved public Identity entity are refined by this decision; historical ADR rationale is not rewritten.
- Mutable global display/profile ownership remains unresolved.
- Identity reconciliation/merge/alias semantics remain deferred.
- No Identity implementation, storage schema, authentication/account/profile system, migration, or WORK is authorized by this record.

## Canonical Knowledge Impact

- `ARCHITECTURE.md` - adds Identity as an accepted bounded context and records `Identity.User` / `UserUUID` as the public referenceable identity concept.
- `identity/README.md` - creates the accepted Identity domain model.
- `identity/CURRENT_STATE.md` - records that Identity is not implemented.
- `game/README.md` - replaces the unresolved external Identity reference for SessionActor with `SessionActor -> Identity.User` through `user_uuid`.
- `docs/ai/KNOWLEDGE_MAP.md` - registers Identity as an accepted domain.
- `docs/engineering/standards/cross-domain-reference-naming.md` - synchronizes the now-accepted `user_uuid -> Identity.User` naming context.

## Implementation Impact

Future implementation work must introduce Identity and SessionActor/User correlation through normal Feature Development and WORK. Migration strategy for naming changes remains FUTURE CODE ONLY. This ADR does not authorize repository-wide renaming, Identity implementation, authentication implementation, schema changes, or Session Runtime implementation.
