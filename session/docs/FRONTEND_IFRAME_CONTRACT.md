# Frontend Iframe Delivery Contract (Accepted Design)

Status: ACCEPTED DESIGN, NOT YET IMPLEMENTED. Records `docs/projects/active/js-runtime-migration/works/WORK-0045-frontend-iframe-delivery-contract-specification.md`'s accepted shape - the concrete specification of the contract `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md` establishes at the architecture-principle level. This document is a canonical contract other WORK/repositories build against. It does not itself define a shared Go/JS/TypeScript type, and it does not authorize building the frontend application, its SDK, or its repository (see `ADR-0015`'s own Explicitly Out Of Scope and `docs/projects/active/js-runtime-migration/PROJECT.md`).

## What This Document Is

Playhoot's own (separately built, not-yet-existing) frontend loads a Session's pinned frontend package inside a sandboxed iframe. This document specifies the one channel through which that iframe communicates with anything outside it: a small, fixed, host-provided client library, `playhoot`. The generated frontend script never implements or exposes these calls itself - it only registers listeners on, or calls methods of, the `playhoot` object the host hands it at bootstrap.

## Trust Boundary And Network Isolation

The iframe is isolated in context and origin from Playhoot's main application. It receives no embedded credentials, no authentication token, and no direct network handle of any kind.

**Generated frontend code has no direct network capability of any kind** - no `fetch`, no `WebSocket`, no externally-loadable image/resource reference, no top-level navigation. This is a **binding requirement the host/iframe security configuration must enforce** (for example via `sandbox` attributes, Content-Security-Policy, or an equivalent mechanism) - not a convention merely documented for authored/generated scripts to follow. A generated script is assumed adversarial or buggy regardless of its author, including Playhoot's own AI authoring tooling, exactly as `ADR-0015` already establishes for backend script sandboxing; the same posture applies to the frontend.

The four `playhoot.*` calls below are the *only* channel between the generated script and anything outside the iframe. All authenticated transport is owned by Playhoot's own (separately built) frontend, which relays between the iframe and the backend; the iframe itself never calls Session Runtime, any backend script, or any authenticated endpoint directly.

The exact enforcement mechanism (which `sandbox` attributes, which CSP directives, or another equivalent) is left to the frontend implementation. What is fixed here is the requirement itself: network denial, not its implementation.

The iframe may hold local visual/interaction state and run animations; authoritative game state and rules remain backend-only (`ADR-0015`).

## Client Library Surface

One host-provided object, `playhoot`, is the entire API. These four names and shapes are **locked as this contract's stable v1 surface**. An AI authoring tool learns and generates frontend code against these exact names, the same way it learns and generates backend code against the platform's fixed command vocabulary (`session/workflows/sessionlifecycle/internal/platform/LOGICAL_CONTRACT.md`) - name stability here is not an incidental convenience, it is load-bearing. If these four names/shapes ever need an incompatible change, that is a new, explicitly versioned SDK generation ("Playhoot SDK v2") a game version pins against, not a silent rename of v1.

```text
playhoot.onState(listener)
playhoot.onEvent(listener)
await playhoot.send(name, payload) -> { outcome: "accepted" | "rejected" | "failed" }
await playhoot.requestAsset(key) -> AssetHandle
```

### `playhoot.onState(listener)`

Registers `listener`, called with `{ state, revision }` whenever a fresh view is available: on initial iframe load, and again whenever a live-connection layer determines the viewer's state may have changed (including after reconnect). This document does not decide *when* the host calls it - only that this is the one call carrying view data, and its own two fields:

- `state` - the authored backend script's own `project()` output (`session.GetClientStateResult.ClientState`), opaque JSON to this contract. The frontend script alone decides what any field in it means for the interface; the backend has no opinion about presentation at all (`ADR-0015`).
- `revision` - the authoritative sequence number of the RuntimeTurn `state` reflects. Backed by the already-durable `RuntimeTurn.Sequence`; not yet exposed on `session.GetClientStateResult` today (`docs/projects/active/js-runtime-migration/works/WORK-0055-client-state-revision-and-outbound-event-correlation-identifiers.md`, PLANNED). The frontend must treat `onState` as the sole source of authoritative truth, and may safely discard a delivery whose `revision` is not newer than the last one it already applied.

Unlike `onEvent`, a specific `onState` push attempt is not itself declared best-effort by this contract - but whether any given attempt actually arrives is a property of the live-transport layer delivering it (`session-runtime-v1`'s `WORK-0020`, this Project's `WORK-0046`), not something this document fixes. What this contract does fix is the recovery path: `onState` is callable again at any time (on reconnect, or whenever the transport layer next has a fresh view to push), so a client that missed a prior push is never permanently stale - it only needs to wait for, or trigger, the next delivery.

### `playhoot.onEvent(listener)`

Registers `listener`, called with `{ id, name, payload, revision }` for each cosmetic/transient occurrence a `SEND_EVENT` command produced.

- `name`/`payload` - `session.OutboundEvent.Name`/`Payload`, entirely game-defined and opaque to Playhoot and to this contract (for example, "you were dealt this card," a sound cue, a notification). The backend never sends a presentation/rendering instruction - only logical events carrying data; the frontend script alone decides what any event means for the interface.
- `id` - a stable per-event correlation identifier, letting the frontend recognize an already-applied duplicate.
- `revision` - the RuntimeTurn.Sequence that produced this event.

