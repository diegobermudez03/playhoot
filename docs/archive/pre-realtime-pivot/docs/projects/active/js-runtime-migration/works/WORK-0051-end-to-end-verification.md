# WORK-0051: End-To-End Verification

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `docs/decisions/architecture/ADR-0016-javascript-executor-separately-deployed-infrastructure-service.md`
- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`
- `session/docs/decisions/SESSION-ADR-0025-snapshot-based-session-runtime-persistence.md`

Canonical context:
- `docs/projects/active/js-runtime-migration/PROJECT.md` (Completion Criteria, Capability Coverage)

## Outcome

Gate this Project's completion with evidence that a real authored JavaScript game runs end to end: persistent state across turns, real interactions/validation, timers (ordinary and keyed), per-player private views with privacy verification, effects, durable confirmed-result delivery, disconnect/reconnect-style state recovery, best-effort session reconstruction from the retained signal log, concurrency (serialized mutation under load), duplicate/idempotent requests, and resource-limit enforcement (a script that exceeds its bounds is terminated without corrupting the Session). Per `ADR-0015`'s own closing requirement, documenting behavior does not substitute for this — this WORK exists specifically so a single trivial case is never mistaken for full coverage.

## Context

**Note (2026-09-27, per `ADR-0016`):** verification must exercise the real, separately deployed Executor topology (Session Runtime calling a genuinely separate Executor process/deployment over the network transport `WORK-0052`/`WORK-0053` choose), not an in-process shortcut that calls execution logic directly — the deployment/security boundary is exactly what this Project's mandate requires evidence for, not merely the execution contract's correctness in isolation.

Not yet designed. Depends on nearly every other WORK in this Project being DONE or far enough along to exercise.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must exercise information privacy and timers specifically, not only a stateless quiz-style example.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

- This WORK's own acceptance criteria are the Project's verification; no separate meta-verification.

## Documentation Impact

Not yet designed.

## Blockers

- Depends on the rest of this Project's WORK reaching implementable maturity.

## Completion Record

Not yet started.
