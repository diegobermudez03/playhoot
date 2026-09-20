# play

`play` is the Live Session Coordinator: the connection-binding, delivery/fan-out boundary GAME-ADR-0002 accepts as living outside Session Runtime, owning no authoritative Session truth of its own. It is a top-level package, a sibling of `game/`, `api/`, `identity/` - not nested inside any of them (WORK-0005).

It owns:

- An in-process, per-Session registry of bound live connections (local maps guarded by a mutex - GAME-ADR-0002's accepted no-Redis/no-distributed-routing stance for V1).
- Binding a connection to `(SessionUUID, UserUUID)` (`Coordinator.Bind`).
- Translating a decoded client command into a call against the `SessionRuntime` port it depends on, and fanning out the `Event`s a committed call produces to every currently-bound recipient connection (`Coordinator.Start`/`AnswerInteraction`).

It owns no database handle, no transaction, and no `game` business logic. `Create`/`Join` are forwarded directly to `SessionRuntime` with no connection involved (they happen before any live connection exists).

## The dependency-inversion boundary

`play`'s own package and everything it exports (`Coordinator`'s methods, every struct/interface type in `play.go`) depend only on primitives and `play`-owned types - **never on a `game` package** (`sessionlifecycle`, `engine`, etc.). This is what breaks the circular-dependency risk between `play` and `game`, and what lets them become separate deployment units later by swapping only the `SessionRuntime` implementation for a network client - no change to `play`'s own exported API, hub/registry/fan-out logic, or to `game` at all.

`play` declares the `SessionRuntime` port it depends on:

```go
type SessionRuntime interface {
    Create(ctx context.Context, gameUUID, hostUserUUID, idempotencyKey string) (CreatedSession, error)
    Join(ctx context.Context, joinCode uint, userUUID, displayName, idempotencyKey string) (JoinResult, error)
    Start(ctx context.Context, sessionUUID, userUUID, idempotencyKey string) (StartResult, error)
    AnswerInteraction(ctx context.Context, interactionUUID, userUUID string, answer []byte) (AnswerInteractionResult, error)
}
```

The concrete implementation, `play/sessionruntime`, **is expected to import `game`** (`sessionlifecycle.Manager`, `engine`, `engineservice`) - that is normal and required, since its whole job is bridging to `game`'s real types and translating a committed `RuntimeTurn`'s `OpenQuestionOutput`/`CloseQuestionOutput` Outputs into the `Event` values `Coordinator` fans out. It is deliberately not part of `play`'s own package/exported surface, so `play` itself stays free of any `game` import. Where a `SessionRuntime` implementation physically lives is Implementation Freedom - `play/sessionruntime` today, but it could equally be a different package, or eventually an HTTP/gRPC client to a separately-deployed Session Runtime, without `play` itself changing.

Verify the boundary is real, not just described here:

```sh
go list -deps ./play              # must contain no game/... package
go list -deps ./game/...          # must contain neither play nor api
```

## No shared database transaction with `game`

`play` never receives, stores, or passes through a `*gorm.DB`/transaction handle. Every mutation it triggers happens by calling the `SessionRuntime` interface; the implementation's own call into `sessionlifecycle.Manager` owns that call's entire transaction scope exactly as it already does today, entirely on the `game` side of the boundary. `play/sessionruntime` does hold its own `*gorm.DB`, used only to read back already-durably-committed `session_interactions`/`sessions` rows once `Manager`'s own call has returned - never to share a transaction with it.

## Delivery is best-effort, after commit (GAME-ADR-0020)

`Coordinator.Start`/`AnswerInteraction` call `SessionRuntime` first; only once that call has returned successfully do they fan its `Event`s out to bound connections. A `Conn.Deliver` failure is logged and never retried, queued, or fed back into Session Runtime - it never reopens, rolls back, or reinterprets the already-committed call that produced the `Event`. A recipient with no bound connection simply receives nothing; there is no resync/replay in this slice (Slice 7).

## What this slice deliberately does not do

Disconnect grace/debounce, semantic presence, timer scheduling/reconciliation, an inactivity reaper, full process-loss recovery, and advanced reconnect semantics are all explicitly deferred to Slices 5-8. `Coordinator.Bind`'s rebind-replaces-outright behavior is this slice's entire "disconnect" story.
