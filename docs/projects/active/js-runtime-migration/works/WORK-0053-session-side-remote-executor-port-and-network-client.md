# WORK-0053: Session-Side Remote Executor Port & Network Client

Status: DONE
Created: 2026-09-27
Last status change: 2026-09-27 (IMPLEMENTING -> DONE: independent review APPROVED, no findings)

Related decisions:
- `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`

Canonical context:
- `game/session/jsexecutor/proto/executor.proto` (`WORK-0052`'s wire contract — the `Executor.Execute` RPC this WORK's client calls)
- `game/session/jsexecutor/README.md` (why this is a separate deployment placed inside `game/session/`, and why no domain besides Session may depend on it)
- `game/session/jsexecutor/internal/sandbox/LOGICAL_CONTRACT.md` (historical reference only, for the `ExecutionInput`/`ExecutionOutput`/error-type shape this WORK's port reuses — not importable from here, per Go's own `internal/` rule)

## Outcome

Introduce the `Executor` port at Session Runtime's own caller boundary and the production gRPC-client implementation of it, so Session Runtime depends only on an interface — never on `exec.Command`, a worker binary path, stdin/stdout, local PIDs, local scratch directories, `wazero`, QuickJS, gRPC status codes, or any other Executor-side implementation detail. This is the other half of `ADR-0016`'s split: `WORK-0052` built the separately deployed Executor; this WORK is what Session Runtime actually calls to reach it.

## Context

`WORK-0052` is DONE: a real gRPC service (`game/session/jsexecutor`) with a working `.proto` contract exists to generate a client against. This WORK does not design the wire contract — it consumes the one `WORK-0052` already shipped.

## Scope

### In Scope

