> **SUPERSEDED, 2026-09-20 — HISTORICAL RECORD ONLY.** This checkpoint is stale: WORK-0005 subsequently moved DRAFT -> READY -> IMPLEMENTING the same day (all seven Blockers resolved, implementation complete, verified against real Postgres) - see `../works/WORK-0005-thin-live-coordinator.md`'s own Blockers/Completion Record for the authoritative current state. Kept only as a historical record of the Blocker 1 refinement decision it documents (still accurately recorded in WORK-0005 itself). For current pending checkpoints, see `../internal/HUMAN_REVIEW.md`.

# Slice 4 — Thin Live Coordinator / WebSocket: DRAFT (Blocker 1 refined)

Process: Feature Development (Slice 4 of `session-runtime-v1`; architecture is CLOSED, the broader initiative remains governed by `PLAN.md`)

Status: **DRAFT, 2026-09-20.** `docs/work/active/WORK-0005-thin-live-coordinator.md` briefly reached READY, then you corrected Blocker 1 twice in the same day, before any code was written. This supersedes the prior checkpoint in this file.

## What Changed (Round 2)

My first correction over-specified your rule as "no `game` import anywhere under `play/`, the implementation must live entirely outside `play/`." You clarified it's narrower than that: only `play`'s own package and what it exports need to stay clean. The concrete implementation of the interface `play` depends on is *expected* to import `game` - it has to, since it's translating to/from `sessionlifecycle.Manager`'s real types - and it's fine for that implementation to live in a subpackage under `play/`. The only hard rule is that `play`'s exported methods/structs never reference a `game` type, so no consumer of `play` transitively depends on `game`, and there's no `play`<->`game` cycle.

## Current Design

- `play` (top-level package) declares a `SessionRuntime` interface + DTOs, all primitive/`play`-owned types. `play`'s own package imports nothing from `game`.
- The `SessionRuntime` implementation imports `game` freely (expected, not a violation) and can live in a subpackage (e.g. `play/sessionruntime`) or elsewhere - just not inside `play`'s own package.
- `game` imports nothing from `play`/`api`. `api` imports only `play`'s exported API.
- `play` never holds a database handle or transaction - still structural, unaffected by this refinement.

Blockers 2-4 are untouched and remain approved.

## Consequence

`WORK-0005` is still **DRAFT**. No code has been written for this slice yet.

## Next Human Action

Confirm this matches what you meant, and I'll move it back to READY and start implementation.
