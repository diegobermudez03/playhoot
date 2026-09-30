# executor Logical Contract

Status: PACKAGE-LOCAL IMPLEMENTATION CONTRACT

Records Session Runtime's own caller-side port onto the separately deployed JavaScript Executor; see `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md` for the accepted architecture this implements, and `session/jsexecutor/internal/sandbox/LOGICAL_CONTRACT.md` for the execution semantics on the other side of this boundary.

## Operation

Two logical operations: `Execute(ctx, ExecutionInput{Script, PreviousState, Event, Context}) -> (ExecutionOutput, error)` and `Project(ctx, ProjectionInput{Script, State, Viewer, Context}) -> (ProjectionOutput, error)`.

- `Script`/`PreviousState`/`Event` (`Execute`) and `Script`/`State`/`Viewer` (`Project`) are opaque content, passed to the Executor unexamined. `ProjectionInput.State` is already viewer-scoped content the caller constructed before calling `Project` - this package never receives the full authoritative state for a `Project` call.
- `Context` carries the only sources of non-determinism authored code may observe.
- On success, `ExecutionOutput`/`ProjectionOutput` is exactly what the Executor returned.
- `error` distinguishes two categories: `*ScriptRejectedError` (the script itself declined the input — a business-level outcome) and `*ExecutorError` (the Executor was unreachable, the call failed, or the deadline was exceeded — an infrastructure-level failure). Callers must not conflate the two.

Callers depend only on the `Executor` interface, never on `GRPCClient` directly, so a test may substitute `Fake` without any network dependency.

## Retry Policy

Both operations are pure functions with no side effects: a request that never reached a worker is always safe to retry. `GRPCClient.Execute`/`GRPCClient.Project` each retry only a `codes.Unavailable` failure (connection-level — no healthy Executor instance reached), up to a bounded number of additional attempts, each waiting a short fixed backoff first, and only while the caller's own `ctx` has not already expired. Every other failure — including `codes.DeadlineExceeded` and any real, reported failure from a successfully-reached Executor (`codes.Internal`, etc.) — is surfaced immediately, never retried. Retrying a deadline failure would silently grant a second time budget the caller never authorized; retrying a reported failure assumes it was transient, which this client does not know to be true.

## What This Package Does Not Do

- Does not choose the network target/address — the caller supplies it to `NewGRPCClient`.
- Does not choose transport credentials — the caller supplies `grpc.DialOption`s; this package never defaults to an insecure connection.
- Does not implement service discovery or load balancing beyond what the supplied target/gRPC's own resolution provides.
- Does not validate the business meaning of `RequestedCommands` — that remains a separate concern for whichever caller (Session Runtime's own lifecycle steps) consumes them.
