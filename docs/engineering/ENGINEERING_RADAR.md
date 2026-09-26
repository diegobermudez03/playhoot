# Engineering Radar

Status: NON-AUTHORITATIVE TECHNICAL RADAR

This is the canonical location for Playhoot's current engineering recommendations and future concerns, but the content is NON-AUTHORITATIVE.

A radar item is:

- an AI recommendation or concern;
- not an accepted decision;
- not an engineering standard;
- not architecture;
- not a roadmap commitment;
- not an approved WORK specification;
- not implementation authority.

NOW != READY

A NOW radar item does not authorize a Codebase Agent to implement anything.

## Authority Boundaries

If a radar recommendation later produces an accepted product decision, architecture/domain decision, or engineering standard, the accepted fact must live in its canonical owner.

The Radar must not become a duplicate source of accepted truth. After such a decision, remove the radar item if the concern has been fully resolved at the recommendation level, or update it to describe only the meaningful remaining concern.

A radar item must never directly authorize implementation. If a recommendation reaches the point where concrete implementation is approved, use `FEATURE_DEVELOPMENT` and the WORK specification system.

The Radar is not:

- a product backlog;
- an engineering task tracker;
- a sprint board;
- a product roadmap;
- a list of all future features;
- a list of all technical debt.

Only persist concerns or recommendations whose continued visibility is useful at the Principal Engineer level.

## Horizons

The horizon is primarily an action/attention recommendation.

It is not:

- a WORK status;
- a decision status;
- a formal incident severity;
- an implementation priority automatically accepted by the human.

Impact or risk can be explained inside an item when useful. Do not introduce a separate mandatory severity system here.

## NOW

The concern/opportunity exists under current conditions and deserves active follow-up now or alongside near-term work.

NOW does not itself mean:

- implementation is approved;
- an incident exists;
- architecture has been decided.

No items have yet been persisted under the Engineering Radar mechanism.

## SOON

The concern does not need immediate action but is likely worth addressing before a meaningful near-term milestone or before continued development makes it materially more expensive/risky.

No items have yet been persisted under the Engineering Radar mechanism.

## LATER

The concern is legitimate, but current conditions do not justify acting on it.

A LATER item should have a meaningful reevaluation trigger where possible.

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
- `Manager.SubmitUserIntent` (`game/session/workflows/sessionlifecycle/step_submit_user_intent.go`) mitigates this defensively at the Session Runtime boundary: it validates intent-name existence and per-declared-`Parameter` presence, and, for a builtin-typed (`user`/`bool`/`number`/`string`) parameter, its value's shape via `engine.Value.Validate(Type)`, before ever calling `AdvanceTurn`. A parameter whose declared type is a named type (record/union/enum/newtype, or a list/map/optional wrapping one) is checked for field presence only, not deep type conformance - resolving a `program.TypeReference` to its compiled `engine.Type` requires the compiler's internal named-type resolution, not exposed on `engine.Program`.
- A structurally wrong-shaped value bound into a transition and later used by an operation expecting a different `Value` type risks a nil/type-assertion panic at evaluation time, not a clean `ErrInputRejected`.
- Today this is not reachable by an untrusted caller: no authored game declares a named-typed `UserIntent` parameter yet (the one real Definition, `parques.json`, has none), and no live client-facing transport exists yet for `SubmitUserIntent` at all (Phase 2/WORK-0020 onward).

**Risk / opportunity**
- If a future authored game declares a named-typed `UserIntent` parameter, and/or once Phase 2 exposes `SubmitUserIntent` to real untrusted client input, this residual gap becomes a real availability risk (a crafted or buggy client payload panicking Turn execution) rather than a latent one.
- Fixing it properly (either exposing compiled `UserIntent` parameter types on `engine.Program`, or having the engine itself validate `SignalKindIntent.Fields` the way it already validates `InteractionAnswered.Answer`) is an engine-level change, out of `game/session`'s own scope.

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

## NOT NEEDED

The concern/technology/sophistication has been considered and is not justified under current conditions.

NOT NEEDED is not a permanent universal rejection. Include a reevaluation trigger where future conditions could change the assessment.

Persist NOT NEEDED items selectively. Do not fill the radar with every technology Playhoot could theoretically use.

