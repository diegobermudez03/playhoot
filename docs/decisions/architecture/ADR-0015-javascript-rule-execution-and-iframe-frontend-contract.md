# ADR-0015: Sandboxed JavaScript Game Rule Execution and Iframe-Isolated Frontend Contract

Status: ACCEPTED
Created: 2026-09-27
Last status change: 2026-09-27
Supersedes: None
Superseded by: None

## Context

Playhoot's only current rule-authoring/execution mechanism is Game Language (`game/language/v1/program` + `engine`): a closed-variant, statically-typed, deterministic, bounded declarative DSL, compiled ahead of time and interpreted by a pure Go AST-walking engine (`engineservice`). It already provides real capabilities — per-player private views (`Projection`/`View`/`Presentation` declarations), ordinary and keyed timers, cosmetic effects, and a replay-capable execution model — but its expressiveness is deliberately closed: no user-defined functions beyond pure, non-recursive `FunctionDeclaration`s, no unbounded loops, no arbitrary control flow beyond `if`/`match`/bounded `for_each`, and a bespoke, non-standard declarative UI/presentation language (`UIElement`/`UILayout`/`UIAction`) that only Playhoot's own (not-yet-built) client can interpret.

No prior decision record in this repository ever evaluated JavaScript, or any general-purpose scripting language, against this DSL. The DSL's closed shape is a foundational, undocumented premise, not a previously reasoned architecture decision this record supersedes.

Two pressures now require revisiting that premise:

1. **Product ambition.** `docs/product/PRODUCT_STATE.md` targets game expressivity "comparable to Parques, UNO, or poker" and beyond — real programming constructs (arbitrary functions, richer data manipulation, composable logic) become valuable for authors, not merely convenient, as game complexity grows. The DSL's closed-variant design (`isX()` sealed-interface pattern spanning `engine`+`compiler`+`runtime`+`codec` in lockstep for every new construct, per `game/language/v1/engine/IMPLEMENTATION.md`) also makes the language itself expensive for Playhoot to extend.
2. **AI-assisted authoring.** Playhoot's authoring flow (`game/language/v1/program/DEFINITION.md`, `GAME_BRIEF.md`) already depends on an LLM emitting a bespoke JSON AST correctly from a prompt-injected specification of a language no foundation model was trained on. A widely used, large-corpus language a foundation model already understands natively is a materially better fit for both an externally connected AI authoring tool and Playhoot's own integrated authoring assistant, and removes the need to keep re-teaching an LLM a proprietary language via prompt engineering as the language grows.

Separately, Playhoot's frontend does not exist as a repository yet, and no accepted contract exists for how a game's presentation is delivered to or rendered by a client. Game Language's declarative `UIElement`/`UILayout`/`UIAction` model implicitly assumed Playhoot itself would ship one generic renderer interpreting that tree; no such renderer has been built (`game/CURRENT_STATE.md`: transport is a non-functional skeleton).

## Decision

### Rule execution moves to sandboxed JavaScript

Game Language (`program`+`engine`, current v1) is retired as Playhoot's rule-authoring/execution mechanism and replaced by author-written JavaScript executed inside a sandboxed, untrusted-code runtime under Session Runtime's ownership (per `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`, restated below). The conceptual execution contract is:

```text
execute(previousState, event, context) -> { newState, requestedCommands }
```

The exact function signature, serialization shape, and supported JavaScript profile are design decisions for the owning WORK (see Implementation Impact), not frozen by this record. What is fixed here:

- Authored code receives explicit inputs and returns a serializable new state plus zero or more declarative commands as data. It never returns functions, callbacks, or instructions for Go to execute directly, and never calls back into Go/host capabilities mid-execution.
- Authored code cannot persist state directly, open connections, access secrets, call internal services, or run its own long-lived process/server. It cannot depend on module-level globals, closures, or a live in-memory instance to carry information between invocations.
- The runtime that executes authored code is stateless as a library, exactly as `engineservice` is today: every invocation is a pure function of its explicit inputs. Playhoot (Session Runtime) durably owns all state between invocations.
- Time, randomness, and any other source of non-determinism are Playhoot-controlled inputs, not ambient authored-code capabilities. Static analysis/lint enforcement (a required capability of this migration, not optional hardening) flags authored use of nondeterministic host APIs (wall clock, `Math.random`, network, filesystem) before a script is accepted; this reduces but does not eliminate the chance of accidentally nondeterministic authored code — see the persistence-model consequence below, which does not depend on perfect determinism for correctness.

