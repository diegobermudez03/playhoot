Status: RESOLVED (2026-09-24) — see WORK-0024's own Completion Record, "Human Resolution (2026-09-24, HUMAN-APPROVED)" section, for the recorded decision. Kept here as the historical record of what was asked.

# WORK-0024 — Session Runtime Scope Deviation (2026-09-24)

Process: independent review of `WORK-0024-remove-child-workflow-and-task-group.md` (`docs/projects/active/game-language-flat-execution-model/works/`), fresh review requested after self-reported implementation completion. REQUIRED_FIX findings (stale documentation/comments/testdata, one `gofmt` regression) were already applied in the same pass — see WORK-0024's own Completion Record for the full list. This is the one finding that could not be resolved without a human decision.

## What was found

WORK-0024's own "Out of Scope" section states "Any Session Runtime (`game/session`) change" is out of scope, and its Constraints require any discovered need for one to be raised as a DISCOVERY rather than silently implemented. Its Acceptance Criteria require `go test ./game/session/...` to "continue to pass unmodified."

The implementation nonetheless mechanically edited 4 Session Runtime production files (`interaction_capture.go`, `internal/runtimeturn/runtimeturn.go`, `step_answer_interaction.go`, `replay.go`) and 3 test files, because they referenced `engine.Signal.Path`/`PathStep`, which this WORK correctly removed from the engine — the repository would not otherwise build. This was reported after the fact in the implementation report's "Known limitations" section, not escalated as a DISCOVERY during implementation as the WORK's own Constraints require.

The independent review verified the change is genuinely behavior-preserving: the new `interaction_capture.go` constant `emptyEnginePath = []byte("[]")` is byte-identical to what the old `encodeEnginePath(step.Path)` always produced for every real (non-nested) call site Session Runtime ever exercised — confirmed by reading the old encoder and the only two paths that ever called it (initial signal: always nil Path; answers: always decoded from `[]`). `go test ./game/session/... -count=1` passes in full against real Postgres.

The review also found this removal leaves `runtimeturn.MaxSteps` (the accepted `MAX_STEPS_PER_RUNTIME_TURN` bound) with no reachable test: its only producer of the internal-signal cascade that could reach the bound was spawning, which this WORK correctly removed. Two subtests (`exceeds_step_bound`, `commits_exactly_at_bound`) were deleted along with it.

## Please confirm or correct

1. **Accept the 4 mechanical Session Runtime edits as within WORK-0024**, on the basis that they are a forced, verified-behavior-preserving consequence of the in-scope engine change, not a Session Runtime design change — and record that as a non-material clarification in the WORK (per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`'s "Decision Required / Reapproval Loop", non-material acceptance does not require returning the WORK to DRAFT)? Or should this have been sequenced differently — e.g., folded into WORK-0027 (Session Runtime Interaction-Addressing Rework), which already owns the Session Runtime side of this redesign?
2. **Accept the now-unreachable, untested `MaxSteps` bound as an accepted limitation** (the property is still guaranteed by the code — `Drain`'s loop still checks the bound on every iteration — it simply cannot be exercised by any producible signal today), to be picked up again if/when a future cause (WORK-0025/0026/0027, or a later Project) reintroduces a way to produce a multi-Turn internal-signal cascade? Or should a synthetic/fixture-level test be added now to keep the bound under direct test coverage regardless?

## Next action

WORK-0024 stays IMPLEMENTING. No further code changes are pending on this WORK beyond this decision. Once resolved:
- If non-material: record the resolution in WORK-0024's Completion Record, close the DECISION_REQUIRED finding, and this WORK can proceed toward closure (Documentation Impact/Completion Record already otherwise satisfied per the Independent Review report).
- If material (i.e., re-sequencing into WORK-0027, or a new required test): return WORK-0024 to DRAFT only if the change actually alters its approved scope; otherwise apply as a further LOCAL FIX pass.
