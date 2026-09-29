# platform Logical Contract

Status: PACKAGE-LOCAL IMPLEMENTATION CONTRACT

Records the closed, Playhoot-owned vocabulary crossing the sandboxed JavaScript execution boundary: what drives an `execute` call and what its output may request. Successor role to the retired `game/language/v1/engine`'s own `Signal`/`Output` taxonomy.

## Two Extension Levels

- **Platform vocabulary** (this package's Event/Command kinds): finite, Go-owned, validated. Adding to it is a deliberate platform change - new file, new type, one registration call, nothing else edited.
- **Game vocabulary** (`PlayerEvent`/`SendEvent`'s own `Name`/`Payload`): arbitrary, owned entirely by each authored game, opaque to this package, requires zero platform code changes to extend.

## Presentation-Agnostic By Design

No member of this vocabulary describes a screen, a visual effect, a question, a card, a button, or any other rendering concept. A game's own action/data names and payloads are opaque; only the envelope around them (identity, size, recipients) is validated here. This is a deliberate constraint, not an oversight: the backend script owns game rules/data, the frontend script owns presentation/UX, and Playhoot owns session/platform concerns and transport - the three never trade places.

## Authority Boundary

Playhoot-authoritative, never trusted from a Command's own declared content: an Event's own kind, actor/participant/session identity, sequence/order, timer identity/expiration, disconnect/reconnect/leave facts, session cancellation, recipient validity, terminal session state. `ParseCommand`'s `KnownActors` argument enforces the recipient/actor half of this: a `SendEvent`/`PlayerEvent` naming an actor the caller never supplied is rejected, never trusted because the authored script or a client claimed it.

Game-authored, opaque to Playhoot: `PlayerEvent`/`SendEvent`'s own `Name`/`Payload`, and authoritative game state contents.

## `execute` vs. `project`

This package's Event/Command vocabulary belongs to `execute(state, event, context) -> {state, commands}`, the entry point that drives a Turn. The backend script's second entry point, `project(state, viewer, context) -> ClientState`, is pure and read-only - it cannot mutate state or emit a Command, so it has no vocabulary of its own beyond the opaque `ClientState` JSON it returns. This package does not own invoking `project` or verifying its privacy properties; that mechanism belongs to whichever WORK builds per-player view computation.

## What This Package Does Not Do

- Does not call the Executor, persist state, or dispatch a parsed Command to whatever mechanism owns it (timers, delivery, session-lifecycle transitions) - it only encodes Events and validates/decodes Commands.
- Does not validate a game-specific action/data contract - `PlayerEvent`/`SendEvent`'s `Name`/`Payload` are opaque to this package regardless of what a specific game expects them to contain.
- Does not enforce a resource limit beyond a provisional payload size bound; the platform's actual resource-limit policy is designed on its own terms elsewhere.