Game Language's `program.Definition` type system, compiler, and Go-interpreter runtime (`internal/compiler`, `internal/runtime`) are retired once this migration lands; they are not kept as a parallel execution path. The declarative `Projection`/`View`/`Presentation`/`UIElement` model is retired as a Playhoot-owned rendering contract — authored JavaScript now computes per-player view/effect data directly as part of its command output (see Consequences on privacy).

### Untrusted code requires real sandboxing, not merely a closed language

Game Language's current safety model is "the language cannot express anything dangerous" (no I/O, no unbounded loops, closed type/operation set). A general-purpose language requires the opposite model: the code is assumed adversarial or buggy regardless of its author (including Playhoot's own AI authoring tooling), and safety comes from runtime isolation, not from what the language can syntactically express. This is a new platform infrastructure responsibility, not a Session Runtime business rule: process/isolate-level execution boundary, no direct access to the database, secrets, network, host filesystem, or arbitrary host modules/processes, and enforced limits on compute time, memory, input/output/state size, and emitted-command count per invocation, with the ability to terminate a non-cooperative execution without compromising the host process. This extends to every pipeline that processes authored/generated content on Playhoot's infrastructure (script validation, compilation/bundling, asset processing, authoring-time simulation/rendering), not only live Session execution.

### Go's ownership is unchanged in kind, extended in scope

Go (Session Runtime, and the platform capabilities `ARCHITECTURE.md` already assigns to Composer/Orchestrator/Identity) remains the sole owner of transport, authentication/authorization, session lifecycle, event ordering, idempotency, persistence, timer scheduling, execution invocation, and delivery of results. JavaScript computes; it never materializes an external effect, timer, or delivery on its own authority. Concretely, this restates rather than reopens: `GAME-ADR-0018`'s per-Session mutation serialization, `GAME-ADR-0019`'s per-invocation execution bound (a JavaScript-runtime equivalent replaces the DSL-specific step-chain bound), `GAME-ADR-0006`'s root player-roster contract, and `GAME-ADR-0008`/`GAME-ADR-0012`'s timer-ownership model (Playhoot schedules/fires timers; authored code only requests/reacts to them as commands/events). Each is carried forward at the architecture-principle level; the owning WORK for each concrete mechanism confirms or adapts its precise shape against the new execution model rather than assuming it is unaffected by construction.

### Snapshot-based persistence replaces replay-first persistence

`GAME-ADR-0024`'s replay-first model (no durable Snapshot; current/historical state recomputed by deterministically replaying the full durable signal log) is superseded. It depended on Game Language's engine being provably, closedly deterministic by construction — a guarantee sandboxed JavaScript can approach (via the constraints above and static analysis) but cannot make absolute, since JavaScript is a general-purpose language a lint pass cannot fully police.

Session Runtime instead persists the current confirmed state (the JavaScript `newState`) as the authoritative record after every committed step, so a disconnected player or a resumed process continues from the correct prior state without depending on replay for correctness. The full ordered signal/event log is still durably retained — not for correctness, but so that a best-effort session reconstruction/replay capability can eventually be offered for audit/inspection, on an explicit best-effort basis (a replay divergence in that reconstruction is a diagnostic signal, not a live-correctness failure, since the live path never depends on replay succeeding). `game/docs/decisions/GAME-ADR-0029-snapshot-based-session-runtime-persistence.md` records the concrete persistence-model decision this implies.

### Frontend delivery: versioned package loaded in an isolated iframe

