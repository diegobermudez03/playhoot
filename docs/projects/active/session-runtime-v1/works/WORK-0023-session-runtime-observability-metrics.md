# WORK-0023: Session Runtime Observability Metrics

Status: PLANNED
Created: 2026-09-22
Last status change: 2026-09-22

Related decisions:
- GAME-ADR-0024 (Replay-First Session Runtime Persistence - this WORK exists specifically to give WORK-0019's own deferred ephemeral-cache decision (Blocker 4) real data to be decided against)

Canonical context:
- `monitoring/alerts.go`, `monitoring/README.md` (the existing `monitoring` package - already scaffolded as the intended home for "metrics, alerts, etc.", but only `Alert` exists today; this WORK is the package's first real metrics capability, not a new parallel mechanism)
- `docs/projects/active/session-runtime-v1/works/WORK-0019-replay-first-session-runtime-persistence-migration.md` (Blocker 4 - deferred the ephemeral current-state cache decision specifically pending this WORK's data)
- `docs/engineering/standards/logging.md` (the existing per-request/per-connection logging model this WORK's metrics are additive to, not a replacement for)

## Outcome

Session Runtime (and, as a byproduct, the `monitoring` package generally) gains a real metrics-instrumentation capability - starting with, at minimum, replay/state-reconstruction duration (WORK-0019's own replay-reconstruction path) - so that future decisions such as WORK-0019's deferred ephemeral-cache Blocker can be made from measured data rather than speculation. This WORK also establishes where in the codebase future metrics of this kind get defined and recorded, so later WORK does not each invent its own ad hoc instrumentation.

## Context

This WORK exists because WORK-0019's Blocker 4 (whether to build an ephemeral in-memory current-state cache to avoid replaying a Session's full input history on every read) was deliberately deferred rather than decided speculatively - the right answer depends on how expensive replay actually is against a real authored Game Definition, which is not yet known. That question surfaced that no metrics capability exists anywhere in the repository today: `monitoring.Alert` (added ad hoc, per its own doc comment, with a literal `TODO: forward message to a real alerting system once one is integrated`) is the package's only capability, even though `monitoring/README.md` already states the package's intended scope is broader ("metrics, alerts, etc."). This WORK is the first implementation of that broader intent, not a new, separate mechanism.

## Scope

Not yet designed - this is a PLANNED WORK recording a known-required outcome. Expected to cover, once drafted:

- A metrics-recording capability added to `monitoring` (counters/durations at minimum) that domain code can call without depending on which concrete metrics backend eventually receives them.
- Instrumenting WORK-0019's replay/state-reconstruction path specifically, so its cost is actually measurable before any cache decision is revisited.
- Deciding where else, if anywhere, Session Runtime should be instrumented at this WORK's own drafting time (for example, RuntimeTurn commit latency) - not assumed here.
- Explicitly NOT deciding the concrete metrics backend/export destination, dashboard, or alerting integration - `monitoring.Alert`'s own precedent (log first, forward later once a real system is integrated) suggests the same shape: define and call the recording surface now, wire it to a real backend later. Which backend, and when, is a separate infrastructure decision outside this WORK's own scope unless escalated and decided at drafting time.

## Out of Scope

- Choosing/integrating a concrete metrics backend (Prometheus, a hosted APM, etc.), dashboards, or alerting rules - infrastructure decisions to be escalated separately when actually needed.
- Retrofitting metrics onto every existing code path - this WORK's own drafting decides which paths matter first (at minimum, WORK-0019's replay path); broad instrumentation is not assumed.
- Revisiting WORK-0019's Blocker 4 cache decision itself - this WORK only supplies the data that decision needs; the decision remains WORK-0019's own (or a future follow-up once data exists).

## Sequencing

Placed at the end of Phase 1 in `PROJECT.md`'s Work table - it needs WORK-0019's replay-reconstruction mechanism to actually exist before there is anything real to measure.

## Blockers

- None yet identified beyond normal PLANNED-to-DRAFT design work - see Scope's open items above.

## Completion Record

Not yet started. Status: PLANNED.