Neither `id` nor `revision` exists on `session.OutboundEvent` today (`WORK-0055`, PLANNED) but both are required by this contract: delivery here is best-effort per `session/docs/decisions/SESSION-ADR-0019-session-runtime-post-commit-client-delivery-semantics.md` and `docs/projects/active/js-runtime-migration/works/WORK-0042-effects-delivery-best-effort.md` - an event may be duplicated, delayed, or missed regardless of `id`/`revision`, and the frontend must never treat receiving, or not receiving, one as authoritative game state. Without a stable identifier, a frontend has no way to recognize an already-applied duplicate; without a revision, no way to know whether an event still applies to the view currently on screen.

### `await playhoot.send(name, payload)`

Submits a `PLAYER_EVENT` (`name`/`payload` are entirely game-defined, mirroring `platform.PlayerEvent`). The returned promise **never resolves on mere transport acknowledgement** - it resolves only once the confirmed outcome is known, as `{ outcome: "accepted" | "rejected" | "failed" }`, mapped directly from `session.SubmitPlayerEventOutcome`:

| `playhoot.send` outcome | `SubmitPlayerEventOutcome` |
|---|---|
| `"accepted"` | `Accepted` |
| `"rejected"` | `Rejected` |
| `"failed"` | `RuntimeExecutionFailed` |

This satisfies `ADR-0015`'s requirement that a client distinguish a transport acknowledgement from actual acceptance/rejection of its action. The concrete mechanism that durably delivers this resolution back to a disconnected/reconnected client is `docs/projects/active/js-runtime-migration/works/WORK-0043-durable-confirmed-turn-result-delivery-outbox.md`'s own scope (PLANNED, not yet designed) - this contract only fixes the three-way distinction a client library must expose, not how it is durably delivered.

### `await playhoot.requestAsset(key)`

Resolves a declared `Assets[].Key` (`session/docs/GAME_VERSION_ARTIFACT_MODEL.md`) into a safe, Playhoot-provided `AssetHandle` (for example a `Blob`, an `ArrayBuffer`, or a local object URL) that the generated script consumes directly. It **never** returns a URL the game then fetches itself, and the generated script's own source must never contain a raw URL in the first place - this is a deliberate security decision (`GAME_VERSION_ARTIFACT_MODEL.md`'s own "Assets" section), not a placeholder. `AssetHandle`'s exact concrete shape and the resolution mechanism behind it are `docs/projects/active/js-runtime-migration/works/WORK-0046-frontend-package-serving-and-versioned-asset-delivery.md`'s own scope (PLANNED, not yet designed).

## Frontend Script Loading

A Session's pinned `FrontendScript` (`GAME_VERSION_ARTIFACT_MODEL.md`) is immutable for the Session's whole lifetime and is loaded once, at iframe/bootstrap creation - never retransmitted on every state update. A browser refresh or a freshly recreated iframe naturally reloads it again; this invariant is about retransmission cadence during a session's live lifetime, not a literal one-time-ever guarantee across arbitrary client-side reloads. The actual serving mechanism (endpoint/transport) is `WORK-0046`'s own scope.

## What This Document Defers

Named as dependencies, not filled in with placeholder behavior:

- **Transport wire format** - the actual over-the-wire message shapes, HTTP endpoints, and connection lifecycle between Playhoot's own frontend and Session Runtime. Owned by `session-runtime-v1`'s `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` and this Project's own `WORK-0046`.
- **Asset resolution mechanism and `AssetHandle`'s exact shape** - `WORK-0046`.
- **Durable delivery of a confirmed `PLAYER_EVENT`'s own outcome** - `WORK-0043`.
- **`revision`/`id` backing on `GetClientStateResult`/`OutboundEvent`** - `WORK-0055`.
- **Disconnect/reconnect rendering behavior and the full resync flow** - coordinated with, not owned by, `session-runtime-v1`'s `docs/projects/active/session-runtime-v1/works/WORK-0015-disconnect-reconnect-full-resync.md`. This contract's only commitment: `playhoot.onState` is callable again on reconnect to re-sync the view, without this document inventing the reconnect trigger/mechanism itself.
- **A frontend-internal "current screen state" mirror object** - an internal implementation detail of the not-yet-built frontend SDK, left for whoever eventually builds it to decide.
- **Building the frontend application, its SDK, or its repository** - out of this Project's scope entirely (`ADR-0015`, `docs/projects/active/js-runtime-migration/PROJECT.md`).

## Rationale And History

Recorded in `docs/projects/active/js-runtime-migration/works/WORK-0045-frontend-iframe-delivery-contract-specification.md`, including the human review that corrected this contract's first proposed shape on five points (listener-registration surface rather than exposed functions; `revision`/`id` metadata on `onState`/`onEvent`; locked rather than renameable names; network isolation as a binding requirement rather than documented convention; a safe asset handle rather than "content/location"). The `revision`/`id` correction directly produced `WORK-0055`, a small additive follow-up to the already-DONE `WORK-0041`/`WORK-0042`.
