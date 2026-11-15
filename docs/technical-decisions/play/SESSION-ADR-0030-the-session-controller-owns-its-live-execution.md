# SESSION-ADR-0030: The Session Controller Owns Its Live Execution

Status: ACCEPTED
Created: 2026-10-18
Last status change: 2026-10-18
Supersedes: None
Superseded by: None

## Context

The first Runtime implementation split one live Session across three components:

- a `liveconnection` Manager, which admitted connections and also orchestrated Start: durable checks, frontend signing, creating the sandbox context, running `start()`, committing, aborting and adopting a Session already RUNNING;
- an internal, state-only `controller` package holding the Session's memory (phase, connections, lobby, roster, the confirmed game mirror, SAMPLE timeline), exposing loose steps (`BeginStart`, `CancelStart`, `CompleteStart`, `AbortStart`, `TakeBoundary`, `Requeue`, `AcknowledgeOnly`, `CommitBoundary`, `AbortRunning`) that are only correct when called in the right order;
- a `gameloop` Manager, which took over a started game: ticks, executing SAMPLE batches in the sandbox, retries, restores, aborting and replay offers.

The protocol of a Session lived outside the owner of its state. Reading one operation meant following it across the three components. Both workflows called the sandbox, each with its own copy of the restore and project-everyone logic. The sandbox context was created by one component and replaced by another, and nobody discarded it when the Session ended. The controller depended on its callers using its steps correctly.

Session Runtime was split from the durable Session (`play/session`) precisely so that `play/runtime` would be the real-time execution of a live Session, which is what the controller is.

## Decision

1. **The Session controller is the live Session.** One controller per Session this instance owns. It owns the Session's in-memory state, its sandbox execution context and every operation on it: admission into its lobby, SAMPLE ingress, Start (including adoption of a Session the database already has RUNNING), the running game's ticks, retries, restores and aborts. It is the only component that talks to the sandbox for its Session.

2. **The controller has dependencies, not callers that orchestrate it.** It receives the durable Session operations it needs (start eligibility, commit, aborts, loading a running Session), frontend signing, the sandbox, the replay recorder and a clock, and calls them itself. Its state transitions are private. From outside it only exposes logical operations, through the connection callbacks.

3. **Only the controller locks its state.** Its mutex is private. Fast operations (admission, a SAMPLE, a disconnect) run on the caller's goroutine under a short lock. Slow work is never done holding the lock.

4. **One main goroutine per controller, from creation until it ends.** Everything that touches the sandbox or makes a durable write (Start, adoption, each tick, aborting) runs on it, which makes it the Session's single mutation lane. A Start requested on an admin connection is handed to that goroutine and the caller waits for its outcome. The tick is a timer the main goroutine selects on. It is not a separate component. SAMPLEs arriving while the main goroutine executes a batch go to the next batch, so ingress never waits for the sandbox.

5. **A resolver is the only non-controller operation.** The Runtime's entry point only exists because a new connection has no controller yet. It only finds or creates the Session's controller under the ownership epoch, without asking the durable Session anything, and hands the attach to it. A controller may therefore exist for a connection that is then rejected. The controller checks durable eligibility itself and reads its manifest on the first admission that needs it. The resolver also stops every controller on shutdown. Once attached, a connection's callbacks are bound directly to its controller.

6. **Placement.** The controller lives in `play/runtime/workflows/liveconnection` as an unexported type, next to the resolver (that package's `ControllerResolver`), with one file per operation. The `play/runtime` root stays the zero-dependency public contract (`docs/engineering/standards/domain-logic-placement.md`). Callers receive the callback structs (`runtime.AdminConnection`, `runtime.PlayerConnection`) and never the controller. Pure logic shared by its operations stays in `play/runtime/internal/business/*`.

## Rationale

- The owner of a live Session's state is the only component that can keep its protocol correct. Making that owner also run the protocol means one operation reads in one place, and no caller can call steps in the wrong order.
- A controller that is a dependency of the workflows cannot call the durable Session or the sandbox, so its orchestration had to live in whoever held it. That is how one Session ended up split across two orchestrators.
- One owner for the sandbox context gives it one lifecycle: created at Start or adoption, replaced on retries, discarded when the Session ends.
- A single goroutine per Session doing all slow, ordered work is the single mutation lane the plan requires. Keeping fast ingress off it keeps SAMPLE latency independent of script execution.

## Alternatives Considered

### Keep the split and extract a shared sandbox executor

It removes the duplicated restore and project logic, but the protocol still lives outside the state owner, and the context still has no single owner across Start and the running game.

### Move Start and adoption into the game loop

It gives the sandbox one caller, but the state stays a passive object used by two components, and Start's durable orchestration ends up in a component named for ticks.

### Fixed service methods taking a connection identifier instead of bound callbacks

Inbound messages would call the Runtime with a connection id, and the Runtime would look up the controller each time. It was rejected because the controller is the Runtime: a callback bound to it says exactly that and needs no lookup. Outbound messages need callbacks either way.

### Exporting the controller type

Rejected: no caller needs it. The callbacks are the contract.

## Consequences

- `play/runtime/internal/controller` and `play/runtime/workflows/gameloop` stop existing as separate components.
- The controller's tests use mocks of its dependencies (durable Session, sandbox, replay) in the repository's table style. Its pure state behavior can still be tested without them.
- Every live controller has a goroutine, idle in its lobby. Ending lobbies that are never used is still pending (inactivity and lifetime are not built).
- Future live operations (commands, timers, `update()`, recovery, cancelling) are added as operations of the controller, on its main goroutine when they touch the sandbox or the database.

## Canonical Knowledge Impact

- `play/README.md` - the Runtime's internal structure: one controller per live Session owns its state, execution and sandbox context; the resolver only finds or creates it.

## Implementation Impact

A refactor without changing behavior: merge the state-only controller, the Start orchestration and the game loop into the controller in `play/runtime/workflows/liveconnection`, and keep the public Runtime contract and the API edge unchanged.
