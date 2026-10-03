# Engineering Radar items removed at the real-time pivot

Archived verbatim from `docs/engineering/ENGINEERING_RADAR.md` (LATER section). Both concerned the retired Game Language engine.

### Game Language `Value`/`Type` Algebra May Be More General Than The Target Game Range Needs

**Area**
Game Language / Architecture

**Current state / evidence**
- `engine.Value` (`game/language/v1/engine/value.go`) is a closed interface with 11 variants (`Unit`/`Bool`/`Number`/`String`/`User`/`Enum`/`Record`/`Union`/`NewType`/`Optional`/`List`/`Map`), several recursive, each referencing engine's own separate `Type` algebra. `program/DEFINITION.md` places no depth/composition limit on how these nest.
- The one real Definition in the repository (`game/language/v1/program/testdata/parques.json`) uses only a small subset - one `enum`, one flat `record`, one `list<record>` - no `map`/`optional`/`union`/`new_type` anywhere in any authored or example content today.
- This rhymes with `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md`, which removed Child Workflows/Task Groups after an exhaustive audit across the full committed target-game range found zero real use - but the evidence here is weaker: only one simple example exists, and GAME-ADR-0026's own target-coverage table cites `Map` as plausibly load-bearing for per-player/per-object state in other committed-range games (UNO's discard pile, Poker's hand/fold state) that have not been authored yet.

**Risk / opportunity**
- If the generality is genuinely unneeded, it adds ongoing complexity cost to the compiler, the runtime, and (per `docs/projects/active/session-runtime-v1/works/WORK-0029-session-owned-output-value-schema.md`) to whatever session-owned mirror schema eventually has to represent it for a consumer/UI.
- If it is needed (plausible for `Map`, less clear for `Union`/`NewType`), narrowing it now would be premature and costly to reverse.

**Recommendation**
- Do not act on this now. Once a second or third real Definition is authored, revisit with concrete evidence one way or the other - the same kind of exhaustive target-range audit GAME-ADR-0026 itself performed, not a guess from a single simple example.

**Why this horizon**
- LATER: the concern is legitimate but the evidence is not yet conclusive enough to justify a redesign, and no current WORK is blocked on resolving it.

**Reevaluate when**
- A second real Definition is authored and its actual `Value`/`Type` usage is known.

**Next process**
- `ARCHITECTURE_DISCUSSION`, if/when reevaluated.

**Last reviewed**
2026-09-24 - reevaluated per this item's own trigger, `docs/projects/active/session-runtime-v1/works/WORK-0029-session-owned-output-value-schema.md` was drafted for real and had to decide its own mirror schema's coverage. Outcome: this item's own reasoning was applied directly - WORK-0029 does not narrow `engine`'s `Value`/`Type` algebra on the weaker single-Definition evidence available today (consistent with "do not act on this now" above), and its own session-owned mirror covers all 11 variants rather than only Parqués's subset, so no redesign pressure was created in either direction. This item stays LATER, now waiting only on a second real Definition.

### `SignalKindIntent` Does Not Validate Submitted Fields Against Declared Parameter Types

**Area**
Game Language / Engine

**Current state / evidence**
- `signalSchemaFields` (`game/language/v1/engine/internal/runtime/step.go`) copies a `SignalKindIntent` signal's `Fields` verbatim (plus injecting `actor`), with no check against the intent's declared `Parameters` - unlike `SignalKindInteractionAnswered`, which the engine does validate (`Answer` against the question's declared response type and `Validation` expression) before any transition is considered.
- `Manager.SubmitUserIntent` (`session/workflows/sessionlifecycle/step_submit_user_intent.go`) mitigates this defensively at the Session Runtime boundary: it validates intent-name existence and per-declared-`Parameter` presence, and, for a builtin-typed (`user`/`bool`/`number`/`string`) parameter, its value's shape via `engine.Value.Validate(Type)`, before ever calling `AdvanceTurn`. A parameter whose declared type is a named type (record/union/enum/newtype, or a list/map/optional wrapping one) is checked for field presence only, not deep type conformance - resolving a `program.TypeReference` to its compiled `engine.Type` requires the compiler's internal named-type resolution, not exposed on `engine.Program`.
- A structurally wrong-shaped value bound into a transition and later used by an operation expecting a different `Value` type risks a nil/type-assertion panic at evaluation time, not a clean `ErrInputRejected`.
- Today this is not reachable by an untrusted caller: no authored game declares a named-typed `UserIntent` parameter yet (the one real Definition, `parques.json`, has none), and no live client-facing transport exists yet for `SubmitUserIntent` at all (Phase 2/WORK-0020 onward).

**Risk / opportunity**
- If a future authored game declares a named-typed `UserIntent` parameter, and/or once Phase 2 exposes `SubmitUserIntent` to real untrusted client input, this residual gap becomes a real availability risk (a crafted or buggy client payload panicking Turn execution) rather than a latent one.
- Fixing it properly (either exposing compiled `UserIntent` parameter types on `engine.Program`, or having the engine itself validate `SignalKindIntent.Fields` the way it already validates `InteractionAnswered.Answer`) is an engine-level change, out of `session`'s own scope.

**Recommendation**
- Do not act on this now. When a future WORK touches `SignalKindIntent`/`UserIntentSignalSource` engine-side (or when an authored game first declares a named-typed intent parameter), extend the engine's own validation to cover this case, mirroring `SignalKindInteractionAnswered`'s existing treatment.

**Why this horizon**
- LATER: the concern is legitimate and the mechanism gap is real, but no current authored content or live transport can trigger it today, and no WORK is currently blocked on it.

**Reevaluate when**
- An authored game declares a named-typed `UserIntent` parameter, or Phase 2 (WORK-0020 onward) exposes `SubmitUserIntent` to real client input.

**Next process**
- `FEATURE_DEVELOPMENT`, scoped to the Game Language engine, if/when reevaluated.

**Last reviewed**
2026-09-26 - identified during WORK-0010's (User Intent Runtime Path) design/implementation.
