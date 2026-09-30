Checkpoint: WORK-0042 DONE (not a new human-decision gate — informational).
Date: 2026-09-29

**Update:** You said "proceed." Implementation completed exactly per the corrected shape below, and independent review (a fresh agent with no memory of the implementation) returned **APPROVED with no findings** - it independently re-verified every claim against the actual diff/code/tests rather than trusting the implementation report, including the specific nil-on-replay guarantee and confirming the same two pre-existing test failures are genuinely unrelated. One thing worth your awareness, not a decision needed now: this is the third consecutive WORK in this Project to flag (not fix) the same stale-documentation gap in `session/CURRENT_STATE.md`/`session/docs/FLOWS.md` (both still describe the fully retired Game-Language-engine mechanism). It's accumulating - a dedicated documentation-catch-up WORK is probably due soon, whenever you'd like to schedule one. No further decision is needed from you on WORK-0042 itself. Full detail: WORK-0042's own Completion Record.

Original DRAFT/READY-authorization checkpoint, preserved below:

## What this WORK does

`SEND_EVENT` (a backend script asking Playhoot to send a cosmetic/transient occurrence to one or more players — animation, sound, notification, whatever the frontend chooses to do with it) is already validated by every RUNNING-phase call today, then silently thrown away. This WORK makes it available in memory on that call's own result. It does not build any live-transport layer itself — nothing in the repository can push a message to a connected client yet, and this WORK does not pretend otherwise.

## Why this WORK, right now

You said to continue with the next logical work. `WORK-0042` is the first unblocked item in the project's own ordering (its only recorded blocker, `WORK-0039`, is DONE); `WORK-0043` — the other candidate at this point — needs an outbox-mechanism decision and cross-project coordination resolved first, so it's more blocked.

