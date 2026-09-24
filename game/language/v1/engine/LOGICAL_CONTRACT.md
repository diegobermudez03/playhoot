# Logical contract to preserve throughout development

The engine will expose three primary operations:

```
Definition
    → compile
    → immutable Program + diagnostics
```

```
Program + initialization input
    → initialize
    → initial Snapshot
```

```
Program + Snapshot + Signal
    → step
    → Commit
```

Accepted Session Runtime root initialization contract, not yet implemented as a
complete validated contract:

```text
players: list<user>
```

Session Runtime supplies `players` from active Participants at Start. Each
`user` represents the Session-local runtime identity derived from
SessionActorID, not Identity.UserUUID.

Accepted, not yet implemented, Operational Lifecycle contracts to preserve:

- `UserDisconnected`/`UserReconnected` are standard `NamedSignalSource`
  signals exposing only `user: user`, delivered to the one workflow
  instance a Session runs; handling is optional and an unhandled delivery
  has no automatic gameplay consequence and produces no RuntimeTurn (see
  `game/docs/decisions/GAME-ADR-0011-game-language-disconnect-reconnect-authored-semantics.md`).

Question, Ask Group, and Timer slots each have an implemented keyed family
(`KeyedQuestionSlot`/`KeyedAskGroupSlot`/`KeyedTimerSlot`) holding several
independently addressable, simultaneously pending occurrences per `(slot,
key)` instead of one occurrence per slot, exposing the authored key to a
matching transition and, for Question, to a mounted presentation's
projection arguments (see
`game/docs/decisions/GAME-ADR-0012-game-language-keyed-timer-slots.md` for
the original Timer case and
`game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md`
for the generalization). A keyed ask group's own configured presentation
compiles but is never mounted at runtime, matching the identical
pre-existing gap in the ordinary, non-keyed `AskGroupSlot` family it
mirrors. Presentation itself has no keyed family — deliberately deferred
pending a concrete demonstrated need.

A Commit represents, as a single unit:

- the new snapshot;
- declarative outputs;
- internal signals that must be processed later;
- the transition trace;
- the consumed signal.

Permanent constraints:

- no database integration;
- no WebSockets, HTTP, or gRPC;
- no real timer scheduling;
- no external clock or randomness during a step;
- no operational session management;
- deterministic execution;
- every step is atomic;
- Program is immutable and shareable;
- the engine does not recursively execute multiple transitions inside one step
  — a structural fact, not a policy constraint on a tree: a compiled Program
  has exactly one instantiated workflow for the lifetime of a Session (see
  `game/docs/decisions/GAME-ADR-0026-flat-workflow-execution-model-and-keyed-interaction-slots.md`),
  so there is no other instance a step could recurse into.

## Why engine depends on program

`program` is designed to be an isolated package: it is only ever imported, and it never imports anything else in this repository. That isolation is what lets `program` stay a reusable, engine-agnostic language definition — usable by this engine or, in principle, by a different one.

`engine` exists to add compilation and runtime behavior on top of a `program.Definition`, so it depends on `program`. This dependency is one-directional (`program` never depends on `engine`), so it cannot create an import cycle.

This is also fine from a caller's perspective: to use the engine at all, a caller must already have a `program.Definition` to compile, which means it already imports `program`. The engine depending on `program` does not add a new dependency the caller didn't already need.
