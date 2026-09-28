# WORK-0036: Execution Resource Limits & Isolation Boundary

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`
- `game/docs/decisions/GAME-ADR-0030-sandbox-runtime-quickjs-wasm-wazero-process-isolated-workers.md`

Canonical context:
- `game/docs/decisions/GAME-ADR-0019-runtimeturn-execution-bound-and-terminal-cleanup.md` (the DSL-specific precedent this generalizes)
- `docs/projects/active/js-runtime-migration/works/WORK-0052-javascript-executor-service.md` (this WORK hardens that service's internal worker pool; it does not stand the service up)

## Rescoped (2026-09-27)

Originally scoped as hardening Session Runtime's own direct subprocess management. `ADR-0016` moves sandbox worker management into a separately deployed JavaScript Executor service (`WORK-0052`); this WORK's resource-limit/isolation-hardening scope moves with it — it is now internal hardening of the Executor service's own worker pool, not of Session Runtime's process tree. Nothing about the actual guarantees below changes; only which deployment owns enforcing them does.

## Outcome

Untrusted JavaScript (including Playhoot's own AI-generated code) must run inside a real isolation boundary with enforced resource limits, per `ADR-0015`'s "sandboxing is a separate, infrastructure-owned concern" principle: no direct access to the database, secrets, network, host filesystem, or arbitrary host modules/processes; enforced limits on compute time, memory, input/output/state size, and emitted-command count per invocation; the ability to terminate a non-cooperative execution without compromising the host process; and concurrency/consumption controls above the single-execution level (the platform-abuse concern `session-runtime-v1`'s own WORK-0022 already flagged as unowned at the DSL level, now more urgent given JavaScript's larger attack surface). This extends beyond live Session execution to every pipeline that processes authored/generated content (script validation, compilation/bundling, asset processing, authoring-time simulation/rendering) — all of which, per `ADR-0016`, are candidates for routing through the same Executor service rather than running with Session Runtime's own privileges.

## Context

`GAME-ADR-0030` resolves the sandbox runtime technology; `ADR-0016` resolves deployment topology (a separately deployed Executor service, not a Session Runtime subprocess). This WORK still needs its own design, now scoped inside that service: exact worker-pool mechanics (sizing, reuse, respawn-on-violation), the exact OS-level isolation mechanism beyond a plain process boundary (seccomp/cgroups/gVisor, or none beyond a plain restricted-privilege process for V1), the Executor service's own minimal-privilege deployment posture (`ADR-0016`'s Security boundary: no Session database credentials, no unnecessary application secrets, no general outbound internet access, restricted network policy), and the concurrency/consumption controls `session-runtime-v1`'s own WORK-0022 already flagged as unowned at the DSL level.

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

- Sandbox runtime technology and deployment topology are resolved (`GAME-ADR-0030`); remaining open design is this WORK's own worker-pool/OS-isolation mechanics, not blocked on a further decision.
- Concurrency/consumption-limit enforcement shape and thresholds require security/product input, mirroring `session-runtime-v1`'s own still-open WORK-0022.

## Completion Record

Not yet started.
