# WORK-0036: Execution Resource Limits & Isolation Boundary

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- `game/docs/decisions/GAME-ADR-0019-runtimeturn-execution-bound-and-terminal-cleanup.md` (the DSL-specific precedent this generalizes)

## Outcome

Untrusted JavaScript (including Playhoot's own AI-generated code) must run inside a real isolation boundary with enforced resource limits, per `ADR-0015`'s "sandboxing is a separate, infrastructure-owned concern" principle: no direct access to the database, secrets, network, host filesystem, or arbitrary host modules/processes; enforced limits on compute time, memory, input/output/state size, and emitted-command count per invocation; the ability to terminate a non-cooperative execution without compromising the host process; and concurrency/consumption controls above the single-execution level (the platform-abuse concern `session-runtime-v1`'s own WORK-0022 already flagged as unowned at the DSL level, now more urgent given JavaScript's larger attack surface). This extends beyond live Session execution to every pipeline that processes authored/generated content (script validation, compilation/bundling, asset processing, authoring-time simulation/rendering).

## Context

Not yet designed. Depends on the sandbox runtime technology decision (`WORK-0035`'s Blocker) and the deployment-topology decision (`PROJECT.md` Material Decisions #2 — in-process isolate vs. separate worker pool/process vs. separate service).

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- A single non-cooperative or malicious execution must never be able to affect another Session's execution, the host process's stability, or any credential/secret the host process holds.
- Limits must be enforceable independent of whether the authored code cooperates (a hard boundary, not a convention the code is asked to respect).

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed — must include negative/resource-exhaustion tests (per `ADR-0015`'s explicit requirement that sandboxing evidence include adversarial/exhaustion testing, not merely a "sandbox" label).

## Documentation Impact

### Accepted / Canonical Knowledge

- A new security/infrastructure decision record (domain or global, per `docs/decisions/README.md`'s scope-resolution rule, decided when this WORK is drafted) recording the isolation boundary's exact guarantees.

### Current-State Documentation After Implementation

- Not yet designed.

## Blockers

- Sandbox runtime technology (shared with `WORK-0035`) and deployment topology — both block DRAFT.
- Concurrency/consumption-limit enforcement shape and thresholds require security/product input, mirroring `session-runtime-v1`'s own still-open WORK-0022.

## Completion Record

Not yet started.
