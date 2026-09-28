# Project: JavaScript Rule Execution & Frontend Contract Migration

Status: ACTIVE
Created: 2026-09-27
Last updated: 2026-09-27

## Goal

Implement `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`, `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`, `game/docs/decisions/GAME-ADR-0029-snapshot-based-session-runtime-persistence.md`, and `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md` end to end: retire Game Language (`program`/`engine` v1) as Playhoot's rule-authoring/execution mechanism in favor of author-written JavaScript running in a sandboxed, resource-limited runtime hosted by a separately deployed JavaScript Executor service that Session Runtime calls over an internal network boundary (not a subprocess of Session Runtime's own deployment); revert Session Runtime's persistence model from replay-first to snapshot-based; give Session Runtime its own persisted script/frontend artifact independent of Game Management (completing the Game Management/Session Runtime domain split `ADR-0014` accepted); build the backend capabilities required to deliver a versioned frontend package into an isolated iframe (views, effects, durable interaction-result delivery); and build the backend capabilities required for incremental, tool-driven, and integrated-chat AI-assisted authoring against this new execution model. This Project supersedes and absorbs `docs/projects/completed/management-session-domain-split/`'s goal (see that Project's own closing note) and coordinates with, rather than duplicates, `docs/projects/active/session-runtime-v1/` for session-lifecycle/lobby/live-connection capabilities that are independent of rule-execution language.

## Explicitly Out Of Scope

- Building the frontend application, its browser SDK/client library, its authoring-chat UI, or their repository. That repository does not exist yet; `ADR-0015` documents the contract this Project's backend WORK must serve, and is verified with test clients/fixtures, not a production frontend.
- Selecting AI provider(s), model(s), or their commercial/licensing terms for AI-assisted authoring (`WORK-0047`–`WORK-0049`) — these WORK build the backend orchestration/adapter capabilities against whatever provider is later chosen; provider selection is a separate business/vendor decision.
- Session lifecycle, lobby, live-connection-role, disconnect/reconnect transport, and inactivity-expiration mechanisms already owned by `docs/projects/active/session-runtime-v1/` (WORK-0008, WORK-0015, WORK-0020, WORK-0021, WORK-0023, WORK-0030) — these are independent of which language executes game rules and are not duplicated here. Where this Project's WORK materially affects one of them (see Ordering / Dependencies), it is flagged for coordination in both Projects' own files, not silently resolved in one.
- Host "kick a participant" / "transfer host" and any other product-scope question already tracked as open in `session-runtime-v1`'s own Material Decisions — unaffected by this migration.

## Current Work

**WORK-0035** is DONE (2026-09-27) — the sandboxed JavaScript execution runtime is implemented and independently reviewed APPROVED (three rounds; two rounds found real gaps — filesystem escape via the binding's default directory mount, and ambient `Date`/`Math.random`/`performance`/`os` leaking real non-determinism — both fixed and re-verified; see the WORK's own Completion Record).

**Topology revised (2026-09-27), same day, post-closure.** A review of WORK-0035's own implementation found its same-host subprocess topology (Session Runtime directly spawning and piping to its own re-exec'd worker) structurally coupled to a shared local filesystem and process tree in a way that conflicts with running untrusted execution at arm's length from Session Runtime's own credentials/secrets. `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md` now requires the JavaScript Executor to be a separately deployed workload, not a Session Runtime subprocess. WORK-0035 itself is not reopened (see its own Completion Record); two new WORK carry the migration forward: `WORK-0052` (stands up the separately deployed Executor, reusing WORK-0035's execution logic) and `WORK-0053` (the `Executor` port and network client at Session Runtime's own boundary, replacing WORK-0035's direct local invocation). `WORK-0036`, `WORK-0038`, `WORK-0039`, `WORK-0044`, `WORK-0047`, and `WORK-0051` are each revised to route through this new topology rather than assume a local call — see each WORK's own file.

Next candidates per Ordering / Dependencies: `WORK-0052` and `WORK-0053` (the actual split), which the rest of the execution-dependent WORK now waits on.

**WORK-0052** is DONE (2026-09-27) — the JavaScript Executor is now a genuinely separate deployable gRPC service (`game/session/jsexecutor/` — relocated post-closure, per explicit human direction, from a repository-root `jsexecutor/` into Session's own package tree specifically to signal it is Session's own deployable unit, not a repository-wide capability; see that package's own `README.md` and `WORK-0052`'s "Relocated" note), reusing `WORK-0035`'s execution logic faithfully (independently confirmed byte-for-byte during review); the old same-host `game/session/internal/jsengine`/`game/session/bootstrap` are deleted. Independent review: two rounds, first found two documentation-sync gaps (fixed), final APPROVED with no findings. See the WORK's own Completion Record.

**WORK-0053** is DONE (2026-09-27) — Session Runtime now has a real `Executor` port/client (`game/session/internal/executor`): the interface reusing `WORK-0035`'s proven `ExecutionInput`/`ExecutionOutput` shape, a production `GRPCClient` (pure-function-safe bounded retry on `Unavailable` only, never on `DeadlineExceeded` or any other status — the retry safety argument was independently stress-tested, not just implemented), and an in-memory `Fake` for tests. Independent review: one round, APPROVED with no findings. See the WORK's own Completion Record.

Both halves of `ADR-0016`'s topology split are now DONE. No `sessionlifecycle` call site invokes the new port yet — that wiring, and the persistence-model migration it depends on, is `WORK-0038`'s scope, the next candidate per Ordering / Dependencies (alongside `WORK-0044`, still needed for the artifact-transfer mechanism `WORK-0034`/`WORK-0033` depend on).

## Work

| Order | Work | Status |
|------:|------|--------|
| 1 | WORK-0035 — Sandboxed JavaScript Execution Runtime (Pure Function Contract) | DONE |
| 2 | WORK-0052 — JavaScript Executor Service (separately deployed workload) | DONE |
| 3 | WORK-0053 — Session-Side Remote Executor Port & Network Client | DONE |
| 4 | WORK-0036 — Execution Resource Limits & Isolation Boundary (now inside the Executor service) | PLANNED |
| 5 | WORK-0037 — Deterministic-Authoring Static Analysis / Lint Enforcement | PLANNED |
| 6 | WORK-0044 — Game Version Artifact Model (Script + Frontend Package + Contracts + Assets) | PLANNED |
| 7 | WORK-0034 — Session-Owned Executable Script Artifact & Package Restructuring (supersedes cancelled WORK-0031) | PLANNED |
| 8 | WORK-0032 — Composer-Mediated Session Creation Visibility Composition (reparented from `management-session-domain-split`) | PLANNED |
| 9 | WORK-0038 — Snapshot-Based Session Runtime Persistence Migration (implements GAME-ADR-0029) | PLANNED |
| 10 | WORK-0039 — Platform Command Protocol & Runtime Validation | PLANNED |
| 11 | WORK-0040 — Timer Obligations Adapted To JS Commands | PLANNED |
| 12 | WORK-0041 — Per-Player View Computation & Privacy Verification | PLANNED |
| 13 | WORK-0042 — Effects Delivery (Best-Effort, Restated From GAME-ADR-0020) | PLANNED |
| 14 | WORK-0043 — Durable Confirmed-Turn Result Delivery (Outbox) | PLANNED |
| 15 | WORK-0045 — Frontend Iframe Delivery Contract Specification | PLANNED |
| 16 | WORK-0046 — Frontend Package Serving & Versioned Asset Delivery | PLANNED |
| 17 | WORK-0033 — Cross-Domain Game Publish/Authoring Composition (reparented from `management-session-domain-split`) | PLANNED |
| 18 | WORK-0047 — Incremental Authoring Session Backend | PLANNED |
| 19 | WORK-0048 — MCP Tool Adapter For External AI Authoring | PLANNED |
| 20 | WORK-0049 — Integrated Authoring Chat Backend Orchestration | PLANNED |
| 21 | WORK-0050 — Scoped & Revocable Authoring Authorization | PLANNED |
| 22 | WORK-0051 — End-To-End Verification (gates project completion) | PLANNED |

22 WORK total: 3 DONE, 0 IMPLEMENTING, 0 READY, 0 DRAFT, 19 PLANNED.

## Ordering / Dependencies

- **Execution core first (1–5).** `WORK-0035` defines the execution contract (`previousState, event, context -> newState, commands`) and proved the sandbox technology; `WORK-0052` (the separately deployed Executor service) and `WORK-0053` (the Session-side port/network client) are the actual topology split `ADR-0016` requires, and gate essentially everything else that executes JavaScript. `WORK-0036` (isolation/resource-limit hardening) now lives inside `WORK-0052`'s service and depends on it existing first. `WORK-0037` (determinism lint) can be designed in parallel with 2–4 but converges with them before any is READY, since the sandbox boundary and the supported JavaScript profile are two views of the same runtime.
- **Artifact model before ownership migration, and before the Executor can resolve artifacts (6 before 7, and before 2–3 can be READY).** `WORK-0034` (Session owning its own executable-artifact table) needs `WORK-0044`'s artifact shape decided first; `WORK-0052`/`WORK-0053` also need `WORK-0044`'s remote-safe artifact-transfer mechanism decided before their own design can be READY, since an execution request can no longer carry a caller-local path.
- **Domain restructuring (7–8)** completes `ADR-0014`'s goal against the new artifact type: `WORK-0034` first, then `WORK-0032` (Composer-mediated visibility), exactly as the superseded Project ordered WORK-0031 before WORK-0032.
- **Persistence and protocol (9–11)** depend on `WORK-0035`'s contract existing (they consume its `newState`/`commands` shapes) and, per `ADR-0016`, on `WORK-0053`'s `Executor` port existing (that is what `WORK-0038` actually wires Session Runtime's call sites to) — but not on the domain restructuring landing first, so these can proceed in parallel with group 6–8 once 2–3 are far enough along.
- **Timers, views, effects, delivery (11–14)** depend on `WORK-0039`'s command vocabulary. `WORK-0043` (durable outbox for confirmed results) additionally depends on `WORK-0038`'s persistence model and must coordinate with `session-runtime-v1`'s `WORK-0020` (live-connection layer) for where delivery actually attaches — flagged, not resolved, here.
- **Frontend contract and serving (15–16)** depend on `WORK-0039` (command/view vocabulary) and `WORK-0044` (artifact model). `WORK-0046` must explicitly reconcile with `session-runtime-v1`'s `WORK-0009` (Client-Safe Game UI Manifest) — see that Project's own flag.
- **Publish composition (17)** depends on `WORK-0044` and, as before, on Game Management's not-yet-built authoring/publish write path — unchanged blocker from the superseded Project.
- **AI-assisted authoring (18–21)** depends on the execution/lint/artifact capabilities above existing enough to validate/simulate/render against — specifically `WORK-0053`'s `Executor` port (per `ADR-0016`, simulation must use the same real, separately deployed execution path production Sessions use, not an in-process shortcut), plus `WORK-0037`, `WORK-0044`. `WORK-0048`/`WORK-0049` both depend on `WORK-0047`'s shared authoring-session capability; `WORK-0050` can be designed in parallel with it.
- **`WORK-0051`** is this Project's own completion gate, not a capability of its own — it depends on essentially everything above being DONE, and must exercise the real Executor-as-a-separate-deployment topology, not an in-process substitute.

## Capability Coverage

Traceability from the accepted mandate's required capabilities to the WORK that owns each. No capability is "required but unowned."

| Capability | Owner |
|---|---|
| Sandboxed JS execution contract (pure function, stateless runtime) | WORK-0035 |
| JavaScript Executor as a separately deployed workload (network-facing API, worker-pool hosting, minimal-privilege deployment) | WORK-0052 |
| Session-side `Executor` port and production network client (no `exec.Command`/local-process dependency in Session Runtime) | WORK-0053 |
| Execution isolation, resource/time/memory/output/command-count limits, non-cooperative termination, concurrency/consumption controls | WORK-0036 (inside the Executor service, per ADR-0016) |
| Determinism encouragement (static analysis/lint for authored code) | WORK-0037 |
| Snapshot-based persistence (current state authoritative; signal log retained for best-effort reconstruction) | WORK-0038 |
| Platform command protocol + runtime (not just static-type) validation, including game-specific contracts | WORK-0039 |
| Timers as Playhoot-managed commands (schedule/cancel/fire), duplicates/competing/terminal-session semantics | WORK-0040 |
| Per-player private views computed by untrusted code, with privacy verification; recoverable view on load/reconnect without full replay | WORK-0041 |
| Cosmetic effects, best-effort delivery, correlation for repeated/late/absent effects | WORK-0042 |
| Durable delivery of confirmed interaction results (accepted/rejected/failed distinguished from transport ack) | WORK-0043 |
| Game version artifact model (script + frontend package + contracts + assets + capability versions, immutable per-session pin) | WORK-0044 |
| Session-owned executable artifact, Game Management/Session Runtime domain split completed | WORK-0034 |
| Composer-mediated create-time visibility composition | WORK-0032 |
| Orchestrator-mediated cross-domain publish/authoring composition | WORK-0033 |
| Frontend iframe trust boundary, permissions, view/effect/interaction API contract (documentation) | WORK-0045 |
| Backend serving of a session's pinned frontend package/assets | WORK-0046 |
| Incremental authoring (read/edit/validate/simulate/render draft) | WORK-0047 |
| External AI authoring tool integration (MCP adapter) | WORK-0048 |
| Integrated authoring chat backend orchestration (consumption/budget controls, resumability) | WORK-0049 |
| Scoped, revocable authoring authorization (edit/test vs. publish, per-project) | WORK-0050 |
| End-to-end verification (real JS game, persistence, timers, private views, effects, reconnect/recovery, best-effort reconstruction, concurrency, duplicates, failure limits) | WORK-0051 |
| Frontend application, SDK, authoring-chat UI implementation | Explicitly out of scope (future frontend repository) |
| AI provider/model selection and commercial terms | Explicitly out of scope (separate business decision) |
| Session lifecycle, lobby, live-connection roles, disconnect/reconnect transport, inactivity expiration | Owned by `session-runtime-v1`, coordinated not duplicated |

## Material Decisions Needing Human Input

None of these block creating this Project or its PLANNED WORK; each blocks the specific WORK named from reaching DRAFT/READY until resolved.

1. ~~**Sandbox runtime technology**~~ — **Resolved 2026-09-27**: `game/docs/decisions/GAME-ADR-0030-sandbox-runtime-quickjs-wasm-wazero-process-isolated-workers.md` (QuickJS-on-WASM via `wazero`, layered with OS-process-isolated workers — verified via web research that `wazero`'s own context-cancellation is coarse-grained and insufficient alone against a tight loop with no host calls).
2. ~~**Deployment topology for sandboxed execution**~~ — **Resolved 2026-09-27, revised same day**: initially resolved as OS-process-isolated workers within Session Runtime's own deployment; superseded the same day by `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md` after a review of `WORK-0035`'s implementation found that topology's shared-filesystem/shared-process-tree coupling incompatible with keeping untrusted execution away from Session Runtime's own credentials. Now: a separately deployed JavaScript Executor service, communicating with Session Runtime over an internal network protocol, internally using the same OS-process-isolated workers.
3. **Exact command/state wire schema** (`WORK-0035`/`WORK-0039`) — deferred until the runtime technology decision (1) is made, since it constrains what's efficient/safe to serialize across the sandbox boundary.
4. **Durable outbox mechanism for confirmed-turn results** (`WORK-0043`) — same-database durable table vs. an external queue/broker; must also reconcile with `GAME-ADR-0020`'s existing "no outbox for presentation" stance, which this Project extends rather than reopens.
5. **Artifact bundle format and storage** (`WORK-0044`) — how script + frontend package + contracts + assets are packaged/versioned/stored (single row, object storage, hybrid); "no build pipeline is required for a functional game" per the mandate must remain true regardless of the answer. Now also constrained by `ADR-0016`: whatever this decides must be resolvable by the Executor service without a filesystem shared with Session Runtime.
6. **AI provider(s)/model(s) and commercial terms** (`WORK-0047`–`WORK-0049`) — explicitly out of this Project's scope to decide, but each WORK's own design needs at least a placeholder/interface assumption to proceed; flagged so it is not silently assumed away.
7. **Whether strict deterministic replay is ever required later** (for example, for a future anti-cheat or audit requirement) — `GAME-ADR-0029`'s Alternatives Considered explicitly leaves this open rather than foreclosing it; no WORK currently depends on it.
8. ~~**Network transport between Session Runtime and the JavaScript Executor**~~ — **Resolved 2026-09-27**: gRPC. `WORK-0052`/`WORK-0053` design their service/client against a `.proto`-defined API rather than an ad hoc wire format.

## Completion Criteria

This Project is complete when:

1. Every WORK in the table above is DONE, or explicitly moved to this Project's Out Of Scope with human confirmation.
2. Every row in the Capability Coverage matrix reads DONE or "explicitly out of scope" — none reads "not implemented"/"partial" against a capability this Project's Goal requires.
3. `game/language/v1/program`/`engine` (Game Language) is fully retired from Session Runtime's live execution path — no production code path still depends on it.
4. `docs/projects/active/session-runtime-v1/`'s flagged coordination points (`WORK-0017`, `WORK-0009`) have been explicitly resolved (redesigned, reconciled, or confirmed unaffected) — not left silently stale.
5. `WORK-0051`'s end-to-end verification has run against a real authored JavaScript game exercising private views, timers, effects, disconnect/reconnect, best-effort reconstruction, concurrency, and resource-limit enforcement — against the real, separately deployed JavaScript Executor topology, not an in-process substitute.
6. Session Runtime's own code imports no `wazero`, QuickJS, worker-pool, or `exec.Command`/local-subprocess dependency, directly or transitively — per `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`, it depends only on the `Executor` port (`WORK-0053`).

When satisfied, this Project moves from `docs/projects/active/` to `docs/projects/completed/`.
