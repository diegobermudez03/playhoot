# WORK-0009: Client-Safe Game UI Manifest

Status: PLANNED
Created: 2026-09-20
Last status change: 2026-09-20

Related decisions:
- GAME-ADR-0001 (Game Management/Session Runtime persistence and transaction boundary; pinned Definition resolution)
- GAME-ADR-0004 (pinned Game Definition immutability for a Session)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/language/v1/program/README.md`, `game/language/v1/program/{ui.go,view.go,presentation.go}`
- `game/management` (existing pinned-Definition read capabilities this WORK's manifest derivation would build on)

## Outcome

A client can obtain whatever client-side metadata it needs to render a game's authored UI (Views, action declarations, client-local declarations, presentation-related and effect-related metadata as needed) without ever receiving the full authoritative Game `Definition` - which also contains workflow logic, authoritative runtime-state shape, and other server-only semantics that must not reach the client.

Today, no such capability exists anywhere: `grep` across `game/session/`, `play/`, and `api/` for anything manifest/client-view-shaped returns no hits. A client currently has no way to know what Views/UI a pinned Game version even declares except by being told out of band.

## Context

This is a prerequisite for building any real Session frontend, not an enhancement: WORK-0006 delivers Presentation/Effect *events* live, but a client needs the corresponding *static* declarations (what a named View/Presentation looks like, what fields its data model has, what action a UI element triggers) to know how to render what those events reference. Without this WORK, a frontend would need those declarations delivered some other, undesigned way.

`program/` already has the source data this manifest derives from (`ui.go`/`view.go`/`presentation.go`), and `game/management` already has the pinned-Definition read capability this WORK's derivation would sit behind (parallel to how `getgamedefinition` already serves Session Runtime's own pinned-Definition needs, per WORK-0001).

## Scope

### In Scope (known required outcome; design not yet started)

- A derived, client-safe manifest/projection of a pinned Game Definition's UI-relevant declarations (Views, UI action declarations/metadata, client-local declarations as appropriate, presentation- and effect-related metadata as needed by supported UI semantics).
- A read capability/endpoint a client can call (or receive on connect) to obtain this manifest for the Session's pinned Game version.

### Out of Scope

- Exposing workflows, authoritative runtime state, private server projections, or server execution internals - explicitly excluded, not merely deprioritized.
- Any change to the authoritative `program.Definition` decode/compile path (Game Management/engine) - this WORK derives a projection, it does not change the source model.
- Runtime Presentation/Effect delivery itself (WORK-0006) - this WORK provides the static declarations those events reference, not the events.

## Approved Design

Not yet designed. The exact manifest contract (which fields of `ui.go`/`view.go`/`presentation.go` are included, exact wire shape, whether it is delivered once at connect or fetched on demand, versioning against the pinned Definition) remains an open design question, deliberately left open per the reconciliation prompt that created this WORK.

## Constraints and Invariants

- Must not expose authoritative server-only semantics: workflows, authoritative runtime state, private server projections, server execution internals.
- Must be derived from the same pinned Game version a Session already resolved/pinned at Create (GAME-ADR-0004) - never re-resolved against "current version".

## Acceptance Criteria

Not yet defined - to be written when this WORK moves to DRAFT.

## Blockers

- None yet identified beyond the manifest-contract design question itself, which is the point of the DRAFT phase, not a blocker to creating this WORK as PLANNED.

## Documentation Impact

Not yet assessed in detail; expected to touch `game/language/v1/program/README.md`, `game/CURRENT_STATE.md`, `play/README.md`/`api/README.md` once designed.

## Completion Record

Not started. PLANNED.
