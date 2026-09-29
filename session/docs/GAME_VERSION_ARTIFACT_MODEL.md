# Game Version Artifact Model (Accepted Design)

Status: ACCEPTED DESIGN, NOT YET IMPLEMENTED AS PERSISTED SCHEMA. Records `docs/projects/active/js-runtime-migration/works/WORK-0044-game-version-artifact-model.md`'s accepted shape. Describes what a Game version *is* going forward, not current implementation. For the actually persisted schema, see `game/docs/DATA_MODEL.md`; today it still reflects the retired Game Language shape (`management.Game.Definition program.Definition`) until `WORK-0034` lands.

This document is a canonical contract other WORK reference. It does not itself define a shared Go type: per `ARCHITECTURE.md`'s Dependency Principles and `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`'s own rationale, each consuming domain/WORK declares its own narrow type against this shape rather than importing one concrete type across domain boundaries.

## What A Game Version Artifact Is

```text
GameVersionArtifact
  DefinitionUUID          - the same immutable version identity already pinned at Create (sessions.game_definition_uuid)
  BackendScript           - authored JavaScript source (required); executed by the JavaScript Executor
  FrontendScript          - authored frontend source (required); format not yet decided; stored, never parsed or executed by Playhoot itself
  GameContract            - this game's own declared event/data vocabulary, alongside (not replacing) the platform's own fixed command vocabulary
  ParticipantConstraints  - required structural Session admission/capacity range: {Min, Max} (Max nil/absent means unlimited)
  Assets                  - optional list of {Key, Kind} - images/sounds/etc. uploaded directly to Playhoot; identified only by an internal logical Key, never a URL
  PlatformContractVersion - which version of the platform command vocabulary this artifact was authored against
```

Both `BackendScript` and `FrontendScript` are mandatory. A valid artifact always has both scripts; there is no fallback/default-rendering path for a missing frontend script, and no "backend-only" artifact shape. `GameContract`, `Assets`, and `PlatformContractVersion` are the only fields that may be absent; `ParticipantConstraints` is required (`Max` alone may be absent, meaning unlimited).

## Participant Constraints

**Added 2026-09-29, by explicit human decision, discovered missing during `WORK-0038`'s implementation** (see that WORK's own Completion Record and this document's Rationale And History section below). `ParticipantConstraints{Min, Max}` is structural Session admission/capacity metadata, not a game rule: Playhoot itself (`Join`/`Start`) must be able to enforce it *before* any authored JavaScript ever runs, so a lobby's supported participant range is known independently of executing untrusted game code. It travels alongside `GameContract` rather than inside it, because `GameContract`'s own schema is owned by the platform command-protocol WORK and is opaque to this record, while `ParticipantConstraints` is this record's own required field, always present, the same way `BackendScript`/`FrontendScript` are.

The responsibility split this restates:
- **Artifact/platform metadata (`ParticipantConstraints`)**: the structural supported participant range - reject a Join beyond `Max`, reject a Start below `Min`, know lobby capacity without executing the authored script.
- **Authored JavaScript (`GameContract`/backend script logic)**: any additional game-specific restriction or behavior involving participants, layered inside the platform's structural bounds, never replacing them.

## Backend Script

Authored JavaScript, executed by the separately deployed JavaScript Executor (`session/jsexecutor`) per `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`. Transfer is confirmed **inline**, not reference-based: Session Runtime holds the durable script content and sends it as part of every `Execute` request (`session/jsexecutor/proto/executor.proto`'s `ExecuteRequest.script`), rather than handing the Executor a path or identifier to resolve itself. A typical authored rule script is realistically well under gRPC's default 4 MiB message ceiling; if a future script legitimately exceeds a reasonable bound, that requires its own separately justified decision, not an assumption made here.

## Frontend Script

Authored frontend source. Its own language/format (React, plain JavaScript, or something else) is not decided by this record - a future frontend-specific design WORK (`docs/projects/active/js-runtime-migration/works/WORK-0045-frontend-iframe-delivery-contract-specification.md`) owns that, along with the actual backend/frontend communication protocol and asset-request mechanism.

What is decided: Playhoot stores this script's own content directly, the same way it stores `BackendScript` - there is no separate build/publish pipeline producing a package for Playhoot to merely reference. Playhoot's own backend never parses or executes this content; it is opaque payload, handed off to whatever actually renders the game (a future frontend client, loaded in an isolated iframe per `ADR-0015`) exactly as `BackendScript` is opaque payload to Session Runtime before the Executor evaluates it.

## Game Contract

Opaque data from this record's own perspective - a JSON value (or similar), whose exact schema is owned by the platform command-protocol WORK, not this one. This record only establishes that a game's own declared event/data vocabulary travels as part of the same artifact, pinned immutably alongside the two scripts it governs, so the backend and frontend scripts share one fixed understanding of what an event means for their entire pinned lifetime.

## Assets

Optional. Each asset is identified by an internal logical `Key` (its name, or whatever the platform assigns) and a `Kind` (image, sound, etc.) - **never a URL**, and never resolvable to one by the frontend script itself. Only assets uploaded directly to Playhoot are supported; an external link is not a valid asset reference at all, regardless of format.

This is a deliberate security decision, not a placeholder: a frontend script is injected/untrusted content, and an arbitrary external URL it happens to contain cannot be trusted the way platform-hosted, platform-validated content can (unknown content type, unverified origin, potential tracking or attack surface). How a key is actually resolved into real bytes/location at serving time is a separate WORK's concern (`docs/projects/active/js-runtime-migration/works/WORK-0046-frontend-package-serving-and-versioned-asset-delivery.md`); this record only fixes that the artifact stores keys, never links.

## Immutability

Identical to `game/docs/decisions/GAME-ADR-0001-game-capability-persistence-transaction-boundary.md`'s existing invariant: once a Session pins `DefinitionUUID`, every field of that artifact is fixed for the Session's whole lifetime, including `FrontendScript` and `Assets` - a later-published update to the same Game does not retroactively change what an in-progress Session serves.

## Ownership Split (Cross-Domain)

- No cross-domain database foreign key exists to Game Management's own tables (`ARCHITECTURE.md -> Cross-Domain Public Entity References`); `DefinitionUUID` is a logical reference only.
- Session Runtime owns its own persisted copy of this artifact, keyed by `DefinitionUUID` (`docs/projects/active/js-runtime-migration/works/WORK-0034-session-owned-executable-script-artifact-and-package-restructuring.md`).
- Game Management's own publish-time write, and keeping both domains' representations consistent, is `docs/work/active/WORK-0033-cross-domain-game-publish-composition.md`'s concern (standalone, not part of any Project owning this document).
- No domain imports another domain's concrete artifact type merely to declare a narrow interface against it - each consumer declares its own type against this document.

## Rationale And History

Recorded in `docs/projects/active/js-runtime-migration/works/WORK-0044-game-version-artifact-model.md`, including why the frontend was initially (incorrectly) modeled as an optional reference to a separately-built package before being corrected to a mandatory, directly-stored script.

`ParticipantConstraints` was added afterward, discovered missing during `WORK-0038`'s own implementation: `WORK-0038` found that `Join`/`Start`'s existing lobby-capacity gate (`definition.Players.Min`/`Max`) had no replacement once `program.Definition` was retired, and this record had no field for it. Resolved by explicit human decision (2026-09-29) to keep platform-level enforcement rather than delegate it to authored JavaScript - recorded here as an amendment to this WORK's own already-DONE Completion Record, not a rewrite of its Approved Design, per this repository's convention for amending closed WORK (see `WORK-0044`'s own file for the equivalent terminology-fix precedent).
