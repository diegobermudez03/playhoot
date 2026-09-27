# Project: Game Management / Session Runtime Domain Split

Status: ACTIVE
Created: 2026-09-27
Last updated: 2026-09-27

## Goal

Implement `docs/decisions/architecture/ADR-0014-management-session-domain-split.md`: dissolve the "Game" bounded-context grouping so Game Management and Session Runtime become independent business domains, with Game Language reclassified as Session Runtime's own internal implementation detail and Session Runtime owning its own persisted copy of the executable Game Language definition it runs — removing Session Runtime's live runtime dependency on Game Management entirely, replacing the direct cross-domain calls the review that produced ADR-0014 found with either no dependency at all (most operations) or explicit Composer/Orchestrator-mediated composition (Create-time visibility, future publish).

## Explicitly Out Of Scope

- Building Game Management's own game-creation/authoring/publish write path — it does not exist yet in this codebase (`game/management` currently exposes only `getgame`/`getgamedefinition` read use cases). WORK-0033 tracks the future cross-domain composition once that authoring path exists; this Project does not build the authoring path itself.
- Session Runtime's live-transport/application-edge layer (`play`, `api/session`) — owned by `session-runtime-v1`'s own Phase 2/3 WORK (WORK-0020 onward). WORK-0032 only concerns where the Create-time Game-visibility check moves to, not building live transport.
- Any change to Session Runtime's own RuntimeTurn/replay/persistence model beyond adding the new executable-definition entity and removing the Game Management read calls — everything `game/docs/SESSION_RUNTIME_PERSISTENCE_MODEL.md` and `session-runtime-v1` already cover remains unchanged.

## Current Work

**WORK-0031** is DRAFT — approved design exists (see its own file), not yet human-approved to READY.

## Work

| Order | Work | Status |
|------:|------|--------|
| 1 | WORK-0031 — Session-Owned Executable Game Definition (schema + read-path + package-restructuring migration) | DRAFT |
| 2 | WORK-0032 — Composer-Mediated Session Creation (Game visibility composition) | PLANNED |
| 3 | WORK-0033 — Cross-Domain Game Publish/Authoring Composition | PLANNED |

Three WORKs exist even though the decision that created this Project was described by the human as "one change" to plan: WORK-0032 and WORK-0033 are each a known-required future outcome in their own right (removing `Create`'s direct Game Management call requires *something* to replace the visibility check it performed; the future publish flow requires *something* to keep both domains' representations consistent) — `docs/projects/README.md` Invariant 1 requires each to have its own WORK, even PLANNED, rather than surviving only as prose inside WORK-0031 or this file.

## Ordering / Dependencies

- WORK-0031 has no dependency beyond the already-DONE `session-runtime-v1` Phase 1 WORK it touches (Create/Join/Start/AnswerInteraction/SubmitUserIntent/CancelSession/ExpireTimer already exist and are DONE there — this Project modifies their persistence-dependency shape, not their business behavior).
- WORK-0032 depends on WORK-0031 (it replaces the direct call WORK-0031 removes) **and must not leave a deployed gap**: per ADR-0014's Consequences, nothing must ever run in production with `Create`'s direct Game Management call removed and no Composer-mediated replacement live yet. If WORK-0031 is implemented and deployed before WORK-0032, the two must be sequenced/flagged so that gap is never actually exposed (for example, keeping the direct call until WORK-0032 lands, or gating deployment). WORK-0032 also depends on `session-runtime-v1`'s own live-transport WORK (WORK-0020 onward) existing enough to have an entry point for Composer to call into.
- WORK-0033 depends on WORK-0031 (Session Runtime must already own its executable-definition entity before a publish flow can write to it) and on Game Management's not-yet-built authoring/publish write path existing (explicitly out of scope of this Project — see above).

## Material Decisions Needing Human Input

1. **WORK-0031's exact new-table schema and package-move mechanics** are drafted in WORK-0031 itself but not yet human-approved to READY — see its own Blockers.
2. **Deployment sequencing between WORK-0031 and WORK-0032** (see Ordering above) — whether WORK-0031 is allowed to ship with `Create`'s direct call temporarily kept (contradicting part of its own migration goal, but avoiding a real product gap) versus requiring WORK-0032 to land in the same release. Not yet decided; flagged as a Blocker on WORK-0031 itself.

## Completion Criteria

This Project is complete when WORK-0031 is DONE (Session Runtime no longer depends on Game Management for anything beyond, at most, a Composer-mediated Create-time visibility check) and WORK-0032 is DONE (that visibility check has a real home). WORK-0033 is required future work per ADR-0014 but does not gate this Project's own completion, since it depends on a Game Management authoring capability this Project does not build — it remains PLANNED under this Project (or is re-parented to a future "Game authoring/publishing" Project if one is created) until that capability exists.
