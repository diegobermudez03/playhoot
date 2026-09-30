# WORK-0042: SEND_EVENT Delivery (Best-Effort, Restated From SESSION-ADR-0019)

Status: DONE
Created: 2026-09-27
Last status change: 2026-09-29 (independent review returned APPROVED with no findings; closed same day)

Related decisions:
- `session/docs/decisions/SESSION-ADR-0019-session-runtime-post-commit-client-delivery-semantics.md`
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`

Canonical context:
- `session/workflows/sessionlifecycle/internal/platform/command.go` (`SendEvent`, already defined and validated by `platform.ParseCommand` — `Kind`/`Recipients`/`Name`/`Payload`, `Name`/`Payload` opaque to Playhoot)
- `session/workflows/sessionlifecycle/step_start.go`, `step_submit_player_event.go`, `step_cancel_session.go`, `step_expire_timer.go` (all four RUNNING-phase steps already call `parseCommands`, but today only consume the result for `timers.Apply`/`completion.Detect` — a validated `SendEvent` is parsed and then silently dropped; see Context)
- `session/types.go` (`StartResult`/`SubmitPlayerEventResult`/`CancelSessionResult`/`ExpireTimerResult` — today carry no effect/event field at all)

## Outcome

Adapt cosmetic/transient event delivery to the new JS command source (`WORK-0039`'s `SEND_EVENT`), restating `SESSION-ADR-0019`'s existing accepted stance unchanged: a `SEND_EVENT` is best-effort, correctness never depends on a client receiving one, and correlation exists for repeated/late/absent delivery. This is explicitly distinct from `WORK-0043` (durable delivery of a confirmed `PLAYER_EVENT`'s own *outcome*, which `ADR-0015` extends beyond best-effort) — this WORK does not change delivery guarantees, only the source of what's being delivered.

## Context

**Ground-truth check found this WORK's own prior premise stale, not merely undesigned.** Its Canonical context previously cited `game/session/workflows/sessionlifecycle/manager.go`'s "already-implemented in-memory Effect/Presentation Output return" (built by `session-runtime-v1`'s `WORK-0006`/`WORK-0029`) as the mechanism this WORK would adapt. That mechanism no longer exists: `WORK-0038` (this Project, DONE 2026-09-29) retired `session.Output`/`Value`/`Type` outright, "not preserved as a compatibility shell" (`PROJECT.md`'s own WORK-0038 entry), together with the whole engine-based `Manager.Start`/`AnswerInteraction` Output-return path it depended on. Confirmed directly: `session/types.go`'s `StartResult`/`SubmitPlayerEventResult`/`CancelSessionResult`/`ExpireTimerResult` today carry no effect/event field of any kind.

The actual current gap, confirmed by reading every RUNNING-phase step: all four (`Start`, `SubmitPlayerEvent`, `CancelSession`, `ExpireTimer`) already call `parseCommands(output.RequestedCommands, known)` and already validate a `SEND_EVENT` into a `*platform.SendEvent` (recipients checked against the known-actor roster, payload size-bounded — `command.go`'s `decodeSendEvent`). The resulting `commands` slice is then only consumed by `timers.Apply` (SCHEDULE_TIMER/CANCEL_TIMER) and `completion.Detect` (SESSION_COMPLETE/SESSION_FAIL) — a `SendEvent` inside it is parsed, validated, and then silently dropped on every path, the same "validated but never consumed" gap shape `WORK-0040` found and fixed for timers before `WORK-0039` unified the command vocabulary. This WORK is that same fix, for `SEND_EVENT`.

**DRIFT flagged, not fixed here (out of this WORK's own ownership):** `docs/projects/active/session-runtime-v1/PROJECT.md` (a different active Project) still describes `Manager.Start`/`AnswerInteraction` returning `engine.Output`/`Value`-shaped Presentation/Effect data as "DONE" (its own WORK-0006/WORK-0029 rows, and its Capability Coverage table). That description is now stale — `WORK-0038` deleted the entire mechanism it refers to. This Project does not own `session-runtime-v1/PROJECT.md` and does not correct it here; flagged for that Project's own next synchronization pass.

**Vocabulary note (carried from 2026-09-28):** `WORK-0039` names this command `SEND_EVENT`, deliberately not `EMIT_EFFECT` — its `name`/`payload` are entirely game-defined and opaque to Playhoot (this WORK validates/delivers only the envelope: recipients, session, size/count limits, correlation); whether the generated frontend treats a given `SEND_EVENT` as an animation, a sound, a notification, or nothing at all is frontend logic this WORK has no visibility into and must not assume.

**Live-transport reality, confirmed against `session-runtime-v1`'s own current state:** its `WORK-0020` (the Coordinator/live-connection registries that would actually push a message to a connected client) is still DRAFT, not implemented — there is no live-transport code in the repository today beyond a route/WebSocket-upgrade skeleton. Exactly as `WORK-0006` did for the now-retired engine model, this WORK's own scope stops at making a validated `SEND_EVENT` available in memory to whatever calls `sessionlifecycle.Manager`; actually reaching a connected client remains deferred to `WORK-0020` and its own not-yet-drafted follow-on, per this Project's Ordering/Dependencies (Coordination Flag, unchanged by this WORK).

## Scope

### In Scope

- Collect every validated `*platform.SendEvent` already present in each RUNNING-phase step's own parsed `commands` (`Start`, `SubmitPlayerEvent`, `CancelSession`, `ExpireTimer` — all four, since all four already share the same `parseCommands`/`commands` shape and `WORK-0039`'s command vocabulary is uniform across them) into a new public, session-owned result type (see Approved Design) and return it in memory on that call's own Result value. Additive data only; no change to any existing parameter, business logic, or persistence.
- Carry each `SendEvent.Recipients` forward as-is (the same opaque, session-scoped actor identity `platform.ParseCommand` already validated it against), mirrored into a public `session`-owned type — **not** resolved to `session.UserUUID` or any other transport/routing identity by this WORK (see Human Decision below).

### Out of Scope

- Actually delivering the returned event over a live transport (WebSocket, Coordinator fan-out) to a connected client — no such transport exists yet; deferred to `session-runtime-v1`'s `WORK-0020` and its own not-yet-drafted follow-on WORK, exactly as `WORK-0006` deferred the equivalent step for the retired engine model.
- Any change to delivery reliability/guarantees — `SESSION-ADR-0019`'s best-effort, no-outbox, no-replay rule is restated, not extended or weakened.
- `SCHEDULE_TIMER`/`CANCEL_TIMER` (`WORK-0040`'s own scope, already DONE) and `SESSION_COMPLETE`/`SESSION_FAIL` (`internal/completion`, already DONE) — unaffected by this WORK.
- `WORK-0043`'s durable confirmed-outcome delivery — a `SEND_EVENT` is never durable and never the vehicle for a `PLAYER_EVENT`'s own accept/reject/fail outcome.

## Approved Design

- New public types in the `session` package (`session/types.go`), decoupled from the internal `platform` package the same way `WORK-0029` decoupled `session.Output`/`Value` from `engine.Output`/`Value` — and for the identical structural reason: `session/types.go` is package `session`, outside `session/workflows/sessionlifecycle/`, so it cannot import that tree's own `internal/platform` package (Go's `internal/` visibility rule) even if it wanted to expose `platform.SendEvent`/`platform.ActorRef` directly:

  ```go
  // ActorRef is an opaque, session-scoped actor identity — the same identity
  // platform.ParseCommand already validates a SEND_EVENT's own recipients
  // against, mirrored publicly. It is stable within one Session only, and is
  // deliberately NOT a UserUUID or any other transport/routing identity:
  // whatever future delivery layer consumes an OutboundEvent decides how (or
  // whether) to map this to a connection/routing identity of its own.
  type ActorRef string

  // OutboundEvent is one game-defined SEND_EVENT command a RUNNING-phase
  // call's own execute pass requested. Recipients are session-scoped only,
  // not yet resolved to any transport identity. Name/Payload remain entirely
  // game-defined and opaque to Playhoot (see platform.SendEvent); only the
  // envelope is Playhoot's own. Nothing named "delivered" here — this value
  // is produced before any live-transport attempt, which does not exist yet.
  type OutboundEvent struct {
      Recipients []ActorRef      `json:"recipients"`
      Name       string          `json:"name"`
      Payload    json.RawMessage `json:"payload"`
  }
  ```

- `StartResult`, `SubmitPlayerEventResult`, `CancelSessionResult`, `ExpireTimerResult` (`session/types.go`) each gain an additive `Events []OutboundEvent` field.
- New shared, pure helper (mirroring `parseCommands`'s own placement, likely `step_submit_player_event.go` alongside it, or a new small file): `collectOutboundEvents(commands []platform.Command) []session.OutboundEvent` — filters `commands` for `*platform.SendEvent` in original order and converts each one directly (`platform.ActorRef` and `session.ActorRef` are both single-field opaque strings; the conversion is a plain type conversion, not a lookup, and cannot fail). No database access, no `ctx`/`tx`/repo dependency, no repo call — unit-testable with plain Go values.
- Wired into all four RUNNING-phase steps, on every path that actually executed the script and obtained `commands` (the same paths `timers.Apply`/`completion.Detect` already run on) — a declined, fatal-terminalization, or non-executing path leaves `Events` nil, mirroring `WORK-0006`'s own precedent for the identical reason (nothing executed, nothing to report).
- **Idempotency-replay path returns nil `Events`, not a reconstruction of the original.** `Events` is tagged `json:"-"` so it is never part of the JSON payload a RUNNING-phase step persists for idempotent replay, and a replayed result therefore always has nil `Events` — see Human Decision below for why this is deliberate, not incidental.

## Constraints and Invariants

- Must not introduce a durable outbox for effects — `SESSION-ADR-0019`'s "no outbox for presentation" conclusion is unchanged by this WORK.
- Must not implement, assume, or stub any live-transport/Coordinator delivery mechanism — that remains `session-runtime-v1`'s `WORK-0020` and its own follow-on, exactly as `WORK-0006` deferred it.
- Must not translate a `SEND_EVENT`'s opaque `Name`/`Payload` into any Playhoot-understood presentation concept (animation, sound, notification) — per `platform/LOGICAL_CONTRACT.md`'s presentation-agnostic design, only the envelope (recipients) is Playhoot's own.
- Must not resolve `Recipients` to `UserUUID` or any other transport/routing identity — that decision belongs to whichever WORK designs the live-transport/Coordinator layer (`session-runtime-v1`'s `WORK-0020`), not this one (see Human Decision below). This WORK's own `collectOutboundEvents` must never fail or drop a recipient, since it performs no lookup at all — every `ActorRef` it emits is exactly the one `platform.ParseCommand` already validated.

## Acceptance Criteria

- A backend script issuing one `SEND_EVENT` command from `Start`'s first Turn results in `StartResult.Events` containing one `OutboundEvent` with the same `Name`/`Payload`/`Recipients` (as opaque `session.ActorRef` values, unresolved) — verified by a test inspecting the returned Go value directly, no live transport, no database lookup involved.
- The same, independently, for `SubmitPlayerEvent`, `CancelSession`, and `ExpireTimer`.
- Multiple `SEND_EVENT` commands from a single `execute` call all appear in `Events`, in original order, each with its own `Recipients`.
- A declined, fatal-terminalization, or already-RUNNING/no-op path returns nil `Events`.
- A genuine idempotency-replay (the same request retried with the same idempotency key) returns nil `Events` — never a reconstruction of the original call's events.
- This WORK adds no new durable column/table (verified by an empty migration diff).
- `go build ./...`, `go vet ./...`, `go test ./... -count=1` pass, with no new failure beyond this Project's already-recorded pre-existing, unrelated failures.

## Implementation Freedom

- Exact placement/naming of `collectOutboundEvents` and its call-site wiring inside each of the four step files.
- Whether `OutboundEvent`'s `Recipients` empty-vs-nil rendering follows any particular convention beyond valid JSON.

## Verification

- Unit tests for `collectOutboundEvents` (order preservation, multi-command handling, pure conversion with no I/O) — no database needed for this helper at all.
- Integration tests (real Postgres, per this Project's established practice) for at least one non-replay case per RUNNING-phase step, plus one idempotency-replay case confirming `Events` is nil on replay.

## Documentation Impact

### Current-State Documentation, Checked Against Reality Before Implementation

`session/CURRENT_STATE.md` and `session/docs/FLOWS.md` were checked directly rather than assumed synchronizable by this WORK alone: both still describe the fully retired Game-Language-engine mechanism end to end (`engineservice.Compile`/`StartTurn`/`AdvanceTurn`, `AnswerInteraction`/`SubmitUserIntent`, `clientoutputs.ClientFacing`, `mapOutputs`, `session.Output`/`Value`) - a pre-existing gap that predates this WORK entirely (already flagged, not fixed, by `WORK-0038`'s own Completion Record and reflagged in this Project's `PROJECT.md`/`internal/AI_CONTEXT.md` history at `WORK-0040`'s own closure) and spans nearly this entire capability, not only the effect/event path this WORK touches. A narrow patch limited to the SEND_EVENT-related sentences would leave the surrounding text internally contradictory (describing components - `engineservice`, `AnswerInteraction`, `mapOutputs` - that no longer exist) rather than genuinely synchronized. Per this Project's own established practice for the identical situation, this is flagged again rather than silently patched or silently left unmentioned:

- `session/CURRENT_STATE.md`, `session/docs/FLOWS.md`, `docs/architecture/SYSTEM_MAP.md`, `docs/ai/KNOWLEDGE_MAP.md` - still describe the retired Game-Language-engine mechanism throughout; a dedicated documentation-catch-up pass (spanning `WORK-0032`/`WORK-0038`/`WORK-0039`/`WORK-0040`/`WORK-0041`/this WORK's own consequences together) is the right-sized fix, not a per-WORK patch. Not yet scheduled as its own WORK as of this writing.

### Flagged, Not Owned Here

- `docs/projects/active/session-runtime-v1/PROJECT.md` — stale WORK-0006/WORK-0029 "DONE" description of the now-retired engine-based Output-return mechanism (see Context's DRIFT note). That Project's own concern to synchronize, not this WORK's.

## Human Decision: API Shape Corrected (2026-09-29)

The human reviewed this WORK's first DRAFT pass and corrected the proposed public-API shape on four points, all incorporated above:

1. **Naming.** `DeliveredEvent` was rejected as semantically false — nothing is delivered by this WORK; it stops before any live-transport attempt, and the value could easily be produced yet never reach a client. Renamed to `OutboundEvent`.
2. **No replay of transient events.** The first pass proposed reconstructing and re-returning the same `Events` on an idempotency replay, reasoning from "correlation exists for repeated delivery" — but that reasoning was internally inconsistent with `SEND_EVENT`'s own best-effort/no-outbox nature: a missed transient event on replay is explicitly acceptable (`project()`/`ClientState` restores correctness independently), and `WORK-0043` — not this WORK — owns the class of output that actually needs durable retry semantics. Corrected: idempotency replay returns nil `Events`. This also means no correlation/event-id field is needed on `OutboundEvent` (it would only matter if replay re-emitted), keeping the type to exactly `Recipients`/`Name`/`Payload`.
3. **No early transport-identity resolution.** The first pass resolved `Recipients` to `session.UserUUID`, implicitly choosing the future Coordinator's routing identity before any WORK has actually decided what that identity is (`session-runtime-v1`'s `WORK-0020`, which owns that decision, is still DRAFT). Corrected: `Recipients` stays a session-scoped `session.ActorRef` (a public mirror of the same identity `platform.ParseCommand` already validates against), and resolving it to a connection/routing identity is left entirely to whatever WORK builds the delivery layer.
4. **No silent recipient drop.** The first pass had `collectOutboundEvents`'s underlying resolution "drop" a recipient defensively if `FindActorsByIDs` unexpectedly failed to return it, treating that as an ordinary best-effort limitation. The human's correction: since `platform.ParseCommand` already guarantees every recipient is a known, valid actor, a failure to account for one downstream would be an invariant violation, not a normal delivery failure — silently narrowing `SEND_EVENT`'s own declared recipient set would be worse than a hard failure. This is now moot rather than merely fixed: point 3's correction removes the resolution step (and therefore the failure mode) entirely — `collectOutboundEvents` performs a pure type conversion that cannot fail.

## Blockers

None.

## Completion Record

### Implementation Report (2026-09-29)

Work: `docs/projects/active/js-runtime-migration/works/WORK-0042-effects-delivery-best-effort.md`

Work status: IMPLEMENTING

Implemented:
- `session/types.go`: new public `ActorRef` (opaque, session-scoped identity) and `OutboundEvent` (`Recipients []ActorRef`, `Name string`, `Payload json.RawMessage`) types; an additive `Events []OutboundEvent` field, tagged `json:"-"`, on `StartResult`, `SubmitPlayerEventResult`, `CancelSessionResult`, `ExpireTimerResult`. Stale doc comments claiming these types "carry no client-facing view/effect payload" (leftover phrasing from `WORK-0038`'s own removal pass, not a ratified constraint) corrected to describe `Events`.
- `session/workflows/sessionlifecycle/outbound_events.go`: new `collectOutboundEvents(commands []platform.Command) []session.OutboundEvent` - a pure, infallible filter/conversion (no database access), extracting every `*platform.SendEvent` in original order.
- Wired into all four RUNNING-phase steps (`step_start.go`, `step_submit_player_event.go`, `step_cancel_session.go`, `step_expire_timer.go`), only on the path that actually executed the script and obtained `commands`; every declined/fatal/non-executing/script-rejected path leaves `Events` nil (unchanged from before this WORK).
- Idempotency-replay paths return nil `Events` (the `json:"-"` tag keeps `Events` out of the persisted response payload `Start`/`SubmitPlayerEvent`/`CancelSession` marshal for replay, so a replayed result's `Events` is always nil) - deliberate, not incidental (see Human Decision above).

Local implementation decisions:
- Exact placement of `collectOutboundEvents` in its own new file, `outbound_events.go`, alongside `outbound_events_test.go` (mirroring `known_actors.go`'s own existing precedent for a small shared conversion helper).
- None beyond ordinary naming/placement freedom already granted by Implementation Freedom.

Deviations from the approved WORK:
- None.

Discoveries:
- `session/CURRENT_STATE.md`/`session/docs/FLOWS.md` are far more stale than this WORK's own original Documentation Impact assumed - both describe the fully retired Game-Language-engine mechanism end to end, not only the effect/event path. Reported as DRIFT under Documentation Impact rather than silently patched or silently skipped; this is a pre-existing gap already flagged (not fixed) by `WORK-0038`'s own Completion Record and reflagged at `WORK-0040`'s closure, not newly introduced or newly discovered by this WORK.

Verification performed:
- `go build ./...`, `go vet ./...` - clean, repository-wide.
- `gofmt -l` flagged 7 changed files; confirmed pure pre-existing CRLF-line-ending noise via a line-ending-normalized diff (consistent with this repository's already-documented Windows-checkout finding), not a real formatting defect.
- New unit test `TestCollectOutboundEvents` (`outbound_events_test.go`) - order preservation, no-SEND_EVENT-commands nil case, multi-command independent recipients.
- New/extended integration tests against a real, reachable Postgres container (`TestManagerStart_Integration`, `TestManagerSubmitPlayerEvent_Integration`, `TestManagerCancelSession_Integration`, `TestManagerExpireTimer_Integration`): one case per RUNNING-phase step proving `Events` is populated and correctly ordered/shaped from a real `Execute` call, plus an idempotency-replay case for `Start`/`SubmitPlayerEvent`/`CancelSession` proving a replay returns nil `Events` (not a reconstruction) despite the original call having genuinely collected one, plus a `CancelSession` case proving a script-rejected cancellation (no execute pass) also leaves `Events` nil.
- `go test ./... -count=1` against real Postgres: clean except the same two confirmed pre-existing, unrelated failures this Project's history already documents repeatedly (`TestNoInternalDocCitationsInComments` against an untouched pre-existing migration file; `TestManagerJoin_Integration_ConcurrentOperationRacingLobbyExpiration`, the pre-existing `session-runtime-v1`-owned Join/Leave race) - confirmed via `git status`/`git diff` that neither failing test's file is touched by this WORK.

Documentation synchronized:
- None beyond `session/types.go`'s own doc comments (part of the implementation itself, not separately-tracked current-state documentation).
- `session/CURRENT_STATE.md`/`session/docs/FLOWS.md` explicitly NOT touched - see Documentation Impact/Discoveries above for why a narrow patch would be dishonest given their actual current state.

Known limitations:
- Recipients remain a session-scoped `ActorRef`, not resolved to any transport/routing identity, and no live-transport layer consumes `Events` yet - both by design (see Approved Design/Human Decision), not gaps in this WORK's own scope.

Ready for independent review:
YES.

### Independent Review (2026-09-29)

A fresh agent, with no access to this session's own context, reviewed this WORK per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`. It determined the exact change-set itself via `git status`/`git diff --stat` (confirming it matched this WORK's own scope, nothing unrelated mixed in), read `session/types.go`/`outbound_events.go`/`outbound_events_test.go` directly, traced every branch of all four step files to confirm `Events` is nil on every non-executing path, traced the `json:"-"`-driven replay mechanism end to end (not merely accepted the claim), confirmed the integration tests specifically rule out a vacuous replay-returns-nil assertion (asserting the original call actually collected events first), confirmed no migration/schema diff exists, ran its own fresh `go build`/`go vet`/`go test ./... -count=1` against real Postgres (same two pre-existing failures, independently confirmed unrelated via `git diff`), ran `gofmt -l` repository-wide (confirming the CRLF noise is pre-existing/repository-wide, not introduced by this WORK), and checked the new doc comments against this repository's own code-comments engineering standard.

Verdict: **APPROVED, no findings.**

Two documentation/drift notes were logged (not blocking): (1) independently confirmed the Documentation Impact framing (session/CURRENT_STATE.md/session/docs/FLOWS.md left untouched) is accurate, not evasive, but flagged that this is now the third consecutive WORK deferring the same documentation-catch-up gap - worth this Project's own planning attention, not a fix this WORK itself owes; (2) reconfirmed the `session-runtime-v1/PROJECT.md` cross-Project drift note is accurate and correctly not owned here.

No REQUIRED_FIX or DECISION_REQUIRED finding remains. Closed to DONE.
