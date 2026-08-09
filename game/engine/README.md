Wrote by AI, human dev notes added as `Dev note:`

# game/engine

The `engine` package compiles and executes game programs.

It consumes a source-level `program.Definition`, validates its semantics, and produces an immutable executable `engine.Program`.

The package also advances a game by applying one runtime signal to one snapshot and returning an atomic commit.

The core execution model is:

    Program + Snapshot + Signal -> Commit

The engine performs no external I/O.

`Dev note: this is one of the implementations of an engine, there could be as many implementations of engines as we want, "program" pkg exports a way to define a game, there could be as many ways to implement an engine as we want, this is just one of them`

## Responsibilities

The package owns:

- semantic compilation;
- symbol collection and resolution;
- type checking;
- control-flow validation;
- structured-concurrency validation;
- executable program representation;
- runtime values;
- game snapshots;
- workflow instances;
- workflow-local state;
- task groups;
- ask groups;
- pending questions;
- pending logical timers;
- signal dispatch;
- transition selection;
- expression evaluation;
- operation execution;
- workflow control;
- deterministic randomness;
- invariant evaluation;
- projection evaluation;
- declarative outputs;
- execution limits;
- transition traces;
- atomic commit creation.

## Source and executable programs

The package distinguishes between two concepts:

- `program.Definition`: the source-level authored representation;
- `engine.Program`: the validated, immutable, executable representation.

A source definition is not executed directly.

It must first be compiled:

    program.Definition
            |
            v
      engine.Compile
            |
            v
       engine.Program

The compiled program may resolve names into internal identifiers, normalize operations, precompute lookup tables, and remove source-level information that is not required during execution.

## Public API

The main exported entry points are:

- `Compile`: compiles a `program.Definition`;
- `Program`: an opaque and immutable executable program;
- `Program.NewSnapshot`: creates the initial state for one game instance;
- `Program.Step`: applies one signal to one snapshot;
- `Diagnostics`: compilation diagnostics;
- `CompileOptions`: compilation configuration;
- `Limits`: compilation and execution limits;
- `Snapshot`: an opaque snapshot of one game instance;
- `Signal`: one runtime input;
- `Commit`: the atomic result of one step;
- `Output`: a declarative external action;
- `Trace`: an explanation of one executed transition;
- exported engine error types and sentinel errors.

Where useful, the package may also export constructors for valid signals and runtime values.

## Program ownership

`engine.Program` owns the compiled representation of one game version.

A compiled program is:

- immutable;
- safe to share between game instances;
- safe to read concurrently;
- independent of any single session;
- independent of persistence;
- independent of network connections.

A single compiled program may be used by many snapshots:

    Parques Program v1
    ├── Snapshot A
    ├── Snapshot B
    ├── Snapshot C
    └── Snapshot D

The internal compiled representation is private to the package.

Other packages must not construct, mutate, or depend on internal workflow IDs, state IDs, opcodes, symbol tables, or instruction layouts.

## Snapshot ownership

A `Snapshot` represents the complete logical position of one game instance.

It includes the information required to continue execution, including:

- global game state;
- the root workflow instance;
- child workflow instances;
- workflow states;
- workflow-local state;
- task-group ownership;
- ask-group progress;
- pending questions;
- logical timers;
- deterministic random state;
- the current engine sequence.

Snapshots are opaque outside the package.

A step must not mutate the input snapshot in place. It produces a new snapshot inside the returned commit.

## Execution contract

`Program.Step` processes exactly one signal.

A successful step:

1. verifies that the snapshot belongs to the program;
2. resolves the target workflow;
3. selects one valid transition;
4. evaluates its guard;
5. executes its bounded operations;
6. applies one control result;
7. validates game invariants;
8. recalculates affected projections;
9. creates declarative outputs;
10. returns a new snapshot and trace as one commit.

The step is atomic from the caller's perspective.

If execution fails before commit creation:

- the original snapshot remains unchanged;
- no output is considered published;
- no partial state change is returned.

## Determinism

Given the same:

- compiled program;
- snapshot;
- signal;
- engine limits;

the engine must return the same commit.

The package must not read directly from:

- the system clock;
- the network;
- environment variables;
- databases;
- operating-system randomness;
- global mutable state.

Time enters the engine through explicit signal data.

Random behavior uses deterministic random state stored in the snapshot.

This makes game execution reproducible and supports replay, simulation, and debugging.

## Concurrency

A compiled `Program` may be shared safely across concurrent calls.

The engine does not serialize signals belonging to the same game instance.

The caller must ensure that two signals are not committed concurrently against
the same snapshot sequence.

A future session or application layer will own:

- per-session signal ordering;
- optimistic concurrency;
- retries;
- idempotency;
- persistence;
- output publication.

The engine only defines the deterministic result of one step.

## Compiler responsibilities

Compilation includes:

