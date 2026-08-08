Wrote by AI, human dev notes added as `Dev note:`

# game/program

The `program` package owns the source-level representation of the game programming language.

It defines the vocabulary used to describe a game, independently of its serialized format and independently of the engine that will execute it.

A `program.Definition` represents an authored game program. It may originate from JSON, YAML, an AI generation pipeline, an editor, or programmatic
construction.

This package does not compile or execute game programs.

## Responsibilities

The package owns the source definitions for:

- metadata and language-version information;
- user-defined types;
- built-in types and built-in capabilities;
- immutable resources;
- global state declarations;
- workflow declarations;
- workflow states and transitions;
- signal patterns;
- expressions;
- operations;
- workflow control operations;
- questions and ask-group policies;
- user intents;
- UI effects;
- projections;
- views and UI elements;
- presentations;
- local UI actions;
- invariants.

The package also owns serialization and deserialization of source definitions.

## Public API

The package exports the source model required to construct or inspect a game definition.

The main exported type is:

- `Definition`: the root source-level game definition.

Other exported types include:

- metadata and language-version declarations;
- type declarations and type references;
- resource and state declarations;
- workflow, state, transition, and signal declarations;
- expression variants;
- operation variants;
- control-operation variants;
- question and interaction declarations;
- projection and view declarations;
- UI element and UI action variants;
- invariant declarations;
- source-level errors;
- source encoding and decoding functions.

Interfaces representing closed language variants use unexported marker methods.
This prevents packages outside `program` from introducing unsupported expression, operation, control, or UI element implementations.

## Private implementation

The package keeps the following implementation details private:

- wire-format structs;
- JSON or YAML discriminator fields;
- custom union decoding;
- codec helpers;
- normalization helpers;
- default-value insertion;
- serialization compatibility logic;
- internal validation used only to decode a structurally valid document.

The wire representation is not the public language model.

Changing the serialized format must not require the engine to change, provided the decoded `Definition` remains semantically equivalent.

## Validation boundary

The package performs only source-format and structural validation required to produce a valid `Definition`.

Examples include:

- required fields are present;
- a discriminator identifies a supported source variant;
- malformed literal values are rejected;
- duplicate object keys in the source format are rejected when detectable;
- the document can be converted into the source model.

The package does not perform full semantic validation.

The following responsibilities belong to `engine.Compile`:
`Dev note: there might be different engines, this pkg shouldn't be dependent on any specific engine implementation, it should simply expose its game definition behavior, however a specific engine wants to validate, compile and execute is up to them`

- symbol resolution;
- duplicate declaration detection;
- type checking;
- reference validation;
- workflow control-flow validation;
- child-workflow ownership validation;
- question and response compatibility;
- projection and view compatibility;
- execution-limit validation;
- invariant compilation.

A definition may therefore be structurally decodable while still failing to compile.

## Built-in types

Platform-provided concepts such as `User` are represented as built-in language types.

The `program` package declares that these types and operations exist, but does not implement their runtime behavior.

The runtime meaning of built-in types and operations belongs to the `engine` package.
`Dev note: again, this shouldn't be coupled with the specific engine pkg, there could be many, consider this comment as just saying that those built-in types and operations belongs to the implementation of the engine consumer that we want, there could be muiltiple, even at some point we might decide to export this pkg as non internal pkg, so any dev in any project can define their own engine`

## Mutability

A source `Definition` is an authoring object.

It may be created, edited, decoded, transformed, or regenerated before compilation.

Once a definition has been compiled, modifying the original source object must not affect the resulting `engine.Program`.

The compiler is responsible for creating an independent, immutable executable representation.

## Dependency rules

`program` must not import `engine`.

The intended dependency direction is:

    program <- engine

The package should depend only on the Go standard library unless a serialization dependency is explicitly accepted.

It must not depend on:

- databases;
- network transports;
- WebSockets;
- session management;
- authentication;
- timers;
- persistence repositories;
- infrastructure adapters.
  `Dev note: this is true, but again, dont consider "engine" as the only specific engine pkg, but any consumer, the point is, program pkg doesnt care about how any consumer wants to use it`

## Non-responsibilities

This package does not own:

- executable instructions;
- compiled symbol identifiers;
- runtime values;
- workflow instances;
- game snapshots;
- runtime signals;
- transition execution;
- deterministic randomness;
- commits;
- traces;
- persistence;
- session concurrency;
- delivery of UI outputs.

## File organization

`Dev note: this is initial organization given by AI, it might change after manual reviewing, so dont take it as source of truth, I might update organization and forget to update this doc`

- `definition.go` defines the root game definition.
- `metadata.go` defines identity, version, and language metadata.
- `builtin.go` declares built-in types and built-in operation names.
- `types.go` defines user-defined types and type references.
- `state.go` defines resources and state declarations.
- `workflow.go` defines workflows, states, and transitions.
- `signal.go` defines source-level signal patterns.
- `expression.go` defines expression variants.
- `operation.go` defines operations and workflow control variants.
- `interaction.go` defines questions, intents, effects, and interaction policies.
- `ui.go` defines projections, views, elements, presentations, and UI actions.
- `invariant.go` defines game invariant declarations.
- `codec.go` exposes source encoding and decoding.
- `wire.go` contains private serialization structs and codec helpers.
- `errors.go` defines source-format and decoding errors.

## Testing

Package tests should focus on:

- source decoding;
- source encoding;
- round-trip preservation;
- union and discriminator handling;
- malformed document rejection;
- default-value behavior;
- wire-format compatibility.

Semantic compilation and execution behavior are tested by the `engine` package.
