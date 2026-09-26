# WORK-0018: Game Language Disconnect/Reconnect Signal Support

Status: DONE
Created: 2026-09-20
Last status change: 2026-09-26 (independent re-review verdict APPROVED, no findings)

Related decisions:
- GAME-ADR-0011 (Game Language disconnect/reconnect authored semantics - accepted target design)

Canonical context:
- `docs/projects/active/session-runtime-v1/PROJECT.md`
- `game/language/v1/engine/internal/compiler/compile_signals.go` (`namedLifecycleSignals` catalog - `UserDisconnected` currently a placeholder empty-schema entry; `UserReconnected` absent entirely)
- `game/language/v1/program/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md` (both already document this as the accepted target design, not yet implemented)
- `docs/projects/active/session-runtime-v1/works/WORK-0015-disconnect-reconnect-full-resync.md` (the Session Runtime capability this WORK gates)

## Outcome

`UserDisconnected` is given its accepted `{user: user}` shape (currently a placeholder empty schema), and `UserReconnected` is added to the compiler's `namedLifecycleSignals` catalog (currently absent entirely) - so an authored Game Language workflow can actually receive and react to either signal.

## Context

This is a hard prerequisite for WORK-0015 (Session Runtime Disconnect/Reconnect/Full Resync): Session Runtime cannot deliver a signal Game Language does not yet correctly support. It is Game Language/compiler domain work, not Session Runtime application logic, but it is tracked under this Project (the same way Keyed Timers, WORK-0013, is) because it directly and solely gates a required Session Runtime V1 capability, and both GAME-ADR-0011 (the accepted target semantics) and the current implementation gap already live inside `game/language/v1/`.

**Open scope note recorded for human review** (not decided here): whether Game Language-domain compiler work should be tracked under a Session-Runtime-named Project at all, versus as its own small Game Language initiative, is a judgment call - see `PROJECT.md`'s "Material Decisions Needing Human Input".

## Scope

### In Scope (known required outcome; design not yet started)

- `UserDisconnected`'s schema changed from placeholder-empty to `{user: user}`.
- `UserReconnected` added to `namedLifecycleSignals`, with an accepted schema.
- Whatever engine-side handling either signal's delivery requires beyond the compiler catalog entry itself.

### Out of Scope

- Session Runtime's own consumption of these signals (WORK-0015's scope, not this WORK's).
- Any other Game Language capability - this WORK is narrowly scoped to these two signals.

## Approved Design (2026-09-26)

**Blocker resolved by re-reading GAME-ADR-0011's own text (its "verify against the ADR text during design" instruction, below):** the ADR already fully pins down both schemas. Its Decision section states plainly: *"Each exposes exactly one authored field: `user: user`"* (referring to both `UserDisconnected` and `UserReconnected` together), and its Consequences section repeats this explicitly: *"must eventually be extended ... with `UserDisconnected: {user: user}` and a new `UserReconnected: {user: user}` entry"*. There is no remaining open schema question - `UserReconnected` is the identical one-field shape as `UserDisconnected`, not a separate design decision.

**Compiler change (the entire in-scope surface):** in `game/language/v1/engine/internal/compiler/compile_signals.go`, change the `namedLifecycleSignals` catalog:

```go
var namedLifecycleSignals = map[string]map[string]engine.Type{
	"WorkflowStarted":  {},
	"SessionCancelled": {},
	"UserDisconnected": {"user": engine.UserType{}},
	"UserReconnected":  {"user": engine.UserType{}},
}
```

The map's own doc comment (currently: "every entry's schema is currently empty") must be updated to describe the new asymmetry - `WorkflowStarted`/`SessionCancelled` remain empty; `UserDisconnected`/`UserReconnected` now expose `user: user`, citing GAME-ADR-0011.

**No engine-side change beyond the catalog entry.** This was the WORK's own open scope question ("whatever engine-side handling either signal's delivery requires beyond the compiler catalog entry itself") and it resolves to "nothing," confirmed by reading `game/language/v1/engine/signal.go` and `signalSchemaFields` (`game/language/v1/engine/internal/runtime/step.go:1101-1148`):

