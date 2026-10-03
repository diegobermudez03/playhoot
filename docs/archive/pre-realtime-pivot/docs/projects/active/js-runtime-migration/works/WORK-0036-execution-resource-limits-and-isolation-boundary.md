# WORK-0036: Execution Resource Limits & Isolation Boundary

Status: DONE
Created: 2026-09-27
Last status change: 2026-09-29 (READY -> IMPLEMENTING -> DONE, same day; 4 independent-review rounds, each narrow CHANGES_REQUIRED fixed in turn, round 4 returned APPROVED with no findings - see Completion Record)

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`
- `game/docs/decisions/GAME-ADR-0030-sandbox-runtime-quickjs-wasm-wazero-process-isolated-workers.md`

Canonical context:
- `session/docs/decisions/SESSION-ADR-0018-runtimeturn-execution-bound-and-terminal-cleanup.md` (the DSL-specific precedent this generalizes)
- `docs/projects/active/js-runtime-migration/works/WORK-0052-javascript-executor-service.md` (this WORK hardens that service's internal worker pool; it does not stand the service up)

## Rescoped (2026-09-27)

Originally scoped as hardening Session Runtime's own direct subprocess management. `ADR-0016` moves sandbox worker management into a separately deployed JavaScript Executor service (`WORK-0052`); this WORK's resource-limit/isolation-hardening scope moves with it — it is now internal hardening of the Executor service's own worker pool, not of Session Runtime's process tree. Nothing about the actual guarantees below changes; only which deployment owns enforcing them does.

## Outcome

Untrusted JavaScript (including Playhoot's own AI-generated code) must run inside a real isolation boundary with enforced resource limits, per `ADR-0015`'s "sandboxing is a separate, infrastructure-owned concern" principle: no direct access to the database, secrets, network, host filesystem, or arbitrary host modules/processes; enforced limits on compute time, memory, input/output/state size, and emitted-command count per invocation; the ability to terminate a non-cooperative execution without compromising the host process; and concurrency/consumption controls above the single-execution level (the platform-abuse concern `session-runtime-v1`'s own WORK-0022 already flagged as unowned at the DSL level, now more urgent given JavaScript's larger attack surface). This extends beyond live Session execution to every pipeline that processes authored/generated content (script validation, compilation/bundling, asset processing, authoring-time simulation/rendering) — all of which, per `ADR-0016`, are candidates for routing through the same Executor service rather than running with Session Runtime's own privileges.

## Context

`GAME-ADR-0030` resolves the sandbox runtime technology; `ADR-0016` resolves deployment topology (a separately deployed Executor service, not a Session Runtime subprocess). Both records explicitly delegate the remaining mechanics — exact OS-level isolation mechanism, worker-pool sizing/reuse, concrete resource-limit values — to this WORK as implementation-level design, not a further blocked decision (`GAME-ADR-0030`: "exact OS mechanism... is `WORK-0036`'s own implementation-level design choice, not fixed here"; "worker-process management mechanics... are `WORK-0035`/`WORK-0036`'s own implementation-level choices").

**Current implementation audit (2026-09-29), against `session/jsexecutor`'s actual code, not assumed:**

- **Already done, carried over from `WORK-0035`/`WORK-0052`, this WORK does not redo it:** WASM-level memory containment (`wazero`'s linear-memory cap via `qjs.Option.MemoryLimit`, GAME-ADR-0030's Layer 1, "hard, not best-effort"); the authoritative CPU/hang backstop (`Execute` uses `exec.CommandContext`, killing the entire worker process on caller deadline — `TestExecute_CallerDeadline_KillsRunawayWorker` already proves this against a genuine tight loop, GAME-ADR-0030's Layer 2); filesystem/environment/timing isolation (`sandbox/LOGICAL_CONTRACT.md`'s "Isolation Guarantees" section, independently verified twice against the binding's actual behavior).
- **The real gap, per `LOGICAL_CONTRACT.md`'s own "Explicitly Not Guaranteed Here" section:** resource-limit *values* (`MemoryLimit`/`MaxStackSize`/`MaxExecutionTime`) are "conservative best-effort defaults" never adversarially tested; no test proves a memory-exhaustion or stack-overflow script is actually rejected within budget rather than crashing the worker or the WASM runtime.
- **Missing entirely, confirmed by reading `internal/grpcserver/server.go`:** no concurrency control of any kind. Every `Execute` gRPC call spawns a brand-new OS process unconditionally — a burst of concurrent requests can spawn an unbounded number of worker processes, exhausting host memory/CPU with no backpressure. This is the "concurrency/consumption controls above the single-execution level" this WORK's Outcome already names, distinct from `session-runtime-v1`'s own `WORK-0022` (per-user/session business-policy rate limiting, a different layer, still PLANNED/blocked on its own business-number question) — this WORK's concern is protecting the Executor service's own host from being overwhelmed, not enforcing per-user fairness.
- **Missing entirely:** no cap on emitted output size (`NewState`/`RequestedCommands` combined) or requested-command count per invocation, despite the Outcome naming both as required.
- **No deployment manifest exists for any service in this repository yet** (confirmed: no `Dockerfile` anywhere in the repo; `docker-compose.yaml` covers only local-development Postgres/Adminer). `ADR-0016`'s minimal-privilege deployment posture (no DB credentials, no app secrets, restricted network policy) therefore has no established pattern to extend — see Material Decisions.

## Scope

### In Scope

- **Concurrency ceiling on the Executor service**: a bounded semaphore around `Execute`, rejecting or queuing (exact behavior below) once a configurable maximum in-flight-executions count is reached, so a request burst cannot spawn unbounded worker processes. Configurable via an environment variable with a conservative, documented default (proposed: derived from available CPU, e.g. `runtime.NumCPU() * 4`, since each worker process is a short-lived, single-purpose spawn, not a long-held resource) — an engineering default subject to tuning, explicitly not a per-user/business policy (`WORK-0022`'s own territory, unaffected here).
- **New resource caps not currently enforced**: a maximum serialized size for the combined `NewState`+`RequestedCommands` output, and a maximum `RequestedCommands` count per invocation. Exceeding either is a business-level outcome (`*ScriptRejectedError`, exactly like a thrown exception or malformed shape today) — never a crash, never silently truncated data.
- **Adversarial/exhaustion test suite** (per `ADR-0015`'s explicit requirement): a script that allocates until it hits the memory cap, a script that recurses until it hits the stack cap, a script whose `execute` returns an oversized state/output, a script whose `execute` returns an excessive command count, and a burst of concurrent executions exceeding the new concurrency ceiling — each proving the *documented* behavior (rejection/backpressure), not merely that the process survives.
- **Tuning `MemoryLimit`/`MaxStackSize`/`MaxExecutionTime`** if the adversarial tests above reveal the current best-effort defaults do not actually hold under test (`LOGICAL_CONTRACT.md`'s own flagged gap) — the existing values are a starting point, not treated as already correct.
- **`sandbox/LOGICAL_CONTRACT.md` rewritten** once this WORK lands: every item currently listed under "Explicitly Not Guaranteed Here" that this WORK actually closes must move out of that section into "Isolation Guarantees," accurately describing implemented reality, not aspirational intent.

### Explicitly Out Of Scope

- **A deployment manifest** (Dockerfile, non-root user, restricted egress, Kubernetes NetworkPolicy) for `jsexecutor` or any other service — no service in this repository has one yet, and building the first one only for `jsexecutor` would get ahead of an established pattern. Deferred to a future deployment/infrastructure initiative covering every service, not invented piecemeal here. `ADR-0016`'s minimal-privilege posture remains an accepted requirement, just not one this WORK operationalizes into an actual manifest.
- **OS-level isolation beyond a plain restricted process** (seccomp, cgroups, gVisor). `GAME-ADR-0030` already frames this as optional ("or none beyond a plain restricted-privilege process for V1"). Each adds a real new dependency/deployment-runtime requirement (a container runtime supporting gVisor; a Linux-only syscall-filtering profile to author and maintain; cgroup filesystem access typically only available under a container runtime) that this repository's current dev/target environment does not establish anywhere else, for a guarantee GAME-ADR-0030 already places on a *different*, already-implemented layer (WASM memory cap + process-level deadline kill are the two layers the design's correctness actually depends on — see Rationale in that record). Classified NOT NEEDED now, LATER if a real deployment target or a security review demands a harder OS boundary.
- **True worker-process pooling/reuse.** The current spawn-fresh-process-per-call model is already proven correct (`TestExecute_ConcurrentSessionsDoNotInterfere`, no state ever crosses invocations by construction) and already the authoritative isolation boundary GAME-ADR-0030 depends on. Pooling would add real complexity (reuse hygiene, respawn-on-violation, a compromised/corrupted worker outliving its own execution) for a performance optimization, not a correctness or security requirement. Classified NOT NEEDED now; revisit only if real load/latency data shows spawn-per-call is an actual bottleneck.
- Per-user/per-session business-policy rate limiting (concurrent Sessions per user, request rate per endpoint/IP) — `session-runtime-v1`'s own `WORK-0022`, a different layer, unaffected by this WORK.
- Re-deriving or replacing the WASM memory cap or the process-deadline-kill mechanism themselves — both are already-accepted, already-implemented layers this WORK verifies/tunes, not redesigns.

## Approved Design

- A new bounded semaphore (`chan struct{}` or `golang.org/x/sync/semaphore`, no new runtime dependency either way) inside `internal/grpcserver.Server`, acquired before calling `sandbox.Execute` and released after it returns. Exceeding the ceiling returns `codes.ResourceExhausted` (a real gRPC status the client — Session Runtime's `GRPCClient` — can distinguish from `codes.Unavailable`/`codes.Internal`; whether `GRPCClient`'s own retry policy should treat `ResourceExhausted` as retryable is this WORK's own design task, consistent with its existing "retry only `Unavailable`" stance unless a concrete reason to widen it is found). Default ceiling: `runtime.NumCPU() * 4`, overridable via an env var (name decided during implementation).
- **Reject, never queue (confirmed 2026-09-29).** A caller exceeding the ceiling gets an immediate `ResourceExhausted`, not a wait for a freed slot. Queuing would move the same unbounded-accumulation risk from "too many OS processes" to "too many blocked gRPC calls/goroutines waiting on a channel," which is harder to observe and still consumes memory per waiter. Rejecting keeps the Executor itself simple (accept or reject, no queue/timeout state to manage) and puts the backpressure decision (retry now, retry later, surface a user-facing failure) where the business context to make it actually lives - Session Runtime's own `GRPCClient`, not the Executor.
- Output-size and command-count checks added to `sandbox.Execute` (the host-process side, after decoding the worker's response, before returning to the caller) — not inside the worker itself, so no worker-side code path needs to reason about the caller's limits. Exceeding either returns `*ScriptRejectedError`, reusing the existing business-level-outcome path `internal/grpcserver` already maps to `ExecuteResponse_Rejected`.
- Exact numeric defaults for the two new caps (output size, command count) and the concurrency ceiling's env var name are Implementation Freedom, tuned against the adversarial test suite this WORK also writes — proposed starting points: 256 KiB combined output, 100 commands per invocation, `runtime.NumCPU() * 4` concurrent executions.

## Constraints and Invariants

- A single non-cooperative or malicious execution must never be able to affect another Session's execution, the host process's stability, or any credential/secret the host process holds.
- Limits must be enforceable independent of whether the authored code cooperates (a hard boundary, not a convention the code is asked to respect).
- A resource-limit rejection is always a business-level outcome (`*ScriptRejectedError`/`ExecuteResponse_Rejected`), never a crash, and never silently accepted with truncated/corrupted data.
- The concurrency ceiling protects the Executor service's own host from aggregate overload; it is not a substitute for `session-runtime-v1`'s own `WORK-0022` per-user/business rate-limiting layer, and must not be described as one.

## Acceptance Criteria

- A burst of concurrent `Execute` calls beyond the configured ceiling is rejected with `codes.ResourceExhausted`, not left to spawn unbounded worker processes (proven under test, not merely argued).
- A script that allocates past the memory cap, recurses past the stack cap, or runs past the execution-time cap is rejected as a business-level outcome, not a worker crash or a hung `Execute` call — each proven by its own adversarial test.
- A script whose `execute` returns an oversized combined `NewState`/`RequestedCommands`, or an excessive `RequestedCommands` count, is rejected as a business-level outcome.
- `sandbox/LOGICAL_CONTRACT.md`'s "Explicitly Not Guaranteed Here" section accurately reflects only what genuinely remains unguaranteed after this WORK, not what was true before it.
- `go build`/`go vet`/`go test ./...` clean, including the new adversarial/exhaustion test suite.

## Implementation Freedom

- Exact concurrency-ceiling default value and its env var name.
- Exact output-size/command-count cap values (proposed starting points above, tunable against adversarial testing).
- Whether the concurrency semaphore lives in `internal/grpcserver` directly or a small new internal helper package.
- Whether `GRPCClient`'s retry policy is widened to treat `codes.ResourceExhausted` as retryable (a narrow, reversible client-side choice, not a change to the Executor's own contract).

## Verification

- `go test ./session/jsexecutor/...` including the new adversarial suite (memory exhaustion, stack overflow, output-size/command-count overflow, concurrency-ceiling backpressure).
- Manual/integration-level check that a deliberately pathological script (large allocation, deep recursion, huge output, tight loop) is rejected/killed within its documented bound when run against the real `jsexecutor` binary, not only against the in-process test harness.

## Documentation Impact

### Accepted / Canonical Knowledge

- `sandbox/LOGICAL_CONTRACT.md` — "Isolation Guarantees" and "Explicitly Not Guaranteed Here" sections updated to reflect implemented reality.

### Current-State Documentation After Implementation

- `session/CURRENT_STATE.md`'s "JavaScript Execution Runtime" row — currently states "Resource-limit tuning/hardening and worker pooling are not yet implemented"; update to reflect what this WORK actually closes (resource-limit tuning/adversarial testing, concurrency ceiling) versus what remains deliberately deferred (worker pooling — Explicitly Out Of Scope above, NOT NEEDED for now).

## Blockers

None remaining — both Material Decisions below are resolved.

## Material Decisions (Resolved 2026-09-29)

1. **Does this WORK build a deployment manifest (Dockerfile, minimal-privilege posture) for `jsexecutor`?** Resolved: no — explicitly out of scope for now, since no service in this repository has one yet; deferred to a future deployment/infrastructure initiative covering every service, not invented piecemeal here for `jsexecutor` alone.
2. **Does the Executor's concurrency ceiling need a business/security-reviewed numeric threshold before READY (mirroring `WORK-0022`'s own blocked status), or is a documented, configurable engineering default acceptable for V1?** Resolved: a documented, configurable engineering default is acceptable — this is a defensive ceiling protecting the Executor service's own host from aggregate overload, not a per-user/business rate-limiting policy (that remains `WORK-0022`'s own, separately blocked, concern).

## Completion Record

Implemented 2026-09-29, same day as READY.

**`session/jsexecutor/internal/grpcserver/server.go`:** `Server` gained a `sem chan struct{}` field; `New(maxConcurrentExecutions int) *Server` replaces the old no-arg constructor. `Execute` acquires the semaphore via a non-blocking `select`/`default` before doing anything else; a call arriving once the ceiling is already held is rejected immediately with `codes.ResourceExhausted`, never spawning a worker process and never queued (per the confirmed design decision).

**`session/jsexecutor/main.go`:** new `readMaxConcurrentExecutions` reads an optional `MAX_CONCURRENT_EXECUTIONS` env var, defaulting to `runtime.NumCPU() * 4` when unset (the confirmed engineering default, not a business-reviewed number) - wired into `grpcserver.New`.

**`session/jsexecutor/internal/sandbox/protocol.go`/`execute.go`:** new `defaultMaxOutputBytes` (256 KiB) and `defaultMaxCommandCount` (100) constants; `Execute` checks the decoded worker response against both, after `Rejected`/`Fatal` are already handled, returning `*ScriptRejectedError` on either violation - enforced host-side, no worker-side change needed.

**Adversarial test suite (the real point of this WORK), all passing against the actual `fastschema/qjs` binding, not assumed:**
- Memory exhaustion (exponential string doubling): throws a catchable `InternalError: out of memory`, cleanly surfaced as `*ScriptRejectedError` - confirmed deterministic across repeated runs.
- Stack overflow (unbounded recursion, no base case): does **not** throw a catchable JS exception, unlike memory exhaustion - it is an infrastructure-level failure inside the binding itself (a WASM trap surfacing through the runtime's own cleanup), always recovered cleanly as `*WorkerExecutionError`, confirmed deterministic across repeated runs. This asymmetry (memory exhaustion is graceful, stack overflow is not) was previously unverified and is now documented in `LOGICAL_CONTRACT.md`. Both failure modes are safely contained by the existing OS-process boundary - direct confirmation that `GAME-ADR-0030`'s layered design (not either layer alone) is what the isolation guarantee actually depends on.
- Oversized output and excessive command count: both new caps confirmed rejected as `*ScriptRejectedError`.
- Concurrency ceiling: confirmed a runaway call holding the ceiling's only slot causes a concurrent second call to be rejected with `ResourceExhausted`, not queued. This test initially flaked (a genuine two-contender race for the single slot, since semaphore acquisition happens before the comparatively slow worker-process spawn, so the probing call could otherwise win the race repeatedly by chance) - fixed with a short deterministic settle delay after starting the holding call, confirmed stable across 10 repeated runs.
- Memory/stack overflow tests confirmed deterministic across 5 repeated runs each; the full `jsexecutor` suite confirmed stable across 3 repeated full runs.

**Explicitly not changed:** `session/internal/executor.GRPCClient`'s retry policy still retries only `codes.Unavailable`, not `codes.ResourceExhausted` - left unchanged deliberately (Implementation Freedom explicitly allowed widening it, but no concrete reason to do so surfaced during implementation; a caller can still decide to retry a `ResourceExhausted` failure at its own call site).

**Documentation synchronized:** `sandbox/LOGICAL_CONTRACT.md` (`Isolation Guarantees`/`Explicitly Not Guaranteed Here` rewritten to reflect adversarially-verified reality, including the memory-vs-stack asymmetry); `session/CURRENT_STATE.md`'s "JavaScript Execution Runtime" row.

**Verification:** `go build`/`go vet` clean repository-wide. `go test ./session/jsexecutor/...` clean (including the new adversarial suite), confirmed stable across repeated runs. Full `go test ./...` clean except the same two pre-existing, unrelated failures already documented in `WORK-0038`'s Completion Record (`TestNoInternalDocCitationsInComments` on an untouched file; the pre-existing `session-runtime-v1`-owned Join/Leave race) - independently re-confirmed still pre-existing and unrelated to this WORK's diff.

**Independent review, round 1:** performed by a fresh, read-only agent per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`. Verdict: **CHANGES_REQUIRED**. Two REQUIRED_FIX findings, both applied:
- This WORK's own new code violated the repository's mechanically-enforced "no citing internal documents as a stand-in for explanation" comment standard: 7 new comments cited "WORK-0036" directly as justification instead of stating the reasoning in plain language (`session/jsexecutor/internal/grpcserver/server.go`, `server_test.go`, `session/jsexecutor/internal/sandbox/execute_test.go` (x2), `protocol.go`, `session/jsexecutor/main.go` (x2)). Fixed: all 7 reworded to state their reasoning directly; `TestNoInternalDocCitationsInComments` re-confirmed only the 3 pre-existing, untouched-file violations remain (unrelated to this WORK, already documented in `WORK-0038`'s own Completion Record).
- The WORK file's own `Status:` field read READY while already carrying a filled-in Completion Record describing implementation as done - inconsistent with `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`'s status model. Fixed: set to IMPLEMENTING.

One NON_BLOCKING finding accepted as-is, not acted on: `docs/projects/active/js-runtime-migration/internal/AI_CONTEXT.md`'s own WORK-0036 checkpoint paragraph was stale (still described DRAFT/awaiting-review) relative to the WORK file's own state in the same diff - refreshed as part of this same fix pass regardless, since `AI_CONTEXT.md`'s resume header should stay current at meaningful checkpoints. One additional NON_BLOCKING observation: `TestExecute_CallerDeadline_ReturnsDeadlineExceeded` (pre-existing, untouched by this WORK beyond an unrelated test-helper signature change) failed once under real-Postgres full-suite load but passed 10/10 in isolation and on a second full-suite run - not attributable to this WORK, flagged for independent future attention.

**Independent review, round 2:** performed by a fresh, read-only agent. Verdict: **CHANGES_REQUIRED** (one small finding). One REQUIRED_FIX: `grpcserver.New`'s doc comment still evaded only the letter of the mechanical citation check ("see this WORK's own file for why immediate rejection was chosen over queuing") rather than actually stating the reasoning in plain language, as the standard's substance requires. Fixed: reworded to state the actual reasoning directly (queuing would move the same unbounded-accumulation risk to blocked callers instead of OS processes; rejection keeps the Server simple and puts the backpressure decision where the business context to make it actually lives). `go build`/`go vet` clean; `TestNoInternalDocCitationsInComments` still reports only the 3 pre-existing, unrelated violations; `go test ./session/jsexecutor/...` re-confirmed clean across repeated runs. Round 2 otherwise re-confirmed every other round-1 fix and the full substance of the implementation as correct (scope discipline, Approved Design compliance, adversarial-suite determinism, documentation accuracy) - independently re-verified, not merely re-read from the Completion Record.

**Independent review, round 3:** performed by a fresh, read-only agent. Verdict: **CHANGES_REQUIRED** (one finding; confirmed the round-2 comment fix was genuinely adequate, not just re-checking the letter of it). One REQUIRED_FIX: `session/CURRENT_STATE.md`'s "Current Gaps" section (a separate location from the capability-table row this WORK's Documentation Impact already updated) still asserted "resource-limit enforcement/tuning and worker-process pooling... are not yet implemented," directly contradicting the already-updated row four lines above - an internal inconsistency this WORK's own Documentation Impact should have caught. Fixed: reworded to state resource-limit enforcement/tuning is now implemented and adversarially tested, while keeping worker-pooling/OS-isolation/deployment-manifest correctly listed as still deferred. (The surrounding sentence's separate claim that "every step still executes the Game Language engine directly" is itself stale - Game Language was fully retired by `WORK-0038` - but that predates this WORK and is out of its own scope; already tracked as a known documentation gap in this Project's own `AI_CONTEXT.md`, not invented or expanded here.)

**Independent review, round 4:** performed by a fresh, read-only agent, independently re-verifying every substantive claim from primary evidence (diffs, live test runs) rather than trusting this Completion Record's own narrative. Verdict: **APPROVED, no findings.** Confirmed: round-3's `session/CURRENT_STATE.md` fix is real and complete with no other remaining contradiction in that file; comment-standard compliance holds (only the 3 pre-existing, unrelated violations remain); full adversarial suite deterministic across 5 repeated runs; repository-wide test suite clean except the same two already-documented pre-existing failures; Approved Design compliance confirmed directly from the diff (semaphore, output/command caps, env var default); scope discipline held (no Dockerfile, no seccomp/cgroups/gVisor, no worker-pooling, no per-user rate-limiting anywhere in the repo). One NON_BLOCKING note (this file's own header phrasing and `PROJECT.md`'s checkpoint paragraph still described round-1 state) - resolved as part of this same closure pass.
