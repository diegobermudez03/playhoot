# Project: JavaScript Rule Execution & Frontend Contract Migration

Status: ACTIVE
Created: 2026-09-27
Last updated: 2026-09-27

## Goal

Implement `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`, `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`, and `game/docs/decisions/GAME-ADR-0029-snapshot-based-session-runtime-persistence.md` end to end: retire Game Language (`program`/`engine` v1) as Playhoot's rule-authoring/execution mechanism in favor of author-written JavaScript running in a sandboxed, resource-limited runtime owned by Session Runtime; revert Session Runtime's persistence model from replay-first to snapshot-based; give Session Runtime its own persisted script/frontend artifact independent of Game Management (completing the Game Management/Session Runtime domain split `ADR-0014` accepted); build the backend capabilities required to deliver a versioned frontend package into an isolated iframe (views, effects, durable interaction-result delivery); and build the backend capabilities required for incremental, tool-driven, and integrated-chat AI-assisted authoring against this new execution model. This Project supersedes and absorbs `docs/projects/completed/management-session-domain-split/`'s goal (see that Project's own closing note) and coordinates with, rather than duplicates, `docs/projects/active/session-runtime-v1/` for session-lifecycle/lobby/live-connection capabilities that are independent of rule-execution language.

## Explicitly Out Of Scope

- Building the frontend application, its browser SDK/client library, its authoring-chat UI, or their repository. That repository does not exist yet; `ADR-0015` documents the contract this Project's backend WORK must serve, and is verified with test clients/fixtures, not a production frontend.
- Selecting AI provider(s), model(s), or their commercial/licensing terms for AI-assisted authoring (`WORK-0047`–`WORK-0049`) — these WORK build the backend orchestration/adapter capabilities against whatever provider is later chosen; provider selection is a separate business/vendor decision.
- Session lifecycle, lobby, live-connection-role, disconnect/reconnect transport, and inactivity-expiration mechanisms already owned by `docs/projects/active/session-runtime-v1/` (WORK-0008, WORK-0015, WORK-0020, WORK-0021, WORK-0023, WORK-0030) — these are independent of which language executes game rules and are not duplicated here. Where this Project's WORK materially affects one of them (see Ordering / Dependencies), it is flagged for coordination in both Projects' own files, not silently resolved in one.
- Host "kick a participant" / "transfer host" and any other product-scope question already tracked as open in `session-runtime-v1`'s own Material Decisions — unaffected by this migration.

## Current Work

Nothing is yet DRAFT. Per `docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md`'s just-in-time design guidance, this Project's roadmap is planned broadly now (all WORK below PLANNED, two reparented from the superseded domain-split Project); the first candidates for PLANNED -> DRAFT are `WORK-0035` (the execution contract everything else integrates against) and `WORK-0044` (the artifact model `WORK-0034`/`WORK-0033` both need) — see Ordering / Dependencies.

## Work

| Order | Work | Status |
|------:|------|--------|
| 1 | WORK-0035 — Sandboxed JavaScript Execution Runtime (Pure Function Contract) | PLANNED |
| 2 | WORK-0036 — Execution Resource Limits & Isolation Boundary | PLANNED |
| 3 | WORK-0037 — Deterministic-Authoring Static Analysis / Lint Enforcement | PLANNED |
| 4 | WORK-0044 — Game Version Artifact Model (Script + Frontend Package + Contracts + Assets) | PLANNED |
| 5 | WORK-0034 — Session-Owned Executable Script Artifact & Package Restructuring (supersedes cancelled WORK-0031) | PLANNED |
| 6 | WORK-0032 — Composer-Mediated Session Creation Visibility Composition (reparented from `management-session-domain-split`) | PLANNED |
| 7 | WORK-0038 — Snapshot-Based Session Runtime Persistence Migration (implements GAME-ADR-0029) | PLANNED |
| 8 | WORK-0039 — Platform Command Protocol & Runtime Validation | PLANNED |
| 9 | WORK-0040 — Timer Obligations Adapted To JS Commands | PLANNED |
| 10 | WORK-0041 — Per-Player View Computation & Privacy Verification | PLANNED |
| 11 | WORK-0042 — Effects Delivery (Best-Effort, Restated From GAME-ADR-0020) | PLANNED |
| 12 | WORK-0043 — Durable Confirmed-Turn Result Delivery (Outbox) | PLANNED |
| 13 | WORK-0045 — Frontend Iframe Delivery Contract Specification | PLANNED |
| 14 | WORK-0046 — Frontend Package Serving & Versioned Asset Delivery | PLANNED |
| 15 | WORK-0033 — Cross-Domain Game Publish/Authoring Composition (reparented from `management-session-domain-split`) | PLANNED |
| 16 | WORK-0047 — Incremental Authoring Session Backend | PLANNED |
| 17 | WORK-0048 — MCP Tool Adapter For External AI Authoring | PLANNED |
| 18 | WORK-0049 — Integrated Authoring Chat Backend Orchestration | PLANNED |
| 19 | WORK-0050 — Scoped & Revocable Authoring Authorization | PLANNED |
| 20 | WORK-0051 — End-To-End Verification (gates project completion) | PLANNED |

20 WORK total: 0 DONE, 0 IMPLEMENTING, 0 READY, 0 DRAFT, 20 PLANNED.

## Ordering / Dependencies

