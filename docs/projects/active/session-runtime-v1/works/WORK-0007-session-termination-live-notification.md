# WORK-0007: Notify Connected Clients on Session Termination

Status: DRAFT
Created: 2026-09-20
Last status change: 2026-09-20 (Part H reconciliation, same day: broadcast scope generalized to both connection roles - see "Scope Addition (Part H Reconciliation, 2026-09-20)" below)

Related decisions:
- GAME-ADR-0002 (Live Session Coordinator responsibility boundary)
- GAME-ADR-0019 (RuntimeTurn execution bound and terminal cleanup - the fatal-path/terminal-cleanup invariant this WORK adds a live-notification consequence to, without changing the invariant itself)
- GAME-ADR-0020 (post-commit client delivery semantics, best-effort)

Canonical context:
- `docs/projects/active/session-runtime-v1/works/WORK-0005-thin-live-coordinator.md` (the live Coordinator this WORK extends)
- `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (the second, ADMIN, registry this WORK's broadcast must also reach - see "Scope Addition (Part H Reconciliation, 2026-09-20)" below)
- `docs/projects/active/session-runtime-v1/works/WORK-0006-broaden-live-fanout-effects-presentations.md` (the "Question/Presentation/Effect only cross the wire" principle this WORK's own termination message follows, as a generic UI-shaped signal rather than a raw domain event)
- `game/session/workflows/sessionlifecycle/step_answer_interaction.go` (`terminalizeAnswerInteractionFatal`), `game/session/workflows/sessionlifecycle/step_start.go` (`terminalizeStartFatal`) - the existing fatal paths this WORK's notification covers
- `game/session/workflows/sessionlifecycle/internal/repo/interaction.go` (`CloseAllActiveInteractionsForSession` - confirms closed rows from these paths have `closed_by_turn_id = NULL`, structurally excluded from the existing live-event query)
- `play/coordinator.go` (`Deliver`'s existing per-recipient fan-out, which this WORK's broadcast mechanism is deliberately different from - see Scope)
- `game/language/v1/engine/output.go` (`WorkflowCompletedOutput` - `Path == nil` is the root instance; per its own doc comment, "there is no parent to notify... this is how a session layer observes that directly, ending the game instance")
- `game/language/v1/engine/instance.go` (`WorkflowInstance.Outcome` - nil while running, set only once a `Complete`/`Fail`/`Cancel` control applies and "no further transition may apply" - confirms a completed root instance can structurally never produce anything else, so treating it as terminal is forced, not a judgment call)

## Outcome

Every client currently connected to a Session receives a live "session ended" message, and then has their connection closed server-side, whenever that Session becomes `TERMINAL` for any reason while clients are connected - a fatal RuntimeTurn execution failure (already existed), or the game's own root workflow completing naturally (confirmed deterministic and newly wired by this WORK). Today only whoever's request directly triggered a fatal failure learns anything; every other connected client, and every client connected when the game simply finishes normally, is left with no signal at all.

**Generalized invariant (added 2026-09-20, migration reconciliation - no scope change to this WORK's own two causes)**: the underlying mechanism this WORK builds - "broadcast a generic termination signal to every currently-bound connection for a Session, then close those connections" - must be built as a reusable mechanism on `play.Coordinator`, because every future terminal cause this Project's remaining WORK introduces is required to reuse it rather than reinventing "notify connected clients on termination" per cause: manual cancellation (WORK-0011), timer-driven termination if a game logic path terminalizes on one (WORK-0012), and inactivity/reaper termination (WORK-0016) all must plug into this same mechanism when they materialize. This WORK's own In Scope/Acceptance Criteria remain the two causes below; it is not expanded to implement those future causes itself.

The message itself is a generic, cause-agnostic "session ended" UI signal - the same fixed screen for every client regardless of why. A personalized results screen (winners/losers/score) is explicitly deferred - see `docs/product/IDEAS.md -> Personalized End-of-Game Screen`.

## Context

**Why the existing fan-out (`Deliver`) cannot cover this without a separate mechanism**: `Deliver` fans out `Event`s derived from a specific `RuntimeTurn`'s own Outputs, addressed one `Event`-one-`Recipient`. Both termination causes this WORK covers are structurally different from that: the fatal-path cleanup (`CloseAllActiveInteractionsForSession`) is deliberately *not* Turn-produced (`closed_by_turn_id = NULL` - "Session lifecycle cleanup, not gameplay"), and a root `WorkflowCompletedOutput` is a fact about the *whole Session*, not about any one recipient - it needs to reach every currently-bound connection, not be matched against a `Recipient` field. Both need their own broadcast-to-everyone-bound mechanism, not an extension of `Deliver`'s per-recipient contract.

**Root workflow completion is a deterministic, forced signal, not a design choice** (confirmed during this WORK's own design, against the engine's actual source): a `Program` has exactly one root workflow (`Program.RootWorkflow` is a single name, `Snapshot.Root` a single instance - there is no "multiple parallel top-level workflows" case). `WorkflowInstance.Outcome` is nil while running and, once set by a terminal control, the instance can never receive another transition - so a completed root instance is not merely "probably done," it is structurally guaranteed to never do anything else. Treating a root-`Path` `WorkflowCompletedOutput` as "the game is over, terminalize the Session" is therefore the only coherent interpretation, not an assumption requiring its own product decision the way it looked before this was checked against the engine's actual invariants. A game that wants a post-game continuation (rematch, lobby) simply does not apply a terminal control at what a player perceives as "the end" - it loops back to a non-terminal state instead; that is an authoring choice, not something Session Runtime needs to accommodate specially.

Human-confirmed direction (2026-09-20): send a message to every connected client, then close the connection; the message is a generic "ended" signal, not a full Presentation - every client shows the same fixed screen for now.

## Scope

### In Scope

- **New Session Runtime business logic** (a genuine addition, not just additive plumbing like WORK-0006's): `sessionlifecycle.Manager`'s `Start`/`AnswerInteraction` recognize a root-`Path` `WorkflowCompletedOutput` produced during their own drain, and terminalize the Session on it - mirroring the existing fatal path's shape (`sessions.phase = TERMINAL`, a new `TerminalReason` value, closing any still-`ACTIVE` interactions via the existing `CloseAllActiveInteractionsForSession`), but recorded as an ordinary, expected outcome (the game finished correctly), not a failure.
- A way for `play.Coordinator` to learn "this session just became `TERMINAL`" as a single, cause-agnostic fact (fatal failure or natural completion both produce it) - distinct from an ordinary `Event`, since it is not per-recipient and applies to every currently-bound connection for the Session at once.
- Broadcasting one generic "session ended" message to every currently-bound connection for that Session (not filtered by recipient, unlike `Deliver`) - a UI-shaped signal per WORK-0006's wire-protocol principle (every client renders the same fixed "ended" screen), not a raw cause/outcome dump.
- Closing each of those connections server-side after sending the message.
- Covers all three termination-while-connected paths: `AnswerInteraction`'s fatal path, `Start`'s fatal path, and either method's new root-completion path.

### Out of Scope

- A personalized/configurable end-of-game screen (winners/losers/score, derived from `WorkflowOutcome.Result`) - `docs/product/IDEAS.md -> Personalized End-of-Game Screen`, explicitly deferred.
- Lobby-phase expiration (`LOBBY_EXPIRED`) notification - a Session can only expire from `LOBBY`, before `Start`, when whether any client is even connected yet is a separate question this WORK does not resolve; may be worth folding in later but is not assumed here. This becomes concretely reachable once WORK-0008 (Live Lobby / Session Bootstrap) lets clients hold a live connection before `Start` - worth resolving when WORK-0007 and WORK-0008 are jointly refined, not decided here.
- The general "other connected players learn about ordinary gameplay state changes" gap (WORK-0006) - a different mechanism (per-recipient `Event` translation vs. this WORK's session-wide broadcast); the two WORKs are related but not the same code path.
- Reconnect/resync (Slice 7) - a client not connected at the moment of termination simply never received anything, consistent with this whole initiative's best-effort/no-replay stance (GAME-ADR-0020).
- Any change to how a non-fatal, non-terminal decline (`AnswerInteractionOutcomeRejected`/`Conflict`) is reported - unaffected, still only a direct reply to the caller.

## Blockers

Status: **PARTIALLY RESOLVED**. Direction (send-then-close, generic message, cover natural completion too) is human-confirmed. Mechanism needs the same design scrutiny as WORK-0006's before READY.

1. **Exact `TerminalReason`/business-logic shape for natural completion.** Mirrors the existing fatal path's shape closely, but is a new code path inside `Manager`, not a copy-paste: what `TerminalReason` value, does it need the `WorkflowOutcome.Result` payload recorded anywhere durably (even if not delivered to clients yet, per the deferred personalized-screen idea), and does this detection happen inside the same drain loop that already looks for `OpenQuestionOutput`/`CloseQuestionOutput` (Blocker: does this belong in `interaction_capture.go`-adjacent code, or is it clearly separate enough to be its own step)?
2. **Where does "broadcast to everyone bound" live** - a dedicated `Coordinator` method reading its own registry directly (`c.sessions[sessionUUID]`, no per-recipient filtering), versus overloading `Event`/`Deliver` with a "broadcast" meaning. Leaning toward a dedicated method, consistent with WORK-0006's own "Question/Presentation/Effect are the only Event-shaped things" principle - a session-wide termination signal is not any of those three, so it should not pretend to be an `Event` at all.
3. **How `play/sessionruntime` signals "this call's result means the Session is now `TERMINAL`" up to `Coordinator`** - an explicit field on `play.StartResult`/`AnswerInteractionResult` (e.g. `Terminated bool` plus a reason string), rather than `Coordinator` needing to know which outcome strings imply termination.
4. **Transport-level "send this message, then close the socket"** - `api/session`'s write pump has no such concept today; a small, local addition once Blockers 1-3 are resolved.

## Acceptance Criteria (Draft, Pending Blocker Resolution)

- A Definition authored to complete its root workflow after a specific answer, with two connected clients (the one answering, and a bystander with their own open interaction), results in both receiving the generic "session ended" message and both connections closing - proving natural completion, not only fatal failure, is covered.
- The existing fatal-path proof (two connected clients, one triggers a fatal `AnswerInteraction`, both receive the message and are closed) continues to pass.
- The same proof for `Start`'s own fatal path with a second already-connected client.
- No change to any already-correct non-terminal outcome's delivery behavior.
- `go build ./...`, `go vet ./...`, `go test ./... -count=1` pass, with no new failure beyond the already-recorded out-of-scope `getgame` JSONB-comparison defect.

## Documentation Impact

- `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` - describe the new termination-notification flow, including natural completion, once implemented.
- `game/session/session.go` - new `TerminalReason` constant, documented alongside the existing three.
- `play/README.md` - document the new broadcast mechanism as a second, distinct fan-out path alongside per-recipient `Deliver`.

## Scope Addition (Part H Reconciliation, 2026-09-20)

`game/docs/decisions/GAME-ADR-0025-role-aware-live-connections.md` (accepted by a broader reconciliation session, alongside `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md`) establishes that a Session's live transport has two independent connection roles, `ADMIN` and `PARTICIPANT`, and that the same `UserUUID` may legitimately hold one of each simultaneously. This WORK's "broadcast to every currently-bound connection for a Session" mechanism (Blocker 2) must reach every bound connection of *either* role, not only the participant registry this WORK's design was originally sketched against - including both of a host-as-Participant's two independent connections. This is a broadening of what "every currently-bound connection" already meant in this WORK's own Outcome/Scope above, not a new mechanism: the same broadcast-not-filtered-by-recipient design already called for reaching everyone bound to the Session; it must simply iterate both registries once WORK-0020's second registry exists, rather than assuming one registry. This WORK depends on WORK-0020 landing (or being designed concurrently) for this reason, in addition to its existing WORK-0005 dependency. No other part of this WORK's scope changes.

## Completion Record

Not yet DONE. Status: **DRAFT**, revised 2026-09-20 to broaden scope from "fatal termination only" to "any Session termination while clients are connected," after confirming the root workflow's completion is a deterministic, structurally-forced "game over" signal (not a design ambiguity as first thought) and that the human wants it wired the same way as the fatal path - one generic message, same screen for everyone, personalization deferred; further revised the same day (Part H reconciliation) to require the broadcast to reach both ADMIN and PARTICIPANT connections. Blockers 1-4 need resolution before READY.
