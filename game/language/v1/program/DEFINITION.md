Wrote by AI, human dev notes added as `Dev note:`

# Game definition authoring rules (for an AI author)

This document is written to be pasted into a prompt, alongside a human's game description (see `GAME_BRIEF.md` for the structured intake format that description should already follow), for an AI whose job is to produce a `program.Definition` as JSON.

It has two jobs, in order:

1. Tell you, the authoring AI, **what is and isn't buildable** on this platform — so you can refuse or push back on a request *before* trying to force it into a shape that will never validate, rather than producing broken JSON and hoping it slips through.
2. Tell you **exactly what JSON to produce** when a request *is* buildable — every field name, every discriminator string, every gotcha that a strict, unforgiving decoder will reject.

Your output is always exactly one JSON object: a `program.Definition`. Nothing else — no prose, no markdown fences, no explanation — unless you are refusing or asking a clarifying question instead of producing a definition.

## 1. What kind of game this platform can express

A game here is a **deterministic, signal-driven, turn/step-based state machine**, not a continuous simulation. Concretely:

- The whole game is a tree of **workflow instances** (finite-state machines), each reacting to one **signal** at a time, one at a time, all the way down. There is no continuous time, no physics, no "every frame" tick, no concurrent execution within one game instance.
- Everything that changes state does so inside a **transition**: match a signal, optionally check a guard, run a bounded list of operations, then decide what happens next (stay, move to another state, or end). There are no `on_enter`/`on_exit`/mount/unmount hooks on a state — if something needs to happen when a state is entered, put it in the transition's own operations, not in the destination state.
- Randomness only happens through an explicit `draw_random` operation, backed by a seed the platform supplies once per game instance. There is no other source of randomness, and no way to read a real clock, the network, or any external data mid-game.
- Time only enters through explicit signals: a scheduled timer *output* is a request for the surrounding application to actually wait and deliver a `timer_expired` signal back later. The engine itself never waits for anything.
- Every player-visible piece of information is either the result of a **projection** (a pure, per-viewer transformation of authoritative state) rendered through a **view**, or a **question** you explicitly open and wait for an answer to. There is no other way for a client to learn game state, and no way for a client to change state except by submitting a **user intent** or answering a question.

If the description you're given requires continuous motion/physics, an always-on background process unrelated to any signal, client-side authority over any rule, or reading live external data mid-game, **that part is not buildable here** — say so plainly instead of approximating it with a workaround, unless the human brief has already reframed it as a discrete, signal-driven equivalent (e.g. "roll to move" instead of "watch the piece slide").

### Other hard limits worth knowing before you commit to a design

- **No private, separate per-player storage.** There is one shared, mutable "global state" (visible to the whole compiled program) plus per-workflow-instance local state and parameters. If a design needs one player to hold secret information other players can't see (a hidden hand of cards, a secret role), that information still has to live in global state — privacy is enforced entirely by **projections** deciding what to reveal to which viewer, never by the engine hiding storage from itself. Say this explicitly when it's relevant, since it affects how you shape global state (e.g. "every player's hand lives in one global map, and each player's own projection filters out everyone else's").
- **Recursive/self-referential types and resources are not supported.** A named type that refers back to itself (directly, or through a list/map/optional), and a resource whose value depends on itself (directly or through another resource), are compile errors, not something to work around with a trick.
- **Match cases are never required to be exhaustive.** A `match` (expression, operation, or control) that doesn't cover every case is legal — it just means "nothing happens" / "no result" for the uncovered cases. If a design needs exhaustiveness, cover every case explicitly yourself; nothing enforces it for you.
- **No arbitrary code.** Every computation is one of the fixed `Expression`/`Operation` variants below — no user-defined control flow beyond `if`, `for_each` (bounded), and `match`, no external function calls, no reflection.
- **Bounded execution.** One step's operations, one loop's iterations, how deep the child-workflow tree can nest, and how many interaction slots (pending questions/timers/children/groups) one workflow instance can hold at once are all capped by the executing application (generous defaults: 10,000 operations, 10,000 loop iterations, depth 8, 256 active slots per instance). Don't design a mechanic that depends on unbounded recursion, unbounded fan-out, or a loop with no natural termination.
- **No built-in asset type.** Images, sounds, and any other binary/external asset are referenced only as opaque string values — see "Asset references" below.
- **Structured concurrency, not free-form async.** A child workflow, an ask group, or a task group is always owned by exactly one parent, is always spawned into a statically named slot declared up front (never a dynamic/computed slot name), and — the one non-obvious rule — a parent that completes automatically discards every child/group it still owns, without needing them to be individually joined or cancelled first. Use this model (slots declared per workflow, one child per named slot, or a task group for a runtime-determined number of homogeneous children) for anything resembling a sub-process, mini-game, or "wait for N players to do something."