- **Execution core first (1–3).** `WORK-0035` defines the execution contract (`previousState, event, context -> newState, commands`) every other WORK integrates against; `WORK-0036` (isolation/resource limits) and `WORK-0037` (determinism lint) can be designed in parallel with it but converge with it before any of them is READY, since the sandbox boundary and the supported JavaScript profile are two views of the same runtime.
- **Artifact model before ownership migration (4 before 5).** `WORK-0034` (Session owning its own executable-artifact table) needs `WORK-0044`'s artifact shape decided first, or it risks designing a table for the wrong artifact shape twice.
- **Domain restructuring (5–6)** completes `ADR-0014`'s goal against the new artifact type: `WORK-0034` first, then `WORK-0032` (Composer-mediated visibility), exactly as the superseded Project ordered WORK-0031 before WORK-0032.
- **Persistence and protocol (7–9)** depend on `WORK-0035`'s contract existing (they consume its `newState`/`commands` shapes) but not on the domain restructuring landing first — these can proceed in parallel with group 4–6.
- **Timers, views, effects, delivery (9–12)** depend on `WORK-0039`'s command vocabulary. `WORK-0043` (durable outbox for confirmed results) additionally depends on `WORK-0038`'s persistence model and must coordinate with `session-runtime-v1`'s `WORK-0020` (live-connection layer) for where delivery actually attaches — flagged, not resolved, here.
- **Frontend contract and serving (13–14)** depend on `WORK-0039` (command/view vocabulary) and `WORK-0044` (artifact model). `WORK-0046` must explicitly reconcile with `session-runtime-v1`'s `WORK-0009` (Client-Safe Game UI Manifest) — see that Project's own flag.
- **Publish composition (15)** depends on `WORK-0044` and, as before, on Game Management's not-yet-built authoring/publish write path — unchanged blocker from the superseded Project.
- **AI-assisted authoring (16–19)** depends on the execution/lint/artifact capabilities above existing enough to validate/simulate/render against (`WORK-0035`, `WORK-0037`, `WORK-0044`). `WORK-0048`/`WORK-0049` both depend on `WORK-0047`'s shared authoring-session capability; `WORK-0050` can be designed in parallel with it.
- **`WORK-0051`** is this Project's own completion gate, not a capability of its own — it depends on essentially everything above being DONE.

## Capability Coverage

Traceability from the accepted mandate's required capabilities to the WORK that owns each. No capability is "required but unowned."

| Capability | Owner |
|---|---|
| Sandboxed JS execution contract (pure function, stateless runtime) | WORK-0035 |
| Execution isolation, resource/time/memory/output/command-count limits, non-cooperative termination, concurrency/consumption controls | WORK-0036 |
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

1. **Sandbox runtime technology** (`WORK-0035`/`WORK-0036`) — for example an embedded pure-Go JS interpreter (e.g. goja), a V8-isolate-based runtime, a WASM-compiled engine (e.g. QuickJS), or a subprocess-per-execution model. Each has different determinism, performance, isolation-strength, and operational-complexity tradeoffs; this needs a dedicated technical exploration before `WORK-0035` can move to DRAFT.
2. **Deployment topology for sandboxed execution** (`WORK-0036`) — in-process isolate vs. a separate worker pool/process vs. a separate service — materially affects latency, resource accounting, and blast radius of a compromised/runaway execution.
3. **Exact command/state wire schema** (`WORK-0035`/`WORK-0039`) — deferred until the runtime technology decision (1) is made, since it constrains what's efficient/safe to serialize across the sandbox boundary.
4. **Durable outbox mechanism for confirmed-turn results** (`WORK-0043`) — same-database durable table vs. an external queue/broker; must also reconcile with `GAME-ADR-0020`'s existing "no outbox for presentation" stance, which this Project extends rather than reopens.
5. **Artifact bundle format and storage** (`WORK-0044`) — how script + frontend package + contracts + assets are packaged/versioned/stored (single row, object storage, hybrid); "no build pipeline is required for a functional game" per the mandate must remain true regardless of the answer.
6. **AI provider(s)/model(s) and commercial terms** (`WORK-0047`–`WORK-0049`) — explicitly out of this Project's scope to decide, but each WORK's own design needs at least a placeholder/interface assumption to proceed; flagged so it is not silently assumed away.
7. **Whether strict deterministic replay is ever required later** (for example, for a future anti-cheat or audit requirement) — `GAME-ADR-0029`'s Alternatives Considered explicitly leaves this open rather than foreclosing it; no WORK currently depends on it.

## Completion Criteria

This Project is complete when:

1. Every WORK in the table above is DONE, or explicitly moved to this Project's Out Of Scope with human confirmation.
2. Every row in the Capability Coverage matrix reads DONE or "explicitly out of scope" — none reads "not implemented"/"partial" against a capability this Project's Goal requires.
3. `game/language/v1/program`/`engine` (Game Language) is fully retired from Session Runtime's live execution path — no production code path still depends on it.
4. `docs/projects/active/session-runtime-v1/`'s flagged coordination points (`WORK-0017`, `WORK-0009`) have been explicitly resolved (redesigned, reconciled, or confirmed unaffected) — not left silently stale.
5. `WORK-0051`'s end-to-end verification has run against a real authored JavaScript game exercising private views, timers, effects, disconnect/reconnect, best-effort reconstruction, concurrency, and resource-limit enforcement.

When satisfied, this Project moves from `docs/projects/active/` to `docs/projects/completed/`.
