# WORK-0044: Game Version Artifact Model (Script + Frontend Package + Contracts + Assets)

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`
- `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`

Canonical context:
- `game/management/models.go` (the current `game_definitions` versioning precedent this generalizes)

## Outcome

Define the artifact model a Game version actually is going forward: backend JavaScript (rules), a reference to a compiled frontend package (React/TypeScript/CSS/SVG), the platform/game-specific contract this version exposes (`WORK-0039`'s vocabulary plus this game's own action/view shapes), optional assets, and the capability/engine versions relevant to executing it. A running Session is pinned to one immutable Game version's artifact bundle for its whole lifetime, restating `GAME-ADR-0001`'s existing immutability invariant against the new artifact shape. This is foundational for `WORK-0034` (Session's own persisted copy), `WORK-0033` (publish composition), `WORK-0046` (frontend serving), and `WORK-0052` (the JavaScript Executor service, which must receive/resolve this same artifact's backend-script content) — each needs this shape decided to avoid designing against a placeholder.

## Context

Not yet designed. Must explicitly preserve the mandate's requirement that assets remain optional and that no build pipeline is required for a functional game — the artifact model must not implicitly require a frontend package or asset bundle to exist before a game can run.

**Scope addition (2026-09-27, per `ADR-0016`):** the artifact model must define a mechanism for getting a version's backend-script content to the separately deployed JavaScript Executor that does not depend on a filesystem shared with Session Runtime — Session and the Executor may run on different machines, in different containers, or in different Kubernetes pods. Concretely, this WORK must decide between (or combine) inline bytes in the execution request where bounded and appropriate, an immutable artifact identifier the Executor resolves itself, an object-storage-backed reference, or another explicit remote-safe mechanism — and must not solve this by giving the Executor broad access to Session Runtime's own storage, database, or secrets (per `ADR-0016`'s Security boundary, the Executor keeps the minimum capability it actually needs).

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Immutable once a Session pins to it (`GAME-ADR-0001`).
- No cross-domain database foreign key to Game Management's own tables (`ARCHITECTURE.md -> Cross-Domain Public Entity References`).
- Assets/frontend package are optional; a valid artifact may consist of backend script alone.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

### Accepted / Canonical Knowledge

- A new package-local contract document recording the artifact model's exact shape.

### Current-State Documentation After Implementation

- `game/docs/DATA_MODEL.md` — new artifact-related tables.

## Blockers

- Artifact bundle format and storage decision (`PROJECT.md` Material Decisions #5).

## Completion Record

Not yet started.
