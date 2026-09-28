# WORK-0035: Sandboxed JavaScript Execution Runtime (Pure Function Contract)

Status: DONE
Created: 2026-09-27
Last status change: 2026-09-27 (IMPLEMENTING -> DONE: independent review APPROVED after two fix/re-review rounds)

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`
- `game/docs/decisions/GAME-ADR-0030-sandbox-runtime-quickjs-wasm-wazero-process-isolated-workers.md`

Canonical context:
- `game/language/v1/engine/LOGICAL_CONTRACT.md` (the contract being replaced, for shape/precedent only)
- `game/language/v1/engine/README.md`, `game/language/v1/engine/engineservice/` (current call-site shape: `Compile`/`StartTurn`/`AdvanceTurn`)

## Outcome

Build the execution boundary `GAME-ADR-0028` establishes — `execute(previousState, event, context) -> {newState, requestedCommands}` — implemented against the runtime `GAME-ADR-0030` selected: a QuickJS engine compiled to WebAssembly, hosted by `wazero`, invoked inside a process separate from Session Runtime's own. This is the foundational capability nearly every other WORK in this Project integrates against.

This WORK owns making one execution correct and functionally isolated (process boundary + no host capability wiring); it does not own hardening that boundary into a resource-limited, adversarially-tested worker pool (`WORK-0036`), validating the business meaning of commands (`WORK-0039`), or enforcing determinism (`WORK-0037`) — each is designed against this WORK's contract, not folded into it.

## Context

Today, Session Runtime's `sessionlifecycle` steps call `engineservice.Compile`/`StartTurn`/`AdvanceTurn` (a synchronous, in-process, pure-Go function call) against a compiled `program.Definition`. This WORK replaces that call shape with a call to the new execution boundary, invoking an out-of-process worker instead of an in-process interpreter. Switching each of the seven call sites (`step_create.go`, `step_join.go`, `step_start.go`, `step_answer_interaction.go`, `step_submit_user_intent.go`, `step_cancel_session.go`, `step_expire_timer.go`) is explicitly folded into `WORK-0038` (see that WORK's own scope note), since it already touches the same call sites to change the persistence model — this WORK builds the new boundary itself, not its callers.

## Scope

### In Scope

- The Go-side execution contract: a package-internal function/interface Session Runtime calls with `(previousState, event, context)` and receives `(newState, requestedCommands)` or a distinguishable rejection/error, per Approved Design.
- Invoking the QuickJS-on-WASM (`wazero`) engine inside a process separate from the caller's own, for one execution — the simplest correct mechanism sufficient to prove the process boundary and the language contract both work; `WORK-0036` replaces/hardens the pooling, resource-limit, and forced-termination mechanics behind the same interface.
- The host-process <-> worker-process communication (serialization/IPC) needed to pass inputs in and results out.
- Wiring `context`'s logical-time/random-seed inputs as the *only* time/randomness source visible to the script (overriding/backing `Date`/`Math.random` equivalents), with no host filesystem/network/process import wired into the WASM module at all.
- Accepting already-resolved script content (source or the chosen binding's expected compiled form) as an input parameter — this WORK does not fetch, cache, or version artifacts itself.
- Distinguishing a script-level rejection (threw, or returned a malformed/invalid shape) from an infrastructure-level execution failure (worker crash, IPC failure, timeout).

### Out of Scope

- Resource limits, forced interruption on budget overrun, capability-restriction hardening beyond "no host functions are imported," worker-pool sizing/reuse, and adversarial isolation testing — `WORK-0036`.
- Validating the business/semantic shape of `requestedCommands` against the platform command vocabulary or a game's own contract — `requestedCommands` is treated here as an ordered list of opaque, serializable values; `WORK-0039` defines and validates their vocabulary.
- Determinism static-analysis/lint enforcement of authored scripts — `WORK-0037`.
- Timer, view, and effect semantics specifically — `WORK-0040`/`WORK-0041`/`WORK-0042`.
- Persisting `newState`, and switching Session Runtime's own call sites to use this boundary — `WORK-0038`.
- Artifact storage/versioning/compilation pipeline — `WORK-0034`/`WORK-0044`.

## Approved Design

- **Contract shape** (illustrative; exact Go types/names are implementer's call per Implementation Freedom):

  ```go
  type ExecutionContext struct {
      LogicalTime time.Time
      RandomSeed  uint64
      ActingActor string // opaque identity reference, per GAME-ADR-0006's preserved root-roster concept
  }

  type ExecutionInput struct {
      Script        ResolvedScript // already-resolved content; this package does not fetch it
      PreviousState []byte         // opaque to this package
      Event         []byte         // opaque to this package
      Context       ExecutionContext
  }

  type ExecutionOutput struct {
      NewState          []byte
      RequestedCommands []RawCommand // opaque, ordered; WORK-0039 validates meaning
  }

  func Execute(ctx context.Context, in ExecutionInput) (ExecutionOutput, error)
  ```

  `Execute`'s own `error` return distinguishes an infrastructure-level execution failure (worker crash, IPC failure, deadline exceeded) from a script-level rejection, which is instead represented as a value in `ExecutionOutput` (or a dedicated rejection type) — mirroring the existing engine's `ErrSignalRejected`/`ErrInputRejected` vs. `ExecutionError` distinction (`game/language/v1/engine/LOGICAL_CONTRACT.md`), restated for this runtime rather than reused verbatim, since the failure causes themselves differ (a WASM trap or worker crash has no DSL equivalent).

- **Worker invocation**: for this WORK, the simplest mechanism that genuinely runs the QuickJS-on-WASM engine in a separate OS process is sufficient (for example, a single long-lived worker subprocess communicating over stdin/stdout, or a spawn-per-call model) — it must sit behind an interface `WORK-0036` can replace with a real pool without changing `Execute`'s own signature or callers.
- **Determinism inputs**: the worker's QuickJS environment must not expose real wall-clock or OS-randomness — `Date`/`Math.random` (or their equivalents) are backed by `ExecutionContext.LogicalTime`/`RandomSeed`, never the host's own clock/RNG.
- **No host capability wiring**: no filesystem, network, or process-related host function is imported into the WASM module, per `GAME-ADR-0030`.

## Constraints and Invariants

- `Execute` is stateless as a library: no invocation may depend on or mutate state left behind by an unrelated prior invocation (a harmless performance cache, such as a compiled-module cache keyed by script identity, is allowed; a correctness-relevant cache is not).
- `Execute` must be safe for concurrent use across different Sessions; it does not itself serialize concurrent calls (Session Runtime's existing per-Session DB locking, `GAME-ADR-0018`, is unaffected and unchanged).
- Every source of non-determinism available to authored code must come from `ExecutionContext`, never ambiently from the runtime.

## Acceptance Criteria

- A fixture script executed twice with byte-identical inputs produces byte-identical `NewState`/`RequestedCommands` output.
- A fixture script that throws, or returns a malformed/invalid shape, produces a distinguishable rejection outcome — never a Go panic or a crash of the calling process.
- A fixture script that attempts to read an environment variable, open a file, or make a network connection fails to do so from inside the sandbox.
- A fixture script that crashes its own worker process (for example, an unrecoverable WASM trap) is observed by the caller as a clean execution error — the caller's own process/goroutine is unaffected.
- Concurrent `Execute` calls for different Sessions do not observably interfere with each other's state or output.

## Implementation Freedom

Exact QuickJS-on-`wazero` binding library (among the candidates `GAME-ADR-0030`'s Sources name, or an equivalent actively maintained option), exact host<->worker IPC/serialization mechanism, exact Go package layout, and whether this WORK's worker mechanism is a persistent process or spawned per call are implementer's choices within the architecture `GAME-ADR-0030` fixes.

## Verification

- `go test ./...` for the new package, including the round-trip determinism test, the malformed-output rejection test, the capability-isolation fixture test, and the worker-crash-isolation test named in Acceptance Criteria.
- Independent review per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`, given this introduces a new runtime dependency and a new process-boundary security mechanism.