Each game version ships a compiled frontend package (React/TypeScript/CSS/SVG) alongside its backend JavaScript. Playhoot's own frontend (a separate, not-yet-built repository) loads a session's pinned frontend package inside a sandboxed iframe, isolated in context and origin from the main application, with restricted permissions and no embedded credentials. The iframe communicates only through a small Playhoot-provided client library exposing conceptually: receive current view, receive effects, send an interaction. It never calls Session Runtime, any backend JavaScript, or any authenticated endpoint directly; Playhoot's own (already-existing-in-principle, not-yet-built) frontend owns the authenticated transport and relays between the iframe and the backend. The iframe may hold local visual/interaction state and run animations; authoritative game state and rules remain backend-only.

This record establishes the contract and its trust boundary; it does not authorize building the frontend application, its SDK, or its repository — see Explicitly Out Of Scope below and `docs/projects/active/js-runtime-migration/PROJECT.md`.

**Refined (2026-09-27) by `game/docs/GAME_VERSION_ARTIFACT_MODEL.md`** (via `docs/projects/active/js-runtime-migration/works/WORK-0044-game-version-artifact-model.md`): "compiled frontend package" above is refined at the implementation-shape level to a mandatory, directly stored frontend script — Playhoot holds the authored source itself, not merely a reference to a separately built/compiled package. This does not change the trust boundary, isolation, or client-library principles this record establishes; it fixes how the frontend content is actually held and pinned per version.

### Delivery is not uniformly best-effort

`GAME-ADR-0020`'s "best-effort, no durable outbox" stance is restated, unchanged, for cosmetic/presentation effects (a missed animation must never change correctness). It is extended, not merely inherited, for the outcome of a confirmed interaction: a client must be able to distinguish transport acknowledgement from actual acceptance/rejection of its action, and Session Runtime must provide a durable mechanism (an outbox or equivalent) so a confirmed Turn's result is not silently lost to a transient delivery failure the way a cosmetic effect may be. `GAME-ADR-0020`'s own text already anticipated this: a future capability with irreversible/durable delivery needs "must explicitly design its own delivery/idempotency/retry semantics as a separate decision" rather than inherit the "no outbox" conclusion. This record is that decision for confirmed interaction results; the concrete mechanism is a required WORK under the Project below, not designed here.

## Rationale

A widely adopted general-purpose language lets authors (human or AI) express real programs — functions, composable data structures, arbitrary (bounded, sandboxed) control flow — instead of being limited to what a bespoke, closed DSL's maintainers chose to add. It also removes the recurring cost of teaching every authoring LLM a language it was never trained on, and removes the need for Playhoot to build and maintain its own general-purpose-enough language, compiler, and interpreter merely to keep pace with authoring ambition.

The tradeoff this record accepts deliberately: JavaScript requires Playhoot to build and operate real sandboxing/isolation infrastructure and to accept that perfect determinism cannot be guaranteed by construction the way the closed DSL guaranteed it. Reverting to snapshot-based persistence (rather than keeping replay as the correctness mechanism) is the direct, deliberate answer to that tradeoff: correctness depends on durably persisting the state JavaScript actually produced, not on being able to reproduce it later.

## Alternatives Considered

### Keep Game Language, add a JavaScript-callable extension mechanism for narrow cases

Rejected. This still requires authors (and the authoring LLM) to primarily think in the closed DSL, does not solve the AI-authoring-corpus problem for the primary authoring surface, and adds a second execution model to maintain instead of retiring the DSL's ongoing maintenance cost.

### Compile a JavaScript/TypeScript-like authoring surface down to the existing DSL

Rejected. This still bounds authored expressiveness to whatever the DSL's closed operation/expression set can represent, so it does not solve the product-ambition pressure; it only changes the authoring syntax, not the execution model's ceiling.

### General-purpose language without sandboxing (trust authored/generated code)

Rejected outright, including for Playhoot's own AI-generated code. Generated code is not inherently safe merely because Playhoot's own tooling produced it; a bug or an adversarial prompt-injection during authoring is exactly the failure mode sandboxing exists to bound.

### Keep replay-first persistence, pin an exact JavaScript engine build/version for bit-for-bit determinism

Rejected for V1. Technically possible, but it trades an ongoing, open-ended operational obligation (pinning and indefinitely supporting an exact interpreter build per authored version, for as long as any session might replay) for a correctness guarantee snapshot persistence no longer needs. This record does not forbid revisiting deterministic-replay guarantees later if a concrete need (for example, a stronger anti-cheat or audit requirement) justifies the cost; it is not required to reach this migration's accepted product/architecture scope.

