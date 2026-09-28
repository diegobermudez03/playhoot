# ADR-0016: JavaScript Executor As A Separately Deployed Infrastructure Service

Status: ACCEPTED
Created: 2026-09-27
Last status change: 2026-09-27
Supersedes: None
Superseded by: None

## Context

`game/docs/decisions/GAME-ADR-0030-sandbox-runtime-quickjs-wasm-wazero-process-isolated-workers.md` accepted a two-layer sandbox (QuickJS-on-WebAssembly via `wazero`, plus OS-process-isolated workers) but left the workers as same-host subprocesses of Session Runtime's own deployment: Session Runtime resolves its own executable path, re-execs itself via `exec.Command`, and communicates over that child process's stdin/stdout. `docs/projects/active/js-runtime-migration/works/WORK-0035-sandboxed-javascript-execution-runtime.md` implemented exactly this and was reviewed APPROVED and closed DONE against that topology.

A review of that implementation after closure found the same-host coupling runs deeper than convenience:

- `Execute` (the Session-side entry point) directly calls `exec.Command`, resolves the worker's own binary path via `os.Executable()`, and depends on stdin/stdout — none of this is behind an abstraction a caller could substitute.
- The sandbox's filesystem confinement mechanism (`ScratchDir`) is a directory path created by the caller and handed to the worker to mount — this only works because both sides are guaranteed to share a local filesystem (they are, in fact, the same re-exec'd binary).
- Independent review of `GAME-ADR-0030`'s chosen sandbox library (`WORK-0035`'s Completion Record) found it repeatedly exposed more ambient host capability than its own documentation claimed — a real host directory mounted by default, live wall-clock/timing globals reachable despite an explicit context parameter, and a subprocess that inherits its parent's full environment by default. Each was fixed, but the pattern itself is evidence that running genuinely untrusted (including AI-generated) code deserves a harder isolation boundary than "a child process of the same trusted deployment," not merely process-level isolation within it.

Separately, Session Runtime and JavaScript execution have different operational profiles: Session Runtime is a trusted application workload (coordination, persistence, authorization, timers, client transport) that should hold database credentials and application secrets; JavaScript execution runs untrusted code and should not need to hold most of what Session Runtime holds at all. Coupling them into one deployment forces them to share a privilege boundary, a scaling profile, and a failure blast radius that their actual risk/operational profiles do not justify sharing.

## Decision

### Two separately deployed workloads

Session Runtime and the JavaScript Executor become independent deployable workloads, communicating over an internal network protocol (transport choice — gRPC, Connect, plain HTTP, or another option — is implementation-planning detail for the owning WORK, not fixed here):

```text
Session Runtime deployment
        |
        | internal network RPC
        v
JavaScript Executor deployment
        |
        +-- isolated worker process
        |      +-- wazero
        |            +-- QuickJS.wasm
        |
        +-- isolated worker process
        +-- isolated worker process
```

The JavaScript Executor is written in Go (reusing the execution logic `WORK-0035` already built and proved) and owns: receiving execution requests, managing its own worker/process pool, hosting `wazero`+QuickJS, enforcing execution resource limits, terminating non-cooperative executions, and returning a validated execution response envelope. Session Runtime must not itself spawn sandbox worker processes or depend on local subprocess/stdin/stdout semantics.

This adds a deployment boundary on top of, not instead of, the process/WASM layering `GAME-ADR-0030` already accepted:

```text
Deployment boundary
    + process boundary
        + WASM boundary
```

All three are complementary. `GAME-ADR-0030`'s technology choice (QuickJS-on-WASM via `wazero`) and its process-isolated-worker layering are retained; what changes is that this layering now lives inside a separately deployed service rather than inside Session Runtime's own process.

### A real port at Session Runtime's caller boundary

Session Runtime depends on an abstraction, not on any of the mechanics that implement it:

```go
type Executor interface {
    Execute(ctx context.Context, req ExecutionRequest) (ExecutionResult, error)
}
```

Session Runtime's own code must not reference `exec.Command`, a worker binary path, stdin/stdout, local PIDs, local scratch directories, `wazero`, QuickJS, or worker-pool implementation details. The production implementation of this interface communicates with the separately deployed Executor over the network; a test/fake implementation may exist for tests that do not need a real Executor running.

### Artifacts no longer travel as local paths

`WORK-0035`'s `ScratchDir` mechanism — a caller-local filesystem path the worker was expected to mount — is incompatible with this topology and is retired. A request from Session Runtime to the Executor must not contain a path meaningful only on the caller's own disk. The concrete replacement (inline bytes where bounded and appropriate, an immutable artifact identifier the Executor resolves itself, an object-storage-backed reference, or another explicit remote-safe mechanism) is the owning WORK's design decision, made against whatever `docs/projects/active/js-runtime-migration/works/WORK-0044-game-version-artifact-model.md` ultimately defines an artifact to be — this record only requires that Session and Executor be able to run on different machines, in different containers, or in different Kubernetes pods with no shared filesystem, and that the mechanism not be solved by giving the Executor broad access to Session Runtime's own storage, database, or secrets (the Executor keeps the minimum capability it actually needs).

### Security/privilege boundary is the point, not a side effect

