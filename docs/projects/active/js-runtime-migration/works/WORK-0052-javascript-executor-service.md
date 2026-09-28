# WORK-0052: JavaScript Executor Service

Status: DONE
Created: 2026-09-27
Last status change: 2026-09-27 (IMPLEMENTING -> DONE: independent review APPROVED after one fix/re-review round)

Related decisions:
- `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`
- `game/docs/decisions/GAME-ADR-0030-sandbox-runtime-quickjs-wasm-wazero-process-isolated-workers.md` (technology/layering restated, not superseded, by ADR-0016)

Canonical context:
- `game/session/internal/jsengine/` (`WORK-0035`'s implementation — the execution logic this WORK relocates and fronts with a network boundary, not rewrites from scratch)
- `game/session/bootstrap/` (`WORK-0035`'s worker re-exec dispatch — retired by this WORK, see Consequences)
- `ARCHITECTURE.md` (Component Model's new Infrastructure Service category)
- `main.go` (Session Runtime's own entrypoint — unchanged in shape by this WORK; the Executor gets its own, separate entrypoint)

## Relocated (2026-09-27, post-closure, human-directed)

This WORK's own Scope/Approved Design/Completion Record below (unedited, left as accurate history of what was originally decided and built) describe the Executor as `jsexecutor/`, a top-level package sibling to `game/`/`identity/`/`composer/`/`orchestrator/`, deliberately *not* nested under `game/session/`. Per explicit human direction after closure, it has been moved to `game/session/jsexecutor/` instead — still a genuinely separate deployable workload (own binary, own gRPC boundary, ADR-0016's security/deployment reasoning is completely unchanged), but physically placed inside Session Runtime's own package tree specifically to signal that it is Session's own deployable unit, not a repository-wide capability another domain may call. See `game/session/jsexecutor/README.md` for the full ownership statement, including why this boundary is documentation-enforced rather than compiler-enforced (it is intentionally not nested under an `internal/` directory). Every path below reading `jsexecutor/...` should be read as `game/session/jsexecutor/...`; this note is the correction, not a rewrite of the historical text itself.

## Outcome

Stand up the JavaScript Executor as a genuinely separate deployable workload from Session Runtime, per `ADR-0016`: its own deployment artifact, a gRPC API surface receiving execution requests, and internal ownership of the worker/process pool hosting `wazero`+QuickJS. This WORK reuses `WORK-0035`'s already-built and already-reviewed execution logic (the sandboxed-execution contract, the determinism prelude, the process-isolated worker mechanism) — it relocates and fronts that logic with a network boundary; it does not redesign the sandbox itself.

This is required because Session Runtime's own process must not spawn sandbox workers or hold the credentials/secrets a compromised execution could otherwise reach — see `ADR-0016`'s Security boundary section.

## Context

Network transport is decided: gRPC (`PROJECT.md` Material Decisions #8). The Executor only ever needs a version's backend script content — never the frontend script, assets, or contracts `WORK-0044`'s full artifact model also covers — so this WORK does not need to wait on that WORK's complete design; see Approved Design for the narrow transfer mechanism this WORK adopts now.

## Scope

### In Scope

- A new top-level package, `jsexecutor/`, sibling to `game/`, `identity/`, `composer/`, `orchestrator/` — not nested under `game/session/`, since it is not Session-owned code (`ADR-0016`'s "not a new business domain" framing). It is its own `package main` (its own deployable binary), exactly as the repository root already is for Session Runtime — introducing a `cmd/` convention for only one of the two binaries would be asymmetric, not a genuine improvement.
- A gRPC service (`.proto`-defined, see Approved Design) exposing one RPC, `Execute`, plus the standard gRPC health-checking protocol (`grpc.health.v1`) for readiness.
- Relocating all of `game/session/internal/jsengine` (`jsengine.go`'s worker-spawning `Execute` logic, `worker.go`, `protocol.go`, `determinism.go`, and their tests) into this new package. The existing `Execute` function's own local-worker-spawning implementation (`exec.Command`, scratch-directory confinement, empty worker environment, caller-deadline process kill) is not replaced — it becomes the gRPC handler's own internal implementation, called with the request's deserialized fields instead of Go function parameters, and returning through the `ExecuteResponse`/gRPC-status mapping in Approved Design instead of a Go return value.
- Accepting the pinned version's backend script as inline bytes in the RPC request (see Approved Design) — not a caller-local filesystem path.
- Retiring `game/session/bootstrap`'s worker re-exec dispatch and `main.go`'s corresponding hook: Session Runtime's own binary no longer needs to know how to act as a sandbox worker once this WORK lands, since that responsibility moves entirely into `jsexecutor`'s own binary/entrypoint.

### Out of Scope

- Resource-limit tuning, pool sizing/reuse, and forced-termination policy beyond what `WORK-0035` already built — `WORK-0036` (revised).
- The Session-side port/client — `WORK-0053`.
- Any artifact-transfer mechanism beyond a single inline script (frontend script/assets/contracts references) — `WORK-0044`'s own scope; the Executor does not need them.
- Deployment manifests (Dockerfile, `docker-compose.yaml` service entry, orchestration), autoscaling policy, and detailed observability — implementer's call within this WORK's own design, not frozen here, though a new deployable artifact necessarily needs at least a minimal Dockerfile/compose entry to be runnable; exact shape is not prescribed.

## Approved Design

- **Proto contract** (illustrative field numbering; exact naming/package path is implementer's call):

  ```proto
  service Executor {
    rpc Execute(ExecuteRequest) returns (ExecuteResponse);
  }

  message ExecuteRequest {
    bytes script = 1;           // authored JavaScript source, inline
    bytes previous_state = 2;   // opaque JSON
    bytes event = 3;            // opaque JSON
    ExecutionContext context = 4;
  }

  message ExecutionContext {
    string logical_time = 1;  // RFC3339Nano
    string random_seed = 2;   // decimal string (may exceed a JS/proto int64's safe range as text)
    string acting_actor = 3;
  }

  message ExecuteResponse {
    oneof outcome {
      Success success = 1;
      Rejected rejected = 2;
    }
  }

  message Success {
    bytes new_state = 1;
    repeated bytes requested_commands = 2;
  }

  message Rejected {
    string reason = 1;
  }
  ```

- **Script transfer**: inline bytes in `ExecuteRequest.script`, per Context — sufficient for a single JavaScript source file (realistically well under gRPC's default 4 MiB message ceiling); this WORK does not need `WORK-0044`'s full artifact-bundle design to proceed. If a future authored script legitimately exceeds a reasonable inline size, `WORK-0044`/`WORK-0053` may add a reference-based alternative later without changing `Execute`'s own RPC shape (an additional field, not a redesign).
- **Failure-channel mapping**: only two outcomes are modeled as `ExecuteResponse` field data — a successful execution (`Success`) and a business-level script rejection (`Rejected`, mirroring `WORK-0035`'s `*ScriptRejectedError`: the script threw, returned a malformed shape, or defined no `execute` function). Every infrastructure-level failure (worker crash, sandbox creation failure, deadline exceeded) is surfaced as a non-OK gRPC status (`INTERNAL`, `DEADLINE_EXCEEDED`, etc.) from the RPC itself, not as response data — this is what gRPC's own error channel exists for, and mirrors `WORK-0035`'s `*WorkerExecutionError` at the transport level instead of reinventing it in the message schema.
- **Reused unchanged from `WORK-0035`**: the scratch-directory-per-invocation filesystem confinement (now created/owned by the `jsexecutor` process itself, which was already true — only the caller identity changes, from Session Runtime's own `Execute` to this service's gRPC handler), the determinism prelude neutralizing `Date`/`Math.random`/`performance`/`os`, the empty worker-process environment, and the caller-deadline-kills-the-worker mechanism (now driven by the incoming gRPC context's deadline instead of a Go `context.Context` passed by a same-process caller — `grpc-go`'s server handler context already carries the client's deadline, so this requires no new mechanism, only using the request handler's own `ctx`).
- **Health check**: implement `grpc.health.v1.Health`, reporting `SERVING` once the service can accept `Execute` calls (no dependency to wait on beyond process startup, since the Executor holds no database connection).

## Constraints and Invariants

- Stateless with respect to a game session between calls (`GAME-ADR-0028`); no correctness may depend on two calls reaching the same Executor instance or worker.
- Holds no Session Runtime database credentials or application secrets; this WORK's own configuration must not require any (a listen port and basic runtime tuning are the only expected inputs).
- Every isolation guarantee `WORK-0035`'s `LOGICAL_CONTRACT.md` records must hold unchanged after relocation — this WORK moves that logic, it does not re-derive or weaken it.

## Acceptance Criteria

- A running `jsexecutor` binary, listening on a configured port, accepts `Execute` calls and returns `Success` for a valid deterministic script, byte-identical across two identical calls (same test fixture as `WORK-0035`'s own determinism test, exercised over gRPC instead of an in-process call).
- A script that throws, or returns a malformed shape, or defines no `execute` function, returns `ExecuteResponse.Rejected` — the RPC itself still completes with an OK status; a business rejection is not a gRPC error.
- A script attempting to escape its scratch directory, read `performance`/`os`, or observe ambient `Date`/`Math.random` fails to do so — the exact fixture scripts `WORK-0035`'s test suite already proved this against, re-run over gRPC.
- A script that never yields (an infinite loop) and exceeds the caller's gRPC deadline causes the RPC to return `DEADLINE_EXCEEDED`, and the worker process backing that call is confirmed terminated (no leaked process or scratch directory) — the same proof `WORK-0035`'s own `TestExecute_CallerDeadline_KillsRunawayWorker`/`TestExecute_ScratchDirectoryNotLeakedAfterKill` established, now against a real gRPC deadline rather than a same-process `context.Context`.
- The health-check RPC reports `SERVING` once the process has started and `Execute` is callable.
- The `jsexecutor` binary's own process starts with no database connection string, no application secret, and no credential beyond what its own minimal configuration requires.

## Implementation Freedom

Exact `jsexecutor/` internal package layout, the protobuf toolchain (`protoc` vs. `buf`), exact configuration/env-var names, and exact Dockerfile/compose wiring are implementer's choices within this WORK's own Approved Design.

## Verification

- `go build ./...` and `go test ./jsexecutor/...` (new package).
- A real gRPC client-server integration test (in-process `bufconn` or a real listening port) exercising every Acceptance Criterion above, not merely unit-testing the relocated worker logic in isolation (that logic already has `WORK-0035`'s own test suite; this WORK's tests must prove the network boundary itself works).
- Independent review per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`, given this introduces a new deployable service and a new dependency (gRPC/protobuf).

## Documentation Impact

### Accepted / Canonical Knowledge

- `ARCHITECTURE.md` — Component Model's Infrastructure Service category gains a concrete example once this service exists.
- A new package-local contract document at `jsexecutor/` (successor to `game/session/internal/jsengine/LOGICAL_CONTRACT.md` for the Executor's own side of the boundary).

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md` — the JavaScript Execution Runtime row updated to describe the new topology; the old same-host-subprocess description retired, and `game/session/internal/jsengine`/`game/session/bootstrap`'s removal noted. This is safe to do within this WORK itself (not deferred to `WORK-0053`) because neither package has any production caller yet — `WORK-0035`'s own Completion Record already recorded that `Execute` was only ever called by its own tests, and `WORK-0038` (which would have wired a real caller) has not landed.

## Blockers

- None material. The artifact-transfer question that previously blocked this WORK is narrowed to "inline script bytes," which this WORK's own Approved Design resolves without waiting on `WORK-0044`.

## Completion Record

Implemented as designed. New top-level package `jsexecutor/` (`main.go`, `proto/executor.proto` + generated code, `internal/sandbox/`, `internal/grpcserver/`) stands up the JavaScript Executor as a genuinely separate deployable gRPC service. `jsexecutor/internal/sandbox` is a faithful relocation of `WORK-0035`'s execution logic (confirmed byte-for-byte identical apart from package/sentinel-string renaming during review) — every isolation guarantee that WORK's own independent review found and fixed (scratch-directory confinement, empty worker environment, the determinism prelude neutralizing `Date`/`Math.random`/`performance`/`os`, caller-deadline process kill) survived unchanged. `jsexecutor/internal/grpcserver` wraps it: a business-level script rejection returns `ExecuteResponse.Rejected` with an OK gRPC status; an infrastructure-level failure (worker crash, deadline exceeded) returns a non-OK gRPC status (`DEADLINE_EXCEEDED`/`Internal`), never response data — verified by review to have no bug in either direction. The incoming gRPC context's deadline propagates unchanged through to the worker-process kill mechanism (traced end to end, no `context.Background()` substitution). The standard `grpc.health.v1` health service reports `SERVING`.

The old `game/session/internal/jsengine` and `game/session/bootstrap` packages were deleted, and `main.go`'s worker-dispatch hook removed, since neither had any production caller yet (confirmed in `WORK-0035`'s own Completion Record).

Independent review ran two rounds (first CHANGES_REQUIRED, one fix pass, final APPROVED with no findings). First round found two documentation-synchronization gaps (`game/CURRENT_STATE.md` still describing the deleted packages; this Project's own `PROJECT.md` still showing WORK-0052 as DRAFT despite a completed implementation) — both fixed and re-verified. It also found a low-severity `go.mod` classification issue (`grpc`/`protobuf` listed as indirect despite direct imports), fixed the same pass. A residual low-severity observation (no Dockerfile/compose entry yet) was left as-is, consistent with this WORK's own explicit scope (deployment manifests are implementer's-call/out of scope) and is now tracked in `game/CURRENT_STATE.md`'s own Current Gaps.

Verification performed: `go build ./...`, `go vet ./...` clean at every round; `go test ./jsexecutor/...` — all tests pass, including a real gRPC client-server integration test (`bufconn`) proving the network boundary itself (determinism round-trip, rejection-as-OK-response, sandbox isolation, and deadline-exceeded-kills-the-worker-and-leaves-no-scratch-directory, all exercised over real gRPC, not merely against the relocated Go logic directly). `go test . -run "TestNoInternalDocCitationsInComments|TestExportedDocCommentsStayAtPublicContract"` confirmed clean for every new `jsexecutor/` file at every round; the same 23 pre-existing violations elsewhere in the repository (unrelated to this WORK) remain, unworsened.

Known limitations, explicitly out of this WORK's scope: resource-limit tuning/pooling (`WORK-0036`, now rescoped to live inside this service), no deployment manifest yet (Dockerfile/compose — implementer's call, not required by any Acceptance Criterion), and network-capability isolation specifically remains unverified rather than proven safe (inherited caveat from `WORK-0035`'s own `LOGICAL_CONTRACT.md`, unchanged by the relocation). No caller in the repository invokes this service yet; `WORK-0053` (the Session-side `Executor` port and network client) is what gives it one.
