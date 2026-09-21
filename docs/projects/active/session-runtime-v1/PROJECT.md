# Project: Session Runtime V1

Status: ACTIVE
Created: 2026-09-20 (migrated from the earlier `docs/ai/workspaces/active/session-runtime-v1/` initiative workspace and its numbered-Slice `PLAN.md`, which together tracked this initiative from 2026-09-08)
Last updated: 2026-09-20 (replay-first persistence and role-aware live connections reconciliation pass, same day - see "2026-09-20 Reconciliation Pass" below)

## Goal

Implement the complete Session backend required to execute the supported Game Definition semantics end-to-end, and provide the server-side surfaces necessary to build and run the Session frontend: a host creates a Session, players join a lobby, the host starts it, the authored Game Language program executes turn by turn through interactions/user intents/timers, players may disconnect/reconnect, a host may cancel, and the Session reaches a terminal state with no dangling runtime obligations - all under the accepted persistence, serialization, and failure-handling model.

## Explicitly Out Of Scope

- **Authentication / authorization / full Identity integration.** The backend uses the current simplified identity assumption (a trusted caller supplies an already-authenticated `UserUUID` directly; no credential verification exists anywhere yet) for the duration of this Project. Real Identity/Auth integration is a separate future initiative.
- Host "kick a participant" / "transfer host" - not currently known to be required for a V1 frontend; not owned by any WORK. See Material Decisions below.
- A personalized/configurable end-of-game results screen - deferred (`docs/product/IDEAS.md`), WORK-0007 delivers only a generic termination signal.
- Multi-instance/distributed live-transport routing (Redis, sticky routing) - excluded from all of V1 by GAME-ADR-0002, not a gap.
- Building an actual replay/rewatch feature - WORK-0019 only keeps replay theoretically possible (the durable replay-input model, per GAME-ADR-0024); no replay feature is built by this Project.

## Current Work

