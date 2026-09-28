# WORK-0045: Frontend Iframe Delivery Contract Specification

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- `docs/projects/active/js-runtime-migration/works/WORK-0044-game-version-artifact-model.md` (owns the fact that a frontend script is stored content, not a package reference; this WORK owns how that script actually talks to the backend once loaded)

## Outcome

Produce the accepted, canonical specification of the frontend delivery contract `ADR-0015` establishes at the architecture-principle level: the trust boundary (iframe isolated in context/origin from the main application, no embedded credentials, restricted permissions), the small client-library API surface (conceptually: receive current view, receive effects, send an interaction — exact method names/shapes decided here), frontend-script loading (one version's `FrontendScript`, per `WORK-0044`, loaded per Session, not re-sent per update), the asset-request mechanism (logical key in, resolved content out — never a raw URL in the script), and the send/receive/apply/reject/fail distinction the mandate requires (a transport acknowledgement is never treated as an accepted play). This is a documentation deliverable per `ADR-0015`'s explicit scope boundary — implementing the frontend, its SDK, or its repository is out of this Project's scope — but the specification itself is approved design work, going through the same DRAFT/READY process as any other WORK, not freelanced outside it.

## Context

Not yet designed. Depends on `WORK-0039`'s command/view vocabulary existing in at least draft form, since the contract must be concrete about what a view/effect/interaction actually contains.

**Design points surfaced while drafting `WORK-0044` (2026-09-27), recorded here so they are not lost before this WORK is actually drafted — none of these are decided yet:**

- The backend must never send a presentation/rendering instruction (no "show this screen," no UI-element description) — only logical events carrying data (for example, "you were dealt this card," "you lost"). The frontend script alone decides what any event means for the interface; the backend has no opinion about presentation at all. This is closer to an API boundary (backend exposes behavior/data, never presentation) than the old engine's model, where the backend's own declarative language dictated the UI tree.
- Conceptually, the backend and frontend scripts behave as if they call each other directly (the frontend "calls" a backend action; the backend "calls back" with an event), but the actual mechanism is asynchronous: a frontend interaction becomes a real request to Session Runtime, which drives the Executor, and the result comes back as an event later, not a direct return value.
- Asset requests: the frontend script must never contain a raw URL. It requests an asset by its logical key through a host-provided call (exact shape not decided), and receives the actual content/location back through that call — never by having a URL baked into its own source.
- Open idea, not a decision: a frontend-side "current screen state" object, mirroring the backend's own state-object/mutation pattern, that could make rendering (and recovering a screen after a disconnect/reconnect) deterministic. Worth real consideration when this WORK is actually drafted, but not committed to now.
- Disconnect/reconnect rendering behavior generally (what a returning player's screen should show) is a real requirement this WORK must eventually address, not something to invent a full mechanism for prematurely.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must not describe a synchronous, infallible call shape — send/receive/apply/reject/fail must be explicitly distinguished.
- Must not grant the iframe direct network/credential access — all authenticated transport is owned by Playhoot's own (separately built) frontend, relayed to the iframe.

## Acceptance Criteria

- A published specification document exists covering the trust boundary, API surface, package loading, and delivery-state distinctions above, verifiable against `ADR-0015`'s own requirements.

## Implementation Freedom

Not yet designed.

## Verification

- Review against `ADR-0015`'s own text for completeness; no code verification, since this WORK produces no production code.

## Documentation Impact

### Accepted / Canonical Knowledge

- A new canonical specification document (location decided when this WORK is drafted, likely alongside `WORK-0046`'s implementation).

## Blockers

- Depends on `WORK-0039`.

## Completion Record

Not yet started.
