Status: RESOLVED (2026-09-24) — Blocker 1 accepted as recommended (Timer included); Blocker 2 NOT accepted as recommended (Presentation's keyed capability deferred, not implemented). See WORK-0025's own "Human Resolution" section for the recorded decision. Kept here as the historical record of what was asked.

# WORK-0025 — Two Design Blockers (2026-09-24)

Process: drafting `WORK-0025-keyed-interaction-slots.md` for real (PLANNED -> DRAFT), following this Project's just-in-time drafting practice, immediately after WORK-0024 closed DONE. Design was checked directly against the actual current code (`program`/`engine`/`internal/compiler`/`internal/runtime`'s Question/AskGroup/Timer/Presentation implementations), not only against GAME-ADR-0012/0026's prose. No production code was implemented by this session - this is design-drafting only, exactly as WORK-0025's own status (DRAFT, not READY) requires.

For the prior resolved checkpoint (WORK-0024's Session Runtime scope deviation), see WORK-0024's own Completion Record, "Human Resolution" section - not repeated here.

## What changed

WORK-0025's Outcome/Context/Scope/Approved Design/Constraints/Acceptance Criteria/Implementation Freedom/Verification/Documentation Impact were filled in for real. Question, Ask Group, and Timer's keyed families are designed as direct, mechanical generalizations of their existing ordinary counterparts (add a dynamic `Key Expression` alongside the existing static `Slot` string, exactly as GAME-ADR-0012 already specified for Timer back in 2026-09-07). Presentation's keyed family needed a genuinely different mechanism, discovered by reading `internal/runtime/presentation.go` directly: Presentation has no open/close operation at all (it is fully declarative, recomputed from `Targets` on every transition) and its current occupancy identity is already `(Slot, Recipient)`, not `Slot` alone - a *targeted* presentation already mounts one independent occupant per user. The actual gap a keyed presentation closes is letting one same user hold several simultaneously active presentations under one `Slot`, differentiated by key.

Two Blockers were left explicitly unresolved, per this Project's Blocker practice (see WORK-0003's Blocker 1 for the precedent of "propose, recommend, escalate rather than silently decide").

## Please confirm or correct

1. **Should Timer's own keyed-slot implementation (still-unimplemented GAME-ADR-0012, accepted 2026-09-07) land in this same WORK, alongside Question/AskGroup's keyed families?**

   Recommended: **yes.** GAME-ADR-0026 itself already anticipated this ("the two are expected to share an implementation approach once either is built"). All three (Question/AskGroup/Timer) share the identical shape - a dynamic `Key` added to an existing imperative open/schedule-then-close/cancel operation, atomic-occupied-tuple-failure semantics - so implementing them together in one reviewable diff against one settled design is more consistent than three now and a fourth reconciled later against already-shipped code. It also finally closes a multi-week-old implementation gap that's been sitting on GAME-ADR-0012 since before this Project existed, which independently unblocks `session-runtime-v1`'s WORK-0015/WORK-0018 (disconnect/reconnect keyed timers) once this Project's own WORK-0027 lands.

   Alternative, if preferred: track Timer's keyed family as its own later WORK (WORK-0028 or similar), keeping WORK-0025 to Question/AskGroup/Presentation only, as the WORK-0025 placeholder originally scoped it before this drafting pass.

2. **Presentation's exact keyed mechanism** - a concrete proposed design exists (see WORK-0025's own "Approved Design" section, "Presentation: keyed capability"): a new `KeyedPresentationDeclaration` replacing `Targets: list<User>` with `KeyedTargets: list<record{key: KeyType, user: user}>`, occupancy identity becoming `(Slot, Key, Recipient)`, reusing the existing `deriveActivePresentations`/`diffPresentations` recompute-and-diff mechanism unchanged in shape, plus one new implicit `"key"` projection-argument binding.

   Recommended: **approve as proposed.** It is the minimal extension of the existing mechanism consistent with how Question/AskGroup/Timer's own keyed families minimally extend theirs (`Slot` -> `(Slot, Key)`; here `(Slot, Recipient)` -> `(Slot, Key, Recipient)`), and needs no new runtime concept beyond the one binding.

   Worth knowing before deciding: GAME-ADR-0026's own Target Game Coverage table (the audit that justified removing Child Workflow/Task Group) never actually needed `KeyedPresentationSlot` for any of its worked examples - only `KeyedQuestionSlot`/`KeyedTimerSlot` appear in its "Covered by" column. Keyed Presentation is still in scope because GAME-ADR-0026 Decision 2 explicitly accepted it, but if there's appetite to defer it (since no concrete game has yet demonstrated the need, mirroring the same "no demonstrated need" reasoning GAME-ADR-0026 itself used to remove Child Workflow/Task Group), that is also a legitimate answer - say so and WORK-0025's scope narrows to Question/AskGroup(/Timer, per Blocker 1).

## Next action

WORK-0025 stays DRAFT. No implementation should proceed on either Blocker until resolved. Once both are resolved: WORK-0025's Approved Design/Scope/Acceptance Criteria are revised to reflect the resolutions (if either recommendation is rejected, the corresponding family is removed from this WORK's scope rather than left half-specified), the WORK moves DRAFT -> READY, and a Codebase Agent implements it per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`.
