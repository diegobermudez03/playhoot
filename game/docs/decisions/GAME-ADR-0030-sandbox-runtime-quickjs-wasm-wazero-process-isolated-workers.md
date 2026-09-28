# GAME-ADR-0030: Sandbox Runtime — QuickJS-on-WASM (wazero) In OS-Process-Isolated Workers

Status: ACCEPTED
Created: 2026-09-27
Last status change: 2026-09-27
Supersedes: None
Superseded by: ADR-0016 (in part — the deployment-topology portion only: same-host subprocess workers become a separately deployed Executor service. The QuickJS-on-WASM/`wazero` technology choice and the process-isolated-worker layering inside that service are conserved and restated by ADR-0016)

## Context

`docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md` requires Playhoot to execute untrusted (including Playhoot's own AI-generated) JavaScript under real sandboxing, not merely a closed language. `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md` fixes the execution contract's shape but explicitly leaves the sandbox runtime technology as a required, separate decision (`docs/projects/active/js-runtime-migration/PROJECT.md`, Material Decisions #1/#2).

Four realistic approaches were compared: an embedded pure-Go interpreter (goja), V8 via CGO, a WASM-compiled JS engine run through a pure-Go WASM runtime, and a subprocess pool with OS-level sandboxing. goja was rejected as the sole boundary because its isolation is language-level only (same process/address space as the host, no hard memory ceiling, safety depends entirely on the interpreter never having an escape bug) — it does not meet the "real sandbox, not a label" bar this migration requires. V8/CGO was rejected for V1 because it breaks Playhoot's current pure-Go, single-binary, easily cross-compiled build (`go.mod` has no CGO dependencies today) for a strength advantage not yet demonstrated to be necessary. A bare subprocess pool alone was rejected as the *only* layer because it adds real new deployment machinery (process pool, IPC/serialization) without the additional memory/capability containment WASM provides for free.

## Decision

Session Runtime's rule-execution sandbox is **two complementary, independently-enforced layers**, not either one alone:

1. **WASM-level containment**: author-written JavaScript runs inside a QuickJS engine compiled to WebAssembly, hosted by `wazero` (a pure-Go WebAssembly runtime — no CGO). This bounds linear memory to a hard, configurable cap (`wazero`'s `WithMemoryLimitPages`/`WithMemoryCapacityFromMax`, verified as a real enforced trap on overflow, not merely advisory) and removes ambient host access by construction: a WASM module has no filesystem, network, or process capability unless Go explicitly imports a host function into it, so "no I/O, no secrets, no host modules" is enforced by the absence of any wiring, not by policy the authored code is trusted to respect.
2. **OS-process-level containment**: each execution runs inside a worker process separate from Session Runtime's main process, under OS-level isolation. This layer exists specifically because WASM-level interruption is verified to be insufficient alone (see Rationale) — the worker process is the authoritative backstop that can be killed outright regardless of what the WASM module is doing internally.

Concrete resource limits, forced interruption, capability restriction, and isolation testing (per `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`'s Consequences and `docs/projects/active/js-runtime-migration/works/WORK-0036-execution-resource-limits-and-isolation-boundary.md`) are designed against both layers together:

- **Memory**: enforced natively and deterministically by `wazero`'s linear-memory cap (Layer 1). This is a hard limit, not best-effort.
- **CPU time / runaway-execution termination**: **not** guaranteed by WASM-level interruption alone. Verified (see Rationale) that `wazero` has no native fuel-style deterministic instruction counting, and its `context.Context`-based cancellation is checked only at coarse "WASM operation boundaries" — a tight loop or recursive function that never calls back into a host-imported function may not be reliably interrupted this way. The authoritative CPU/wall-clock limit is therefore the worker process's own wall-clock deadline, enforced by the host (Session Runtime's worker manager) killing the worker process outright (OS-level, e.g. `SIGKILL`/process termination) if it does not return within budget. A `context.Context` deadline is still wired into the WASM call as a best-effort/faster-in-the-common-case mechanism, but the process-level kill is what the design's correctness actually depends on.
- **Capability restriction**: enforced at both layers — no host functions imported into the WASM module beyond what the execution contract (`GAME-ADR-0028`) requires (Layer 1), and the worker process itself runs with no filesystem/network/credential access beyond what it strictly needs to receive input and return output (Layer 2, exact OS mechanism — plain OS process with restricted privileges, seccomp, cgroups, or a stronger sandbox like gVisor — is `WORK-0036`'s own implementation-level design choice, not fixed here).
- **Isolation testing**: must include an adversarial test proving a WASM module attempting an unbounded tight loop with no host calls is still terminated within budget by the process-level backstop, not merely by (unverified) WASM-level interruption.

Concrete library selection (for example `fastschema/qjs`, `aperturerobotics/go-quickjs-wasi-reactor`, or an equivalent actively maintained QuickJS-on-`wazero` binding) and worker-process management mechanics (process pool sizing, respawn-on-violation, IPC/serialization shape) are `WORK-0035`/`WORK-0036`'s own implementation-level choices within this architecture, not fixed by this record.

## Rationale

This record's layered design is not a preference between "WASM sandboxing" and "process isolation" — it is a response to a verified, concrete gap: `wazero`'s own documentation and community discussion (see Sources) confirm it has no native deterministic fuel/instruction-counting mechanism, and its `context.Context` cancellation is coarse-grained, checked only at WASM operation boundaries, with an explicitly acknowledged caveat that a tight loop never calling a host function may not be interrupted reliably by that mechanism alone. Relying on WASM-level interruption as the sole CPU-time control would mean a runaway script (accidental or adversarial) could hang a worker indefinitely despite passing every other sandboxing check — an unacceptable gap given `ADR-0015`'s explicit requirement that a non-cooperative execution be terminable without compromising the host. Process-level termination has no equivalent gap: killing an OS process always stops it, regardless of what it was doing internally.

Combining the two layers rather than choosing one gets the actual benefit of each: WASM gives cheap, fine-grained memory/capability containment without new deployment machinery (no CGO, no new runtime binary dependency); OS-process isolation gives the CPU-time/hang guarantee WASM cannot make alone, and additionally bounds the blast radius of a WASM-runtime-itself bug (an escape from the WASM sandbox would still be contained by the worker process boundary).

## Alternatives Considered

### WASM/wazero alone (in-process, no separate worker process)

Rejected as the sole mechanism. Per the verified gap above, this leaves runaway-execution termination dependent on a coarse, best-effort mechanism with a known caveat for tight loops — not an acceptable authoritative guarantee for untrusted code.

### goja (embedded pure-Go interpreter) alone

Rejected — see Context. Language-level containment only; no hard memory ceiling; safety depends on the interpreter and on Playhoot never exposing a dangerous global, not on a structural boundary.

### V8 via CGO

Rejected for V1 — see Context. The strongest JS-engine-native isolation available, but breaks the current pure-Go build/cross-compilation story for a strength advantage this record's layered QuickJS/WASM/process design already achieves without CGO. Not forbidden to revisit later if QuickJS's feature coverage or performance proves insufficient at scale.

### Subprocess pool running a full Node/V8 runtime (no WASM layer)

Rejected as the sole mechanism — it achieves the process-isolation guarantee this record also adopts, but without WASM's additional memory/capability containment layer, and with a heavier runtime dependency (a full Node/V8 binary) than a QuickJS-on-WASM module.

## Consequences

- `docs/projects/active/js-runtime-migration/works/WORK-0035-sandboxed-javascript-execution-runtime.md` designs the execution contract against this runtime (QuickJS-on-WASM via `wazero`).
- `docs/projects/active/js-runtime-migration/works/WORK-0036-execution-resource-limits-and-isolation-boundary.md` designs the worker-process pool, its OS-level isolation mechanism, resource-limit enforcement (memory via `wazero`, CPU/hang via process-level kill), and the adversarial isolation test suite against this layered model.
- Playhoot takes on a new runtime dependency (a QuickJS-on-WASM Go binding) and new deployment topology (a worker-process pool separate from the main Session Runtime process) — a real, deliberate increase in operational surface accepted specifically because untrusted-code execution requires it; this is not introduced elsewhere in the codebase without equivalent justification.
- Authored JavaScript remains plain JavaScript; packaging/compilation into the WASM-hosted runtime's input form (source text, or a precompiled bytecode form if the chosen binding supports one) is Playhoot's own build/validate pipeline responsibility (`docs/projects/active/js-runtime-migration/works/WORK-0044-game-version-artifact-model.md`, `WORK-0037`), never something the author or an authoring AI needs to handle directly.

## Canonical Knowledge Impact

- `game/docs/decisions/INDEX.md` — this record added.
- `docs/projects/active/js-runtime-migration/PROJECT.md` — Material Decisions #1/#2 resolved by this record.

## Implementation Impact

Not authorized by this record beyond its own creation. Routed to `WORK-0035`/`WORK-0036`, now unblocked to move PLANNED -> DRAFT.

## Sources

Verified 2026-09-27 via web research (not assumed from training data, per explicit human direction):

- wazero resource-limiting/interruption mechanics: [Wazero Hardening for Go Embedders: Resource Limits, WASI Capabilities, and Plugin Isolation](https://www.systemshardening.com/articles/wasm/wazero-hardening/) — confirms native `WithMemoryLimitPages` hard memory cap; confirms no native fuel-style instruction counting; confirms `context.Context` cancellation is coarse-grained ("operation boundary, not per-instruction") with an explicit caveat that a tight loop never calling the host may not be interrupted reliably.
- Experimental (non-canonical) third-party fuel-metering package exists but is not part of stable `wazero`: [fuel package - github.com/thevilledev/wazero/experimental/fuel](https://pkg.go.dev/github.com/thevilledev/wazero/experimental/fuel) — not relied upon by this record given its experimental/unofficial status.
- QuickJS-on-`wazero` Go bindings evaluated as candidate implementations (library selection left to `WORK-0035`): [fastschema/qjs](https://github.com/fastschema/qjs) (CGO-free, QuickJS-NG/ES2023, async/await, filesystem/network isolated by default, exposes memory limit/stack size/execution timeout/GC threshold options), [aperturerobotics/go-quickjs-wasi-reactor](https://github.com/aperturerobotics/go-quickjs-wasi-reactor) (reentrant scheduling variant).
