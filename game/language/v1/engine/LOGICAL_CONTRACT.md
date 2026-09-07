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
- the engine does not recursively execute multiple transitions inside one step.

## Why engine depends on program

`program` is designed to be an isolated package: it is only ever imported, and it never imports anything else in this repository. That isolation is what lets `program` stay a reusable, engine-agnostic language definition — usable by this engine or, in principle, by a different one.

`engine` exists to add compilation and runtime behavior on top of a `program.Definition`, so it depends on `program`. This dependency is one-directional (`program` never depends on `engine`), so it cannot create an import cycle.

This is also fine from a caller's perspective: to use the engine at all, a caller must already have a `program.Definition` to compile, which means it already imports `program`. The engine depending on `program` does not add a new dependency the caller didn't already need.
