# play

`play` is the Live Session Coordinator: the connection-binding, delivery/fan-out boundary GAME-ADR-0002 accepts as living outside Session Runtime, owning no authoritative Session truth of its own. It is a top-level package, a sibling of `game/`, `api/`, `identity/` - not nested inside any of them (`docs/projects/active/session-runtime-v1/works/WORK-0005-thin-live-coordinator.md`).

It owns:

- An in-process, per-Session registry of bound live connections (local maps guarded by a mutex - GAME-ADR-0002's accepted no-Redis/no-distributed-routing stance for V1).
- Binding a connection to `(SessionUUID, UserUUID)` (`Coordinator.Bind`).
- Translating a decoded client command into a call against the `SessionRuntime` port it depends on, and fanning out the `Event`s a committed call produces to every currently-bound recipient connection (`Coordinator.Start`/`AnswerInteraction`).

It owns no database handle, no transaction, and no `game` business logic. `Create` and `Join` are forwarded directly to `SessionRuntime`; neither call itself touches the connection registry (`Join`'s own signature takes no `Conn`). `Create` happens before any Participant or connection exists. `Join`'s caller (`api/session`) does pair it with a connection in practice - it calls `Join`, and only once that succeeds does it upgrade the client's connection and call `Bind` separately - but that sequencing is the transport layer's responsibility, not something `Coordinator.Join` does or assumes on its own.

**Known gap (tracked as a Blocker on WORK-0005, not yet fixed)**: `game/README.md`'s accepted architecture states host and Participant are independent - a host is not automatically a gameplay Participant. Today, the only way to obtain a bound live connection at all is `GET /ws`, which unconditionally calls `Join` first - so a host must become an active roster Participant (counted against `playersMax`) merely to obtain a connection through which to send `Start`. This currently contradicts the accepted architecture; see `docs/projects/active/session-runtime-v1/works/WORK-0005-thin-live-coordinator.md`'s Blockers section and `docs/projects/active/session-runtime-v1/PROJECT.md`.

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

## Wire-protocol design principle: only Question/Presentation/Effect cross the boundary

A client is never told that some internal/domain fact happened as its own event - only three shapes of message ever cross the live-transport boundary: a **Question** opening/closing, a **Presentation** (a mounted UI component with its data) activating/updating/being removed, and an **Effect** (a presentation-only animation/sound). If some other fact matters to a client, it is because the backend already decided it changes what that client's UI shows - which means it must manifest as a Presentation update or an Effect addressed to that client, never as a raw "an interaction was answered" or "a turn committed" notification. This governs every `Output` this Coordinator is ever extended to fan out, not only the ones already wired (`docs/work/active/WORK-0006-broaden-live-fanout-effects-presentations.md`).

A session-wide fact that is not about any one recipient's UI state at all - for example, the whole Session ending - does not fit this model and is not forced into it as a fourth `Event` kind; it uses its own broadcast-to-everyone-bound mechanism instead (`docs/projects/active/session-runtime-v1/works/WORK-0007-session-termination-live-notification.md`), separate from `Event`/`Deliver`'s per-recipient contract.

## Delivery is best-effort, after commit (GAME-ADR-0020)

`Coordinator.Start`/`AnswerInteraction` call `SessionRuntime` first; only once that call has returned successfully do they fan its `Event`s out to bound connections. A `Conn.Deliver` failure is logged and never retried, queued, or fed back into Session Runtime - it never reopens, rolls back, or reinterprets the already-committed call that produced the `Event`. A recipient with no bound connection simply receives nothing; there is no resync/replay yet (planned in `docs/projects/active/session-runtime-v1/works/WORK-0015-disconnect-reconnect-full-resync.md`).

## What this package deliberately does not do yet

Disconnect grace/debounce, semantic presence, timer scheduling/reconciliation, an inactivity reaper, full process-loss recovery, and advanced reconnect semantics are all explicitly deferred to future WORK under `docs/projects/active/session-runtime-v1/` (see `PROJECT.md`). `Coordinator.Bind`'s rebind-replaces-outright behavior is the entire "disconnect" story today.
