# WORK-0039: Platform Command Protocol & Runtime Validation

Status: DRAFT
Created: 2026-09-27
Last status change: 2026-09-28 (Vocabulary revised by explicit human architectural decision: presentation-agnostic backend protocol, `PLAYER_EVENT`/platform-fact events replace `INTERACTION_ANSWERED`/`USER_INTENT`, `SEND_EVENT` replaces `EMIT_EFFECT`, a separate pure `project()` entry point replaces `REQUEST_VIEW` — see Context and Approved Design)

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`

Canonical context:
- `game/session/workflows/sessionlifecycle/outputs.go`, `internal/clientoutputs/clientoutputs.go` (the closed `Value`/`Output` schema this generalizes, per WORK-0029's precedent)

## Outcome

Define the closed vocabulary of platform-level commands authored JavaScript may request (`requestedCommands` in `WORK-0035`'s contract) — schedule/cancel a timer, emit an effect, emit a per-player view, request a session-lifecycle transition (completion/failure/cancellation) — and build runtime validation for both this platform vocabulary and any game-specific contract (a specific game's own action/view shapes) at the sandbox boundary. Static types in authored TypeScript, if used, do not survive to the sandboxed runtime boundary as a guarantee; this WORK is what actually enforces shape/content correctness at runtime, the same role Game Language's compiler played for the DSL.

## Context

**The wire-schema blocker this WORK's own file previously named is already resolved by what `WORK-0035`/`WORK-0052`/`WORK-0053` actually built (all DONE), independent of this WORK's own design.** `session/internal/executor/executor.go` already fixes the outer shape: `Executor.Execute(ctx, ExecutionInput{Script ResolvedScript, PreviousState json.RawMessage, Event json.RawMessage, Context ExecutionContext}) (ExecutionOutput{NewState json.RawMessage, RequestedCommands []json.RawMessage}, error)`. `PreviousState`/`Event`/`NewState` are opaque JSON at every layer (Go port, sandbox, gRPC wire). `RequestedCommands` exists as a name and a container shape at every layer already, with **zero internal structure** — three independent LOGICAL_CONTRACT.md files and `WORK-0035`'s own Scope section name this gap as deferred to this WORK. Per `ADR-0016`, validation happens in Session Runtime itself (the caller), never inside the Executor service.

**Vocabulary revised by explicit human architectural decision (2026-09-28), superseding this WORK's own first-pass proposal.** The governing principle, stated directly by the product/architecture owner: **the backend game script owns game rules and game data; the frontend game script owns presentation and interaction UX; Playhoot owns session/platform concerns and transport.** Consequently, **the backend JavaScript protocol must be completely presentation-agnostic** — Playhoot's platform vocabulary must not contain screens, visual effects, questions, cards, buttons, UI interactions, or any presentation-tree concept. The generated backend and frontend may agree on arbitrary game-specific data contracts; Playhoot only transports and governs the platform-level envelope around them, never their game-defined meaning.

**No accepted ADR contradicts this** (checked directly against `GAME-ADR-0028` and `ADR-0015` before revising, per instruction): both records state explicitly that the exact command/event vocabulary is this WORK's own design decision, not fixed by either ADR — `GAME-ADR-0028` line 31: "the exact command vocabulary [is a] concrete design decision of the owning WORK... This record fixes the shape of the contract, not its encoding"; `ADR-0015` line 32: "the exact function signature, serialization shape... are design decisions for the owning WORK... not frozen by this record." The only ADR text that named "emit an effect"/"request a specific view" (`GAME-ADR-0028` line 29) was illustrative of the *kind* of thing a command could request, not a binding shape — and this revision satisfies the same underlying principles those words gestured at (JS computes per-player view data directly, per `ADR-0015`'s Consequences; commands remain declarative data, never callbacks) through a cleaner mechanism. `WORK-0045`'s own file, drafted the same day as the ADRs and independent of this revision, already anticipated the same direction under "Design points surfaced while drafting WORK-0044": "The backend must never send a presentation/rendering instruction... The frontend script alone decides what any event means for the interface."

**This collapses two previously separate Session Runtime capabilities at the vocabulary level.** `INTERACTION_ANSWERED` and `USER_INTENT` (mirroring today's distinct `Manager.AnswerInteraction`/`Manager.SubmitUserIntent` steps and `session_interactions`/`SignalKindIntent` mechanisms) are both just "a game-defined action initiated by a player" — the platform does not need, and must not keep, a durable notion of an "open interaction" a player is "answering," since that is exactly the kind of QUESTION/ASK_GROUP presentation concept this revision retires from the platform vocabulary. Both collapse into a single `PLAYER_EVENT`. Whether `sessionlifecycle`'s own public `Manager` method surface also collapses to one method, or keeps two thin methods that both ultimately build a `PLAYER_EVENT`, is `WORK-0038`'s own call-site-design question, not decided here — this WORK only fixes the wire vocabulary a Turn is driven by.

## Scope

### In Scope

- A new internal package (illustrative name: `session/workflows/sessionlifecycle/internal/platform`) owning the full closed vocabulary below for both of the backend script's two entry points: `execute(state, event, context) -> {state, commands}` (drives a Turn) and `project(state, viewer, context) -> ClientState` (pure, read-only, never mutates authoritative state or emits commands — `WORK-0041`'s own privacy-verification mechanism operates on this entry point's output, not designed here).
- An extensible registration mechanism (registry/handler/codec pattern, exact Go mechanism left to Implementation Freedom) such that adding one new platform Event or Command kind in the future means adding its own type + validation + registration in one place, never editing shared execution/dispatch logic. Unknown platform kinds are always rejected regardless of how the vocabulary grows later.
- `ParseCommand`/`ParseEvent` validation, including identity/authority stripping: any field naming a recipient/actor (`SEND_EVENT.recipients`, `PLAYER_EVENT.actor`) may only reference identifiers Playhoot itself already supplied via `Context`/a prior platform-fact Event — never trusted from the script's or a client's own claim.
- The platform's own generic representation for opaque, serializable game-authored data (`PLAYER_EVENT.payload`, `SEND_EVENT.payload`, the state a script returns) — plain, size-bounded JSON (`json.RawMessage`), not `engine.Value` or any other Game-Language-era wrapper type. `engine.Value`'s record/list encoding existed only to serialize the DSL's own closed value/type system; JavaScript emits and consumes plain JSON natively, so no equivalent wrapper is needed, and keeping one merely because it already exists would leave the new protocol semantically coupled to the engine this migration retires.
- A package-local contract document (successor role to `game/language/v1/engine/README.md`'s Output taxonomy) recording the full closed vocabulary, the platform-vocabulary/game-vocabulary distinction, and validation rules.

### Out of Scope

- A specific game's own action/data contract (`GameContract`) — this WORK validates only the platform-level envelope every game shares (`PLAYER_EVENT`/`SEND_EVENT`'s own `name`/`payload` are opaque to Playhoot); a game-specific schema layered on top is separate, later territory (`WORK-0037`/authoring validation).
- Actually dispatching a parsed Command to its owning persistence mechanism (`WORK-0040` timers, `WORK-0041` `project()`/privacy verification, `WORK-0042` `SEND_EVENT` delivery, `WORK-0043` durable `PLAYER_EVENT` outcome delivery). This WORK produces typed, validated Event/Command values; those WORK consume them.
- Switching any `sessionlifecycle` call site to actually call `Execute`/`project`/use this package, and the collapsed-or-not `Manager` method question above — `WORK-0038`'s scope.
- The generated frontend's own `playhoot.onState`/`onEvent`/`send` client-library shape — `WORK-0045`'s scope; this WORK only fixes what crosses the backend-script boundary, not the iframe-to-Playhoot-frontend boundary.

## Approved Design

**Inputs to `execute` (platform Event vocabulary):**

```text
{"kind": "SESSION_STARTED", "root_parameters": <JSON>, "players": [...]}
{"kind": "PLAYER_EVENT", "actor": <actor identifier>, "name": <string, game-defined, opaque>, "payload": <JSON, game-defined, opaque>}
{"kind": "PARTICIPANT_DISCONNECTED", "participant": <actor identifier>}
{"kind": "PARTICIPANT_RECONNECTED", "participant": <actor identifier>}
{"kind": "PARTICIPANT_LEFT", "participant": <actor identifier>}
{"kind": "TIMER_EXPIRED", "timer": <string, opaque identifier>, "data": <JSON, optional, opaque>}
{"kind": "SESSION_CANCELLED"}
```

`PLAYER_EVENT.name`/`.payload` are entirely game-defined and opaque to Playhoot (`play_card`, `answer_question`, `move_piece`, ... — the platform validates only the envelope: valid `actor`, valid session, size limits, sequencing). `PARTICIPANT_DISCONNECTED`/`RECONNECTED`/`LEFT` are distinct, Playhoot-certified facts, never modeled as a `PLAYER_EVENT` (a disconnect is not a player-requested action) and never merged into one generic kind, since each represents a different authoritative session fact a game may react to differently (pause a turn, start a grace timer, ignore it entirely). `TIMER_EXPIRED` uses one generic timer-identifier shape covering both a plain and a "keyed" timer (no separate platform concept for the two, unlike the retired engine's `TimerSlot`/`KeyedTimerSlot<Key>` split) — `WORK-0040` confirms this collapse is sufficient once it designs the actual obligation-persistence adaptation.

**Outputs from `execute` (platform Command vocabulary):**

```text
{"kind": "SEND_EVENT", "recipients": [<actor identifier>...], "name": <string, game-defined, opaque>, "payload": <JSON, game-defined, opaque>}
{"kind": "SCHEDULE_TIMER", "timer": <string, opaque identifier>, "delay_ms": <int>, "data": <JSON, optional, opaque>}
{"kind": "CANCEL_TIMER", "timer": <string, opaque identifier>}
{"kind": "SESSION_COMPLETE"}
{"kind": "SESSION_FAIL", "reason": <string>}
```

`SEND_EVENT.name`/`.payload` are entirely game-defined and opaque to Playhoot (`card_played`, ...) — the platform validates only valid recipients, session, size/count limits, sequencing/correlation. The generated frontend alone decides what a `SEND_EVENT` means (animate, notify, play a sound, or ignore); it is never renamed to anything presentation-specific (no `EMIT_EFFECT`), since that would smuggle a UI concept back into the platform vocabulary under a different name. There is no `OPEN_INTERACTION`/`interaction_kind`/"question" concept anywhere in this vocabulary — a game's own notion that "a question currently exists" is entirely backend-authored data the frontend renders however it chooses; the frontend's own emitted action always arrives back as an ordinary `PLAYER_EVENT`.

**Second backend entry point — `project` (replaces `REQUEST_VIEW` as a command entirely):**

```text
project(state, viewer, context) -> ClientState
```

Pure, read-only: cannot mutate authoritative state, cannot emit Commands, runs in the same controlled execution environment as `execute`, and receives `viewer`'s identity from Playhoot (never from untrusted client input). The platform invokes `project` for the relevant participant and delivers the resulting `ClientState` to that participant — including on initial load/reconnect, deriving current view directly from persisted state (`WORK-0038`) without replaying any transient history. The result is named/thought of as `ClientState`/`PlayerProjection`, never `View`, since it carries authorized game data, not UI instructions, and must never itself contain presentation concepts (`screen`, `button`, `animation`, a component tree) unless a game's own data happens to look like that by the game's own choice — Playhoot has no semantic knowledge of it either way.

**`ClientState` vs. `SEND_EVENT` — a fundamental, non-collapsible distinction:** `ClientState` (from `project`) is the recoverable current truth a disconnected/reconnected participant's view can always be correctly regenerated from. `SEND_EVENT` is a transient occurrence (a frontend might animate a card moving) that may be missed entirely without affecting correctness, since correctness always falls back to the latest `ClientState`. `WORK-0041` owns `project`'s privacy-verification mechanism; `WORK-0042` owns `SEND_EVENT`'s best-effort delivery; neither may be redesigned to substitute for the other.

**Two extension levels, kept explicit in the implementation and its documentation:** the **platform vocabulary** (the kinds listed above) is finite, Go-owned, and validated — adding to it is a deliberate platform change, never something authored code can do by itself. The **game vocabulary** (`PLAYER_EVENT.name`, `SEND_EVENT.name`, and both kinds' `payload`) is arbitrary, owned entirely by each authored game, and requires zero platform code changes to extend.

## Constraints and Invariants

- The platform vocabulary must remain closed and Go-owned; an unrecognized platform `"kind"` is always a validation failure. A game-defined `name`/`payload` inside `PLAYER_EVENT`/`SEND_EVENT` is never validated for platform meaning, only for envelope shape/limits.
- Playhoot-authoritative fields — event `kind` for platform-fact events, actor/participant/session identity, sequence/order, timer identity/expiration, disconnect/reconnect/leave facts, session cancellation, recipient validity, terminal session state — are never trusted from a command's or client's own declared content, only from what Playhoot itself already established.
- Game-authored/opaque-to-Playhoot: `PLAYER_EVENT`/`SEND_EVENT`'s own `name`/`payload`, authoritative game state contents, and `ClientState`/`PlayerProjection` contents.
- `project` cannot mutate authoritative state and cannot emit Commands — a violation of this (a script attempting to do so) is a validation failure, the same disposition as an unrecognized platform kind.
- Validation happens in Session Runtime (the `Executor` port's caller), never inside the Executor service (`ADR-0016`).
- No platform vocabulary member may encode a presentation/UI concept (screen, button, animation, component tree, question/interaction kind) — this is the central invariant this revision exists to enforce, not an incidental style preference.

## Acceptance Criteria

- Every Event kind above has a Go type, encode/decode, and round-trips through its own test fixtures; every Command kind above has a Go type and is accepted by `ParseCommand`.
- An unrecognized platform `"kind"`, a missing/malformed required field, and a `payload`/`data` exceeding a defined size limit are each rejected with a distinguishable error, for both Events and Commands.
- A `SEND_EVENT`/`PLAYER_EVENT` referencing a recipient/actor identifier Playhoot never supplied is rejected by validation, not merely by a downstream consumer.
- Adding a new platform Event or Command kind (verified by actually adding one as a test exercise) requires touching only that kind's own new file plus its registration call — no edit to shared parsing/dispatch/execution logic.
- `project`'s own output is rejected if the authored script attempts to return a Command or a state mutation through it (verified by an adversarial test fixture).
- No platform type/field anywhere is named or shaped after a presentation concept (screen/button/animation/question/interaction-kind) — verified by the vocabulary listing itself, not a runtime check.
- `go test ./...` clean; unit coverage for every kind's encode/decode/parse/reject path and the registry-extension property above.

## Implementation Freedom

- Exact Go type/package/registry mechanism naming (a `map[string]Codec`-style registry is a reasonable default; the exact shape is the implementer's call, provided the "new kind = new file + registration, no shared-logic edit" property holds).
- Exact JSON field names beyond the illustrative shapes above.
- Exact size/count limits for `payload`/`data`/recipients (a resource-limit detail, coordinates with `WORK-0036`).

## Verification

- `go test ./...` for the new package.
- A round-trip test building each Event kind, feeding it through the `executor.Fake`, and parsing a fabricated `RequestedCommands` response of every Command kind.
- A registry-extensibility test: add a throwaway new platform kind in a test-only registration and confirm no other file needed edits.

## Documentation Impact

### Accepted / Canonical Knowledge

- A new package-local contract document (successor role to `game/language/v1/engine/README.md`'s Output taxonomy) recording the closed vocabulary, the platform/game vocabulary distinction, and validation rules.

## Blockers

None. The wire-schema blocker this WORK's file previously named was resolved by the already-DONE `WORK-0035`/`WORK-0052`/`WORK-0053`; the vocabulary itself is now human-decided (see Context) rather than an open design question.

## Material Decisions Needing Human Input

None remaining at the vocabulary level — the human explicitly resolved the vocabulary shape and confirmed (via this same instruction) that no accepted ADR is being reopened by it. Two small items remain genuinely open, but at the Implementation-Freedom level, not requiring further human product/architecture input before READY:

1. Exact size/count limits for payload/recipients (coordinates with `WORK-0036`, not a vocabulary question).
2. How a game-specific `GameContract` schema layers on top of platform-level validation (deferred to `WORK-0037`/authoring-time validation, not this WORK's own mechanism).

## Completion Record

Not yet started.
