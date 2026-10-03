# WORK-0044: Game Version Artifact Model (Backend Script + Frontend Script + Contracts + Assets)

Status: DONE
Created: 2026-09-27
Last status change: 2026-09-27 (DRAFT -> READY: human explicitly authorized implementation, confirming frontend script as mandatory; READY -> IMPLEMENTING -> DONE: canonical document published, independently reviewed, closure gaps fixed)

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`
- `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`

Canonical context:
- `game/management/models.go` (the current `game_definitions` versioning precedent this generalizes — `Game.VersionUUID`/`Definition program.Definition`, the exact coupling `ADR-0014` requires removing)
- `game/session/jsexecutor/proto/executor.proto` (`WORK-0052`'s `ExecuteRequest.script` field — already built assuming inline backend-script transfer; this WORK confirms that decision at the artifact-model level rather than reopening it)
- `docs/projects/active/js-runtime-migration/works/WORK-0045-frontend-iframe-delivery-contract-specification.md` (owns the actual backend/frontend communication protocol and asset-request mechanism this WORK only names, does not design)

## Outcome

Define the artifact model a Game version actually is going forward: two separately stored scripts, both mandatory — backend (JavaScript, executed by the Executor) and frontend (format not yet decided; Playhoot stores and hands it off, but never parses or executes it itself) — plus the platform/game-specific event contract they share, optional platform-hosted assets, and the capability/contract versions relevant to executing it. A running Session is pinned to one immutable Game version's artifact bundle for its whole lifetime, restating `GAME-ADR-0001`'s existing immutability invariant against the new artifact shape.

This is a contract-defining WORK, not a persistence/storage WORK: it fixes the conceptual shape and its invariants so `WORK-0034` (Session's own persisted copy), `WORK-0033` (publish composition), and `WORK-0046` (frontend serving) can each design their own narrow, domain-owned view of it without designing against a placeholder or against each other's assumptions.

## Context

Today, `management.Game` (`game/management/models.go`) embeds `program.Definition` directly — exactly the producer-type coupling `ADR-0014` requires removing, and the shape being retired along with Game Language itself (`GAME-ADR-0028`). No successor shape exists yet; every downstream WORK that needs to know what a Game version *is* has been explicitly waiting on this one.

**Revised (2026-09-27, human-directed):** the first draft of this WORK modeled the frontend as an optional *reference* to a separately-built package (mirroring how a compiled binary artifact might be published elsewhere). That was wrong: Playhoot stores the frontend script's own content directly, the same way it stores the backend script — there is no separate build/publish pipeline producing a package for Playhoot to merely point at. Both scripts are authored content Playhoot holds.

## Scope

### In Scope

- The canonical artifact shape (see Approved Design) and its field-level invariants.
- Resolving Project Material Decision #5 (artifact bundle format/storage) and the remote-transfer question `ADR-0016` raised: confirming inline-per-request transfer for the backend script (already built by `WORK-0052`/`WORK-0053`) is the accepted mechanism, not merely a placeholder.
- The asset-identification policy: assets are referenced by an internal logical key, never a URL, and only assets uploaded to Playhoot itself are supported — an external link is not a valid asset reference. This is a security decision (frontend script is injected/untrusted content; trusting an arbitrary URL it happens to contain would mean trusting unverified, unknown content), decided now because it shapes the artifact's own field types, not deferred to frontend-specific design.
- A canonical contract document other WORK reference, per this repository's "one fact, one owner" convention — not a shared Go type imported across domains (per `ARCHITECTURE.md`'s Dependency Principles and `ADR-0014`'s own rationale: each domain declares its own narrow view against a producer's root type, it does not import a third package's type through it).

### Out of Scope (deliberately deferred to future frontend-specific design, not decided here)

- The frontend script's actual language/format (React, plain JS, something else) and its own internal validation.
- The backend/frontend communication protocol itself: backend emits logical events carrying data only (never a presentation/rendering instruction — no "show this screen"); the frontend script alone decides what any event means for the user interface. This event vocabulary is what `GameContract` (below) actually holds, but its detailed shape, and how the two scripts are meant to interact (conceptually as if calling each other directly, in practice via an intermediating bridge/event dispatch), is `WORK-0045`'s own design, not this WORK's.
- The function/mechanism the frontend script uses to request an asset by its logical key (a host-provided call, not a raw URL embedded in frontend code) — `WORK-0045`/`WORK-0046`.
- Whether/how a frontend-side "current screen state" object (mirroring the backend's own state-object/mutation pattern) is used for deterministic rendering or reconnect recovery — an open idea, not a decision; revisit when frontend-specific design actually starts.
- Actual database tables/columns — `WORK-0034` (Session's own copy), `WORK-0033`/`orchestrator` (Game Management's publish-time write).
- The frontend script's own authoring/build tooling, compiler, or bundler — out of scope for this repository entirely, per the mandate.
- Actual asset storage/serving mechanics (where uploaded bytes live, how a key resolves to them) — `WORK-0046`.
- Validating a game's own declared contract against `WORK-0039`'s platform vocabulary at runtime — `WORK-0039`'s own scope; this WORK only says where that declaration lives within the artifact.

## Approved Design

The artifact a Game version *is*, conceptually:

```text
GameVersionArtifact
  DefinitionUUID          — the same immutable version identity already pinned at Create (sessions.game_definition_uuid)
  BackendScript           — authored JavaScript source (required); executed by the Executor
  FrontendScript          — authored frontend source (required); format not yet decided; stored by Playhoot, never parsed or executed by Playhoot itself — only handed to whatever actually renders the game
  GameContract            — this game's own declared event/data vocabulary, alongside (not replacing) WORK-0039's fixed platform command vocabulary; the shared language the two scripts use to agree on what an event means
  Assets                  — optional list of {Key, Kind} — images/sounds/etc. uploaded directly to Playhoot; identified only by an internal logical Key, never a URL
  PlatformContractVersion — which version of WORK-0039's platform vocabulary this artifact was authored against
