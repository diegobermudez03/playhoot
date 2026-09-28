# WORK-0053: Session-Side Remote Executor Port & Network Client

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`

Canonical context:
- `game/session/internal/jsengine/jsengine.go` (`WORK-0035`'s `Execute` — the direct local invocation this WORK replaces at Session Runtime's own boundary)

## Outcome

Introduce the `Executor` port at Session Runtime's own caller boundary and the production network-client implementation of it, so Session Runtime depends only on an interface — never on `exec.Command`, a worker binary path, stdin/stdout, local PIDs, local scratch directories, `wazero`, QuickJS, or worker-pool implementation details. This is the other half of `ADR-0016`'s split: `WORK-0052` builds the separately deployed Executor; this WORK is what Session Runtime actually calls to reach it.

## Context

Not yet designed. Depends on `WORK-0052` existing enough to have a real network endpoint to call (though the interface's own shape can be designed in parallel).

## Scope

### In Scope

- The `Executor` interface itself (Session-owned, per `ADR-0016`'s sketch: `Execute(ctx, ExecutionRequest) (ExecutionResult, error)` — exact field/type shape is this WORK's own design).
- A production gRPC client implementation of that interface, generated from the same `.proto` `WORK-0052` defines, including retry/timeout policy for ambiguous transport failures (per `ADR-0016`'s scaling/failure assumptions — no correctness may depend on a specific Executor instance or worker; standard gRPC deadline propagation from `ctx` is the expected mechanism, not a bespoke one) and mapping gRPC-level failures (status codes such as `UNAVAILABLE`/`DEADLINE_EXCEEDED` vs. an application-level rejection the Executor returns deliberately) onto the existing `*ScriptRejectedError`/`*WorkerExecutionError` distinction `WORK-0035` established (or its successor types, if this WORK's design supersedes them).
- Retiring `game/session/internal/jsengine.Execute`'s direct local `exec.Command` invocation — this WORK's network-client implementation becomes Session Runtime's actual production dependency; whether `jsengine`'s Go-level types (`ExecutionInput`/`ExecutionOutput`/error types) are kept as the interface's own shape or superseded is this WORK's design decision, not fixed here.

### Out of Scope

- The Executor service itself — `WORK-0052`.
- Persisting execution results / switching `sessionlifecycle`'s call sites to actually call the `Executor` port — `WORK-0038`.

## Approved Design

Not yet designed. Must satisfy `ADR-0016`'s explicit list of what Session Runtime's own code must never reference again once this WORK lands (see Outcome).

## Constraints and Invariants

- Session Runtime's own package(s) must not import `wazero`, QuickJS, or any worker-pool/process-management dependency, directly or transitively, once this WORK lands.
- No correctness may depend on a retried request reaching the same Executor instance as the original attempt.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed. Must include a test double/fake `Executor` implementation Session Runtime's own tests can use without a real network dependency, and at least one test against a real (or realistically simulated) Executor instance proving the network client's retry/timeout/error-mapping behavior.

## Documentation Impact

### Accepted / Canonical Knowledge

- A new package-local contract document at wherever this WORK's `Executor` interface lives, recording the port's contract (successor role to `game/session/internal/jsengine/LOGICAL_CONTRACT.md` for anything Session-Runtime-facing).

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md` — updated alongside `WORK-0052`.

## Blockers

- Depends on `WORK-0052` (needs the `.proto` contract, at minimum, to generate a client against).

## Completion Record

Not yet started.