## Consequences

- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md` records the concrete execution-model decision (supported JavaScript profile, sandbox boundary shape, command/state contract) this record establishes at the architecture-principle level.
- `game/docs/decisions/GAME-ADR-0029-snapshot-based-session-runtime-persistence.md` supersedes `GAME-ADR-0024`'s central no-persisted-Snapshot decision; see that record for exact scope.
- `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`'s domain-split decision (Game Management and Session Runtime as independent bounded contexts) and its composition-flow decisions (Composer-mediated Create-time visibility, Orchestrator-mediated publish) are conserved, not overturned — this record only changes what the artifact Session Runtime persists and executes actually is (authored JavaScript plus a frontend package, not a compiled Game Language `program.Definition`). See ADR-0014's header for the precise partial-supersession scope.
- Authored per-player views are now computed by untrusted code, not compiled from a Playhoot-trusted declarative `Projection`. A privacy-verification mechanism (confirming no player's view/command output leaks another player's private data) is a required capability of this migration, not an incidental afterthought — knowing all game state does not, by itself, guarantee an authored view is correct.
- Every existing `GAME-ADR` whose text is specific to Game Language's compiled-DSL shape (`GAME-ADR-0006`, `GAME-ADR-0007`, `GAME-ADR-0008`, `GAME-ADR-0010`, `GAME-ADR-0011`, `GAME-ADR-0012`, `GAME-ADR-0019`, `GAME-ADR-0026`, `GAME-ADR-0027`) remains historically accurate for the engine it describes; each is either restated at the principle level by this record and `GAME-ADR-0028`, or superseded/refined by a specific new record as the owning WORK reaches that mechanism. None is silently treated as still governing the new engine merely because it was never explicitly mentioned.
- This is a substantial migration spanning Game Management, Session Runtime, and Session Runtime's execution/persistence internals. No implementation beyond what an already-authorized WORK covers is authorized by this record alone — implementation is routed to `docs/projects/active/js-runtime-migration/`.

## Explicitly Out Of Scope

- Building the frontend application, its browser SDK, its authoring-chat UI, or their repository. This record documents the contract and requires the backend capabilities necessary to serve it; it does not plan or authorize frontend implementation, which will live in a separate, not-yet-created repository.
- Selecting the specific sandbox runtime/isolation technology, the exact supported JavaScript/TypeScript profile, and the exact command/state wire schema. These are required design decisions of specific WORK under the Project below, not decided here.
- Selecting AI provider(s), model(s), or their commercial terms for AI-assisted authoring.

## Canonical Knowledge Impact

- `ARCHITECTURE.md` — Accepted Business Boundaries annotated to reference this record alongside ADR-0014; rewritten directly (not as a superseding annotation) once the owning WORK lands.
- `game/README.md`, `game/CURRENT_STATE.md`, `game/docs/DATA_MODEL.md`, `game/docs/FLOWS.md`, `game/language/v1/engine/*`, `game/language/v1/program/*` — superseded in the specific ways `GAME-ADR-0028`/`GAME-ADR-0029` and their owning WORK specify; not rewritten by this record directly.
- `docs/product/PRODUCT_STATE.md`/`docs/product/ROADMAP.md` — unaffected; this is an architecture/execution-model decision, not a product-scope change.
- `docs/decisions/architecture/ADR-0014-management-session-domain-split.md` — `Superseded by:` header annotated (in part).
- `game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md` — `Superseded by:` header annotated (see `GAME-ADR-0029`).

## Implementation Impact

Not authorized by this record beyond its own creation and the two companion domain-scoped records above. Routed to a dedicated Project, `docs/projects/active/js-runtime-migration/`, which supersedes/absorbs `docs/projects/completed/management-session-domain-split/`'s remaining goal (Game Management/Session Runtime domain split and composition) as part of its own scope, and coordinates with (does not duplicate) `docs/projects/active/session-runtime-v1/` for session-lifecycle/lobby/live-connection concerns that are independent of rule-execution language.