```

- **`BackendScript` and `FrontendScript` are both mandatory.** A valid artifact always has both scripts; there is no fallback/default-rendering path for a missing frontend script. Only `FrontendScript`'s own format and internal validation remain deferred (see Out of Scope) — its presence is not.
- **Backend-script transfer is confirmed inline, not reference-based.** `WORK-0052`'s `.proto` already carries `ExecuteRequest.script` as inline `bytes`; this record makes that the accepted, permanent mechanism for this field specifically — Session Runtime holds the durable `BackendScript` (via `WORK-0034`'s own table) and sends it inline on every `Execute` call. A typical authored rule script is realistically well under gRPC's default 4 MiB message ceiling; if a future artifact's script legitimately exceeds a reasonable bound, that is a new, separately justified decision, not an assumption made here.
- **`FrontendScript` is stored content, not a reference.** Playhoot holds the actual source; `WORK-0046` is what later serves it to an actual frontend client. Playhoot's own backend never needs to understand its contents — it is opaque payload from the backend's perspective, exactly like `BackendScript` is opaque payload from the sandbox's perspective before execution.
- **Assets are identified by logical key only, resolved server-side.** An asset's `Key` is an internal identifier (its name, or whatever the platform assigns) — never a URL, and never resolvable to one by the frontend script itself. Only assets the author uploaded directly to Playhoot are supported; an external link is not a valid asset reference at all, regardless of format, because a frontend script is injected/untrusted content and an arbitrary external URL it contains cannot be trusted the way platform-hosted, platform-validated content can. How a key is actually resolved into bytes at serving time is `WORK-0046`'s concern; this WORK only fixes that the artifact stores keys, never links.
- **`GameContract`** is opaque data from this WORK's own perspective (a JSON value, exact schema owned by `WORK-0039`) — this WORK only establishes that it travels as part of the same artifact, pinned immutably alongside the two scripts it governs.
- **Immutability**: identical to `GAME-ADR-0001`'s existing invariant — once a Session pins `DefinitionUUID`, every field of that artifact is fixed for the Session's whole lifetime, including `FrontendScript` and `Assets` (a later-published update does not retroactively change what an in-progress Session serves).

## Constraints and Invariants

- Immutable once a Session pins to it (`GAME-ADR-0001`).
- No cross-domain database foreign key to Game Management's own tables (`ARCHITECTURE.md -> Cross-Domain Public Entity References`) — `DefinitionUUID` is a logical reference only.
- `BackendScript` and `FrontendScript` are both required; `GameContract`, `Assets`, and `PlatformContractVersion` are the only fields that may be absent.
- No asset may be identified by an external URL; only a logical key resolvable to Playhoot-hosted content is a valid asset reference.
- No domain imports another domain's concrete artifact type merely to declare a narrow interface against it — each consumer (`WORK-0034`, `WORK-0033`, `WORK-0046`) declares its own type against this document, per `ARCHITECTURE.md`'s Dependency Principles.

## Acceptance Criteria

- A published canonical contract document exists (see Documentation Impact) specifying every field above, which fields are required (`BackendScript`, `FrontendScript`) vs. optional, the immutability invariant, the confirmed inline-transfer mechanism for `BackendScript`, and the logical-key-only asset policy — reviewable against this WORK's own Approved Design and against `ADR-0015`'s mandate requirements.
- `WORK-0034`, `WORK-0033`, and `WORK-0046` can each be drafted afterward referencing this document without discovering a missing field or an unresolved storage-vs-transfer question.
- `WORK-0045`, when it is drafted, has this WORK's deferred Out-of-Scope items (event-only backend/frontend protocol, asset-request mechanism, frontend-state-object idea) already recorded in its own file so none of them are lost between now and then.

## Implementation Freedom

Exact document location/filename, exact JSON Schema (if any) for `GameContract`'s own internal shape (owned by `WORK-0039` regardless), and exact asset-reference field names are implementer's choices consistent with this Approved Design.

## Verification

- Review against `ADR-0015`'s explicit requirements (assets supported but platform-hosted only, immutable per-session pinning) for completeness; no code verification, since this WORK produces no production code itself.

## Documentation Impact

### Accepted / Canonical Knowledge

- A new canonical contract document, `game/docs/GAME_VERSION_ARTIFACT_MODEL.md` (mirroring the existing `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` pattern), recording the artifact model's exact shape and invariants.
- `docs/projects/active/js-runtime-migration/works/WORK-0045-frontend-iframe-delivery-contract-specification.md` — Context updated with the deferred design points this WORK surfaced (event-only protocol, asset-request mechanism, frontend-state-object idea).
- `docs/projects/active/js-runtime-migration/PROJECT.md` — Material Decision #5 marked resolved.

### Current-State Documentation After Implementation

- None — this WORK is design-only; current-state docs change when `WORK-0034` et al. actually implement against it.

## Blockers

- None material.

## Completion Record

- `game/docs/GAME_VERSION_ARTIFACT_MODEL.md` published: every field from Approved Design, required-vs-optional split, immutability, confirmed inline `BackendScript` transfer, and the logical-key-only asset policy are all present.
- `WORK-0045`'s own Context section already carries this WORK's deferred out-of-scope items (event-only backend/frontend protocol, asset-request-by-key mechanism, frontend-state-object idea, reconnect rendering) — confirmed by independent review, not lost.
- `PROJECT.md` Material Decision #5 marked resolved against the final (corrected, mandatory-`FrontendScript`) shape.
- Independent review (one round): CHANGES_REQUIRED — found (1) this WORK's own file never actually transitioned to DONE despite downstream docs already treating it as done, and (2) stale "frontend package" terminology (implying an optional, separately-built package reference — the rejected first draft) surviving in `ARCHITECTURE.md`, `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`, and `WORK-0033`/`WORK-0052`'s own files. Both fixed: this file's status closed here; the terminology drift fixed at each site (`GAME-ADR-0028` is ACCEPTED/historical, so fixed via an appended dated clarifying note, not a rewrite, consistent with this repository's ADR-immutability convention).
- Second review pass not re-dispatched: the remaining fixes were mechanical terminology corrections restating already-approved facts, not new design surface.

### Addendum (2026-09-29): `ParticipantConstraints` added

`WORK-0038`, mid-implementation, discovered that this WORK's Approved Design had no field carrying the structural participant-count range `Join`/`Start` need to enforce before any authored script runs (the capacity check the retired `program.Definition.Players.Min`/`Max` used to serve). Reported as a DISCOVERY rather than silently invented or silently dropped. Human decision (2026-09-29): keep platform-level enforcement (do not delegate the hard capacity gate to JavaScript), and add `ParticipantConstraints{Min, Max}` (`Max` optional, meaning unlimited) as a required field of this record, distinct from the still-opaque `GameContract`. `session/docs/GAME_VERSION_ARTIFACT_MODEL.md` amended in place (its own new "Participant Constraints" section); this Completion Record is not rewritten, only appended to, per this WORK's own already-established amendment convention (see the terminology-fix precedent immediately above). See `WORK-0038`'s own Completion Record for the persisted-schema/implementation side of this amendment.