- declaration indexing;
- symbol resolution;
- built-in resolution;
- type checking;
- expression validation;
- operation validation;
- workflow-state validation;
- transition-target validation;
- result-type validation;
- signal compatibility validation;
- structured-concurrency validation;
- child-workflow ownership validation;
- ask-group policy validation;
- projection and UI binding validation;
- invariant compilation;
- execution-limit validation;
- generation of the internal executable representation.

Compilation returns diagnostics rather than failing on the first semantic problem whenever possible.

## Structured concurrency

Workflow instances form an ownership tree.

Except for the root workflow, every workflow instance has exactly one parent.

The engine enforces rules such as:

- a child belongs to one parent;
- child handles cannot be silently discarded;
- a parent cannot complete while owning unresolved children;
- children must be joined or cancelled;
- detached workflows are not supported;
- cancelling a parent cancels its owned descendants;
- task groups use explicit completion policies;
- workflow fan-out is bounded.

These rules are compiled and enforced by the engine.

## Outputs

The engine never performs external side effects.

Instead, it returns declarative outputs such as:

- open a question;
- close a question;
- schedule a timer;
- cancel a timer;
- activate a presentation;
- remove a presentation;
- publish a projection update;
- emit a UI effect;
- report workflow completion.

An external application layer will decide how to persist, schedule, or deliver those outputs.

Output variants form a closed set controlled by the engine package.

## UI execution

The engine owns the server-side semantics of UI declarations.

It may:

- determine active presentations;
- evaluate per-user projections;
- open typed questions;
- validate typed answers;
- produce projection changes;
- produce UI effects.

It does not render pixels, execute browser code, or manage client connections.

Client-side rendering consumes the declarative data produced by the engine.

## Private implementation

The package keeps the following details private:

- compiler implementation structs;
- symbol tables;
- source-to-IR mappings;
- internal type identifiers;
- workflow and state identifiers;
- executable instructions;
- opcodes;
- compiled expressions;
- compiled operations;
- mutable working snapshots;
- the execution machine;
- evaluation scopes;
- execution budgets;
- internal random-number generator state;
- trace builders;
- projection caches.

Only stable domain concepts are exported.

## Dependency rules

The intended dependency direction is:

    program <- engine

`engine` may import `program`.

`program` must never import `engine`.

The engine must not import:

- transport packages;
- database packages;
- session packages;
- HTTP or WebSocket handlers;
- authentication packages;
- application services;
- infrastructure adapters.
  `Dev note: So just as said before, this is just one implementation of the engine, engine will always depend on the program, but program doesnt care about the implementation of its consumer engines. Engine only cares about executing a program, but as explained before, it just provides an API to execute it, it doesnt actual handle the specific way in which we want to execute it (online with websockets, CMD, etc)`

## Non-responsibilities

The package does not own:

- game-session identifiers;
- user authentication;
- room membership;
- loading or saving snapshots;
- database transactions;
- signal ordering across requests;
- same-session concurrency control;
- retries after persistence conflicts;
- real timer scheduling;
- WebSocket delivery;
- HTTP or gRPC protocols;
- output outboxes;
- operational monitoring.

Those responsibilities belong to future application and infrastructure layers.

## File organization

`Dev note: this is initial organization given by AI, it might change after manual reviewing, so dont take it as source of truth, I might update organization and forget to update this doc`

### Compilation

- `compile.go` coordinates compilation.
- `diagnostic.go` defines compiler diagnostics.
- `compile_symbols.go` collects declarations and resolves names.
- `compile_types.go` performs type checking.
- `compile_control.go` validates workflows and structured concurrency.
- `compile_ui.go` validates projections, questions, views, and bindings.

### Executable representation

- `program.go` defines the exported immutable executable program.
- `ir.go` defines the private compiled representation.
- `value.go` defines runtime values.

### Runtime state and input/output

- `snapshot.go` defines opaque game snapshots.
- `signal.go` defines runtime signals.
- `commit.go` defines atomic step results.
- `output.go` defines declarative outputs.
- `trace.go` defines execution traces.
- `limits.go` defines compile-time and runtime limits.
- `errors.go` defines engine errors.

### Execution

- `machine.go` coordinates one execution step.
- `expression.go` evaluates compiled expressions.
- `operation.go` executes compiled operations.
- `workflow.go` manages workflow instances and control changes.
- `interaction.go` manages questions, ask groups, and task groups.
- `projection.go` evaluates visible models and presentation changes.
- `invariant.go` evaluates post-transition invariants.
- `random.go` provides deterministic random operations.

## Testing

Compiler tests should cover:

- symbol resolution;
- type errors;
- invalid transition targets;
- invalid workflow results;
- orphaned child workflows;
- invalid task-group policies;
- incompatible questions and answers;
- invalid UI bindings;
- invalid invariants.

Execution tests should cover:

- deterministic steps;
- atomic rollback on failure;
- workflow transitions;
- child completion and joins;
- ask-group completion;
- timeouts;
- cancellations;
- invariant failures;
- projection updates;
- execution-budget enforcement.

Replay tests should verify that the same initial snapshot and ordered signals
produce the same final snapshot, outputs, and traces.
