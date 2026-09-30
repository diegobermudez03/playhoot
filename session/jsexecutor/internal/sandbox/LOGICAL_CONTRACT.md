# sandbox Logical Contract

Status: PACKAGE-LOCAL IMPLEMENTATION CONTRACT

Records the sandboxed JavaScript execution boundary's implemented contract; see `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`, `game/docs/decisions/GAME-ADR-0030-sandbox-runtime-quickjs-wasm-wazero-process-isolated-workers.md`, and `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md` for the accepted architecture this implements.

## Operation

Two logical operations, sharing the exact same worker process/isolation model (Execution Model, Isolation Guarantees below apply identically to both):

- `Execute(ctx, ExecutionInput{Script, PreviousState, Event, Context}) -> (ExecutionOutput, error)`.
- `Project(ctx, ProjectInput{Script, State, Viewer, Context}) -> (ProjectOutput, error)`.

For `Execute`:
- `Script` is already-resolved authored JavaScript source. This package does not fetch, cache, compile-ahead, or version it.
- `PreviousState`/`Event` are opaque JSON values, passed through to the script unexamined.
- `Context` carries the only sources of non-determinism authored code may observe: `LogicalTime`, `RandomSeed`, `ActingActor`. The sandbox never exposes real wall-clock time or OS randomness.
- On success, `ExecutionOutput.NewState`/`RequestedCommands` are exactly what the script's `execute` function returned, unexamined for business meaning.

For `Project`:
- `Script` is the same authored source `Execute` uses - both entry points live in one script.
- `State` is already viewer-scoped, caller-constructed content. This package never sees, and has no way to reach, whatever the caller's own privacy-filtering step excluded from it - `Project` is never handed the full authoritative state.
- `Viewer` is an opaque identifier, echoed to the script unexamined, the same way `Context.ActingActor` is for `Execute`.
- On success, `ProjectOutput.ClientState` is exactly what the script's `project` function returned - unlike `ExecutionOutput`, no fixed shape is required or validated.

For both: `error` distinguishes two categories: `*ScriptRejectedError` (the script threw, returned a malformed shape (`Execute` only) or no value at all (`Project`), or defines no `execute`/`project` function — a business-level outcome) and `*WorkerExecutionError` (the worker process crashed, the IPC exchange failed, or the deadline was exceeded — an infrastructure-level failure). Callers must not conflate the two.

This package is `jsexecutor`'s own internal implementation, invoked by its gRPC handler (`jsexecutor/internal/grpcserver`). It has no caller outside `jsexecutor`.

## Script Contract

An authored script must define a global function:

```js
function execute(previousState, event, context) {
  // ...
  return { newState: /* ... */, requestedCommands: [/* ... */] };
}
```

`context` is `{ logicalTime: <RFC3339Nano string>, randomSeed: <decimal string>, actingActor: <string> }`. `randomSeed` is a string, not a JS number, because a Go `uint64` can exceed `Number.MAX_SAFE_INTEGER`; authored code needing numeric random behavior must derive its own generator from this seed rather than treat it as a plain number.

A script may additionally define a second global function, invoked only by `Project`, never by `Execute`:

```js
function project(state, viewer, context) {
  // ...
  return /* any JSON-serializable ClientState value */;
}
```

`state` here is `Project`'s own `State` field - already viewer-scoped, never the full authoritative state `execute` receives as `previousState`. `context` for `project` carries only `logicalTime`/`randomSeed` - no `actingActor` (`viewer` above already names whose projection this is). Unlike `execute`, `project`'s return value has no required shape: whatever JSON-serializable value it returns becomes `ClientState` verbatim. `project` cannot mutate authoritative state or request platform Commands - it has no way to (its own return value is never fed back as `previousState`, and is never parsed for Commands).

## Execution Model

