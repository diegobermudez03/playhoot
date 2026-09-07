# ADR-0008: Session Public and Internal Identity Boundary

Status: ACCEPTED
Created: 2026-09-06
Last status change: 2026-09-06
Supersedes: None
Superseded by: None

## Context

ADR-0004 introduced the Session-owned `SessionActorID` concept and left the external Identity entity unresolved. ADR-0006 accepted `Identity.User` and `UserUUID`. The lobby operation design then clarified how caller identity should cross Session Runtime's public/application boundary and how engine/runtime identity should remain session-local.

The design must avoid two leaks: exposing internal SessionActor IDs to clients/adapters as if they were public contract, and leaking `Identity.UserUUID` into Game Language semantics where authored games should only know session-local runtime actors.

## Decision

User-originated operations crossing the public/application boundary of Session Runtime identify the caller through authenticated `UserUUID`.

The external client is not the trusted source of `UserUUID`. A trusted authentication/application layer resolves credentials into an authenticated `UserUUID`.

Session Runtime owns the translation:

```text
(SessionUUID, UserUUID) -> SessionActorID
```

Session-internal runtime behavior primarily uses `SessionActorID`.

Game Language / engine user identity must not receive `Identity.UserUUID`. The engine runtime user identity represents the Session-local actor.

`SessionActorID` is a Session-owned internal identity. It is not part of Session Runtime's public/application-facing contract. Do not export it merely for upper layers to call Session operations. Do not require a public SessionActor UUID solely because SessionActor exists. Local IDs remain appropriate unless another explicit requirement later makes SessionActor cross-domain/public.

Coordinator, Orchestrator, HTTP/WebSocket adapters, and clients do not own or maintain the mapping from `UserUUID` to `SessionActorID`.

When engine/runtime behavior targets a Session-local actor, Session Runtime resolves that internal actor back to the cross-domain/public identity needed by the application/Coordinator boundary:

```text
engine.UserID / SessionActorID -> SessionActor.user_uuid -> UserUUID
```

This happens before crossing the Session Runtime boundary. Coordinator does not need knowledge of `SessionActorID`. This ADR does not freeze a final transport DTO/protocol.

Identity/Auth proves who the caller is. Session Runtime decides what that User may do in this Session according to Session business rules, including whether this User is represented by a SessionActor in this Session, whether that actor is the host, whether it is an active Participant, and whether the requested operation is legal in the current lifecycle/runtime state.

Do not move host/participant business authorization into Identity.

## Rationale

`UserUUID` is the right public/application identity because it is the accepted stable cross-domain identity for a person. `SessionActorID` is the right internal runtime identity because it is Session-owned, session-scoped, and can express host/participant/runtime relationships without turning Identity into a game semantics dependency.

Keeping the translation inside Session Runtime prevents clients, adapters, Coordinator, or Orchestrator from owning Session business identity mappings. It also prevents authored Game Language from observing global person identifiers when it only needs session-local actors.

Separating authentication from Session business authorization keeps responsibilities clean: trusted outer layers establish who is calling; Session Runtime decides whether that caller can perform the requested Session operation.

## Alternatives Considered

### Public callers pass SessionActorID

Rejected. It exposes an internal Session identity and forces upper layers or clients to know and maintain SessionActor mappings.

### Engine receives Identity.UserUUID

Rejected. Authored game semantics should operate on Session-local runtime actors, not global identity identifiers.

### Coordinator owns UserUUID-to-SessionActorID mapping

Rejected. Coordinator owns ephemeral delivery/connection mechanics, not durable authoritative Session identity truth.

### Identity owns host/participant authorization

Rejected. Identity proves who the person is. Host/participant authority is Session business state and belongs to Session Runtime.

## Consequences

- Public/application Session Runtime commands should accept authenticated `UserUUID`.
- Session Runtime resolves `UserUUID` to Session-local actor identity internally.
- `SessionActorID` should not be exposed as a public Session API requirement.
- Outbound Session Runtime delivery must translate Session-local actor targets back to `UserUUID` before crossing to application/Coordinator boundaries.
- Existing implementation fields such as `owner_uuid` and `player_uuid` may require future migration/design alignment, but this ADR does not authorize code or schema changes.

## Canonical Knowledge Impact

- `game/README.md` - adds the accepted Session public/internal identity translation boundary.
- `ARCHITECTURE.md` - clarifies authentication proves identity while a domain owns its own business authorization decisions.

## Implementation Impact

Future implementation work must keep SessionActor mapping inside Session Runtime and keep public commands on authenticated `UserUUID`. No production code, migration, transport DTO, or WORK is authorized by this record.