- **WORK-0005** (Thin Live Coordinator / WebSocket) - IMPLEMENTING (returned from DRAFT 2026-09-20; see WORK-0005's "Status Correction (Part E Reconciliation, 2026-09-20)" section). The Create/Join/Start/AnswerInteraction/Deliver path implemented so far remains intact and verified against real Postgres. Blocker 8 (host/Participant drift) is no longer this WORK's own pending redesign - ownership transferred to WORK-0020, which owns implementing the actual fix. This WORK's own remaining gate to DONE is independent review of its already-implemented scope (never yet performed), which may proceed independently of WORK-0020's timeline.
- **WORK-0006** (Broaden Live Fan-Out: Effects + Presentations) - DRAFT. Both Blockers are resolved; Start-`Seed` persistence removed from scope (moved to WORK-0019); awaiting human READY authorization.
- **WORK-0007** (Session Termination Live Notification) - DRAFT. Direction is human-confirmed; Blockers 1-4 (exact mechanism) remain unresolved; broadcast scope now must reach both ADMIN and PARTICIPANT connections (depends on WORK-0020).
- **WORK-0019** (Replay-First Session Runtime Persistence Migration) - DRAFT (new). Implements GAME-ADR-0024; Blockers 1-5 (durable Seed/RootParameters shape, `session_runtime_steps` disposition, per-cause durable representation, ephemeral-cache mechanism, migration sequencing) need human input before READY.
- **WORK-0020** (Role-Aware Live Connections / Host Administration Channel) - DRAFT (new). Implements GAME-ADR-0025 and formally supersedes WORK-0005's own Blocker 8 proposal; Blockers 1-3 (confirm/broaden the inherited mechanism, route shape, registry extensibility) need human approval before READY.

## 2026-09-20 Reconciliation Pass

A dedicated reconciliation session accepted several human architecture decisions that materially change this Project's persistence and live-connection direction without reopening any completed WORK's historical accuracy:

- **Replay-first Session Runtime persistence** (no durable per-Turn Snapshot; durable replay-input causes plus deterministic replay reconstruct current/historical Runtime state) - `game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md`, implemented by new **WORK-0019**.
- **Role-aware live connections** (ADMIN and PARTICIPANT as structurally independent connection roles; a host may hold both) - `game/docs/decisions/GAME-ADR-0025-role-aware-live-connections.md`, implemented by new **WORK-0020**, which formally supersedes WORK-0005's own "Host Connection Design Revision" proposal (Blocker 8).
- **Host Participant spectator view** during RUNNING, read-only visual mirroring only - new **WORK-0021** (PLANNED).
- **Archival moves from GCS to same-database PostgreSQL JSONB compaction** - WORK-0017 rewritten accordingly.
- WORK-0006, WORK-0007, WORK-0008, WORK-0010, WORK-0011, WORK-0012, WORK-0013, WORK-0014, WORK-0015, WORK-0016 each received a targeted scope reconciliation against the two decisions above - see each WORK's own "Scope Addition/Revision/Clarification (... Reconciliation, 2026-09-20)" section for exactly what changed.
- WORK-0001, WORK-0003, WORK-0004 (DONE) and WORK-0018 (PLANNED, untouched - Game-Language-only signal-schema work, not implicated by either decision) were deliberately left unchanged.

## Work

| Order | Work | Status |
|------:|------|--------|
| 1 | WORK-0001 — Session Lobby Foundation | DONE |
| 2 | WORK-0003 — Start + First RuntimeTurn | DONE |
| 3 | WORK-0004 — Interaction Response Processing | DONE |
| — | WORK-0002 — Rename `game/game` → `game/management` (out-of-band structural rename, not sequenced) | DONE |
| 4 | WORK-0005 — Thin Live Coordinator / WebSocket | IMPLEMENTING |
| 5 | WORK-0006 — Broaden Live Fan-Out: Effects + Presentations | DRAFT |
| 6 | WORK-0019 — Replay-First Session Runtime Persistence Migration | DRAFT |
| 7 | WORK-0020 — Role-Aware Live Connections / Host Administration Channel | DRAFT |
| 8 | WORK-0008 — Live Lobby / Session Bootstrap | PLANNED |
| 9 | WORK-0009 — Client-Safe Game UI Manifest | PLANNED |
| 10 | WORK-0021 — Host Participant Spectator View | PLANNED |
| 11 | WORK-0010 — User Intent Runtime Path | PLANNED |
| 12 | WORK-0007 — Session Termination Live Notification | DRAFT |
| 13 | WORK-0011 — Manual Session Cancellation | PLANNED |
| 14 | WORK-0012 — Timer Obligations | PLANNED |
| 15 | WORK-0013 — Keyed Timers | PLANNED |
| 16 | WORK-0018 — Game Language Disconnect/Reconnect Signal Support | PLANNED |
| 17 | WORK-0014 — Runtime Failure Diagnostics + Terminal Cleanup | PLANNED |
| 18 | WORK-0015 — Disconnect / Reconnect / Full Resync | PLANNED |
| 19 | WORK-0016 — Inactivity Expiration / Reaper | PLANNED |
| 20 | WORK-0017 — Archival | PLANNED |

21 WORK total: 4 DONE, 1 IMPLEMENTING, 4 DRAFT, 12 PLANNED.

## Ordering / Dependencies

- **1-3 (DONE)** gate everything: every later WORK needs a correctly modeled, serialized Session with a working RuntimeTurn executor.
- **4 (WORK-0005)** gates all live-transport-dependent WORK (5 onward) - there is no live connection to extend without it. Its Blocker 8 no longer gates anything here (transferred to WORK-0020, item 7 below); WORK-0005's own remaining gate is independent review of already-implemented scope, which can proceed in parallel with everything below.
- **5 (WORK-0006)** needs only WORK-0005 - it is already DRAFT, no longer touches persistence at all (Start-`Seed` moved to WORK-0019), and does not need WORK-0019/WORK-0020 first. Moved earlier than the original plan's placement (which had it after the lobby/manifest WORK) because auditing its actual dependencies during this reconciliation found none beyond WORK-0005.
- **6 (WORK-0019)** and **7 (WORK-0020)** both need only WORK-0005 and are independent of each other - they may be designed/implemented in parallel. WORK-0019 replaces durable per-Turn Snapshot persistence with replay-input persistence; WORK-0020 replaces the single-connection-kind live transport with role-aware ADMIN/PARTICIPANT connections. Neither depends on the other's outcome, though both should land before most of what follows, since later WORK builds on one or both.
- **8 (WORK-0008)** needs WORK-0020 (its role-specific lobby projections require the ADMIN/PARTICIPANT connection model to exist first).
- **9 (WORK-0009)** is independent of WORK-0019/0020/0008 and can be designed/implemented at any point once WORK-0005 exists; kept near WORK-0008 only because both are needed before a real lobby+manifest-driven frontend can render anything, not because either depends on the other.
- **10 (WORK-0021)** needs WORK-0020 (the ADMIN connection its spectator selection attaches to) and WORK-0006 (the Presentation/Effect delivery mechanism it mirrors read-only). Left PLANNED rather than DRAFT specifically because its initial-Presentation-reconstruction mechanism should ideally reuse whatever generic resync/projection capability WORK-0015 (18) eventually builds, rather than duplicating that logic now - its ordering here reflects when its hard dependencies are satisfied, not when its DRAFT design should necessarily start; a human may reasonably choose to defer its actual design work until closer to WORK-0015.
- **11 (WORK-0010)** needs WORK-0020 (PARTICIPANT connection, explicit command-role scoping), WORK-0019 (durable replay-input representation for a submitted intent), and reuses WORK-0006's fan-out for its own Presentation/Effect consequences.
- **12 (WORK-0007)** needs WORK-0005 and now also WORK-0020 (its broadcast must reach both connection registries) - ordered here because WORK-0011/0012(timers)/0016 all reuse its mechanism once it exists.
- **13 (WORK-0011)** needs WORK-0020 (ADMIN-only command surface), WORK-0007's generalized terminal-live mechanism, and WORK-0019 (durable cancellation-cause representation).
- **14 (WORK-0012)** needs WORK-0005 (physical scheduling extends its Coordinator), the RuntimeTurn-from-external-cause pattern WORK-0004 already established, and WORK-0019 (its TimerExpired-occurrence replay-input framing, already confirmed satisfied by the existing `session_timer_obligations` shape).
- **15 (WORK-0013)** extends WORK-0012's mechanism; ordered immediately after it.
- **16 (WORK-0018)** is a Game Language-side prerequisite with no Session Runtime dependency of its own - it can be designed/implemented in parallel with 14-15, but must land before 18 (WORK-0015).
- **Reordering vs. the original plan**: Keyed Timers (15) is still ordered *before* Disconnect/Reconnect (18), reversing the original plan's order, because a per-user disconnect-grace timeout plausibly needs an independent keyed timer per user - see WORK-0013's and WORK-0015's own Context sections. Re-evaluate this dependency once both are designed in more detail; it may turn out WORK-0015 can proceed with WORK-0012's ordinary timer alone.
- **17 (WORK-0014)** retrofits diagnostics/cleanup across every RuntimeTurn-producing path that exists by the time it's designed (ideally after 8-16 have landed, so it does this once rather than repeatedly) but does not strictly require all of them - it can start once at least Start/AnswerInteraction/one external-cause path exist. Also needs WORK-0019 (no durable Snapshot exists for its diagnostics to reference).
- **18 (WORK-0015)** has a hard prerequisite on WORK-0018 (16) and needs WORK-0019 (resync reconstructs via replay, not a stored Snapshot) and WORK-0020 (reconnect is designed per connection role from the outset, not retrofitted). Likely also needs WORK-0013 (15) for per-user grace timing - see above.
- **19 (WORK-0016)** is purely additive once RUNNING operations exist to renew/validate activity against; reuses WORK-0014's cleanup invariant and WORK-0007's notification mechanism. Lower risk than 8-18, so it can trail them without gating anything else.
- **20 (WORK-0017)** needs WORK-0019's replay-input model to be stable enough to define a self-sufficient archive payload (see WORK-0017's own Blockers) but not necessarily fully DONE first; ordered last as the lowest-risk, purely additive item, consistent with the original plan's own reasoning. No longer depends on any GCS/object-storage integration (superseded).

