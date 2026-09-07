Wrote by AI, human dev notes added as `Dev note:`

# game/language/v1/program

`program` defines the source-level language for describing a game: types, state, workflows, expressions, UI, and everything else an author writes down. It's a pure, in-memory data model — an AST, not a compiler or a runtime. It doesn't run games, doesn't talk to a database or the network, and doesn't know anything about any specific engine that consumes it.

Encoding, decoding, and validating a `Definition` are not part of `program` itself — that behavior lives in the sibling package `game/language/v1/program/gameservice`, built on top of `program`'s types.

```go
import (
    "github.com/diegobermudez03/playhoot/game/language/v1/program"
    "github.com/diegobermudez03/playhoot/game/language/v1/program/gameservice"
)

def, err := gameservice.DecodeJSON(data)
if err != nil { ... }

if errs := gameservice.Validate(*def); len(errs) > 0 { ... }

out, err := gameservice.EncodeJSON(*def)
```

## What you get from `gameservice`

- **`EncodeJSON(program.Definition) ([]byte, error)`** — serializes a `Definition` to compact JSON. Deterministic: encoding the same value twice always produces identical bytes. No validation is performed.
- **`DecodeJSON(data []byte) (*program.Definition, error)`** — parses JSON into a `Definition`. Structurally strict (unknown fields and unrecognized discriminators are rejected), but semantically permissive — a decoded definition can still be invalid in the ways `Validate` checks for.
- **`DecodeError`** — the error type returned by `DecodeJSON` on malformed JSON. It carries a path (like `$.workflows[0].states[2].transitions[0]`) pointing at exactly where decoding failed.
- **`Validate(program.Definition) []error`** — checks a `Definition` against the language's own rules: operator/operand type compatibility, named-type resolution, and duplicate names within a namespace (two types, two workflows, two questions with the same name, etc.). Returns `nil` if nothing is wrong. Each error is a `*gameservice.ValidationError` with a `Path` and `Message`.

`Validate` is intentionally narrow. It does **not** resolve references, lexical scope, or anything that depends on where a name is used at runtime (e.g. whether `ReferenceExpression{Name: "foo"}` actually refers to something in scope). That's the job of the engine package that compiles a `Definition`. A `nil` result from `Validate` means the definition doesn't break any rule `program`/`gameservice` itself owns — it does not mean the definition will compile.

## The core type: `Definition`

Everything hangs off one struct:

```go
type Definition struct {
    Metadata Metadata

    Types []TypeDeclaration

    Resources   []ResourceDeclaration
    GlobalState StateDeclaration

    Functions   []FunctionDeclaration
    Invariants  []InvariantDeclaration
    Projections []ProjectionDeclaration
    Views       []ViewDeclaration

    PresentationSlots []PresentationSlotDeclaration

    UserIntents []UserIntentDeclaration
    Questions   []QuestionDeclaration
    Effects     []EffectDeclaration

    RootWorkflow string
    Workflows    []WorkflowDeclaration
}
```

A `Definition` is just a value — build it by hand, decode it from JSON, mutate it, copy it, whatever. Nothing about it is tied to a live game session.

One thing worth knowing up front: declarations are stored as **slices, not maps**, everywhere in this model (types, fields, cases, operations, ...). This is deliberate — it preserves the order the author wrote things in, and it lets a duplicate name exist in the source model instead of silently overwriting an earlier entry. `Validate` is what catches duplicates; `program` itself doesn't.

## Closed variant types

A recurring pattern: several concepts are "one of a fixed set of variants," modeled as a Go interface that only types inside this package can implement (an unexported marker method). This is just `program`'s way of guaranteeing you can type-switch over every case exhaustively — you can't accidentally (or intentionally) add your own `Expression` or `Operation` type from outside the package. As a caller, you mostly just need to know these interfaces exist and switch over the listed variants:

