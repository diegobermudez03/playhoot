# WORK-0045: Frontend Iframe Delivery Contract Specification

Status: DRAFT
Created: 2026-09-27
Last status change: 2026-09-29 (human reviewed the first proposed shape and corrected it on five points, all incorporated below - see "Human Decision: API Shape Corrected"; not yet READY, awaiting final confirmation of the revised shape)

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- `docs/projects/active/js-runtime-migration/works/WORK-0044-game-version-artifact-model.md` (owns the fact that a frontend script is stored content, not a package reference; this WORK owns how that script actually talks to the backend once loaded)

## Outcome

Produce the accepted, canonical specification of the frontend delivery contract `ADR-0015` establishes at the architecture-principle level: the trust boundary (iframe isolated in context/origin from the main application, no embedded credentials, restricted permissions), the small client-library API surface (conceptually: receive current view, receive effects, send an interaction — exact method names/shapes decided here), frontend-script loading (one version's `FrontendScript`, per `WORK-0044`, loaded per Session, not re-sent per update), the asset-request mechanism (logical key in, resolved content out — never a raw URL in the script), and the send/receive/apply/reject/fail distinction the mandate requires (a transport acknowledgement is never treated as an accepted play). This is a documentation deliverable per `ADR-0015`'s explicit scope boundary — implementing the frontend, its SDK, or its repository is out of this Project's scope — but the specification itself is approved design work, going through the same DRAFT/READY process as any other WORK, not freelanced outside it.

## Context

Not yet designed. Depends on `WORK-0039`'s command/view vocabulary existing in at least draft form, since the contract must be concrete about what a view/effect/interaction actually contains.

**Vocabulary update (2026-09-28):** `WORK-0039`'s revised vocabulary independently confirms the design points already surfaced below — the platform-agnostic-presentation stance is now the accepted backend protocol, not merely an anticipated concern. This WORK's own client-library surface should map directly onto it: `onState(...)` delivers `project`'s `ClientState`, `onEvent(...)` delivers a `SEND_EVENT`, and `send(name, payload)` becomes a `PLAYER_EVENT`. The exact method names/shapes remain this WORK's own design task, not fixed here.

**Design points surfaced while drafting `WORK-0044` (2026-09-27), recorded here so they are not lost before this WORK is actually drafted — none of these are decided yet:**

- The backend must never send a presentation/rendering instruction (no "show this screen," no UI-element description) — only logical events carrying data (for example, "you were dealt this card," "you lost"). The frontend script alone decides what any event means for the interface; the backend has no opinion about presentation at all. This is closer to an API boundary (backend exposes behavior/data, never presentation) than the old engine's model, where the backend's own declarative language dictated the UI tree.
- Conceptually, the backend and frontend scripts behave as if they call each other directly (the frontend "calls" a backend action; the backend "calls back" with an event), but the actual mechanism is asynchronous: a frontend interaction becomes a real request to Session Runtime, which drives the Executor, and the result comes back as an event later, not a direct return value.
- Asset requests: the frontend script must never contain a raw URL. It requests an asset by its logical key through a host-provided call (exact shape not decided), and receives the actual content/location back through that call — never by having a URL baked into its own source.
- Open idea, not a decision: a frontend-side "current screen state" object, mirroring the backend's own state-object/mutation pattern, that could make rendering (and recovering a screen after a disconnect/reconnect) deterministic. Worth real consideration when this WORK is actually drafted, but not committed to now.
- Disconnect/reconnect rendering behavior generally (what a returning player's screen should show) is a real requirement this WORK must eventually address, not something to invent a full mechanism for prematurely.

## Scope

**Ground-truth check:** every backend-side capability this contract needs to reference already exists and is DONE, so this WORK grounds the spec in real accepted vocabulary rather than inventing placeholder names: `Manager.GetClientState` (view), `Manager.SubmitPlayerEvent` (`PLAYER_EVENT`, with `SubmitPlayerEventOutcome` `Accepted`/`Rejected`/`RuntimeExecutionFailed` already distinguishing outcomes at the Go API level — `WORK-0038`/`WORK-0039`), `SEND_EVENT`/`session.OutboundEvent` (cosmetic effects, in memory only so far — `WORK-0042`), and `GameVersionArtifact.FrontendScript`/`Assets` (`WORK-0044`).

### In Scope

- The iframe trust boundary: isolated origin/context, no embedded credentials, restricted permissions (sandbox attributes) — specified as requirements the eventual frontend/iframe host must satisfy, not implementation of the host itself (that remains the not-yet-built frontend repository).
- The client-library API surface: a small, fixed set of calls the iframe uses to receive a view, receive cosmetic effects, and send an interaction (see Approved Design for the concrete proposal).
- Frontend script loading: one Session's pinned `FrontendScript` is loaded once per Session, not re-sent per update — consistent with `WORK-0044`'s immutability model. The actual serving mechanism (endpoint/transport) is `WORK-0046`'s own scope; this WORK specifies only what the iframe bootstrap receives (script content plus the client-library object) and when reloading is expected to happen (once, at session load — not on every state update).
- The asset-request mechanism's *shape* (a call taking a declared logical `Key`, returning resolved content/location) — never a URL in the script's own source, per `WORK-0044`'s already-accepted decision. The actual resolution implementation is `WORK-0046`'s scope.
- The send/receive/apply/reject/fail distinction `ADR-0015` requires, mapped directly onto the already-accepted `SubmitPlayerEventOutcome` values, so a client library built against this spec is forward-compatible with whatever concrete durable-delivery mechanism `WORK-0043` designs, without needing to know its details yet.
- A revision/sequence number on the delivered view, and a stable correlation identifier plus its own revision on each delivered transient event - both required by this contract (see Human Decision point 2) so the frontend can safely discard a stale/duplicate delivery, given `onEvent`'s own best-effort nature. Neither field exists on the current backend result types yet (`session.GetClientStateResult`/`session.OutboundEvent`) - this is a genuine discovery, not assumed away; see `WORK-0055` (new, PLANNED) below.

### Out of Scope

- The actual over-the-wire transport (WebSocket message shapes, HTTP endpoints, connection lifecycle) — owned by `session-runtime-v1`'s `WORK-0020` and this Project's own `WORK-0046`, not this WORK.
- The asset storage/serving implementation itself — `WORK-0046`.
- The durable outbox mechanism for a confirmed `PLAYER_EVENT`'s own outcome — `WORK-0043`. This WORK only requires that the contract expose the three-way distinction; it does not design how "accepted" is durably delivered.
- Disconnect/reconnect rendering behavior and the full resync flow — flagged for coordination with `session-runtime-v1`'s `WORK-0015` (Disconnect/Reconnect/Full Resync), not invented here. This WORK's only commitment: `onState` (see Approved Design) is callable again on reconnect to re-sync the view from `GetClientState`, without this WORK inventing the reconnect trigger/mechanism itself.
- A frontend-internal "current screen state" mirror object (the open idea recorded in Context) — an internal implementation detail of the not-yet-built frontend SDK, not part of this backend-facing contract. Left for whoever eventually builds the frontend to decide, not decided here.
- Building the frontend application, its SDK, or its repository (already out of this Project's own scope per `ADR-0015`/`PROJECT.md`).

## Approved Design

**Human-confirmed shape (2026-09-29) — see "Human Decision: API Shape Corrected" below for the full reasoning behind each correction.** One host-provided object, `playhoot`, is the sole channel between the generated frontend script and everything outside the iframe — the generated script never implements or exposes these itself; it only registers listeners on / calls methods of the object the host hands it at bootstrap. These four names and shapes are **locked as this contract's stable v1 surface**, not merely illustrative:

- `playhoot.onState(listener)` — registers `listener`, called with `{ state, revision }` whenever a fresh view is available: on initial iframe load, and again whenever a live-connection layer determines the viewer's state may have changed (including after reconnect). `state` is the authored backend script's own `project()` output (`session.GetClientStateResult.ClientState`, opaque JSON). `revision` is the authoritative sequence number of the RuntimeTurn the view reflects (backed by the already-durable `RuntimeTurn.Sequence` - not yet exposed on `GetClientStateResult`, see `WORK-0055`), letting the frontend safely discard a delivery no newer than the last one it already applied.
- `playhoot.onEvent(listener)` — registers `listener`, called with `{ id, name, payload, revision }` for each cosmetic/transient occurrence a `SEND_EVENT` command produced. `name`/`payload` are `session.OutboundEvent.Name`/`Payload`, opaque to Playhoot and to this contract. `id` is a stable per-event correlation identifier and `revision` is the RuntimeTurn.Sequence that produced it - neither exists on `session.OutboundEvent` today (see `WORK-0055`) but both are required by this contract: best-effort delivery (`SESSION-ADR-0019`/`WORK-0042`) may repeat, delay, or drop this call, and without a stable identifier the frontend has no way to recognize an already-applied duplicate. The frontend must never treat receiving, or not receiving, an event as authoritative game state - `onState` alone is authoritative.
- `await playhoot.send(name, payload)` — submits a `PLAYER_EVENT`, resolving to `{ outcome: "accepted" | "rejected" | "failed" }` only once the confirmed outcome is known - never on mere transport acknowledgement - mapped directly from `SubmitPlayerEventOutcome` (`accepted` = `Accepted`, `rejected` = `Rejected`, `failed` = `RuntimeExecutionFailed`). The exact mechanism producing that resolution durably is `WORK-0043`'s own concern, not decided here.
- `await playhoot.requestAsset(key)` — resolves a declared `Assets[].Key` (`WORK-0044`) into a safe, Playhoot-provided handle (for example a `Blob`/`ArrayBuffer`/local object URL) that the generated script can consume directly - **never a URL it then fetches itself**, and never any form of externally-resolvable network reference. `WORK-0046`'s own design decides the handle's exact concrete shape; this contract only fixes that no raw fetchable URL ever crosses this boundary in either direction.

Naming stability: if these four names/shapes ever need an incompatible change, that is a new, explicitly versioned SDK generation ("Playhoot SDK v2") a game version pins against, not a silent rename of v1 - generated frontend code depends on this contract's own stability the same way it depends on the platform command vocabulary's stability, since an AI authoring tool learns and generates against these exact names.

Trust boundary / network isolation (restating and strengthening `ADR-0015`): generated frontend code has **no direct network capability of any kind** - no `fetch`, no `WebSocket`, no externally-loadable image/resource reference, no navigation - and this is a **binding requirement the host/iframe security configuration must enforce**, not merely a convention documented for authored scripts to follow. The four `playhoot.*` calls above are the *only* channel between the generated script and anything outside the iframe; no credential, token, or direct network handle is ever exposed to it. The exact enforcement mechanism (`sandbox` attributes, CSP, or another equivalent) is left to the frontend implementation, but network denial itself is fixed here, not deferred.

Frontend script loading: the pinned `FrontendScript` is immutable for the Session's whole lifetime and is loaded once, at iframe/bootstrap creation - never retransmitted on every state update. (A browser refresh or a freshly recreated iframe naturally reloads it again; "once per Session" describes retransmission cadence during a session's live lifetime, not a literal one-time-ever guarantee across arbitrary client-side reloads.)

## Constraints and Invariants

- Must not describe a synchronous, infallible call shape — `send`/`requestAsset` are asynchronous and their outcome is distinguished from mere invocation.
- Must not grant the generated frontend script direct network/credential access under any circumstance — enforcement, not mere documentation, per the trust-boundary requirement above; all authenticated transport is owned by Playhoot's own (separately built) frontend, relayed to the iframe through the four `playhoot.*` calls only.
- Must not invent the disconnect/reconnect mechanism itself, the durable-delivery mechanism behind `send`'s own promise resolution, or the concrete backing for `revision`/`id` — each is a named dependency on other WORK (`session-runtime-v1`'s `WORK-0015`, this Project's `WORK-0043`, this Project's new `WORK-0055`), not filled in here with placeholder behavior.
- Must not require the iframe to poll for its own state; `onState`/`onEvent` are push-shaped from the host's perspective (how the host itself learns to call them is the live-transport layer's own concern, not this contract's).
- Must not treat `onState`/`onEvent`/`send`/`requestAsset` as renameable Implementation Freedom — these four names/shapes are locked; an incompatible change is a new versioned SDK generation, not a silent v1 rename.

## Acceptance Criteria

- A published specification document exists at `session/docs/FRONTEND_IFRAME_CONTRACT.md` covering: the trust boundary/network-isolation requirement (enforced, not merely documented), the locked four-call `playhoot.*` surface with each call's own data shape (including `revision`/`id` on `onState`/`onEvent`) and delivery-guarantee class (best-effort vs. confirmed), frontend script loading (immutable, loaded once at bootstrap, not retransmitted per update), the accepted/rejected/failed distinction mapped explicitly onto `SubmitPlayerEventOutcome`, and the safe-asset-handle requirement for `requestAsset`.
- The document is reviewed against `ADR-0015`'s own text point by point and does not contradict it.
- The document explicitly names what it defers (asset-handle resolution mechanism, transport wire format, durable-outcome delivery, reconnect mechanism, and `WORK-0055`'s own revision/id backing) rather than silently implying they're solved.

## Implementation Freedom

- `AssetHandle`'s exact concrete shape (left to `WORK-0046`).
- Exact document structure/section ordering within `FRONTEND_IFRAME_CONTRACT.md`.
- The exact enforcement mechanism for network isolation (sandbox attributes vs. CSP vs. another equivalent) - the requirement itself (no network capability) is fixed, its implementation is not.

## Verification

- Review against `ADR-0015`'s own text for completeness; no code verification, since this WORK produces no production code.

## Documentation Impact

### Accepted / Canonical Knowledge

- New canonical specification document: `session/docs/FRONTEND_IFRAME_CONTRACT.md` (mirroring `session/docs/GAME_VERSION_ARTIFACT_MODEL.md`'s own placement pattern — an accepted-design contract document, not implementation).
- `session/docs/GAME_VERSION_ARTIFACT_MODEL.md`'s own "Frontend Script"/"Assets" sections gain a cross-reference to this new document once it exists (they currently point at this WORK by number only).
- `WORK-0041`'s and `WORK-0042`'s own (already-DONE) files each gain a small dated addendum noting the newly-discovered `revision`/`id` requirement and pointing to `WORK-0055` - not a rewrite of either's own approved scope, per this repository's Completed Work Immutability convention.

## Human Decision: API Shape Corrected (2026-09-29)

The human reviewed this WORK's first proposed shape and corrected it on five points, all incorporated above:

1. **Listener-registration shape, not exposed functions.** The first pass described `onState`/`onEvent` as functions the host calls directly on some implicit surface; corrected to explicit `playhoot.onState(listener)`/`playhoot.onEvent(listener)` registration calls on one host-provided `playhoot` object - the generated frontend never implements or exposes these itself, it only registers against them.
2. **Revision/correlation metadata, not bare opaque payloads.** The first pass's `onEvent(name, payload)` gave the frontend no way to recognize a duplicate or know which state revision an event relates to - a real gap given `onEvent` is explicitly best-effort (may be duplicated/delayed/missed). Corrected: `onState` carries `{ state, revision }` and `onEvent` carries `{ id, name, payload, revision }`. This surfaced a genuine discovery: neither `session.GetClientStateResult` nor `session.OutboundEvent` currently carries a revision or a stable per-event id - see the new `WORK-0055` below, created immediately per this repository's own "known future outcomes get a PLANNED WORK immediately" practice rather than left as unscheduled prose.
3. **Names locked, not left as renameable Implementation Freedom.** The first pass treated `onState`/`onEvent`/`send`/`requestAsset` as illustrative, changeable by whoever eventually builds the frontend SDK. Corrected: these four names/shapes are this contract's own stable v1 surface - an AI authoring tool will learn and generate against them, so an incompatible change is a new versioned SDK generation, not a silent rename.
4. **Network isolation as a binding, enforced requirement, not documented convention.** The first pass's trust-boundary wording described `sandbox` attributes but didn't state network denial as a hard requirement - an iframe sandbox alone doesn't guarantee a script can't attempt outbound requests, external resource loads, or navigation. Corrected: generated frontend code having no direct network capability of any kind is now stated as a binding contract requirement the host/iframe security configuration must enforce, with the exact enforcement mechanism left as Implementation Freedom, not the requirement itself.
5. **Safe handle, not "content/location."** The first pass's `requestAsset` description was ambiguous enough to admit returning a URL the game then fetches itself, contradicting the model's own "never a URL" decision. Corrected: `requestAsset` resolves to a safe, Playhoot-provided handle (e.g. `Blob`/`ArrayBuffer`/local object URL) the script consumes directly - never any form of externally-fetchable reference.

A smaller wording correction, same review: "loaded once per Session" is now phrased as "immutable, loaded at iframe/bootstrap creation, not retransmitted per update" - a browser refresh naturally reloads it again, so the invariant is about retransmission cadence during a live session, not a literal one-time-ever guarantee.

## Blockers

None remaining — `WORK-0039` (this WORK's only recorded blocker) is DONE. All five corrections above are incorporated; awaiting final human confirmation of this revised shape before DRAFT -> READY.

## Completion Record

Not yet started.
