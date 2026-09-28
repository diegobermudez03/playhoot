# WORK-0052: JavaScript Executor Service

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`
- `game/docs/decisions/GAME-ADR-0030-sandbox-runtime-quickjs-wasm-wazero-process-isolated-workers.md` (technology/layering restated, not superseded, by ADR-0016)

Canonical context:
- `game/session/internal/jsengine/` (`WORK-0035`'s implementation — the execution logic this WORK relocates and fronts with a network boundary, not rewrites from scratch)
- `ARCHITECTURE.md` (Component Model's new Infrastructure Service category)

## Outcome

Stand up the JavaScript Executor as a genuinely separate deployable workload from Session Runtime, per `ADR-0016`: its own deployment artifact, a network-facing API surface receiving execution requests, and internal ownership of the worker/process pool hosting `wazero`+QuickJS. This WORK reuses `WORK-0035`'s already-built and already-reviewed execution logic (the sandboxed-execution contract, the determinism prelude, the process-isolated worker mechanism) — it relocates and fronts that logic with a network boundary; it does not redesign the sandbox itself.

This is required because Session Runtime's own process must not spawn sandbox workers or hold the credentials/secrets a compromised execution could otherwise reach — see `ADR-0016`'s Security boundary section.

## Context

Not yet designed. Depends on the network transport decision (`PROJECT.md` Material Decisions) and on `WORK-0044`'s artifact-transfer redesign (this service must receive/resolve artifacts without depending on a caller-local filesystem path).

## Scope

### In Scope

- The Executor's own deployment artifact (not nested under `game/session/`, since it is not Session-owned code per `ADR-0016`'s "not a new business domain" framing — exact package/repository layout is implementer's call).
- A network-facing API implementing the conceptual `Executor.Execute(ctx, ExecutionRequest) (ExecutionResult, error)` contract `WORK-0053` defines the client side of.
- Relocating `WORK-0035`'s worker/`wazero`/QuickJS execution logic, determinism prelude, and protocol envelope into this service, replacing the stdin/stdout IPC with whatever the chosen network transport requires.
- Resolving/receiving the pinned artifact per whatever remote-safe mechanism `WORK-0044` defines — not a caller-local scratch-directory path.
- Basic health/readiness signaling sufficient for Session Runtime to distinguish "no healthy Executor instance available" from a genuine execution failure.

### Out of Scope

- Resource-limit tuning, pool sizing/reuse, and forced-termination policy beyond what already exists — `WORK-0036` (revised).
- The Session-side port/client — `WORK-0053`.
- Deployment manifests, autoscaling policy, and detailed observability — implementer's call within this WORK's own design, not frozen here.

## Approved Design

Not yet designed. Must preserve every isolation guarantee `WORK-0035`'s own `LOGICAL_CONTRACT.md` records (scratch-directory filesystem confinement, determinism prelude, empty worker environment, caller-deadline process kill) — this WORK relocates that logic, it does not weaken it.

## Constraints and Invariants

- Stateless with respect to a game session between calls (`GAME-ADR-0028`); no correctness may depend on two calls reaching the same Executor instance or worker.
- Holds no Session Runtime database credentials or application secrets unnecessary for execution.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

### Accepted / Canonical Knowledge

- `ARCHITECTURE.md` — Component Model's Infrastructure Service category gains a concrete example once this service exists.

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md` — the JavaScript Execution Runtime row updated to describe the new topology; the old same-host-subprocess description retired.

## Blockers

- Network transport choice (`PROJECT.md` Material Decisions).
- Depends on `WORK-0044`'s artifact-transfer redesign.

## Completion Record

Not yet started.