Each `Execute`/`Project` call spawns a fresh worker process (a re-exec of the same `jsexecutor` binary in worker mode, `RunAsWorkerIfRequested`), writes one JSON request to its stdin, and reads one JSON response from its stdout. The worker instantiates a fresh QuickJS-on-WebAssembly (`wazero`) runtime, evaluates the script, invokes `execute` or `project` (per the request's own operation), and exits. No state is retained between invocations — both operations are stateless as a library.

This spawn-per-call shape is deliberately the simplest mechanism sufficient to prove the process boundary and language contract both work. A future hardening pass may replace the process-management mechanics (pooling, reuse, resource-limit enforcement, forced termination) behind these same `Execute`/`Project` signatures — no caller needs to change when that lands.

## Isolation Guarantees

Every guarantee below was independently verified against the pinned QuickJS-on-`wazero` binding's actual behavior (not merely assumed from its documentation), because the binding was found, twice, to expose more ambient host capability by default than its own stated design implied.

- **Filesystem** is confined to a fresh, empty, per-invocation scratch directory, created by the caller (`Execute`, not the worker itself — see Execution Model) before spawning the worker and removed after the worker process ends, however it ends. The underlying binding always mounts *some* directory as the sandbox's filesystem root, defaulting to the host process's own real working directory if left unconfigured, so this confinement is an active choice this package makes, not the binding's own default behavior. A script can still read/write/list within that scratch directory during its own single invocation — that directory contains nothing when the script starts and is destroyed once the invocation ends, so this residual capability never exposes anything real.
- **Host environment variables** are unreachable: the sandbox runtime's own guest environ is already empty by construction (independent of the host process's own environment), and `Execute` additionally starts the worker process itself with no environment at all (`cmd.Env` set to empty) as defense-in-depth against that current behavior ever changing, or against anything else in the worker binary reading a real host environment variable it should not. `std.getenv` itself turned out to be a non-configurable property this package cannot actually remove from the language, so this guarantee rests on the process/runtime boundary, not on hiding that one function.
- **Real elapsed time** is unreachable: `Date`/`Math.random` are replaced (see below), and `performance`/`os` — both of which exposed real, live timing (`performance.now()`, `os.now()`) despite the binding's default isolation stance — are removed from the global scope entirely before the script runs.
- Authored code cannot obtain real wall-clock time or OS randomness even by calling the language's own ambient `Date`/`Math.random` directly: a prelude evaluated before the script replaces both with versions derived from `ExecutionContext`.
- A script-level exception or panic inside the worker never propagates as a Go panic to the caller; it is always converted to a `workerResponse` (`Rejected` or `Fatal`) before the worker's own process exits.
- A worker process crash (non-zero exit, no valid JSON on stdout) is surfaced to the caller as a `*WorkerExecutionError`, never a crash of the caller's own process.
- A caller-supplied `context.Context` deadline is enforced by killing the entire worker process, verified against a script that never yields control back to the runtime (a plain infinite loop) — this is the mechanism this package's design depends on, since the sandbox runtime's own cooperative interruption is not reliable against exactly this case. `jsexecutor`'s gRPC handler passes the incoming request's own context through unchanged, so a client-supplied gRPC deadline propagates all the way to this kill mechanism.
- **Network** access is assumed unreachable based on the binding's own stated default (no network host function is imported by this package), but this specific claim has not been independently probed the way filesystem/timing/environment were — given this binding's track record above, treat it as unverified rather than certain until it is.
- **Memory limit (`qjs.Option.MemoryLimit`, 64 MiB default) is real and enforced, and fails gracefully.** Adversarially verified (WORK-0036) against a script that doubles a string exponentially (`s = s + s` in a tight loop): the runtime throws a catchable `InternalError: out of memory` well before the execution-time deadline, surfaced through the existing "`execute()` threw" path as a clean, business-level `*ScriptRejectedError` — never a worker crash, never a hang.
- **Stack limit (`qjs.Option.MaxStackSize`, 1 MiB default) is real and enforced, but does not fail gracefully.** Adversarially verified (WORK-0036) against unbounded recursion with no base case: unlike memory exhaustion, this does not throw a catchable JS exception — it is an infrastructure-level failure inside the QuickJS-on-`wazero` binding itself (observed as a WASM trap surfacing through the runtime's own cleanup path), always recovered cleanly as a `*WorkerExecutionError`, never a hang and never a crash reaching the caller's own process. Callers must not expect the same graceful rejection memory exhaustion gets for this specific failure mode; the OS-process boundary (not this package's own internal `recover()`) is confirmed to be what actually contains it, exactly the layered-defense reasoning `GAME-ADR-0030` is built on.
- **Execution-time limit is real and enforced** by the caller's own context deadline killing the worker process outright (see Isolation Guarantees above) — this is the authoritative backstop for any script that does not otherwise terminate on its own, adversarially verified against a plain infinite loop with no host calls.
- **Emitted-output size and command count are bounded** (`defaultMaxOutputBytes`/`defaultMaxCommandCount` in `protocol.go`, enforced by `Execute`/`Project` themselves after decoding the worker's response, not inside the worker): a script whose `execute` returns an oversized combined `NewState`/`RequestedCommands`, an excessive `RequestedCommands` count, or whose `project` returns an oversized `ClientState`, is rejected as `*ScriptRejectedError`, never silently accepted.
- **A defensive concurrency ceiling exists one level above this package**, in `jsexecutor/internal/grpcserver` (WORK-0036): a burst of concurrent `Execute` calls beyond a configurable limit is rejected with a gRPC `ResourceExhausted` status before a worker process would even be spawned, protecting the Executor service's own host from unbounded process creation. This is a defensive ceiling on the Executor's own aggregate resource consumption, not a per-user/business rate-limiting policy (a different, separately owned concern).

## Explicitly Not Guaranteed Here

- **Adversarial isolation testing beyond what is now covered above.** Memory/stack/execution-time/output-size/command-count/concurrency are now adversarially tested (WORK-0036); this binding's own history in this document shows it can still expose more than its documentation claims until actually probed, so a new capability added to this package should not be assumed safe merely because these categories were.
- **Command/state business-shape validation** — a separate concern owned by whichever caller (Session Runtime) receives `RequestedCommands` back; this package treats it as opaque.
- **OS-level isolation beyond a plain restricted process** (seccomp, cgroups, gVisor) and **worker-process pooling/reuse** — both deliberately deferred (`WORK-0036`'s own file, "Explicitly Out Of Scope"): the two layers this design's correctness actually depends on (WASM memory cap, process-level deadline kill) are already implemented and adversarially verified above; either addition would introduce new dependency/deployment-runtime requirements for a guarantee already held elsewhere, or real complexity (reuse hygiene, respawn-on-violation) for a performance optimization, not a correctness/security requirement. Revisit only if a real deployment target or security review, or real load/latency data, demands it.
- **A deployment manifest** (Dockerfile, minimal-privilege posture, restricted egress) for this service — no service in this repository has one yet; deferred to a future cross-service deployment initiative (`WORK-0036`'s own file).