A NOT NEEDED item is most useful when:

- the idea is likely to recur;
- recording why it is unnecessary helps prevent repeated architecture-astronautics;
- a future trigger for reconsideration is meaningful.

NOT NEEDED means "currently not justified", not "completed."

No items have yet been persisted under the Engineering Radar mechanism.

## Persisted Item Shape

Use descriptive headings rather than persistent IDs.

Expected shape:

### <Concern / Recommendation>

**Area**
<Observability / Reliability / Data / Security / etc.>

**Current state / evidence**
- <specific evidence from current implementation/canonical context>

**Risk / opportunity**
- <what could go wrong or what value is being missed>

**Recommendation**
- <concrete recommendation at the appropriate abstraction level>

**Why this horizon**
- <why NOW / SOON / LATER / NOT NEEDED>

**Reevaluate when**
- <trigger, when useful>
- None, only when genuinely no trigger is useful.

**Next process**
- <PRODUCT_DISCUSSION / ARCHITECTURE_DISCUSSION / DOMAIN_DESIGN / GUIDED_TECHNICAL_EXPLORATION / ENGINEERING_STANDARD / FEATURE_DEVELOPMENT / None>

**Last reviewed**
YYYY-MM-DD

Optional references may be included when useful:

- canonical path;
- ADR/PDR;
- engineering standard;
- WORK;
- implementation file.

Do not require empty boilerplate fields beyond those needed to make the recommendation understandable.

## Maintenance

The Radar is a living document. A later Principal Engineer Review may:

- add a meaningful new item;
- update evidence;
- update recommendation;
- move an item between horizons;
- change its reevaluation trigger;
- update Next process;
- remove an item that has been resolved or is no longer useful.

Do not create duplicate items for the same underlying concern merely because a new review occurred. Update the existing item.

Do not preserve obsolete items solely for history. Git history and completed WORK/decisions preserve historical information where appropriate.

When a concern is actually resolved, remove it from the current Radar unless there is still a meaningful remaining concern. Do not automatically move every resolved concern to NOT NEEDED.

Do not create a permanent resolved/archive section.

## Evidence and Scope

Radar recommendations must be grounded in current evidence from relevant canonical documentation, current-state domain/system documentation, code, tests, migrations, existing standards, or active work as appropriate.

Do not generate concerns merely from generic "best practices". For every material technology recommendation, apply the anti-overengineering questions from `docs/ai/OPERATING_MODEL.md`.

A Principal Engineer Review may be broad/system-wide or intentionally focused on a technical area.

When a review is scoped, do not silently reclassify unrelated existing Radar items as though they were reevaluated.

Only update:

- items actually reconsidered by the current review;
- newly discovered items in scope;
- items directly invalidated/resolved by evidence inspected in the review.

Absence of an item does not mean:

- the area was comprehensively reviewed;
- the area is guaranteed safe;
- the area has been permanently declared adequate.

Do not persist every adequate area as a Radar item. The Radar is not a compliance matrix.

## Next Process Routing

Radar recommendations should route to the process needed to reason about the next material step.

Typical routing:

- Product question -> `PRODUCT_DISCUSSION`
- Architecture/system design -> `ARCHITECTURE_DISCUSSION`
- Domain ownership/boundary -> `DOMAIN_DESIGN`
- Technical area that needs understanding/exploration -> `GUIDED_TECHNICAL_EXPLORATION`
- Reusable engineering rule -> `ENGINEERING_STANDARD`
- Concrete already-decided implementation -> `FEATURE_DEVELOPMENT`
- No currently justified follow-up -> `None`

Choosing a Next process does not automatically start that process.

## AI and Human Authority

Because Radar content is explicitly NON-AUTHORITATIVE, a Conversational AI may recommend radar items to add, update, move, or remove as part of a Principal Engineer Review, without a human-approval ceremony and without converting them into human-approved decisions. Persisting those changes to this file is a CODEBASE AGENT step, via a CODEBASE AGENT HANDOFF from the Conversational AI.

This does not grant authority to:

- accept architecture;
- accept product behavior;
- establish an engineering standard;
- approve WORK;
- implement the recommendation.

The human decides which recommendations deserve follow-up and remains final authority for material decisions.

The human may also challenge or override radar classifications.