- `engine.Signal` already carries a generic `Fields map[string]Value` (`signal.go:111`), which is exactly the payload channel `SignalKindNamed` signals use - unlike `SignalKindIntent`/`SignalKindInteractionAnswered`, which share one universal cross-occurrence shape (`actor`/`respondent`+`answer`) and so get dedicated typed `Signal` fields (`Actor`, `Respondent`, `Answer`) the engine itself converts. Named signals each have their own ad hoc, signal-specific schema, so they intentionally use the generic `Fields` map instead.
- `signalSchemaFields`'s `default:` case (`step.go:1146`, reached by `SignalKindNamed`) already does `return signal.Fields` verbatim - no per-signal-name special-casing exists or is needed there today.
- Therefore a future caller building a `UserDisconnected`/`UserReconnected` `engine.Signal` (Session Runtime's own delivery, out of this WORK's scope - see below) sets `Fields: map[string]engine.Value{"user": engine.UserValue{ID: <actor's engine.UserID>}}` directly, following the existing `SessionActorID -> engine.UserValue` conversion already used for the root roster (`game/session/workflows/sessionlifecycle/step_start.go:226-238`: decimal-string actor ID wrapped as `engine.UserID`, per GAME-ADR-0005's public/internal identity boundary). This WORK does not write that caller - it has no caller yet (see below) - it only confirms the mechanism it will use already exists and needs no engine change.

**Confirmed still out of scope, unchanged from original Scope section:** no delivery call site exists anywhere yet for either signal (confirmed by repository search - `UserDisconnected`/`UserReconnected`/`SessionCancelled` construction appears nowhere outside compiler fixtures/docs today). Building that call site is Session Runtime's own consumption of these signals, owned by WORK-0015 (and, for the unrelated `SessionCancelled` named signal, WORK-0011) - not this WORK.

## Constraints and Invariants

- Must match GAME-ADR-0011's already-accepted semantics; this WORK implements an accepted design, it does not redesign it.
- `WorkflowStarted` and `SessionCancelled`'s existing empty schemas must not change.
- No new `SignalKind`/`SignalSource` variant is introduced; `UserDisconnected`/`UserReconnected` remain ordinary `program.NamedSignalSource` entries, per GAME-ADR-0011.

## Acceptance Criteria

- AC1: `namedLifecycleSignals["UserDisconnected"]` compiles to `{"user": engine.UserType{}}` (changed from the prior empty schema).
- AC2: `namedLifecycleSignals["UserReconnected"]` exists and compiles to `{"user": engine.UserType{}}`.
- AC3: A transition whose signal source is `program.NamedSignalSource{Name: "UserDisconnected"}` (respectively `"UserReconnected"`) with a binding `{Field: "user", Name: <x>}` compiles successfully, and `<x>` has compiled type `engine.UserType{}` in the transition's Guard/Operations/Control scope.
- AC4: A transition binding an unknown field (e.g. `"nonexistent"`) against `UserDisconnected` or `UserReconnected` is rejected with the existing "signal has no field named ..." diagnostic, exactly as for any other named signal.
- AC5: `namedLifecycleSignals["WorkflowStarted"]` and `["SessionCancelled"]` remain empty schemas - no regression.
- AC6: Full existing compiler test suite (`game/language/v1/engine/internal/compiler/...`) passes unmodified except for any assertion that directly depended on `UserDisconnected`'s prior empty schema (none found during design; verify during implementation).

## Blockers

None remaining - resolved above.

## Documentation Impact

- `game/language/v1/program/README.md`'s "Accepted Disconnect/Reconnect Lifecycle Signal Contract" section (~line 109-115): its "Status: ACCEPTED DESIGN, NOT YET IMPLEMENTED" framing describes the compiler catalog itself as unimplemented scaffolding - update to reflect the compiler-schema half is now implemented, while Session Runtime's own delivery (WORK-0015) remains not yet implemented.
- `game/language/v1/engine/LOGICAL_CONTRACT.md`'s "Accepted, not yet implemented, Operational Lifecycle contracts to preserve" bullet (~line 36-42) for `UserDisconnected`/`UserReconnected` - same update.
- `game/CURRENT_STATE.md` / `game/README.md` - check current wording (both already reference `UserDisconnected`/`UserReconnected` per GAME-ADR-0011's own Canonical Knowledge Impact list) and align precisely: compiler schema DONE, Session Runtime delivery still not implemented.
- `compile_signals.go`'s own `namedLifecycleSignals` doc comment (see Approved Design above).

## Implementation Report (2026-09-26)

Implemented:
- `game/language/v1/engine/internal/compiler/compile_signals.go`: `namedLifecycleSignals["UserDisconnected"]` changed from `{}` to `{"user": engine.UserType{}}`; `namedLifecycleSignals["UserReconnected"]` added with the same `{"user": engine.UserType{}}` schema. Doc comment updated to describe the new asymmetry and cite GAME-ADR-0011.
- Tests added to `game/language/v1/engine/internal/compiler/compile_workflows_test.go`: `TestCompile_UserDisconnectedReconnectedSignalBinding` (both signal names compile with a `user` binding consumed in a `User`-typed `CompleteControl`), `TestCompile_UserDisconnectedSignalBindingTypeIsUser` (forces a type mismatch against a `Number` result to prove the bound type is actually `engine.UserType`, not merely present), `TestCompile_UserDisconnectedSignalBindingUnknownField` (an unknown field binding against `UserDisconnected` is still rejected).

Local implementation decisions:
- None beyond what "Approved Design" already specified. No engine (`game/language/v1/engine/signal.go`, `internal/runtime/step.go`) code was touched - confirmed during design that `SignalKindNamed`'s existing generic `Signal.Fields` passthrough (`signalSchemaFields`'s `default:` case) already supports this without any engine-side change.

Deviations from the approved WORK:
- None.

Discoveries:
- None.

Verification performed:
- `go test ./game/language/v1/engine/internal/compiler/... -run TestCompile -v` - all tests pass, including the three new ones.
- `go build ./...` (whole repository) - clean.
- `go test ./game/language/...` (full Game Language suite: `program`, `program/gameservice`, `program/internal/codec`, `engine/engineservice`, `engine/internal/compiler`, `engine/internal/runtime`) - all pass, no regressions.
- No real-Postgres/integration verification applicable - this WORK touches only the compiler, no persistence.

Documentation synchronized:
- `game/language/v1/program/README.md` - "Accepted Disconnect/Reconnect Lifecycle Signal Contract" section's status line updated from "ACCEPTED DESIGN, NOT YET IMPLEMENTED" to reflect the compiler schema now being implemented while Session Runtime delivery remains future work (WORK-0015).
- `game/language/v1/engine/LOGICAL_CONTRACT.md` - the matching "Accepted Operational Lifecycle contracts" bullet updated the same way.
- `game/README.md` - "Authored Game Language Disconnect/Reconnect Contract" section given one added line stating the compiler schema is implemented and linking WORK-0018/WORK-0015.
- `game/CURRENT_STATE.md` - reviewed; no change needed. Its Session Runtime row already correctly states Session Runtime delivery of `UserDisconnected`/`UserReconnected` "writes into this shared table once its own owning WORK lands" (still true, unaffected by this WORK), and its Game Language row is a coarse summary this change doesn't invalidate.

Known limitations:
- None. The catalog/compiler surface is complete per Acceptance Criteria; Session Runtime's actual delivery of either signal remains explicitly out of this WORK's scope (WORK-0015), as approved.

Ready for independent review:
YES

## Fix Pass (2026-09-26)

Independent review (first round) returned CHANGES_REQUIRED: one REQUIRED_FIX and one NON_BLOCKING finding.

- REQUIRED_FIX: `game/language/v1/engine/README.md`'s "Accepted Disconnect/Reconnect Delivery And Offline-Interaction Invariants" section was missed by the original Documentation Impact list (which named only `LOGICAL_CONTRACT.md`, not its sibling `README.md`, even though GAME-ADR-0011's own "Canonical Knowledge Impact" section names both together) and still read "ACCEPTED DESIGN, NOT YET IMPLEMENTED." Fixed: updated to state the compiler schema is implemented while Session Runtime delivery remains future work (WORK-0015), matching the wording already applied to `LOGICAL_CONTRACT.md`.
- NON_BLOCKING (not applied, per protocol - optional wording polish only, not named by this WORK's Documentation Impact or GAME-ADR-0011's Canonical Knowledge Impact list): `game/language/v1/program/signal.go`'s `NamedSignalSource` doc comment phrasing ("the future compiler validates...") is slightly dated but not inaccurate.

