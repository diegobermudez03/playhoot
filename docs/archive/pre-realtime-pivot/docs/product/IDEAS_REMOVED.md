# Product ideas removed at the real-time pivot

Archived verbatim from `docs/product/IDEAS.md`. Both depended on the retired discrete replay/workflow model.

## Minimize Persisted RuntimeTurn History

Raised 2026-09-20, alongside WORK-0006 (broaden live fan-out) confirming that engine execution is fully deterministic given (compiled Program, a starting Snapshot, a sequence of driving Signals) - no wall-clock, no OS randomness, nothing outside that triple.

Today, `session_runtime_turns` persists a full `snapshot_payload` (the entire resulting Snapshot) after every committed Turn. Given full determinism, that is more than strictly necessary: in principle only the *inputs* (the driving Signal per Turn, plus the one-time Start `Seed`) need to be durable - every Snapshot along the way, including the final one, is mechanically re-derivable by replaying those inputs through the same compiled Program from the beginning. Storing every intermediate Snapshot is redundant with storing the inputs that produced it.

Not approved, not scoped, not V1. Explicitly kept as full-snapshot persistence for now (simpler, already implemented, no replay-reconstruction code needed to serve any current read path). Revisit only if/when storage cost or a concrete need for storage minimization makes it worth trading for replay-reconstruction complexity. Depends on the same input-capture gaps a "replay" feature would need to close (see this file's determinism note and `docs/work/active/WORK-0006-broaden-live-fanout-effects-presentations.md`'s own persistence discussion) - the Start Turn's `Seed` is not currently captured anywhere durably, and later Turns' driving Signals are only reconstructable via a join against `session_interactions`, not stored as a first-class value.

## Personalized End-of-Game Screen

Raised 2026-09-20, alongside `docs/work/active/WORK-0007-session-termination-live-notification.md`.

V1's session-termination notification (fatal failure or natural game completion) sends one generic "session ended" signal and every client renders the same fixed screen, regardless of cause or outcome (winner/loser, score, etc.). A richer, personalized end-of-game screen - for example showing each player whether they won/lost, a final score, or game-specific results - is a real, plausible future need, but requires its own design (how a game defines what a "results" view looks like per player, how that's computed from the root workflow's `WorkflowOutcome.Result`, and how it interacts with the Presentation/View authoring model already used for in-game UI). Not approved, not scoped, not V1.