## Documentation Impact

### Accepted / Canonical Knowledge

- A new package-local contract document (successor role to `game/language/v1/engine/LOGICAL_CONTRACT.md`), located wherever this WORK's implementation places the execution package, recording the exact contract/invariants above.

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md` — new execution runtime's status added; the existing Game Language row is not yet retired by this WORK alone (that depends on `WORK-0038` actually switching the call sites).

## Blockers

- None material. The prior blocking decision (sandbox runtime technology) is resolved by `GAME-ADR-0030`.

## Completion Record

Implemented as designed. New package `game/session/internal/jsengine` (`jsengine.go`, `worker.go`, `protocol.go`, `determinism.go`, `LOGICAL_CONTRACT.md`) implements `Execute(ctx, ExecutionInput) (ExecutionOutput, error)` against the `github.com/fastschema/qjs` QuickJS-on-`wazero` binding, spawning a fresh worker process per call (a re-exec of the same binary in a hidden worker mode). A small `game/session/bootstrap` package exposes the worker-dispatch hook to `main.go` without pulling the new dependency into `game/session`'s own zero-dependency root package, and without violating Go's `internal/` import-visibility rule (`main.go` cannot import `game/session/internal/jsengine` directly).

Independent review ran three rounds (first review CHANGES_REQUIRED, two fix passes, final APPROVED with no findings). The first two rounds surfaced real gaps beyond the original design's own assumptions, each independently verified against the actual pinned library rather than trusted from its documentation:

- The binding always mounts a real directory as the sandbox's filesystem root (defaulting to the host process's own working directory) regardless of what this package wires in — fixed by having the caller (not the worker, so cleanup survives a kill) create and own a fresh, empty, per-invocation scratch directory.
- The language's own ambient `Date`/`Math.random`, plus `performance.now()`/`os.now()`, returned real, live non-deterministic values even though a separate `context` parameter was already supplied — fixed with a prelude evaluated before the authored script that replaces `Date`/`Math.random` with context-derived versions and removes `performance`/`os` from global scope entirely.
- Self-caught during the same fix pass: a spawned child process inherits its parent's full OS environment by default, which would otherwise expose real host environment variables (including secrets) to the worker; fixed by starting the worker with an empty environment. Re-review found the sandbox runtime's own guest environ was already isolated independent of this, but confirmed the fix is still correct, valuable defense-in-depth.

Every Acceptance Criterion is covered by a passing test in `jsengine_test.go` (10 tests total, including the fixes above): deterministic round-trip, script-throw/malformed-shape rejection (as `*ScriptRejectedError`, distinct from `*WorkerExecutionError`), no host filesystem/timing/environment capability reachable (verified by an actual attempted escape, not merely checking for absent Node/browser globals), worker-crash isolation, a caller deadline actually killing a non-yielding infinite loop within its bound, no scratch-directory leak after such a kill, and concurrent calls across different logical Sessions not interfering.

Verification performed: `go build ./...`, `go vet ./...`, and `go test ./game/session/internal/jsengine/... -v` (all pass) at each round; `go test . -run "TestNoInternalDocCitationsInComments|TestExportedDocCommentsStayAtPublicContract"` confirmed clean specifically for this WORK's new files (9 initial violations in `jsengine.go`/`protocol.go` found and fixed; unrelated pre-existing violations elsewhere in the repository were left untouched, out of this WORK's scope). `go.mod`'s `go 1.24.2` toolchain directive was deliberately kept unchanged (an initial `go mod tidy` attempt bumped it to `go 1.25` as a side effect of an unrelated transitive dependency resolution and was reverted).

Known limitations, explicitly out of this WORK's scope per its own Scope section: resource-limit enforcement/tuning beyond conservative best-effort defaults, worker pooling (a fresh process is spawned per call), and network-capability isolation specifically (assumed unreachable based on the binding's stated default and a reviewer's own source-level probe of `wazero`'s WASI socket host functions finding no JS-reachable path today, but not independently proven false the way filesystem/timing/environment were — flagged in `LOGICAL_CONTRACT.md` as unverified rather than claimed safe). All are named as `WORK-0036`'s own scope, not silently dropped.

No caller in the repository invokes this package yet; wiring `sessionlifecycle`'s call sites to it is `WORK-0038`'s own scope, as this WORK's Context section already recorded before implementation began.

**Topology superseded (2026-09-27), not reopened.** `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md` supersedes this WORK's same-host subprocess topology: a review of this exact implementation found `Execute`'s direct `exec.Command`/stdin-stdout/own-binary-path coupling, and the `ScratchDir` mechanism's dependence on a filesystem shared between caller and worker, incompatible with running the JavaScript Executor as a separately deployed workload from Session Runtime. This historical record is left as-is — it accurately describes what was built, reviewed, and why, against the topology accepted at the time. Nothing here is retracted: the execution contract, the QuickJS-on-`wazero` sandbox technology, the determinism prelude, and every isolation gap this WORK's own review found and fixed are reused, not discarded, inside the new separately deployed Executor. Only the deployment topology changes, via `docs/projects/active/js-runtime-migration/works/WORK-0052-javascript-executor-service.md` (moves this WORK's execution logic into a real separately deployed service) and `WORK-0053-session-side-remote-executor-port-and-network-client.md` (replaces `Execute`'s direct local invocation with a network-client implementation of a new `Executor` port at Session Runtime's own boundary).
