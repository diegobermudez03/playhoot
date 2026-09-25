# WORK-0029: Session-Owned Output/Value Schema (Decouple `game/session` From `engine`)

Status: PLANNED
Created: 2026-09-24
Last status change: 2026-09-24 (created, human-directed - see Outcome)

Related decisions:
- None yet. May need a lightweight ADR/standard once the design questions below are actually resolved, if the resulting rule is judged reusable beyond this one boundary.

Canonical context:
- `docs/projects/active/session-runtime-v1/works/WORK-0006-broaden-live-fanout-effects-presentations.md` (introduced `StartResult`/`AnswerInteractionResult.Outputs []engine.Output`, the surface this WORK must replace)
- `game/language/v1/engine/output.go` (`EmitEffectOutput`/`ActivatePresentationOutput`/`UpdatePresentationOutput`/`RemovePresentationOutput` - the four kinds `Outputs` currently carries)
- `game/language/v1/engine/value.go` (`Value`'s 11 variants - `Unit`/`Bool`/`Number`/`String`/`User`/`Enum`/`Record`/`Union`/`NewType`/`Optional`/`List`/`Map`, several recursive, each referencing `engine`'s own separate `Type` algebra) - the payload type this WORK must give `game/session` its own equivalent of
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (the first real consumer of `Outputs` - the play Coordinator this WORK's schema must actually serve)

## Outcome

No exported symbol of `game/session` (directly, or through `sessionlifecycle`) requires a caller to import `game/language/v1/engine` or `game/language/v1/program`. Today it does: `StartResult`/`AnswerInteractionResult.Outputs []engine.Output` returns engine-owned interface types directly, each carrying `engine.Value`-typed Model/Arguments data and `engine.UserID`-typed (internal session-actor-derived) recipients. `game/session` must instead expose its own schema - mapped from the engine's own Output/Value/recipient data, at the boundary, into types designed for what a consumer (eventually a live Coordinator, eventually a UI) actually needs, not for what the engine's own execution model happens to produce.

This was raised as a live architecture concern (2026-09-24, human-directed) while implementing WORK-0006/0007: `AnswerInteraction`'s own input side was already fixed in that same session (`engine.Value` parameter replaced with the already-encoded wire payload, `json.RawMessage`, decoded internally) - this WORK is the output side, and is deliberately **not** implemented immediately, for a concrete reason: no consumer of `Outputs` exists yet (`play`/the Coordinator was deleted, WORK-0020 hasn't rebuilt it). Designing the full mapping now risks guessing the wrong shape and redoing it once a real consumer (WORK-0020 onward) reveals the actual wire/UI contract needed. This WORK exists so that requirement is tracked (per `docs/projects/README.md` Invariant 1 - a known-required future outcome must have a WORK, even PLANNED) rather than left only as a comment in this conversation.

## Context

**Why this is more than a type rename.** `engine.UserID` on `EmitEffectOutput.Recipients`/`ActivatePresentationOutput.Recipient`/etc. is the internal `session_actors.id`, not a real user identity - `Manager` has never exposed that identifier to any caller. Mapping it to something a consumer can actually use (`UserUUID`, the identity `Manager` already exposes everywhere else) needs a new repository lookup (actor id -> `session_actors.user_uuid`), not just a wrapper type.

**Why the Value payload cannot simply become `any`/raw bytes.** `engine.Value`'s type-kind information (this is a Number vs. a Record with named fields of specific types, etc.) is not an internal engine implementation detail - it is the actual schema a UI needs to know how to render/act on. Erasing it into an untyped blob would just push the same problem onto every consumer as undocumented convention (a worse, non-compile-checked coupling). `game/session` needs its own explicit mirror of this schema - not `engine`'s own Go types, but a session-owned equivalent covering the same logical shapes - so a consumer's compile-time contract comes from `session`, never from `engine` directly.

**Why this is not scoped down to "just the primitives used so far".** A quick audit (2026-09-24, during this same discussion) found the engine's `Value`/`Type` algebra has 11 variants with no depth/composition limit, but only one real Definition exists in the repository (Parqués), using a small subset (`enum`, flat `record`, `list<record>`) - no `map`/`optional`/`union`/`new_type` anywhere in any authored or example content today. That is weaker evidence than GAME-ADR-0026's own nested-workflow removal (which was backed by an exhaustive audit across the full committed target-game range finding zero use, not "the one simple example doesn't need it") - `Map` in particular is plausibly load-bearing for per-player/per-object state in other committed-range games (UNO's discard pile, Poker's hand/fold state) that just haven't been authored yet. This WORK does not decide to narrow the engine's Value/Type surface based on that weaker evidence; see the separate Engineering Radar item recording the question of whether the engine's Value/Type generality itself deserves its own GAME-ADR-0026-style audit, independent of this WORK.

## Scope

Not yet designed. Known open design questions to resolve when this WORK moves PLANNED -> DRAFT:

- Exact session-owned `Value` schema shape (a mirrored closed interface with concrete kinds, vs. a single tagged struct) and how deeply it needs to cover the engine's 11 variants vs. only what a real consumer's actual Definitions exercise.
- Exact session-owned `Output` schema shape (one exported type per kind vs. a single discriminated envelope) for the four kinds `Outputs` currently carries.
- The actor-id -> `UserUUID` resolution mechanism (a new narrow repository method; whether it belongs in `internal/clientoutputs` or a new subpackage).
- Whether/how this schema is versioned or kept stable once a real wire/UI consumer depends on it.
- Whether resolving this also implies a reusable engineering standard (e.g. "domain packages must not export types from `game/language/v1/...` in their own public API") worth recording in `docs/engineering/standards/`, or whether it is narrow enough to stay a one-off design decision.

## Approved Design

Not yet designed - PLANNED.

## Constraints and Invariants

- `game/session`'s own exported API (types, function signatures) must not require a caller to import `game/language/v1/engine` or `game/language/v1/program` to use it.
- No change to `AnswerInteraction`'s already-fixed input side (`json.RawMessage`, decoded internally via `engineservice.DecodeValue`).

## Acceptance Criteria

Not yet designed - PLANNED. Will include, at minimum: `go list -deps ./game/session/...`'s own exported package surface contains no `game/language/v1/engine`/`game/language/v1/program` type in any public function signature or exported struct field; recipient identity resolves to `UserUUID`; existing WORK-0006/0007 behavior (which Output kinds are returned, in what order, under what conditions) is preserved exactly, only the exposed shape changes.

## Implementation Freedom

Not applicable yet - PLANNED.

## Verification

Not yet designed - PLANNED. Will include the normal `go build`/`go vet`/`go test ./... -count=1` bar.

## Documentation Impact

### Accepted / Canonical Knowledge

- None yet.

### Current-State Documentation After Implementation

- `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` - once designed, describe the session-owned Output/Value schema in place of the current `engine.Output`/`engine.Value` description.

## Blockers

1. **Timing dependency, not a design blocker**: this WORK is deliberately not drafted for real until a real consumer of `Outputs` exists (WORK-0020 onward), so its schema is driven by an actual wire/UI requirement rather than guessed.

## Completion Record

Not yet DONE. PLANNED (2026-09-24).