## Capability Coverage

Required to run a Session frontend end-to-end against the supported Game Language semantics. No capability below is in the "required but unowned" state - each is DONE, owned by an active WORK, owned by a PLANNED WORK, or explicitly out of this Project's scope. Re-audited 2026-09-20 (reconciliation pass) against the replay-first persistence and role-aware live-connection decisions.

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
| Live transport (Create/Join/Start/AnswerInteraction/Deliver) | Implemented, narrow; participant path complete | WORK-0005 (IMPLEMENTING, review pending) |
| **Replay-input persistence** | Design accepted (GAME-ADR-0024); not implemented | WORK-0019 (DRAFT) |
| **Deterministic runtime reconstruction (process-loss recovery without a stored Snapshot)** | Design accepted; not implemented | WORK-0019 (DRAFT) |
| **Admin live connection** | Design accepted (GAME-ADR-0025); not implemented (WORK-0005's host connection still goes through Join) | WORK-0020 (DRAFT) |
| **Participant live connection (role-formalized)** | Implemented as a single connection kind (WORK-0005); role separation from Admin not yet implemented | WORK-0005 (base) + WORK-0020 (DRAFT, role formalization) |
| **Role-scoped command authorization** | Design accepted; not implemented (Manager's own per-command checks already exist and remain the actual enforcement) | WORK-0020 (DRAFT) |
| Live lobby / bootstrap / roster / leave, role-specific projection | Not implemented | WORK-0008 (PLANNED), depends on WORK-0020 |
| **Host-as-Participant dual connection** | Design accepted; not implemented | WORK-0020 (DRAFT) |
| **Host spectator view** | Design accepted (GAME-ADR-0025); not implemented | WORK-0021 (PLANNED) |
| **Participant-count lobby fan-out** | Not implemented | WORK-0008 (PLANNED) |
| Terminal notifications | Not implemented; must reach both connection roles | WORK-0007 (DRAFT), depends on WORK-0020; reused by WORK-0011/0012/0016 |
| Manual cancellation | Not implemented | WORK-0011 (PLANNED) |
| Timers | Not implemented in Session Runtime (engine-only) | WORK-0012 (PLANNED) |
| Keyed timers | Does not exist in Game Language | WORK-0013 (PLANNED) |
| Disconnect/reconnect | Not implemented; signal schema incomplete; must be per connection role | WORK-0015 (PLANNED), depends on WORK-0018/WORK-0019/WORK-0020 |
| Full resync | Not implemented; must reconstruct via replay, not a stored Snapshot | WORK-0015 (PLANNED), depends on WORK-0019 |
| Inactivity expiration | Not implemented | WORK-0016 (PLANNED) |
| Runtime failure handling | Partial (two narrow fatal paths exist); must not assume a stored Snapshot | WORK-0003/WORK-0004 (existing paths); WORK-0014 (PLANNED, generalizes), depends on WORK-0019 |
| **Session archival/compaction** | Design accepted (GAME-ADR-0024, PostgreSQL JSONB, supersedes the original GCS direction) | WORK-0017 (PLANNED, rewritten), depends on WORK-0019 |
| Authentication / Identity | Not implemented, intentionally | Explicitly out of scope for this Project |

No required outcome from this reconciliation pass emerged ownerless - every bolded new row above already has an owning WORK created or revised by this pass.

## Material Decisions Needing Human Input

These were found during this Project's completeness audit and reconciliation (2026-09-20). None have been silently decided; each is recorded here rather than assumed.

1. **Host "kick a participant" / "transfer host"** - not clearly known to be required for a Session Runtime V1 frontend, so no WORK was created for it. If it is required, it needs its own PLANNED WORK; if not, it should be recorded as explicitly out of scope rather than left ambiguous.
2. **WORK-0007's `LOBBY_EXPIRED`-while-connected gap** - becomes concretely reachable once WORK-0008 (live lobby) lets a client hold a connection before `Start`. Worth resolving when WORK-0007 and WORK-0008 are jointly refined; not resolved by this migration.
3. **WORK-0018's domain placement** - it is Game Language/compiler work (not Session Runtime application logic), tracked under this Project because it directly gates WORK-0015, the same treatment the original plan already gave Keyed Timers (WORK-0013). Confirm this is the right call rather than spinning up a separate Game Language initiative for it.
4. **The recommended execution order above** is derived from actual code dependencies found during this audit, not merely inherited from the original plan (most notably: Keyed Timers now ordered before Disconnect/Reconnect, reversing the original order). It is a recommendation, not a commitment - nothing has been implemented against it yet.

5. **WORK-0005's original host/Participant Blocker (Blocker 8) has been transferred to WORK-0020, not resolved in place.** (2026-09-20 update, superseding this item's prior wording) A broader reconciliation session found the host-connection gap sits inside a larger required capability (role-aware ADMIN/PARTICIPANT live connections, independently needed by WORK-0007/WORK-0008/WORK-0011/WORK-0021) and created `game/docs/decisions/GAME-ADR-0025-role-aware-live-connections.md` (ACCEPTED - the architecture direction itself) and `docs/projects/active/session-runtime-v1/works/WORK-0020-role-aware-live-connections.md` (DRAFT - the concrete implementation, adopting WORK-0005's own prior proposal as its starting point) to own it. What still needs explicit human input is WORK-0020's own Blockers 1-3 (confirming/broadening the inherited mechanism now that its scope is wider than WORK-0005's original narrow fix, the exact route shape, and whether the registry should anticipate WORK-0021's future spectator-subscription needs) before WORK-0020 can move DRAFT -> READY. WORK-0005 itself is no longer blocked by this and returned to IMPLEMENTING (see WORK-0005's own "Status Correction (Part E Reconciliation, 2026-09-20)"), pending only its own independent review.
6. **WORK-0019's Blockers 1-5 (replay-first persistence implementation shape)** - durable Seed/RootParameters column/table shape, `session_runtime_steps`'s final disposition (a genuine tradeoff between debuggability and the same unbounded-growth concern GAME-ADR-0024 otherwise resolves - worth explicit human input rather than a unilateral implementation choice), the durable representation for each future RuntimeTurn cause without an existing normalized home, whether an ephemeral current-state cache is actually needed yet (a NOW/SOON/LATER/NOT NEEDED call), and migration sequencing. See WORK-0019's own Blockers section.
7. **WORK-0020's Blockers 1-3 (role-aware connection implementation shape)** - see item 5 above and WORK-0020's own Blockers section.
8. **GAME-ADR-0024's still-open replay-compatibility question**: whether a future compiler/engine-build change (beyond the already-existing pinned `program.Metadata.LanguageVersion`) ever needs its own tracking mechanism. Not urgent (no such change exists today), but flagged rather than silently assumed away - see GAME-ADR-0024's "Replay compatibility" section.
9. **WORK-0021 (Host Participant Spectator View) was deliberately left PLANNED, not DRAFT**, specifically so its initial-Presentation-reconstruction mechanism can reuse WORK-0015's eventual generic resync/projection capability rather than duplicating that logic before WORK-0015 exists. A human may want to confirm this sequencing judgment (defer WORK-0021's actual design until closer to WORK-0015) rather than accelerate it, given WORK-0021's other two dependencies (WORK-0020, WORK-0006) land much earlier.

## Completion Criteria

Session Runtime V1 is complete when:

1. Every WORK in the table above is DONE, or explicitly moved to this Project's Out Of Scope with human confirmation.
2. Every row in the Capability Coverage matrix reads either DONE or "explicitly out of scope" - none reads "not implemented"/"partial" against a capability this Project's Goal requires.
3. Authentication/Identity integration remains the one deliberately excluded exception, tracked as a separate future initiative rather than blocking this Project's closure.

When satisfied, this Project moves from `docs/projects/active/` to `docs/projects/completed/`.