No code or test changes were needed for the fix - documentation only. Verification (build/tests) unchanged from the original Implementation Report; not rerun since no source changed.

## Independent Re-Review (2026-09-26)

Verdict: APPROVED, no findings. Verified the fix-pass edit to `game/language/v1/engine/README.md` directly resolves the prior REQUIRED_FIX, confirmed no code/test changed since the first round (documentation-only fix), and re-ran the full build/test suite clean. The prior round's one NON_BLOCKING finding (`program/signal.go` doc-comment phrasing) was correctly left unapplied per protocol.

## Completion Record

Implemented the accepted GAME-ADR-0011 compiler schema for `UserDisconnected`/`UserReconnected`: `namedLifecycleSignals` in `game/language/v1/engine/internal/compiler/compile_signals.go` now exposes `{"user": engine.UserType{}}` for both (upgraded from `UserDisconnected`'s prior empty placeholder; `UserReconnected` newly added). No engine (`signal.go`/`step.go`) code change was needed - confirmed both during design and by independent review that `SignalKindNamed`'s existing generic `Signal.Fields` passthrough already carries a caller-populated `user` value with no further plumbing. Three new compiler tests cover binding compilation, the bound type genuinely being `engine.UserType` (not merely present), and unknown-field rejection; full existing test suite (`game/language/...`) and `go build ./...` pass with no regressions.

Documentation synchronized: `game/language/v1/program/README.md`, `game/language/v1/engine/LOGICAL_CONTRACT.md`, `game/language/v1/engine/README.md`, and `game/README.md` all updated to state the compiler schema is implemented while Session Runtime's own delivery of either signal remains future work (WORK-0015) - the `engine/README.md` gap was caught by independent review's first round and fixed same-session.

Independent review: two rounds. First returned CHANGES_REQUIRED (one REQUIRED_FIX - the `engine/README.md` doc-sync miss - plus one NON_BLOCKING wording-polish suggestion, left unapplied per protocol). Fix applied (documentation only, no code/test change). Re-review returned APPROVED, no findings.

No approved deviations. Session Runtime's own consumption/delivery of `UserDisconnected`/`UserReconnected` remains explicitly out of this WORK's scope, owned by WORK-0015, as approved in the Approved Design.
