# WORK-0039: Platform Command Protocol & Runtime Validation

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`

Canonical context:
- `game/session/workflows/sessionlifecycle/outputs.go`, `internal/clientoutputs/clientoutputs.go` (the closed `Value`/`Output` schema this generalizes, per WORK-0029's precedent)

## Outcome

Define the closed vocabulary of platform-level commands authored JavaScript may request (`requestedCommands` in `WORK-0035`'s contract) — schedule/cancel a timer, emit an effect, emit a per-player view, request a session-lifecycle transition (completion/failure/cancellation) — and build runtime validation for both this platform vocabulary and any game-specific contract (a specific game's own action/view shapes) at the sandbox boundary. Static types in authored TypeScript, if used, do not survive to the sandboxed runtime boundary as a guarantee; this WORK is what actually enforces shape/content correctness at runtime, the same role Game Language's compiler played for the DSL.

## Context

Not yet designed. Depends on `WORK-0035`'s execution contract and the wire-schema decision (`PROJECT.md` Material Decisions #3).

**Revised (2026-09-27, per `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`):** the JavaScript Executor is now a separately deployed service, not part of Session Runtime's own process — validation of `requestedCommands` belongs in Session Runtime itself, at the point it receives an `ExecutionResult` back through `WORK-0053`'s `Executor` port, not inside the Executor service. The Executor is trusted Playhoot-owned infrastructure, but it hosts untrusted execution; Session Runtime must not trust a command's shape/content merely because it crossed back through that boundary.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- The platform command vocabulary must remain closed (Go-owned, not extensible by authored code) — an authored script can only request from the set Playhoot defines, never invent a new command kind.
- Identity/session/authority fields (who is acting, which session, which recipients) are established/verified by Playhoot, never trusted from a command's own declared content.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

### Accepted / Canonical Knowledge

- A new package-local contract document recording the closed command vocabulary and validation rules (successor role to `game/language/v1/engine/README.md`'s Output taxonomy).

## Blockers

- Depends on `WORK-0035`'s wire-schema decision.

## Completion Record

Not yet started.
