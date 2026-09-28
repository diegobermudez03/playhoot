# jsengine Logical Contract

Status: PACKAGE-LOCAL IMPLEMENTATION CONTRACT

Successor, for the JavaScript execution boundary, to `game/language/v1/engine/LOGICAL_CONTRACT.md`. Records `WORK-0035`'s implemented contract; see `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md` and `game/docs/decisions/GAME-ADR-0030-sandbox-runtime-quickjs-wasm-wazero-process-isolated-workers.md` for the accepted architecture this implements.

## Operation

One logical operation: `Execute(ctx, ExecutionInput{Script, PreviousState, Event, Context}) -> (ExecutionOutput, error)`.

- `Script` is already-resolved authored JavaScript source. This package does not fetch, cache, compile-ahead, or version it.
- `PreviousState`/`Event` are opaque JSON values, passed through to the script unexamined.
- `Context` carries the only sources of non-determinism authored code may observe: `LogicalTime`, `RandomSeed`, `ActingActor`. The sandbox never exposes real wall-clock time or OS randomness.
- On success, `ExecutionOutput.NewState`/`RequestedCommands` are exactly what the script's `execute` function returned, unexamined for business meaning (`WORK-0039` validates that separately).
- `error` distinguishes two categories: `*ScriptRejectedError` (the script threw, returned a malformed shape, or defines no `execute` function — a business-level outcome) and `*WorkerExecutionError` (the worker process crashed, the IPC exchange failed, or the deadline was exceeded — an infrastructure-level failure). Callers must not conflate the two.

## Script Contract

An authored script must define a global function:

```js
function execute(previousState, event, context) {
  // ...
  return { newState: /* ... */, requestedCommands: [/* ... */] };
}
```

`context` is `{ logicalTime: <RFC3339Nano string>, randomSeed: <decimal string>, actingActor: <string> }`. `randomSeed` is a string, not a JS number, because a Go `uint64` can exceed `Number.MAX_SAFE_INTEGER`; authored code needing numeric random behavior must derive its own generator from this seed rather than treat it as a plain number.

## Execution Model

Each `Execute` call spawns a fresh worker process (a re-exec of the same binary in worker mode, `game/session`'s `RunWorkerIfRequested`), writes one JSON request to its stdin, and reads one JSON response from its stdout. The worker instantiates a fresh QuickJS-on-WebAssembly (`wazero`) runtime, evaluates the script, invokes `execute`, and exits. No state is retained between invocations — `Execute` is stateless as a library, per `GAME-ADR-0028`.

This spawn-per-call shape is deliberately the simplest mechanism sufficient to prove the process boundary and language contract both work (`WORK-0035`'s own scope). `WORK-0036` replaces the process-management mechanics (pooling, reuse, resource-limit enforcement, forced termination) behind this same `Execute` signature — no caller of `Execute` needs to change when that lands.

## Isolation Guarantees (this WORK's scope)

Every guarantee below was independently verified against the pinned QuickJS-on-`wazero` binding's actual behavior (not merely assumed from its documentation), because the binding was found, twice during this WORK's own review, to expose more ambient host capability by default than its own stated design implied.

- **Filesystem** is confined to a fresh, empty, per-invocation scratch directory, created by the caller (`Execute`, not the worker itself — see Execution Model) before spawning the worker and removed after the worker process ends, however it ends. The underlying binding always mounts *some* directory as the sandbox's filesystem root, defaulting to the host process's own real working directory if left unconfigured, so this confinement is an active choice this package makes, not the binding's own default behavior. A script can still read/write/list within that scratch directory during its own single invocation — that directory contains nothing when the script starts and is destroyed once the invocation ends, so this residual capability never exposes anything real.
- **Host environment variables** are unreachable: the sandbox runtime's own guest environ is already empty by construction (independent of the host process's own environment), and `Execute` additionally starts the worker process itself with no environment at all (`cmd.Env` set to empty) as defense-in-depth against that current behavior ever changing, or against anything else in the worker binary reading a real host environment variable it should not. `std.getenv` itself turned out to be a non-configurable property this package cannot actually remove from the language, so this guarantee rests on the process/runtime boundary, not on hiding that one function.
- **Real elapsed time** is unreachable: `Date`/`Math.random` are replaced (see below), and `performance`/`os` — both of which exposed real, live timing (`performance.now()`, `os.now()`) despite the binding's default isolation stance — are removed from the global scope entirely before the script runs.
- Authored code cannot obtain real wall-clock time or OS randomness even by calling the language's own ambient `Date`/`Math.random` directly: a prelude evaluated before the script replaces both with versions derived from `ExecutionContext`.
- A script-level exception or panic inside the worker never propagates as a Go panic to the caller; it is always converted to a `workerResponse` (`Rejected` or `Fatal`) before the worker's own process exits.
- A worker process crash (non-zero exit, no valid JSON on stdout) is surfaced to the caller as a `*WorkerExecutionError`, never a crash of the caller's own process.
- A caller-supplied `context.Context` deadline is enforced by killing the entire worker process, verified against a script that never yields control back to the runtime (a plain infinite loop) — this is the mechanism this package's design depends on, since the sandbox runtime's own cooperative interruption is not reliable against exactly this case.
- **Network** access is assumed unreachable based on the binding's own stated default (no network host function is imported by this package), but this specific claim has not been independently probed the way filesystem/timing/environment were — given this binding's track record above, treat it as unverified rather than certain until it is.

## Explicitly Not Guaranteed By This WORK

- **Resource-limit enforcement.** `qjs.Option{MemoryLimit, MaxStackSize, MaxExecutionTime}` are set to conservative best-effort defaults, but real tuning, verification, and the authoritative OS-process-level deadline/kill mechanism belong to the WORK that hardens this boundary into a real worker pool — this package's own process-level deadline enforcement is proven to work, but its resource *limits* short of that deadline are not yet tuned or adversarially tested.
- **Adversarial isolation testing** beyond the specific capability-absence checks named above — remains a hardening WORK's own scope, and per this contract's own history, should not assume the binding has no further undocumented ambient capability beyond what has actually been probed.
- **Command/state business-shape validation** — a separate WORK's scope; `RequestedCommands` here is opaque.
