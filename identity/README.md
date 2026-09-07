# Identity

Status: CANONICAL DOMAIN MODEL

## Responsibility

Identity maintains a stable Playhoot identity for a person so other domains may refer consistently to that person across interactions and over time.

## Owns

- Stable User identity for people interacting with Playhoot.
- Public `UserUUID` for each `User`.
- Guest-to-registered continuity of the same `UserUUID` in the normal conversion path.
- Resolution of public `UserUUID` to the current internal representation once implemented.

## Does Not Own

- Session-local participation state.
- Session display-name snapshots.
- Game/session behavior.
- Authentication/account/provider details beyond what is later accepted as necessary.
- Authorization policy beyond what is later accepted as necessary.
- Mutable global display/profile ownership; whether this belongs to Identity or a future Profile responsibility remains unresolved.
- Public discovery/search experience.

## Internal Structure

### Business Capabilities

- User Identity: owns stable Playhoot person identity and public `UserUUID` continuity.

## Boundary Notes

`User` is the public cross-domain referenceable entity exported by Identity. `User` represents the stable Playhoot identity of a real person interacting with the platform.

`UserUUID` is stable, immutable as identity, non-reusable, and identifies the logical `User`, not a database-row contract. Internal persistence may change without changing the public identity.

A guest is already a `User`. Guest and registered person are not separate public entities. If an existing guest later registers/authenticates in the normal conversion path, the same `UserUUID` survives; registration adds capabilities/bindings to an existing User rather than replacing the User identity.

The existence of a `User` does not require an authenticated account.

Conceptually:

- User is who this person is within Playhoot.
- Authentication/account concerns are how this person can prove, recover, or use that identity.
- Profile/presentation concerns are mutable information about how the person is represented.

Session Runtime may persist `user_uuid` as a logical cross-domain reference to `Identity.User`. That reference does not authorize cross-domain database foreign keys, direct reads of Identity persistence, or dependency on Identity table layout.

Identity reconciliation is deferred. If a person later proves that an authenticated User and a separately-created guest User correspond to the same person, reconciliation, merge, or alias semantics are an Identity concern to design later. This does not weaken consumers' contract: other domains persist the `UserUUID` they were given under the accepted workflow at the time.

Rationale and alternatives are recorded in `docs/decisions/architecture/ADR-0006-identity-user-public-identity-boundary.md`.