| Interface                   | Represents                                 | Variants                                                                                                                                                                                                                                                                                                                                                                                |
| --------------------------- | ------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `TypeReference`             | the type of a value                        | `BuiltinTypeReference`, `NamedTypeReference`, `ListTypeReference`, `MapTypeReference`, `OptionalTypeReference`                                                                                                                                                                                                                                                                          |
| `TypeDeclaration`           | a user-defined type                        | `EnumTypeDeclaration`, `RecordTypeDeclaration`, `UnionTypeDeclaration`, `NewTypeDeclaration`                                                                                                                                                                                                                                                                                            |
| `Expression`                | a pure, value-producing computation        | literals, constructors, `ReferenceExpression`, `FieldExpression`, `IndexExpression`, `UnaryExpression`, `BinaryExpression`, `ConditionalExpression`, `CallExpression`, `MatchExpression`, and the list-query expressions (`ListMapExpression`, `ListFilterExpression`, `ListFlatMapExpression`, `ListAnyExpression`, `ListAllExpression`, `ListCountExpression`, `ListFirstExpression`) |
| `Operation`                 | one synchronous step inside a transition   | `LetOperation`, `SetOperation`, list/map mutations, `IfOperation`, `ForEachOperation`, question/ask-group/child-workflow/task-group/timer operations, `DrawRandomOperation`, `MatchOperation`                                                                                                                                                                                           |
| `AssignmentTarget`          | a writable location                        | `NameTarget`, `FieldTarget`, `IndexTarget`                                                                                                                                                                                                                                                                                                                                              |
| `WorkflowControl`           | how a transition ends                      | `GotoControl`, `StayControl`, `CompleteControl`, `FailControl`, `CancelControl`, `ConditionalControl`, `MatchControl`                                                                                                                                                                                                                                                                   |
| `SignalSource`              | what a transition reacts to                | `NamedSignalSource` (platform signals), `UserIntentSignalSource`, `QuestionAnsweredSignalSource`, `TimerExpiredSignalSource`, `ChildCompletedSignalSource`, `ChildFailedSignalSource`, `ChildCancelledSignalSource`, `AskGroupCompletedSignalSource`, `TaskGroupCompletedSignalSource`                                                                                                  |
| `MatchPattern`              | what a match case matches against          | `WildcardMatchPattern`, `EnumValueMatchPattern`, `UnionVariantMatchPattern`, `OptionalNoneMatchPattern`, `OptionalSomeMatchPattern`                                                                                                                                                                                                                                                     |
| `RandomGenerator`           | how `DrawRandomOperation` produces a value | `RandomIntegerGenerator`, `RandomElementGenerator`, `RandomShuffleGenerator`                                                                                                                                                                                                                                                                                                            |
| `AskGroupCompletionPolicy`  | when an ask group is done                  | `AskGroupAllResponsesPolicy`, `AskGroupFirstResponsePolicy`, `AskGroupQuorumPolicy`                                                                                                                                                                                                                                                                                                     |
| `TaskGroupCompletionPolicy` | when a task group is done                  | `TaskGroupAllTerminalPolicy`, `TaskGroupFirstTerminalPolicy`, `TaskGroupQuorumTerminalPolicy`                                                                                                                                                                                                                                                                                           |
| `UILayout`                  | how a container arranges its children      | `StackLayout`, `AbsoluteLayout`, `LinearLayout`, `GridLayout`                                                                                                                                                                                                                                                                                                                           |
| `UIElement`                 | a node in a client UI tree                 | `EmptyElement`, `ContainerElement`, `TextElement`, `ImageElement`, `ButtonElement`, `RepeatElement`, `ConditionalElement`                                                                                                                                                                                                                                                               |
| `UIAction`                  | what a UI event handler does               | `SetLocalStateAction`, `AnswerQuestionAction`, `EmitUserIntentAction`                                                                                                                                                                                                                                                                                                                   |

## Concepts you'll actually work with

### Types

`BuiltinType` covers `unit`, `bool`, `number`, `string`, and `user` (a platform-provided session-local runtime actor). Everything else is author-defined via `TypeDeclaration`: enums, records (all fields present), unions (exactly one variant, chosen by name), and "new types" (a nominal wrapper distinct from its underlying type, unlike an alias).

