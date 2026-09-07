# ADR-0009: Game Language Root Player Roster Contract

Status: ACCEPTED
Created: 2026-09-06
Last status change: 2026-09-06
Supersedes: None
Superseded by: None

## Context

Session Runtime starts a running game from a lobby roster. The current Game Language already has a `players` policy on `program.Definition`, a built-in `user` type, engine initialization through root parameters, and `engineservice.NewSnapshot` returning the required first signal for `Step`.

A contract gap remained: there was no accepted standardized way for Session Runtime to provide the actual Session participant roster to the root workflow. Without a standard root input, authored games could invent arbitrary parameter names and shapes, making Session Runtime and Game Language integration inconsistent.

This decision changes the Game Language public initialization contract but does not implement compiler/runtime changes.

## Decision

Game Language accepts a standardized platform-provided root input:

```text
players: list<user>
```

Semantics:

- Session Runtime builds this value from active Participants at Start.
- Each `user` corresponds to the Session-local runtime identity derived from `SessionActorID`.
- `Identity.UserUUID` is never exposed to authored Game Language.
- Games should not invent arbitrary custom root parameter names merely to receive the participant roster.

For V1, do not introduce arbitrary externally supplied game-specific root parameters until an explicit Session Configuration capability is designed.

Start uses the Session's pinned immutable Game definition/version, compiles/loads that exact pinned definition, constructs the platform-provided `players: list<user>` value from active Participants, calls engine initialization, and processes the engine's required initial signal/first Commit according to the accepted engine contract.

This decision preserves the existing engine contract that initialization produces the initial Snapshot plus required first signal, and that `Step` produces atomic Commits with Snapshot, Outputs, InternalSignals, Trace, and ConsumedSignal. The detailed Runtime Turn transaction/output protocol remains the next architecture milestone.

## Rationale

Every multiplayer game needs a stable way to know which session-local actors are in the game at start. Standardizing the root `players` input keeps authored definitions and Session Runtime integration from drifting into game-specific conventions.

Using the built-in `user` type preserves Game Language's session-local identity model. It avoids leaking `Identity.UserUUID` into authored games while still letting Session Runtime translate to and from global User identity at the boundary.

Deferring arbitrary external root parameters avoids accidentally designing Session Configuration while solving the narrower roster problem.

## Alternatives Considered

### Let each game choose its own participant root parameter

Rejected. It creates inconsistent conventions and forces Session Runtime or authoring tools to infer game-specific roster wiring.

### Expose Identity.UserUUID to Game Language

Rejected. Game Language should operate on session-local runtime actors, not global identity identifiers.

### Add arbitrary root configuration now

Rejected. Game-specific configuration is a separate capability and should be designed explicitly rather than smuggled into Start.

### Store roster only in Session Runtime and keep it out of root workflow parameters

Rejected. Authored game logic commonly needs the starting participant roster for turn order, presentation targets, questions, and initial state. The engine already supports root parameters as the initialization boundary.

## Consequences

- Canonical Game Language docs must describe `players: list<user>` as the accepted platform-provided root roster input.
- Current implementation/docs may include examples, but implementation still needs explicit validation/support work before this contract can be treated as implemented.
- Authored games should use `players` rather than custom root parameter names for the roster.
- Runtime Turn design must still decide how subsequent external signals, InternalSignals, Snapshots, outputs, timers, idempotency, and transaction boundaries interact.

## Canonical Knowledge Impact

- `game/language/v1/program/README.md` - records the accepted authored-definition/root-parameter contract.
- `game/language/v1/engine/README.md` - records the accepted Session Runtime initialization contract.
- `game/language/v1/engine/LOGICAL_CONTRACT.md` - records the accepted root roster initialization contract without claiming implementation completion.
- `game/README.md` - references the Game Language roster contract from Session Runtime Start.

## Implementation Impact

Future implementation work must validate and support the standardized `players: list<user>` root input, ensure engine `user` identity represents Session-local actors, and align Start with initialization/first Commit persistence. No compiler, engine, Session Runtime, migration, or WORK change is authorized by this record.