Checking the actual code (not this WORK's own prior text) found that text stale: it assumed an older in-memory "Output" return mechanism (built for the retired Game-Language engine) still existed for this WORK to adapt. It doesn't — `WORK-0038` deleted it outright, with no replacement. The real gap is simpler and more direct: every RUNNING-phase step already parses and validates a `SEND_EVENT` command, then drops it — the same "validated but never used" gap `WORK-0040` found and fixed for timers.

## Your four corrections, all applied

You reviewed my first pass and were right on all four points:

1. **Naming.** `DeliveredEvent` was wrong — nothing is delivered by this WORK; it stops before any live-transport attempt, and could easily be produced yet never reach a client. Renamed to `OutboundEvent`.
2. **No replay of transient events.** I'd proposed re-returning the same events on an idempotency replay, reasoning from "correlation exists for repeated delivery" — but that contradicted `SEND_EVENT`'s own best-effort/no-outbox nature. A missed transient event on replay is fine; `project()`/`ClientState` restores correctness independently, and `WORK-0043` (not this WORK) owns the class of output that actually needs durable retry semantics. Fixed: replay now returns nil `Events`. This also means no correlation/event-id field is needed on the type at all, since nothing is ever re-emitted.
3. **No early transport-identity resolution.** I'd resolved `Recipients` to `UserUUID`, quietly deciding the future Coordinator's routing identity before any WORK has actually decided what it is (`session-runtime-v1`'s `WORK-0020`, still DRAFT). Fixed: `Recipients` stays a session-scoped `session.ActorRef` — the same identity already validated today — and resolving it to a routing identity is left entirely to whichever WORK builds the delivery layer.
4. **No silent recipient drop.** I'd had the (now-removed) resolution step defensively drop a recipient it couldn't resolve. You're right that would have been an invariant violation, not a normal best-effort limitation — but it's moot now anyway: removing the resolution step per point 3 removes the failure mode entirely. The remaining helper is a pure, infallible type conversion.

## What's new (revised)

```go
type ActorRef string // session-scoped, mirrors the already-validated identity

type OutboundEvent struct {
    Recipients []ActorRef
    Name       string
    Payload    json.RawMessage
}
```
— an `Events []OutboundEvent` field on all four Result types, and a small pure helper (`collectOutboundEvents`) with no database access at all.

## What's explicitly NOT done by this WORK

- No live delivery to a connected client, and no decision about what identity that layer will route on — both belong to `session-runtime-v1`'s `WORK-0020`.
- No durable outbox, no replay of missed events — this stays best-effort, exactly as already decided.
- `WORK-0043`'s durable confirmed-outcome delivery is untouched.

## Unresolved blockers

None.

## What READY would authorize

Implementing exactly what's described: the new `session.ActorRef`/`OutboundEvent` types and `Events` field, the pure collection helper, and wiring it into the four existing steps — nothing about live transport, identity resolution, or durability.

## Recommendation

If the revised shape above matches what you had in mind, this is ready for READY authorization. Full detail in `../works/WORK-0042-effects-delivery-best-effort.md`.

---

Prior checkpoint, preserved below:

Checkpoint: WORK-0040 DONE (not a new human-decision gate — informational).
Date: 2026-09-29

**Update:** you confirmed the replace-on-reschedule design after discussion. Implementation is complete and independent review (two rounds - one found a doc-only inconsistency, fixed) returned APPROVED with no findings. `go build`/`go vet`/`go test ./...` are clean except the two confirmed pre-existing, unrelated failures already documented elsewhere in this Project. No further decision is needed from you. Full detail: WORK-0040's own Completion Record.

Original DRAFT/READY-authorization checkpoint, preserved below:

## What this WORK does

Wires the two timer commands a backend script can already request (`SCHEDULE_TIMER`/`CANCEL_TIMER` - accepted since `WORK-0039`) into actual persistence. Right now they're silently dropped: every step already parses them but never acts on them. This closes that gap.

## Why this WORK, right now

You said to proceed with the next logical work and let me choose. `WORK-0040` was the first unblocked item in the project's own ordering once `WORK-0039` landed - and checking the actual code (not assuming) confirmed the gap is real: the old timer-obligation table/columns still exist from the retired engine, `ExpireTimer` already reads from them, but nothing writes a new one when a script asks to schedule a timer.

## The one thing worth your attention before READY

`SESSION-ADR-0011` (the old Game-Language "keyed timer slot" decision) said scheduling a timer that's already pending must be rejected outright - the author has to explicitly cancel first. That decision doesn't carry over cleanly: the new JS command vocabulary only has one opaque `Timer` name (no separate "key"), and rejecting a whole Turn after the script already ran would need real extra machinery (re-check the database before ever creating the Turn's row, roll back cleanly otherwise).

My recommendation, already reflected in the DRAFT: **scheduling a timer that's already active implicitly replaces it** (cancel old, create new) instead of erroring. This is simpler, and an author who wants strict "reject if already scheduled" behavior can always check their own game state before deciding to schedule. If you'd rather it reject/error instead, tell me and I'll change the design before READY.

## What's new

A column rename (`engine_slot`/`engine_key` -> `timer`/`data` - purely a rename, they already hold exactly this data informally today) plus a tightened uniqueness rule (`data` should never have been part of a timer's identity), and a small shared function (`internal/timers.Apply`, same shape as the existing `internal/completion.Detect`) wired into all four steps that can produce a RuntimeTurn.

## What's explicitly NOT done by this WORK

- No physical "fire the timer when the delay elapses" mechanism - that's a separate, already-tracked gap (`session-runtime-v1`'s `WORK-0020`).
- No delivery of the other command kind, `SEND_EVENT` - that's `WORK-0042`.

## Unresolved blockers

None.

## What READY would authorize

Implementing exactly what's described above: the migration, the rename, the new shared package, and wiring it into the four existing steps - nothing about physical scheduling or event delivery.

## Recommendation

Ready for READY authorization if you're comfortable with the replace-on-reschedule call above - full detail in `../works/WORK-0040-timer-obligations-adapted-to-js-commands.md`.

---

Prior checkpoint, preserved below:

Checkpoint: WORK-0037 DONE (not a new human-decision gate — informational).
Date: 2026-09-29

**Update:** You authorized READY ("move to ready and proceed"). Implementation is complete and independent review (three rounds - two found real gaps, both fixed and re-verified) returned APPROVED with no findings. `go build`/`go vet`/`go test ./...` are clean except one confirmed pre-existing, unrelated test failure. No further decision is needed from you. Full detail: WORK-0037's own Completion Record.

Original DRAFT/READY-authorization checkpoint, preserved below:

## What this WORK does

Adds a static-analysis check over authored **backend script** source (not frontend script — different runtime, out of scope) that runs before a script is accepted for publish/use. It flags two kinds of things:

- **ERROR**: the script references something outside what the sandbox actually supports (e.g. `fetch`, `require`, `process`, `fs`) — this is guaranteed to fail at runtime today anyway; catching it earlier is strictly better than a live failure discovered later.
- **WARNING**: the script calls `Math.random()`/`Date()`/`Date.now()` — these are already safe and deterministic (the sandbox silently substitutes them with session-context-derived values), but an author expecting real randomness or a real wall-clock deadline will get quietly confusing behavior, so it's surfaced without blocking.

## Why the scope changed from the original PLANNED text

The original text said this WORK would catch scripts "calling known nondeterministic host APIs (wall-clock reads, `Math.random`, network, filesystem...)". Before drafting a design against that assumption, I checked it against the actual implemented sandbox (`WORK-0035`/`WORK-0036`, both DONE) — and it's stale: `Date`/`Math.random` are already neutralized by a runtime prelude, and no filesystem/network capability is wired into the JS globals at all (calling them already throws a clean, already-handled runtime rejection). So a lint against those specific APIs would not be closing a real gap. I raised this to you directly rather than silently building a lint against a premise that no longer holds.

## The two decisions you already made in this session

1. Lint scope is both categories above (ERROR allow-list violations + WARNING `Math.random`/`Date` usage), not just one.
2. Implementation uses a real JS parser/AST library, not a lightweight regex/token scan. This is a new external Go dependency (none exists in this repository today).

## What's new (illustrative, not frozen — see WORK-0037's own Implementation Freedom)

A Go package exposing something like `Validate(source string) (Result, error)`, callable by a future publish-path caller (`WORK-0033`, still PLANNED and not itself part of this authorization) without needing the sandbox/jsexecutor service running at all — this is pure static analysis, it never executes the script.

## What's explicitly NOT done by this WORK

- No publish/authoring HTTP path or UI surfacing of warnings — that's `WORK-0033`'s own future scope; this WORK only builds the validation capability itself, not its caller.
- Frontend script is never validated by this capability.
- The sandbox's own runtime isolation is unchanged (`WORK-0035`/`WORK-0036` already own it).

## Unresolved blockers

None. The only prior blocker (`WORK-0035`'s supported-JavaScript-profile decision) is resolved.

## What READY would authorize

A Codebase Agent implementing exactly what WORK-0037's own Scope/Approved Design sections describe: the validation package, its allow-list (checked against the actual sandbox where not already documented), its fixture-based tests, and the new authoring-facing documentation page — nothing about `WORK-0033`'s own future publish-path wiring.

## Recommendation

The design is internally consistent with what's actually implemented (not the original stale assumption), and both open questions were resolved by you directly rather than invented. Ready for READY authorization if you're satisfied with the ERROR/WARNING split and the new-dependency decision above — full detail is in `../works/WORK-0037-deterministic-authoring-static-analysis-lint-enforcement.md`.

---

Prior checkpoint, preserved below:

Checkpoint: WORK-0034 implemented, pending independent review (not a new human-decision gate — informational).
Date: 2026-09-28

**Update:** You authorized READY ("Nice, proceed") and the implementation described below is now complete. `go build`/`go vet`/`go test ./...` are clean except one pre-existing, unrelated test failure (verified via diff against the pre-move code, not caused by this WORK). Real-Postgres verification of the two new tables is outstanding only because this sandbox has no reachable database — the same limitation recorded throughout this repository's history. Next step is independent review per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`, then closure if APPROVED; no further decision is needed from you unless review surfaces a DECISION_REQUIRED finding. Full detail: WORK-0034's own Completion Record.

Original READY-authorization checkpoint, preserved below:

## What this WORK does

Session Runtime's `Create` stops resolving the Game's current version through Game Management and instead resolves it entirely from two new tables Session Runtime owns itself. Also moves `game/management`/`game/session` to independent top-level packages (`management/`, `session/`), per `ADR-0014`.

## Scope, in one paragraph

Only `Create` moves. The other five call sites that read a pinned Game version (`Join`, `Start`, `AnswerInteraction`, `SubmitUserIntent`, `CancelSession`, `ExpireTimer`) are explicitly left untouched — still reading Game Management, still running the old Game Language engine — because they can't execute the new JavaScript artifact until `WORK-0038` wires in the JS Executor, which is already `WORK-0038`'s own stated scope. This was a real correctness constraint found while drafting, not a preference.

## The two decisions you already made in this session

1. `Create` receives only `game_uuid` (already true today, no signature change) and resolves the current version purely from Session Runtime's own new tables — never from Game Management, never from data Composer hands it. Composer's own future design (`WORK-0032`) is therefore just a visibility gate with no data handoff.
2. This WORK moves only `Create`, per the scope constraint above.

## What's new (illustrative, not frozen — see WORK-0034's Implementation Freedom)

Two Session-owned tables: `session_games` (`game_uuid` -> `current_definition_uuid`) and `session_game_version_artifacts` (one row per immutable version: `definition_uuid`, `game_uuid`, `backend_script`, `frontend_script`, optional `game_contract`/`assets`/`platform_contract_version`). Real FK between them (same domain, no cross-domain FK). `Create`'s old `engineservice.Compile` validation is removed with no replacement — there's no equivalent JS validation designed yet (that's `WORK-0037`'s future job, at authoring/publish time, not at `Create`).

## What's explicitly NOT done by this WORK

- Game Management still depends on `program`/`gameservice`/`engine` (needed by the 5 untouched call sites).
- `game/language/v1` is not relocated into Session's internal tree yet.
- No publish path exists yet to populate the new tables for a real game (`WORK-0033`) — this WORK's own tests seed fixture rows directly.
- `ADR-0014`'s full goal is therefore only partially closed by this WORK; `WORK-0038` closes the rest.

## Unresolved blockers

None. Both blockers inherited from the cancelled WORK-0031 were resolved by the two decisions above.

## What READY authorizes

A Codebase Agent may implement: the two new tables/migration, `Create`'s new resolution logic, removing `gameCurrentVersionReader` from `Manager`/`New`, and the `game/management`→`management/` / `game/session`→`session/` package moves — nothing beyond what WORK-0034's own Scope section lists. It does not authorize touching the five other call sites, Game Management's use cases, or `game/language/v1`'s location.

## Recommendation

The design is internally consistent and the open questions that existed were resolved by you directly, not invented. Ready for READY authorization if you're satisfied with the table shape and scope split above — full detail is in `../works/WORK-0034-session-owned-executable-script-artifact-and-package-restructuring.md`.