- The `Executor` interface (Session-owned) and its `ExecutionContext`/`ExecutionInput`/`ExecutionOutput` types, reusing the same field shapes `game/session/jsexecutor/internal/sandbox` already established (proven, already-reviewed) rather than inventing a new shape.
- A production gRPC client implementing that interface: connects to a configured target address, marshals/unmarshals against `jsexecutor/proto`'s generated stubs, propagates the caller's `context.Context` deadline unchanged (gRPC's own deadline propagation — no bespoke timeout mechanism), and applies a narrow, explicitly-scoped retry policy (see Approved Design).
- Mapping the gRPC response back to the port's own vocabulary: a `Rejected` response becomes `*ScriptRejectedError`; a non-OK gRPC status becomes `*ExecutorError`; a `Success` response becomes a plain `ExecutionOutput`.
- A test double (`Executor` interface implemented purely in-memory, no network) for Session Runtime's own future tests (`WORK-0038` onward) that do not need a real Executor running.

### Out of Scope

- The Executor service itself, or anything about its internal sandbox mechanics — `WORK-0052`.
- Persisting execution results, or switching `sessionlifecycle`'s call sites to actually call this port — `WORK-0038`.
- Service discovery/load-balancing topology for reaching "any healthy Executor instance" (per `ADR-0016`) beyond what gRPC's own standard target-resolution already provides — deployment-specific (DNS, a service mesh, static address) and left to whoever configures the deployed target string; this WORK only requires that the client accept a configurable target, not a hardcoded one.

## Approved Design

- **Location**: `game/session/internal/executor/` — Session's own internal implementation detail (parallel to `internal/repo`, `internal/timers`), not part of `game/session`'s zero-dependency public contract root, since callers of that root contract do not need to know execution happens via gRPC.
- **Port**:

  ```go
  type ExecutionContext struct {
      LogicalTime time.Time
      RandomSeed  uint64
      ActingActor string
  }

  type ResolvedScript struct {
      Source string
  }

  type ExecutionInput struct {
      Script        ResolvedScript
      PreviousState json.RawMessage
      Event         json.RawMessage
      Context       ExecutionContext
  }

  type ExecutionOutput struct {
      NewState          json.RawMessage
      RequestedCommands []json.RawMessage
  }

  type Executor interface {
      Execute(ctx context.Context, in ExecutionInput) (ExecutionOutput, error)
  }
  ```

  Deliberately the same shape `game/session/jsexecutor/internal/sandbox` already uses internally — this is a proven contract, not a new design; reusing it means `WORK-0038` (the actual call-site migration) faces a familiar shape.

- **Errors**: `*ScriptRejectedError{Reason string}` (business-level: the script itself declined the input — maps from the gRPC response's `Rejected` field, never from a non-OK status) and `*ExecutorError{Reason string, Cause error}` (infrastructure-level: maps from any non-OK gRPC status). Callers distinguish them via `errors.As`, exactly as `WORK-0035`'s original `*ScriptRejectedError`/`*WorkerExecutionError` pair worked — renamed from `WorkerExecutionError` to `ExecutorError` since there is no "worker" concept visible from Session Runtime's side of this boundary anymore.
- **Retry policy**: because execution is a pure function (same inputs always produce the same outputs, no side effects occur inside the Executor), a request that provably never reached a worker is safe to retry blindly. Concretely: `codes.Unavailable` (connection-level failure — no healthy instance reached) is retried up to 2 additional attempts with a short fixed backoff (implementer's exact interval), *if* the caller's own deadline still allows it. `codes.DeadlineExceeded` is never retried (the caller's own budget is already exhausted — retrying would silently grant a second budget the caller never authorized) and is surfaced as `*ExecutorError` immediately. Every other non-OK status (`Internal`, `InvalidArgument`, etc.) is also not retried by this WORK — a real, reported failure from a successfully-reached server is not the "ambiguous, nothing happened" case retrying exists for; a future WORK may reconsider this if evidence shows otherwise.
- **Connection management**: one shared `*grpc.ClientConn` per `Executor` client instance (constructed once, reused across calls — `grpc.ClientConn` is safe for concurrent use by design), created from a configurable target address string passed to the constructor. Reading that address from an environment variable is Session Runtime's own composition-root responsibility, not this package's — the constructor itself takes a plain string so it stays testable without environment coupling.
- **Test double**: an in-memory `Executor` implementation (a simple function-backed fake, not a network stub) for tests that need to control `Execute`'s outcome deterministically without a real Executor process — this is a normal Go testing convention, not a gRPC concern.

## Constraints and Invariants

- Session Runtime's own non-test code must not import `wazero`, QuickJS, `exec`, or any Executor-side worker-management package, directly or transitively, once this WORK lands. It may import gRPC and `jsexecutor/proto`, since gRPC is the accepted transport, not an implementation detail being hidden.
- No correctness may depend on a retried request reaching the same Executor instance as the original attempt (retries are safe only because execution is stateless/pure — this invariant is what makes that true, and must not be violated by a future change that gives the Executor per-call state).
- A caller-supplied `context.Context` deadline must reach the gRPC call unchanged; the client must not silently extend, shorten, or ignore it (including during a retry — a retry attempt must respect whatever deadline remains, not reset the clock).

## Acceptance Criteria

- A `GRPCClient` constructed against a real, running Executor (per `WORK-0052`'s own test infrastructure or an equivalent fixture) correctly returns `ExecutionOutput` for a valid script, `*ScriptRejectedError` for a script that throws, and `*ExecutorError` for a deadline-exceeded runaway script.
- A client call against an unreachable target (nothing listening) retries up to the bounded limit and then returns `*ExecutorError` wrapping the final `Unavailable` status — verified by counting actual connection attempts, not merely checking the final error.
- A client call whose own `ctx` deadline expires does not retry and returns promptly (within the deadline, not after a further retry delay).
- The in-memory test double correctly implements `Executor` and can be configured to return each of the three outcomes (success, rejection, error) for use by future Session Runtime tests.
- `go build ./game/session/...` succeeds with no import of `wazero`, `github.com/fastschema/qjs`, or `os/exec` anywhere in non-test Session Runtime code outside `game/session/jsexecutor/`.

## Implementation Freedom

Exact backoff interval/jitter for the `Unavailable` retry, exact gRPC dial options (keepalive, message size limits), and exact test-double API shape (a struct with configurable function fields vs. a builder) are implementer's choices within this Approved Design.

## Verification

- `go test ./game/session/internal/executor/...` covering every Acceptance Criterion above.
- A real gRPC integration test standing up an actual `game/session/jsexecutor` server process (or an equivalent in-process gRPC server implementing the same `.proto` service, if spawning the real binary is impractical inside this test) to prove the client's wire-level behavior — unit-testing only against a hand-rolled fake server would leave the actual `.proto` compatibility unverified.
- Independent review per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`, given this removes Session Runtime's last remaining dependency on the old local-execution mechanics.

## Documentation Impact

### Accepted / Canonical Knowledge

- A new package-local contract document at `game/session/internal/executor/LOGICAL_CONTRACT.md` recording the port's contract, retry policy, and error semantics.

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md` — new row or update noting Session Runtime now has a real `Executor` port/client, though still uncalled by any lifecycle step until `WORK-0038` lands.

## Blockers

- None material. `WORK-0052` is DONE, delivering everything this WORK's design depended on.

## Completion Record

Implemented as designed. New package `game/session/internal/executor/` (`executor.go`, `grpcclient.go`, `fake.go`, `LOGICAL_CONTRACT.md`) gives Session Runtime a real caller-side port onto the separately deployed Executor: the `Executor` interface reusing `WORK-0035`'s proven `ExecutionInput`/`ExecutionOutput` shape, `GRPCClient` as the production implementation, and `Fake` as an in-memory test double.

The retry policy — bounded retry of `codes.Unavailable` only, never `DeadlineExceeded` or any reported failure — was independently verified, not just implemented: the reviewer hand-traced the loop, confirmed the incoming `ctx` (and its deadline) propagates unchanged through every attempt including retries, and specifically stress-tested the "safe to retry because execution is pure" safety argument (a resent request is byte-identical, and the Executor is stateless/has no side effects per `ADR-0016`, so a retried request producing an identical result on any instance holds regardless of which instance actually receives it).

Independent review: one round, APPROVED with no findings. Verification included a real `.proto`-wire-level test (a hand-written but genuine `pb.ExecutorServer` stub over `bufconn`, not a mocked Go interface) proving success passthrough, rejection passthrough (`*ScriptRejectedError`), deadline-exceeded-no-retry, Unavailable-retry-then-succeed and Unavailable-exhausts-retry (both asserting exact attempt counts), and any-other-status-no-retry.

Verification performed: `go build ./...`, `go vet ./...` clean; `go test ./game/session/internal/executor/...` and `go test ./...` repo-wide, both clean aside from the same pre-existing, unrelated root-package comment-standard drift already established as out of every WORK's scope in this initiative. Confirmed no import of `wazero`, `github.com/fastschema/qjs`, `os/exec`, or `game/session/jsexecutor/internal/...` anywhere in this new package — only the allowed `game/session/jsexecutor/proto` dependency.

`game/CURRENT_STATE.md` updated and confirmed accurate by review.

No caller in the repository invokes this port yet; wiring `sessionlifecycle`'s call sites to it — the actual production integration — is `WORK-0038`'s own scope, unchanged from what this WORK's own Context recorded before implementation began.
