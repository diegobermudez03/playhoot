# WORK-0022: Session/Platform Abuse and Resource-Rate Limits

Status: PLANNED
Created: 2026-09-22
Last status change: 2026-09-22

Related decisions:
- GAME-ADR-0019 (RuntimeTurn execution bound and terminal cleanup - already bounds a *single* Step-chain's runaway work; this WORK's gap is one level above, across a Session's lifetime and across Sessions/users)
- GAME-ADR-0017 (Session Runtime Failure Classification and Diagnostic Persistence - the `session_runtime_failures` entity this WORK's own limits would need to classify/report through once WORK-0014 implements it)
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - `session_runtime_turns` remains the durable, ordered per-Session Turn log this WORK's own counting/rate logic would query; unaffected by WORK-0019's Snapshot/Steps removal)

Canonical context:
- `game/language/v1/engine/limits.go` (`Limits`/`DefaultLimits` - already-implemented per-Step bound this WORK does not duplicate or replace)
- `game/session/workflows/sessionlifecycle/internal/runtimeturn/runtimeturn.go` (`Drain`'s `MaxSteps = 20` - already-implemented per-RuntimeTurn bound this WORK does not duplicate or replace)
- `docs/projects/active/session-runtime-v1/internal/AI_CONTEXT.md` ("platform abuse/resource limits and per-user/session rate boundaries" - the deferred topic this WORK now owns)
- `docs/projects/active/session-runtime-v1/works/WORK-0005-thin-live-coordinator.md` (`RESTRoutes()`/`WebSocketRoutes()` route-declared `Middlewares` mechanism - already built, unused; a rate-limiter middleware is this mechanism's own stated example use case)
- `docs/projects/active/session-runtime-v1/works/WORK-0014-runtime-failure-diagnostics-terminal-cleanup.md` (`session_runtime_failures` - once implemented, gives this WORK's own limits real failure-rate data to reference/calibrate against)

## Outcome

Session Runtime and its transport layer (`api`) enforce bounded resource consumption *above* the single-execution level that GAME-ADR-0019's engine `Limits`/`MaxSteps` already bound: a ceiling on how much a single Session, a single user, or the platform as a whole can consume over time (total RuntimeTurns/interactions per Session, concurrent Sessions per user, inbound message rate per WebSocket connection, request rate per endpoint), so that a user-authored Game Language program that is individually well-behaved per execution cannot still be used to exhaust platform resources or drive infrastructure cost through sheer volume or repetition.

## Context

This WORK exists because Session Runtime executes Game Language programs supplied/authored by users - not a fixed, platform-controlled instruction set - and an individual authored program's single execution is already bounded deterministically (GAME-ADR-0019: `engine.Limits` bounds one `Step`'s internal operations/loops/recursion/fan-out; `runtimeturn.Drain`'s `MaxSteps = 20` bounds one RuntimeTurn's internal Step chain). Neither bound limits how many separate RuntimeTurns/interactions a Session can accumulate over its lifetime, how many Sessions one user can create or hold open concurrently, or how fast a client can send messages/requests. This gap was identified while resolving WORK-0019's Blocker 2 (removing `session_runtime_steps`) - confirming that removal does not affect this WORK's own needs, since the durable per-Session Turn count/timing this WORK would rely on lives in `session_runtime_turns`/`sessions.started_at`, both retained under GAME-ADR-0024's replay-first model.

Today, nothing enforces any such boundary: `docs/projects/active/session-runtime-v1/internal/AI_CONTEXT.md` lists "platform abuse/resource limits and per-user/session rate boundaries" as an explicitly deferred design topic with no owning WORK, and WORK-0005 confirms `api`'s route-declared `Middlewares` mechanism (where a rate limiter would attach) exists but has zero middlewares registered today.

This WORK does not reopen or weaken GAME-ADR-0019's existing per-execution bound - that protection is unrelated and already sufficient for its own concern (a single runaway execution). This WORK addresses the separate concern of bounded aggregate/repeated consumption.

## Scope

Not yet designed - this is a PLANNED WORK recording a known-required outcome, not yet decomposed into an approved design. Expected to need human/product/security input (not implementable by unilateral agent decision, per this repository's escalation rules) on at least:

- Concrete numeric thresholds (max concurrent Sessions per user, max RuntimeTurns per Session, max WebSocket messages/sec per connection, max Session-creation rate per user/IP) - business/risk decisions, not engineering-derivable.
- Whether enforcement lives in `api`'s route-declared `Middlewares` (connection/request-level), in `game/session`'s domain layer (Session/Turn-count-level), or both - likely both, given this Project's existing domain/transport split pattern (see WORK-0006/0007/0010/0011/0012).
- What happens to a Session that exceeds a limit - reject the triggering request only, or terminalize the Session (and if so, under which GAME-ADR-0017 failure class, or a new one) - not decided; must not be invented silently during implementation.
- Whether this needs per-user identity to enforce (today's simplified identity assumption - see `PROJECT.md`'s "Explicitly Out Of Scope" - may itself need revisiting for this WORK specifically, since abuse boundaries are normally identity-scoped).

## Out of Scope

- Re-deciding or loosening GAME-ADR-0019's existing per-execution `Limits`/`MaxSteps` bound.
- Full Identity/Auth integration (remains this Project's own accepted out-of-scope, per `PROJECT.md`) - this WORK may need to state a narrower, explicit assumption about what identity signal it can rely on for rate-scoping under the current simplified model, without silently expanding that boundary.
- Classic network-layer DDoS mitigation (traffic scrubbing, CDN/edge-level protection) - infrastructure/platform concern outside Session Runtime's own ownership.

## Blockers

- Numeric thresholds and enforcement point(s) are open product/security decisions - see Scope above. This WORK cannot move to DRAFT until at least the enforcement shape (transport vs. domain vs. both) and the intended failure-handling behavior are escalated and decided.

## Completion Record

Not yet started. Status: PLANNED.
