# Project: Session Runtime V1

Status: ACTIVE
Created: 2026-09-20 (migrated from the earlier `docs/ai/workspaces/active/session-runtime-v1/` initiative workspace and its numbered-Slice `PLAN.md`, which together tracked this initiative from 2026-09-08)
Last updated: 2026-09-20

## Goal

Implement the complete Session backend required to execute the supported Game Definition semantics end-to-end, and provide the server-side surfaces necessary to build and run the Session frontend: a host creates a Session, players join a lobby, the host starts it, the authored Game Language program executes turn by turn through interactions/user intents/timers, players may disconnect/reconnect, a host may cancel, and the Session reaches a terminal state with no dangling runtime obligations - all under the accepted persistence, serialization, and failure-handling model.

## Explicitly Out Of Scope

- **Authentication / authorization / full Identity integration.** The backend uses the current simplified identity assumption (a trusted caller supplies an already-authenticated `UserUUID` directly; no credential verification exists anywhere yet) for the duration of this Project. Real Identity/Auth integration is a separate future initiative.
- Host "kick a participant" / "transfer host" - not currently known to be required for a V1 frontend; not owned by any WORK. See Material Decisions below.
- A personalized/configurable end-of-game results screen - deferred (`docs/product/IDEAS.md`), WORK-0007 delivers only a generic termination signal.
- Multi-instance/distributed live-transport routing (Redis, sticky routing) - excluded from all of V1 by GAME-ADR-0002, not a gap.
- Building an actual replay/rewatch feature - WORK-0006 only keeps replay theoretically possible (persisting Start's `Seed`); no replay feature is built by this Project.

## Current Work

- **WORK-0005** (Thin Live Coordinator / WebSocket) - IMPLEMENTING. Implementation is complete and verified against real Postgres; independent review has not yet happened. A newly discovered Blocker (host/Participant drift against already-accepted architecture - see WORK-0005's Blockers) must also be resolved before this can reach DONE.
- **WORK-0006** (Broaden Live Fan-Out: Effects + Presentations) - DRAFT. Both Blockers are resolved; awaiting human READY authorization.
- **WORK-0007** (Session Termination Live Notification) - DRAFT. Direction is human-confirmed; Blockers 1-4 (exact mechanism) remain unresolved.

## Work

| Order | Work | Status |
|------:|------|--------|
| 1 | WORK-0001 — Session Lobby Foundation | DONE |
| 2 | WORK-0003 — Start + First RuntimeTurn | DONE |
| 3 | WORK-0004 — Interaction Response Processing | DONE |
| — | WORK-0002 — Rename `game/game` → `game/management` (out-of-band structural rename, not sequenced) | DONE |
| 4 | WORK-0005 — Thin Live Coordinator / WebSocket | IMPLEMENTING |
| 5 | WORK-0008 — Live Lobby / Session Bootstrap | PLANNED |
| 6 | WORK-0009 — Client-Safe Game UI Manifest | PLANNED |
| 7 | WORK-0006 — Broaden Live Fan-Out: Effects + Presentations | DRAFT |
| 8 | WORK-0010 — User Intent Runtime Path | PLANNED |
| 9 | WORK-0011 — Manual Session Cancellation | PLANNED |
| 10 | WORK-0007 — Session Termination Live Notification | DRAFT |
| 11 | WORK-0012 — Timer Obligations | PLANNED |
| 12 | WORK-0013 — Keyed Timers | PLANNED |
| 13 | WORK-0018 — Game Language Disconnect/Reconnect Signal Support | PLANNED |
| 14 | WORK-0014 — Runtime Failure Diagnostics + Terminal Cleanup | PLANNED |
| 15 | WORK-0015 — Disconnect / Reconnect / Full Resync | PLANNED |
| 16 | WORK-0016 — Inactivity Expiration / Reaper | PLANNED |
| 17 | WORK-0017 — Archival | PLANNED |

18 WORK total: 4 DONE, 1 IMPLEMENTING, 2 DRAFT, 11 PLANNED.

## Ordering / Dependencies

- **1-3 (DONE)** gate everything: every later WORK needs a correctly modeled, serialized Session with a working RuntimeTurn executor.
- **4 (WORK-0005)** gates all live-transport-dependent WORK (5 onward) - there is no live connection to extend without it. Its own host/Participant Blocker should resolve before WORK-0008 is designed, since WORK-0008's roster/leave/bootstrap design depends on how a host ends up connected.
- **5 (WORK-0008)** and **6 (WORK-0009)** are independent of each other and can be designed/implemented in either order or in parallel once WORK-0005 is DONE; both are needed before a real lobby+manifest-driven frontend can render anything.
- **7 (WORK-0006)** only needs WORK-0005; it is already DRAFT and does not need WORK-0008/0009 first, though a frontend needs a manifest (WORK-0009) to know what a Presentation's `View`/`Model` actually renders as.
- **8 (WORK-0010)** needs WORK-0005 (live transport) and reuses WORK-0006's fan-out for its own Presentation/Effect consequences, so it is ordered after WORK-0006.
- **9 (WORK-0011)** needs WORK-0007's generalized terminal-live mechanism to exist (or be designed concurrently) to reuse for its own resulting termination.
- **10 (WORK-0007)** needs only WORK-0005; ordered here because WORK-0011/0012/0016 all reuse its mechanism once it exists - it should not still be a two-off special case by the time those land.
- **11 (WORK-0012)** needs WORK-0005 (physical scheduling extends its Coordinator) and the RuntimeTurn-from-external-cause pattern WORK-0004 already established.
- **12 (WORK-0013)** extends WORK-0012's mechanism; ordered immediately after it.
- **13 (WORK-0018)** is a Game Language-side prerequisite with no Session Runtime dependency of its own - it can be designed/implemented in parallel with 11-12, but must land before 15.
- **Reordering vs. the original plan**: Keyed Timers (12) is now ordered *before* Disconnect/Reconnect (15), reversing the original plan's order, because a per-user disconnect-grace timeout plausibly needs an independent keyed timer per user - see WORK-0013's and WORK-0015's own Context sections. Re-evaluate this dependency once both are designed in more detail; it may turn out WORK-0015 can proceed with WORK-0012's ordinary timer alone.
- **14 (WORK-0014)** retrofits diagnostics/cleanup across every RuntimeTurn-producing path that exists by the time it's designed (ideally after 8-13 have landed, so it does this once rather than repeatedly) but does not strictly require all of them - it can start once at least Start/AnswerInteraction/one external-cause path exist.
- **15 (WORK-0015)** has a hard prerequisite on WORK-0018 (13) and needs WORK-0005 (a live connection to disconnect/reconnect) and WORK-0006 (Presentation must be derivable for resync). Likely also needs WORK-0013 (12) for per-user grace timing - see above.
- **16 (WORK-0016)** is purely additive once RUNNING operations exist to renew/validate activity against; reuses WORK-0014's cleanup invariant and WORK-0007's notification mechanism. Lower risk than 8-15, so it can trail them without gating anything else.
- **17 (WORK-0017)** depends on nothing else being DONE first (archival of whatever history already exists is meaningful at any point) but is ordered last as the lowest-risk, purely additive item, consistent with the original plan's own reasoning.

## Capability Coverage

Required to run a Session frontend end-to-end against the supported Game Language semantics. No capability below is in the "required but unowned" state - each is DONE, owned by an active WORK, owned by a PLANNED WORK, or explicitly out of this Project's scope.

| Capability | Status | Owner |
|---|---|---|
| Lobby lifecycle (Create/Join/Leave, expiration) | DONE | WORK-0001 |
| Participant lifecycle (join/leave; kick/transfer-host not yet decided in scope) | DONE (join/leave); open scope question otherwise | WORK-0001; see Material Decisions |
| Host control (Start authorization) | DONE; extended by cancellation | WORK-0003; WORK-0011 |
| Start | DONE | WORK-0003 |
| Runtime turns (Step-drain/bound execution) | DONE | WORK-0003 |
| Interactions/questions | DONE | WORK-0004 |
| User intents | Not implemented | WORK-0010 (PLANNED) |
| Presentation state | Engine-only; not yet fanned out | WORK-0006 (DRAFT) |
| UI effects | Engine-only; not yet fanned out | WORK-0006 (DRAFT) |
| Client-safe UI/game definition | Does not exist | WORK-0009 (PLANNED) |
| Per-viewer output | Partial (Question-only today) | WORK-0004 (question) + WORK-0006 (presentation, DRAFT) |
| Live transport | Implemented, narrow; one open Blocker | WORK-0005 (IMPLEMENTING) |
| Live lobby / bootstrap / roster / leave | Not implemented | WORK-0008 (PLANNED) |
| Terminal notifications | Not implemented; two causes in design | WORK-0007 (DRAFT); reused by WORK-0011/0012/0016 |
| Manual cancellation | Not implemented | WORK-0011 (PLANNED) |
| Timers | Not implemented in Session Runtime (engine-only) | WORK-0012 (PLANNED) |
| Keyed timers | Does not exist in Game Language | WORK-0013 (PLANNED) |
| Disconnect/reconnect | Not implemented; signal schema incomplete | WORK-0015 (PLANNED), depends on WORK-0018 |
| Full resync | Not implemented | WORK-0015 (PLANNED) |
| Inactivity expiration | Not implemented | WORK-0016 (PLANNED) |
| Runtime failure handling | Partial (two narrow fatal paths exist) | WORK-0003/WORK-0004 (existing paths); WORK-0014 (PLANNED, generalizes) |
| Archival | Not implemented | WORK-0017 (PLANNED) |
| Authentication / Identity | Not implemented, intentionally | Explicitly out of scope for this Project |

## Material Decisions Needing Human Input

These were found during this Project's completeness audit and reconciliation (2026-09-20). None have been silently decided; each is recorded here rather than assumed.

1. **Host "kick a participant" / "transfer host"** - not clearly known to be required for a Session Runtime V1 frontend, so no WORK was created for it. If it is required, it needs its own PLANNED WORK; if not, it should be recorded as explicitly out of scope rather than left ambiguous.
2. **WORK-0007's `LOBBY_EXPIRED`-while-connected gap** - becomes concretely reachable once WORK-0008 (live lobby) lets a client hold a connection before `Start`. Worth resolving when WORK-0007 and WORK-0008 are jointly refined; not resolved by this migration.
3. **WORK-0018's domain placement** - it is Game Language/compiler work (not Session Runtime application logic), tracked under this Project because it directly gates WORK-0015, the same treatment the original plan already gave Keyed Timers (WORK-0013). Confirm this is the right call rather than spinning up a separate Game Language initiative for it.
4. **The recommended execution order above** is derived from actual code dependencies found during this audit, not merely inherited from the original plan (most notably: Keyed Timers now ordered before Disconnect/Reconnect, reversing the original order). It is a recommendation, not a commitment - nothing has been implemented against it yet.

**Not a decision needing input, recorded here only for visibility**: WORK-0005's host/Participant Blocker (a host must not be required to become a Participant merely to connect and issue `Start`) is a required compliance fix against already-accepted architecture (`game/README.md`), not a new product/design question - see WORK-0005's Blocker 8.

## Completion Criteria

Session Runtime V1 is complete when:

1. Every WORK in the table above is DONE, or explicitly moved to this Project's Out Of Scope with human confirmation.
2. Every row in the Capability Coverage matrix reads either DONE or "explicitly out of scope" - none reads "not implemented"/"partial" against a capability this Project's Goal requires.
3. Authentication/Identity integration remains the one deliberately excluded exception, tracked as a separate future initiative rather than blocking this Project's closure.

When satisfied, this Project moves from `docs/projects/active/` to `docs/projects/completed/`.