## Accepted Session Runtime Root Roster Contract

Status: ACCEPTED DESIGN, NOT YET IMPLEMENTED AS A COMPLETE VALIDATED CONTRACT.

When Session Runtime starts a game from a lobby, it provides the root workflow with the standardized platform-provided roster input:

```text
players: list<user>
```

Authored games should use `players` for the participant roster rather than inventing arbitrary custom root parameter names for that purpose.

Each `user` value represents a Session-local runtime actor derived from Session Runtime's `SessionActorID`; it is not `Identity.UserUUID` and must not expose global identity to authored Game Language.

For V1, arbitrary externally supplied game-specific root parameters are deferred until a Session Configuration capability is explicitly designed.

Rationale and alternatives are recorded in `docs/decisions/architecture/ADR-0009-game-language-root-player-roster-contract.md`.

### Resources vs. global state

`ResourceDeclaration` is immutable, load-time data — constants, not something that changes during a game. `Definition.GlobalState` is the mutable state that exists once per game session and can be read/written by workflow transitions.

### Workflows

A `WorkflowDeclaration` is a deterministic, signal-driven finite-state machine: parameters, local state, some slots (see below), a set of states, and transitions. Every state change happens through exactly one `TransitionDeclaration`, which matches a `SignalPattern`, binds fields from the signal, checks an optional `Guard`, runs a `Block` of `Operation`s, and finishes with exactly one `WorkflowControl` (go to another state, stay, complete, fail, or cancel).

### Slots

Several concepts are modeled as "slots" — durable, statically named locations owned by a workflow instance that hold something in-flight: `QuestionSlotDeclaration` (a pending question), `AskGroupSlotDeclaration` (a pending multi-recipient ask), `TimerSlotDeclaration` (a pending timer), `ChildWorkflowSlotDeclaration` (one named child workflow), `TaskGroupSlotDeclaration` (a dynamically sized collection of homogeneous children). A slot name is always a static, source-level string — never a runtime expression. Which one to use depends on shape: one child with an individually-handled outcome → `ChildWorkflowSlotDeclaration`; a runtime-determined number of same-type children with one aggregated outcome → `TaskGroupSlotDeclaration`; a one-step ask to one or more users → `AskGroupSlotDeclaration`.

### Questions, user intents, effects

These are the three ways a workflow talks to players. `QuestionDeclaration` is a reusable request contract a workflow can open (with optional `Validation` logic) and later observe an answer to via `QuestionAnsweredSignalSource`. `UserIntentDeclaration` is a typed action a player can submit unprompted. `EffectDeclaration` is a client-facing presentation event (an animation, a sound) — never authoritative state, purely cosmetic.

### Projections and views

`ProjectionDeclaration` is a pure, per-viewer transformation from authoritative state into whatever a specific client is allowed to see — this is the language's privacy boundary. `ViewDeclaration` is a reusable, declarative client UI tree (`UIElement`) that's a pure function of a projection's output plus its own client-local state. `PresentationDeclaration` (and its question-specific cousin `QuestionPresentationDeclaration`) ties a set of target users, a `PresentationSlotDeclaration` (a named place on the client, like "hud" or "modal"), a projection, and a view together.

### Functions and invariants

`FunctionDeclaration` is a pure, deterministic, reusable computation — no mutation, no interaction with a live session. `InvariantDeclaration` is a boolean condition over global state and resources, checked after every committed transition; violating one rejects the whole step, unlike an authored `FailControl`/`CancelControl`, which are ordinary outcomes.

## Mutability

A `Definition` is an authoring value: build it, decode it, edit it, throw it away, whatever you need. There's no hidden shared state and no lifecycle to manage — it only becomes meaningful once something (such as the current compiler/engine) consumes it.

## What this package is not

`program` doesn't validate references or lexical scope, doesn't check that a workflow name in `RootWorkflow` actually exists, doesn't execute anything, and doesn't know about randomness streams, snapshots, sessions, or persistence at runtime. Those belong to the engine package built on top of this model.