Everything else — types, state, workflows, questions, UI — is described in detail below.

## 2. The shape of a `Definition`, in the order you should design it

Design (and it's fine to also *emit*) roughly in this order, since each layer only refers to names declared at or before it:

1. **`types`** — every enum, record, union, and "new type" (nominal wrapper) you'll need. Built-in types (`unit`, `bool`, `number`, `string`, `user` — a connected player) never need declaring.
2. **`players`** — the explicit player-count contract for lobbies using this definition: `{"min": number, "max": number}`. The session layer reads this before start/join, so never leave game-specific player counts implicit in workflow logic.
3. **`resources`** — immutable, load-time constants (board layouts, card definitions, scoring tables, rule config, and asset-reference tables — see below). Not part of mutable state; evaluated once.
4. **`global_state`** — the one mutable record every workflow instance can read/write, declared as a list of typed fields with initializer expressions.
5. **`functions`** — pure, reusable, named computations (no side effects, no interaction with a live session).
6. **`invariants`** — boolean conditions over global state (and resources) checked after every committed transition; a violated invariant rejects the whole step atomically. Use these for rules that must never be false ("score is never negative"), not for ordinary game logic.
7. **`projections`** — pure, per-viewer transformations of state into whatever that viewer is allowed to see. This is the privacy boundary — design one whenever a UI or a question needs to show something to a specific user.
8. **`views`** — declarative client UI trees, each a pure function of one projection's output plus the view's own client-local state.
9. **`presentation_slots`** — named places on the client a view can be mounted into (e.g. `"hud"`, `"modal"`, `"board"`).
10. **`user_intents`** — typed actions a player can submit unprompted (e.g. "Roll", "PlayCard").
11. **`questions`** — reusable request contracts a workflow can open and later receive a validated answer to.
12. **`effects`** — purely cosmetic, client-facing presentation events (an animation, a sound cue) — never authoritative.
13. **`workflows`** (plus `root_workflow` naming which one starts the game) — the actual state machines: parameters, local state, slots (question/ask-group/timer/child-workflow/task-group), presentations, states, and transitions.

## 3. JSON encoding rules (read this before writing any JSON)

- Every field name is **snake_case**, exactly as shown in the reference below — not the CapitalCase Go names you might infer from documentation elsewhere.
- Any concept that's "one of several variants" (a type reference, an expression, an operation, a workflow control, a signal source, a match pattern, a random generator, a completion policy, a UI layout/element/action) encodes as a JSON **object with a required `"kind"` string field** plus that variant's own fields. Getting a `"kind"` string wrong (wrong string, wrong case, or missing) is a hard decode error — use the exact strings from the tables below.
- **A number literal is written as a JSON string**, not a JSON number: `{"kind": "number_literal", "value": "3.5"}`, not `{"kind": "number_literal", "value": 3.5}`. This preserves the authored value exactly; get this wrong and decoding fails immediately.
- **Optional variant-typed fields are always present, explicitly `null` when absent** — never omit the key. For example, a transition with no guard is `"guard": null`, not an absent `guard` key. Conversely, an absent key also decodes as "nothing" — but always prefer writing the explicit key with `null` for clarity and to avoid ambiguity with a genuine JSON error.
- **Some structs are never allowed to be `null`, even when "empty" or "not used"**: `metadata`, `global_state` (and `local_state` anywhere it appears), any `Block` (a transition's `operations`, an `if`'s `then`/`else`, a `for_each`'s `body`, a match case's `body`), a transition's `signal`, and any UI element's `configuration`. When one of these is conceptually "unused," write it as its empty-but-present form instead of `null` — e.g. an operation-less block is `{"operations": []}` (or `{"operations": null}`, both decode as "no operations" — but never `null` for the `Block` itself), and unused UI configuration is `{"properties": [], "events": []}`.
- **The one nullable plain (non-variant) object** is a slot's `presentation` field (on a question slot or ask-group slot) — `null` means "this interaction has no client-facing presentation of its own" (a headless question/ask). Every other plain object (not a `"kind"`-tagged variant) must be a real object when present.
- **Arrays**: write `[]` for "no items, but the collection conceptually exists" and reserve `null` only when you genuinely mean "nothing here at all" — both decode fine, but `[]` is the clearer, preferred choice for "zero of something."
- `item_name`/`index_name` fields (on `for_each`, `repeat`, and the list-query expressions) are plain strings, always present — use `""` for "no index binding needed," never omit the field or use `null`.
- Unknown/extra fields anywhere are a hard decode error — don't add fields that aren't in the tables below, even ones that seem harmless or descriptive.
- The whole payload must be exactly one JSON object — no trailing content, no comments (JSON has none), no wrapping in another object or array.

## 4. Full field reference

### Type reference (`TypeReference`)

| kind | fields |
|---|---|
| `builtin` | `type`: one of `"unit"`, `"bool"`, `"number"`, `"string"`, `"user"` |
| `named` | `name`: string (must name a declared type) |
| `list` | `element`: TypeReference |
| `map` | `key`: TypeReference, `value`: TypeReference |
| `optional` | `element`: TypeReference |

### Type declaration (`TypeDeclaration`, top-level `types[]`)

| kind | fields |
|---|---|
| `enum` | `name`, `values`: `[{"name": string}, ...]` |
| `record` | `name`, `fields`: `[{"name": string, "type": TypeReference}, ...]` (every field always present, unlike a union) |
| `union` | `name`, `variants`: `[{"name": string, "fields": [{"name","type"}, ...]}, ...]` (exactly one variant active at a time, chosen by name) |
| `new_type` | `name`, `underlying`: TypeReference (a distinct nominal type, not interchangeable with `underlying` even though they share a representation) |

### Expression (`Expression`) — pure, value-producing

| kind | fields |
|---|---|
| `unit_literal` | (none) |
| `bool_literal` | `value`: bool |
| `number_literal` | `value`: **string** (e.g. `"42"`, `"3.5"`) |
| `string_literal` | `value`: string |
| `optional_none` | `element_type`: TypeReference |
| `optional_some` | `value`: Expression |
| `list` | `element_type`: TypeReference, `elements`: [Expression] |
| `map` | `key_type`, `value_type`: TypeReference, `entries`: `[{"key": Expression, "value": Expression}, ...]` |
| `enum_value` | `type_name`, `value_name`: string |
| `record` | `type_name`: string, `fields`: `[{"name": string, "value": Expression}, ...]` |
| `union` | `type_name`, `variant_name`: string, `fields`: `[{"name","value"}, ...]` |
| `new_type` | `type_name`: string, `value`: Expression |
| `reference` | `name`: string (a bound name — parameter, `for_each` item, reserved root like `"global"`/`"resources"`, etc.) |
| `field` | `target`: Expression, `field`: string |
| `index` | `target`: Expression, `index`: Expression |
| `unary` | `operator`: `"not"` \| `"negate"`, `operand`: Expression |
| `binary` | `operator`: `"add"` \| `"subtract"` \| `"multiply"` \| `"divide"` \| `"modulo"` \| `"equal"` \| `"not_equal"` \| `"less"` \| `"less_or_equal"` \| `"greater"` \| `"greater_or_equal"` \| `"and"` \| `"or"` \| `"in"` \| `"not_in"`, `left`, `right`: Expression |
| `conditional` | `condition`, `then`, `else`: Expression |
| `call` | `function`: string (names a declared function), `arguments`: `[{"name","value"}, ...]` |
| `match` | `value`: Expression, `cases`: `[{"pattern": MatchPattern, "result": Expression}, ...]` |
| `list_map` | `collection`: Expression, `item_name`, `index_name`: string, `result`: Expression |
| `list_filter` | `collection`, `item_name`, `index_name`, `predicate`: Expression |
| `list_flat_map` | `collection`, `item_name`, `index_name`, `result` (must itself produce a list, flattened one level) |
| `list_any` | `collection`, `item_name`, `index_name`, `predicate` |
| `list_all` | `collection`, `item_name`, `index_name`, `predicate` |
| `list_count` | `collection`, `item_name`, `index_name`, `predicate` |
| `list_first` | `collection`, `item_name`, `index_name`, `predicate` |

### Operation (`Operation`) — one synchronous step inside a `Block`

| kind | fields |
|---|---|
| `let` | `name`: string, `type`: TypeReference, `value`: Expression |
| `set` | `target`: AssignmentTarget, `value`: Expression |
| `list_append` | `target`, `value` |
| `list_insert` | `target`, `index`: Expression, `value` |
| `list_remove_at` | `target`, `index` |
| `map_put` | `target`, `key`: Expression, `value` |
| `map_delete` | `target`, `key` |
| `if` | `condition`: Expression, `then`: Block (never null), `else`: Block (never null; empty `{"operations":[]}` if unused) |
| `for_each` | `collection`: Expression, `item_name`, `index_name`: string, `body`: Block |
| `match` | `value`: Expression, `cases`: `[{"pattern": MatchPattern, "body": Block}, ...]` |
| `open_question` | `slot`: string, `recipient`: Expression (must be statically `user`), `arguments`: [CallArgument] |
| `close_question` | `slot`: string |
| `emit_effect` | `effect`: string, `recipients`: Expression (list of `user`), `arguments`: [CallArgument] |
| `schedule_timer` | `slot`: string, `delay_milliseconds`: Expression |
| `cancel_timer` | `slot`: string |
| `spawn_child_workflow` | `slot`: string, `arguments`: [CallArgument] |
| `cancel_child_workflow` | `slot`: string, `reason`: Expression |
| `open_ask_group` | `slot`: string, `recipients`: Expression (list of `user`), `arguments`: [CallArgument], `completion`: AskGroupCompletionPolicy |
| `finalize_ask_group` | `slot`: string |
| `cancel_ask_group` | `slot`: string |
| `begin_task_group` | `slot`: string, `completion`: TaskGroupCompletionPolicy |
| `spawn_task_group_child` | `slot`: string, `key`: Expression (must match the slot's declared key type), `arguments`: [CallArgument] |
| `seal_task_group` | `slot`: string (mandatory before the same transition ends — see structured-concurrency notes) |
| `finalize_task_group` | `slot`: string |
| `cancel_task_group` | `slot`: string, `reason`: Expression |
| `draw_random` | `name`: string (binds the drawn value for later operations in the same block), `generator`: RandomGenerator |

`AssignmentTarget`: `{"kind": "name", "name": string}` \| `{"kind": "field", "target": AssignmentTarget, "field": string}` \| `{"kind": "index", "target": AssignmentTarget, "index": Expression}`.

`RandomGenerator`: `{"kind": "random_integer", "minimum": Expression, "maximum": Expression}` (inclusive integer range) \| `{"kind": "random_element", "collection": Expression}` \| `{"kind": "random_shuffle", "collection": Expression}`.

`AskGroupCompletionPolicy`: `{"kind": "all_responses"}` \| `{"kind": "first_response"}` \| `{"kind": "quorum", "count": Expression}`.

`TaskGroupCompletionPolicy`: `{"kind": "all_terminal"}` \| `{"kind": "first_terminal"}` \| `{"kind": "quorum_terminal", "count": Expression}`.

### Workflow control (`WorkflowControl`) — how a transition ends

| kind | fields |
|---|---|
| `goto` | `state`: string (must name a state in the same workflow) |
| `stay` | (none) |
| `complete` | `result`: Expression (must match the workflow's `result_type`) |
| `fail` | `error`: Expression (must be statically string) |
| `cancel` | `reason`: Expression (must be statically string) |
| `conditional` | `condition`: Expression, `then`, `else`: WorkflowControl |
| `match` | `value`: Expression, `cases`: `[{"pattern": MatchPattern, "control": WorkflowControl}, ...]` |

### Signal source (`SignalSource`) — what a transition reacts to

| kind | fields |
|---|---|
| `named` | `name`: string — platform/lifecycle signals; `"WorkflowStarted"` is the one every root workflow should handle to actually begin |
| `user_intent` | `intent`: string (names a declared user intent) |
| `question_answered` | `slot`: string (names a question slot) |
| `timer_expired` | `slot`: string (names a timer slot) |
| `child_completed` / `child_failed` / `child_cancelled` | `slot`: string (names a child-workflow slot) |
| `ask_group_completed` | `slot`: string |
| `task_group_completed` | `slot`: string |

A `SignalPattern` (used as a transition's `signal`, never null) is `{"source": SignalSource, "bindings": [{"field": string, "name": string}, ...]}` — `bindings` extracts named fields from the signal's payload into new local names usable in the guard/operations.

### Match pattern (`MatchPattern`)

| kind | fields |
|---|---|
| `wildcard` | (none) — matches anything |
| `enum_value` | `type_name`, `value_name`: string |
| `union_variant` | `type_name`, `variant_name`: string, `bindings`: `[{"field","name"}, ...]` — binds the matched variant's fields to new local names |
| `optional_none` | (none) |
| `optional_some` | `binding`: string — the name the unwrapped value is bound to |

### UI (`UILayout`, `UIElement`, `UIAction`)

`UILayout`: `{"kind":"stack"}` \| `{"kind":"absolute"}` \| `{"kind":"linear","direction":"row"|"column","gap":Expression|null}` \| `{"kind":"grid","columns":Expression,"row_gap":Expression,"column_gap":Expression}`.

| UIElement kind | fields |
|---|---|
| `empty` | (none) |
| `container` | `configuration`, `layout`: UILayout, `children`: [UIElement] |
| `text` | `configuration`, `value`: Expression (must be statically string) |
| `image` | `configuration`, `source`: Expression (an asset reference — see below), `alternative_text`: Expression \| null |
| `button` | `configuration`, `children`: [UIElement] |
| `repeat` | `collection`: Expression, `item_name`, `index_name`: string, `key`: Expression \| null, `body`: UIElement |
| `conditional` | `condition`: Expression (must be statically bool), `then`, `else`: UIElement |

`UIElementConfiguration` (never null) = `{"properties": [{"name": string, "value": Expression}, ...], "events": [{"event": "click"|"double_click"|"pointer_enter"|"pointer_leave", "actions": [UIAction, ...]}, ...]}`.

`UIAction`: `{"kind":"set_local_state","target":AssignmentTarget,"value":Expression}` (target must root at `"local"`) \| `{"kind":"answer_question","value":Expression}` (only valid inside a view reached through a question) \| `{"kind":"emit_user_intent","intent":string,"arguments":[CallArgument]}`.

### Everything else (plain objects, never a `"kind"` field)

```
Definition = {
  "metadata": Metadata,                            // never null
  "types": [TypeDeclaration],
  "players": PlayerPolicy,
  "resources": [ResourceDeclaration],
  "global_state": StateDeclaration,                // never null
  "functions": [FunctionDeclaration],
  "invariants": [InvariantDeclaration],
  "projections": [ProjectionDeclaration],
  "views": [ViewDeclaration],
  "presentation_slots": [PresentationSlotDeclaration],
  "user_intents": [UserIntentDeclaration],
  "questions": [QuestionDeclaration],
  "effects": [EffectDeclaration],
  "root_workflow": string,
  "workflows": [WorkflowDeclaration]
}
// this exact key order — matches what the codec itself emits.

Metadata               = { "id", "name", "description", "version", "language_version": string }
PlayerPolicy           = { "min": number, "max": number }                   // use max 0 only for no upper bound
StateDeclaration       = { "fields": [StateFieldDeclaration] }               // never null
StateFieldDeclaration  = { "name": string, "type": TypeReference, "initializer": Expression }
ResourceDeclaration    = { "name": string, "type": TypeReference, "value": Expression }
FunctionDeclaration    = { "name": string, "parameters": [FieldDeclaration], "result_type": TypeReference, "body": Expression }
InvariantDeclaration   = { "name": string, "condition": Expression }
ProjectionDeclaration  = { "name": string, "parameters": [FieldDeclaration], "result_type": TypeReference, "body": Expression }
ViewDeclaration        = { "name": string, "model_type": TypeReference, "local_state": StateDeclaration, "root": UIElement }
PresentationSlotDeclaration = { "name": string }
UserIntentDeclaration  = { "name": string, "parameters": [FieldDeclaration] }
QuestionDeclaration    = { "name": string, "parameters": [FieldDeclaration], "response_type": TypeReference, "validation": Expression | null }
EffectDeclaration      = { "name": string, "parameters": [FieldDeclaration] }
FieldDeclaration       = { "name": string, "type": TypeReference }
CallArgument           = { "name": string, "value": Expression }
Block                  = { "operations": [Operation] }                       // never null

WorkflowDeclaration = {
  "name": string,
  "parameters": [FieldDeclaration],
  "result_type": TypeReference,
  "local_state": StateDeclaration,                 // never null
  "question_slots": [QuestionSlotDeclaration],
  "ask_group_slots": [AskGroupSlotDeclaration],
  "timer_slots": [TimerSlotDeclaration],
  "child_slots": [ChildWorkflowSlotDeclaration],
  "task_group_slots": [TaskGroupSlotDeclaration],
  "presentations": [PresentationDeclaration],
  "initial_state": string,
  "global_transitions": [TransitionDeclaration],
  "states": [WorkflowStateDeclaration]
}
WorkflowStateDeclaration = { "name": string, "presentations": [PresentationDeclaration], "transitions": [TransitionDeclaration] }
TransitionDeclaration    = { "name": string, "signal": SignalPattern, "guard": Expression | null, "operations": Block, "control": WorkflowControl | null }

QuestionSlotDeclaration      = { "name": string, "question": string, "presentation": QuestionPresentationDeclaration | null }
AskGroupSlotDeclaration      = { "name": string, "question": string, "presentation": QuestionPresentationDeclaration | null }
QuestionPresentationDeclaration = { "slot": string, "projection": string, "projection_arguments": [CallArgument], "view": string }
TimerSlotDeclaration         = { "name": string }
ChildWorkflowSlotDeclaration = { "name": string, "workflow": string }
TaskGroupSlotDeclaration     = { "name": string, "workflow": string, "key_type": TypeReference }

PresentationDeclaration = { "name": string, "slot": string, "targets": Expression, "projection": string, "projection_arguments": [CallArgument], "view": string }
```

## 5. Asset references

`program` has no built-in asset type — an image, sound, or any other external asset a view references is just an opaque **string**, produced the same way any other string value would be: usually `{"kind": "string_literal", "value": "<asset id>"}`, or a lookup into a `resources` table keyed by name if you want one place that maps logical names to asset ids.

Convention to follow: the human brief you're given lists every asset with a **stable id** (see `GAME_BRIEF.md`'s asset block). Use that exact id as the string value everywhere the asset is referenced (an `image` element's `source`, an effect's argument, wherever). Never invent your own id, never embed a real file path or URL unless the brief explicitly gave you one — the id is resolved to an actual file by whatever client/application layer consumes this definition, not by anything in this package or the engine. If the brief describes a visual/audio need but gives no asset id for it, ask for one rather than guessing.

## 6. What happens to your output

Your JSON is decoded strictly (`gameservice.DecodeJSON`) — any unknown field, wrong `"kind"`, or structurally wrong shape fails immediately with a path-pointing error (e.g. `$.workflows[0].states[2].transitions[0].control`). It is then checked against the language's own rules (`gameservice.Validate` — type/operator compatibility, duplicate names) and finally compiled by the engine (name resolution, full type checking, structured-concurrency and UI-binding validation). If you're given error output from any of these stages, treat the `Path` as pointing exactly at the offending part of the JSON you produced, and fix that node without regenerating the whole definition from scratch.

`gameservice.Validate` passing does **not** guarantee the definition compiles — it never resolves references, scope, or exhaustiveness. Don't treat a clean `Validate` pass as proof you're done; expect compiler feedback as a normal part of getting a definition right, the same way you'd expect a real compiler's errors when writing any other program.