Session Runtime keeps every trusted application responsibility (session coordination, persistence, authorization, timers, interaction ordering/idempotency, command validation/materialization, client transport). The JavaScript Executor is an infrastructure component for running untrusted game code, deployable with a materially smaller privilege set: no Session database credentials, no application secrets execution does not need, no general outbound internet access, restricted network policy, and explicit CPU/memory/concurrency limits, on top of the process/WASM isolation it already provides internally.

### Scaling and failure assumptions

Session Runtime and Executor replicas scale independently. Any Session Runtime instance may call any healthy Executor instance; an Executor instance may disappear between executions; no correctness may depend on successive events reaching the same Executor or worker; no game state may live only in Executor memory between calls (the Executor remains stateless with respect to a game session, consistent with `GAME-ADR-0028`'s existing statelessness requirement); and retries/ambiguous transport failures must be accounted for at the protocol boundary, not assumed away. The conceptual execution contract is unchanged by any of this:

```text
previousState + event + executionContext + pinned game artifact
    ->
newState + requested commands
```

### This is an infrastructure boundary, not a new business domain

The JavaScript Executor is not a business bounded context and must not be reinterpreted as one merely because it becomes independently deployable — it exists because Playhoot executes untrusted code with different scaling and security characteristics than the rest of the system, the same reasoning `ARCHITECTURE.md`'s Component Model already applies to a "Technical / Supporting Library," extended here to a technical component that also happens to warrant its own deployment rather than being an in-process library. Session Runtime still owns game/session execution semantics from the domain perspective; the Executor computes, it does not decide.

## Rationale

`ARCHITECTURE.md`'s own "add network, service, queue, or deployment machinery only when an actual problem requires it" principle is not waived here — it is satisfied. The problem is concrete, not hypothetical: this exact sandbox library has already been found, twice, to expose more ambient host capability than assumed, and Session Runtime is the one process that holds database credentials, application secrets, and every other trusted capability a compromised or buggy execution should never be able to reach merely by escaping a language-level sandbox. A deployment boundary is the only one of the three layers that keeps a failure of the other two from reaching Session Runtime's own privileges. This is deliberately decided now, while only one WORK's implementation depends on the old topology, rather than after further WORK (timers, views, effects, artifact storage, authoring/simulation tooling) has deepened the same local-execution assumption throughout the codebase.

## Alternatives Considered

### Keep the same-host subprocess topology `GAME-ADR-0030`/`WORK-0035` already built, harden it further in place

Rejected. Process isolation alone cannot bound the blast radius of a compromised Executor reaching Session Runtime's own address space, database credentials, or secrets, since they are the same OS process tree by construction. Given this sandbox library's own demonstrated pattern of exposing more capability than expected, that residual risk is judged material enough to warrant a real deployment boundary now rather than accepting it as a permanent limitation.

### Give the Executor its own reduced credential set but keep it in the same deployment/process group

Rejected as insufficient — reduced credentials mitigate but do not remove the shared-process/shared-address-space risk a deployment boundary removes structurally.

## Consequences

- `game/docs/decisions/GAME-ADR-0030-sandbox-runtime-quickjs-wasm-wazero-process-isolated-workers.md` — `Superseded by:` header annotated (in part: deployment topology only; the QuickJS-on-WASM/`wazero` technology choice and process-isolated-worker layering are retained, now inside the separately deployed Executor).
- `docs/projects/active/js-runtime-migration/works/WORK-0035-sandboxed-javascript-execution-runtime.md` (DONE) is not reopened or rewritten — its historical record accurately describes what it delivered and why, and its execution-logic/language-contract work is reused, not discarded. Its same-host topology is superseded going forward by new WORK; see that WORK's own Completion Record for the specific pointer.
- New WORK own the actual split: standing up the Executor as a separately deployed service (reusing `WORK-0035`'s execution logic), and introducing the `Executor` port plus its network-client implementation at Session Runtime's own boundary — tracked in `docs/projects/active/js-runtime-migration/PROJECT.md`.
- `ARCHITECTURE.md`'s Component Model gains a category for a separately deployed technical/infrastructure workload that is not a business bounded context, since none of its existing five categories describe one.
- Every WORK downstream of execution (artifact storage/retrieval, command protocol, timers, views, effects, authoring/simulation tooling, end-to-end verification) must be re-audited for an assumption that execution is local, and updated to route through the `Executor` port rather than a direct in-process call — tracked per-WORK in the Project, not silently left as future work.

## Canonical Knowledge Impact

- `ARCHITECTURE.md` — Component Model gains a new category (see Consequences); annotated once the owning WORK lands.
- `game/docs/decisions/GAME-ADR-0030-sandbox-runtime-quickjs-wasm-wazero-process-isolated-workers.md` — `Superseded by:` header updated.
- `docs/decisions/architecture/INDEX.md` — this record added.
- `docs/projects/active/js-runtime-migration/PROJECT.md` — Goal, Work table, Ordering/Dependencies, and Capability Coverage updated to reflect the two-workload topology.

## Implementation Impact

Not authorized by this record beyond its own creation. Routed to `docs/projects/active/js-runtime-migration/works/WORK-0052-javascript-executor-service.md` (standing up the separately deployed Executor) and `WORK-0053-session-side-remote-executor-port-and-network-client.md` (the port/client at Session Runtime's boundary), plus rescoping of `WORK-0036` (resource-limit/isolation hardening now belongs inside the Executor service) and `WORK-0044` (artifact model must define a remote-safe transfer mechanism).
