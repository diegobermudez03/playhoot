# WORK-0020: Role-Aware Live Connections / Host Administration Channel — Rebuilds the `play` Coordinator

Status: DRAFT
Created: 2026-09-20
Last status change: 2026-09-23 (Scope Correction: this WORK was drafted assuming `play`/`play/sessionruntime` still existed as an implementation to extend. They were deleted in full the next day, 2026-09-21, by WORK-0005's Blocker 11, and this WORK was never reconciled against that deletion. This WORK is now explicitly the one that (re)introduces the `play` layer from scratch - see "Scope Correction (Play Reintroduction, 2026-09-23)" below)

Related decisions:
- GAME-ADR-0025 (Role-Aware Live Connections - the accepted direction this WORK implements)
- GAME-ADR-0002 (Live Session Coordinator responsibility boundary - refined, not superseded, by GAME-ADR-0025)
- GAME-ADR-0005 (public/internal identity boundary - refined by GAME-ADR-0025's ADMIN validation)

Canonical context:
- `game/docs/decisions/GAME-ADR-0025-role-aware-live-connections.md` (the accepted model this WORK implements)
- `game/README.md` (Session Runtime Actor and Lifecycle Model - "Host and Participant are independent concepts", the invariant this WORK's transport finally complies with)
- `docs/projects/active/session-runtime-v1/works/WORK-0005-thin-live-coordinator.md` ("Host Connection Design Revision" section - the original proposal this WORK supersedes and formally owns going forward; and its own Blocker 11 (2026-09-21, HUMAN-DIRECTED), which deleted `play`/`play/sessionruntime` in full - `Create`/`Join`/`AnswerInteraction`/`Deliver` and the participant registry no longer exist as code. WORK-0005's own Blocker 10 removed `Start` from the live wire entirely rather than leave it on the Join-coupled connection this WORK's own Blocker 8 already knew was wrong - so this WORK is not replacing a working `Start`-over-wire path, it is adding the first one, on the `ADMIN` connection it builds.)
- `docs/projects/active/session-runtime-v1/PROJECT.md` (Restructuring, 2026-09-21 - the inside-out Phase 1/Phase 2 sequencing that makes this WORK the lead item of Phase 2, deliberately sequenced after Phase 1's domain-completion WORKs)
- `api/server.go`, `api/session/` (the only live-transport code that currently exists - WORK-0005's route/WebSocket-upgrade skeleton, calling no domain package; this WORK is the first thing that wires it to `sessionlifecycle.Manager`)

## Outcome

The live transport distinguishes exactly two connection roles, `ADMIN` and `PARTICIPANT`, bound to structurally separate Coordinator registry entries. A Session host obtains an `ADMIN` connection without `Join` and without consuming a Participant slot, validated against `sessions.host_actor_id`. A host may independently also complete the ordinary `PARTICIPANT` `Join`-then-connect handshake, holding both connections at once with independent lifecycles. Host-only commands (`Start` today; future `Cancel`, spectator-target selection) are reachable only from an `ADMIN` connection; player-authored commands (`AnswerInteraction` today; future `UserIntent`, `Leave`) are reachable only from a `PARTICIPANT` connection. `sessionlifecycle.Manager`'s own authoritative per-command checks remain the sole actual authorization; the connection-role boundary is a routing/defense-in-depth layer on top of them, not a replacement.

This WORK formally supersedes and implements WORK-0005's own "Host Connection Design Revision (2026-09-20, DRAFT PROPOSAL)" - that section's design intent (a distinct host-connect handshake, a `ResolveHost` read capability, a structurally separate host registry) is the starting point for this WORK's own Approved Design, not a competing proposal; WORK-0005 itself no longer owns implementing it (see WORK-0005's revised Blocker 8 and Status Correction). **This WORK is also, as of the 2026-09-23 Scope Correction below, the WORK that rebuilds the entire `play` Coordinator from scratch** - there is no existing participant registry, dispatch, or `Deliver` mechanism left to add a second registry to; both registries, both command-dispatch paths, and the per-recipient delivery mechanism are new code this WORK writes.

## Context

WORK-0005's Blocker 8 discovered that the originally-implemented live transport forced a host through `Join` merely to obtain a connection to send `Start`, contradicting `game/README.md`'s already-accepted Host/Participant independence invariant. WORK-0005's own DRAFT proposal sketched a fix (a second registry, a `ResolveHost` capability), but the reconciliation that produced this WORK broadened the requirement beyond that one gap: Session Runtime V1 also needs role-specific lobby projections (WORK-0008), a host spectator view during RUNNING (WORK-0021), terminal notification reaching every connection regardless of role (WORK-0007), and future host-only commands (`Cancel`, WORK-0011) to be structurally distinguishable from player commands (`UserIntent`, WORK-0010) rather than accumulating ad hoc per-handler checks. This WORK is the single place that ownership now lives, so the connection-role model is designed once, coherently, rather than iteratively patched by each downstream WORK that happens to need a piece of it.

The day after this WORK was drafted, a separate reconciliation (WORK-0005's Blocker 11, 2026-09-21) deleted `play`/`play/sessionruntime` entirely and restructured this Project into inside-out phases specifically because the original `play` was built against an incomplete Session Runtime domain layer (see `PROJECT.md`'s Restructuring section). This WORK was never updated to reflect that deletion until the 2026-09-23 Scope Correction below - it is the intended target of that restructuring's "rebuild the stateful live Coordinator once, correctly" step, not an incremental extension of the deleted code.

## Scope

### In Scope

- **A new `play` Coordinator package, built from scratch** (the prior `play`/`play/sessionruntime` were deleted in full, WORK-0005 Blocker 11) - there is no existing implementation to extend. This includes:
  - A `PARTICIPANT` registry and the ordinary `Join`-then-connect handshake, calling `sessionlifecycle.Manager.Join`.
  - A second, structurally separate `ADMIN` registry, reached via a host-connect handshake (no `Join`, no `session_participants` row) that validates the caller against `sessions.host_actor_id` before upgrading - reusing WORK-0005's own already-proposed `ResolveHost`-shaped read capability and mechanism (mirroring `Start`'s existing host check, restated read-only rather than duplicated as a new authorization rule).
  - Command dispatch wiring the existing `sessionlifecycle.Manager.Start`/`AnswerInteraction` (WORK-0001/0003/0004, unaffected by their absence from the live wire since WORK-0005 Blocker 11) to real transport commands for the first time since the deletion.
  - A per-recipient `Deliver` mechanism translating a committed Turn's `OpenQuestionOutput`/`CloseQuestionOutput` (and, if WORK-0006's domain half has landed by the time this is implemented, `EmitEffectOutput`/`ActivatePresentationOutput`/`UpdatePresentationOutput`/`RemovePresentationOutput`) into `play.Event`s addressed by `UserUUID` - the same non-leakage shape the original, now-deleted `play` established, rebuilt rather than assumed to still exist.
- Command-surface separation at the transport layer: an `ADMIN` connection's read pump dispatches only host-only commands (`START` today); a `PARTICIPANT` connection's read pump dispatches only player commands (`ANSWER_INTERACTION` today), each rejecting the other's commands transport-side with an `ERROR` reply, never forwarding them to Coordinator/Manager.
- Support for the same `UserUUID` holding one `ADMIN` and one `PARTICIPANT` connection simultaneously for the same Session, each with an independent bind/unbind lifecycle.
- Whatever minimal wiring is needed so WORK-0007's terminal broadcast (once WORK-0007 itself is implemented) can reach both registries (this WORK does not itself implement WORK-0007's broadcast, only ensures the registry shape it will broadcast against already includes both).

### Out of Scope

- Role-specific lobby projection payloads (roster, participant count, join/leave events) - WORK-0008's own scope, which depends on this WORK's connection model existing first.
- The host spectator view itself (subscription/selection API, Presentation mirroring) - WORK-0021's own scope, which depends on this WORK's `ADMIN` connection existing first.
- Manual cancellation's own command/business logic - WORK-0011's scope; this WORK only ensures `Cancel` has an `ADMIN`-only place to be dispatched from once WORK-0011 adds it.
- Any change to `sessionlifecycle.Manager`'s existing business logic, persistence model, or method signatures for `Start`/`AnswerInteraction` - unchanged, exactly as WORK-0005 already established.
- Reconnect/resync mechanics for either connection role beyond what already exists (bind-replaces-prior-connection semantics) - WORK-0015's own scope, informed by this WORK's role separation (a physical `ADMIN` connection loss never produces `UserDisconnected`) but not implemented here.
- Real credential verification/Identity implementation - continues WORK-0005's already-established trusted-`UserUUID` stance.

## Approved Design

GAME-ADR-0025 already settles the direction (two structurally separate registries, layered authorization, host-as-Participant via two independent connections). WORK-0005's own "Host Connection Design Revision" section already sketches a concrete mechanism (`GET /ws/host` or equivalent, `ResolveHost`, `Coordinator.BindHost`, a `hosts map[SessionUUID]Conn` registry) consistent with that direction - this WORK adopts that mechanism as its starting Approved Design rather than re-deriving it, subject to the Blockers below (carried forward from WORK-0005, now owned here). For the `PARTICIPANT` registry, command dispatch, and `Deliver` - all deleted by WORK-0005 Blocker 11 - the pre-deletion `play`/`play/sessionruntime` design (per-Session goroutine or mutex-guarded map, `Event`/`EventKind` per-recipient translation, dependency-inversion boundary with no shared `*gorm.DB`) is preserved as historical record in WORK-0005's own file and is this WORK's starting reference for rebuilding them, not a binding constraint - implementation-level choices there remain this WORK's own Implementation Freedom.

## Constraints and Invariants

- A bound `ADMIN` connection and a bound `PARTICIPANT` connection are structurally distinct registry entries, even for the same `UserUUID` on the same Session - `Deliver`'s existing participant-addressed fan-out path never reaches an `ADMIN`-registry entry, and no host-targeted fan-out this WORK introduces ever reaches the participant registry.
- No connection-role claim made by a client at connect time is trusted as command authorization by itself - `sessionlifecycle.Manager`'s own per-command checks remain the sole authoritative enforcement (GAME-ADR-0025).
- No authoritative connection-presence field is added to Session Runtime persistence (GAME-ADR-0003, unchanged) - both registries remain ephemeral Coordinator state.
- A physical `ADMIN` connection's loss/close must never itself produce a Game Language `UserDisconnected` signal or otherwise touch `session_actors.semantic_presence` - that remains a `PARTICIPANT`/runtime-membership concept exclusively.
- No shared database transaction/handle between `play` and `game` - unchanged from WORK-0005's existing dependency-inversion boundary.

## Acceptance Criteria

- A host can obtain a bound `ADMIN` connection (`session_uuid` + `user_uuid`, no `join_code`) and issue `Start` without becoming a Participant - provable by connecting as host, sending `START`, and confirming no `session_participants` row was created for that `UserUUID`.
- A `UserUUID` that is not the Session's host is declined (`NOT_HOST`) when attempting the host-connect handshake, with no upgrade.
- An `ADMIN` connection that sends `ANSWER_INTERACTION` receives an `ERROR` reply and causes no `Coordinator`/`SessionRuntime`/`Manager` call; a `PARTICIPANT` connection that sends `START` receives an equivalent rejection.
- The same `UserUUID` can hold one `ADMIN` connection and one independent `PARTICIPANT` connection (via ordinary Join) for the same Session at once; closing either does not affect the other.
- The Create -> Join -> Start -> receive-interaction -> answer -> receive-resolution end-to-end proof WORK-0005 originally established (deleted along with `play` by its Blocker 11) is rebuilt and passes, with the host connecting via the host handshake (never joining) and a separately-joined Participant receiving/answering the opened interaction.
- `go build ./...`, `go vet ./...`, `go test ./... -count=1` pass, with no new failure beyond the already-recorded out-of-scope `getgame` JSONB-comparison defect.

## Blockers

Carried forward from WORK-0005's own Blocker 8 discovery, now owned by this WORK. Status: **PROPOSED, AWAITING HUMAN APPROVAL** - the mechanism below is a concrete starting proposal (inherited from WORK-0005's own DRAFT design), not yet re-approved under this WORK's own broader scope.

1. Confirm the inherited mechanism (host-connect handshake + `ResolveHost` + separate `hosts` registry + `BindHost`) as this WORK's own Approved Design, or redirect it, now that its scope is broader than WORK-0005's original narrow fix (it must also anticipate WORK-0008's roster projection and WORK-0021's spectator subscription attaching to the same `ADMIN` connection later, without a further registry redesign).
2. Exact route/handshake shape (`GET /ws/host`, a `role` query parameter on the existing `GET /ws`, or another shape) - Implementation Freedom per WORK-0005's own prior framing, unless broadening this WORK's scope changes that judgment.
3. Whether the `ADMIN` registry should anticipate carrying spectator-subscription state (WORK-0021) as an additional per-connection field now, or whether WORK-0021 should extend the registry entry type later without this WORK needing to predict its exact shape - recommend the latter (do not design WORK-0021's mechanism from inside this WORK), listed only so WORK-0021 does not assume a shape this WORK never actually committed to.

## Documentation Impact

### Accepted / Canonical Knowledge

- Already updated by this reconciliation pass: `game/docs/decisions/GAME-ADR-0025-role-aware-live-connections.md` (new), `game/docs/decisions/INDEX.md`.

### Current-State Documentation After Implementation

- `play/README.md` - new file (the original was deleted with the package, WORK-0005 Blocker 11) - describe the two-registry connection model, the `ADMIN`/`PARTICIPANT` command-surface separation, and the `Deliver` mechanism from scratch.
- `api/README.md` - describe the host-connect handshake alongside the participant Join-then-connect handshake, replacing its current "skeleton, no domain coupling" description (WORK-0005 Blocker 11).
- `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` - describe role-aware connections, and the live transport generally, as now-implemented.

### Intentionally Unchanged

- WORK-0001 through WORK-0004 (completed historical WORKs) - not reopened.
- `sessionlifecycle.Manager` - no change to its business logic, persistence model, or method signatures.

## Scope Correction (Play Reintroduction, 2026-09-23)

This WORK was drafted 2026-09-20 against a `play` package that still existed at the time, describing itself as adding a second registry "alongside the existing participant registry" and treating WORK-0005's Create/Join/AnswerInteraction/Deliver path as an "unchanged foundation this WORK builds on, not replaced." The very next day (2026-09-21), a separate reconciliation - WORK-0005's own Blocker 11 - deleted `play`/`play/sessionruntime` in full and restructured this Project into inside-out phases specifically so the Coordinator would be "rebuilt once, correctly" on top of a complete Session Runtime domain layer, rather than patched incrementally (see `PROJECT.md`'s Restructuring section). `PROJECT.md` already reflects this (WORK-0020 is listed as Phase 2's lead item, "registry foundation, first in this phase"), but this WORK's own file was never reconciled against the deletion - a documentation drift, not a new decision. Corrected 2026-09-23, per explicit human direction: this WORK's In Scope/Approved Design/Acceptance Criteria/Documentation Impact above now describe building the whole Coordinator (both registries, both dispatch paths, `Deliver`) from scratch, with GAME-ADR-0025's role-aware model designed directly into it from the start, rather than layered onto a nonexistent existing implementation. No production code was touched by this correction; this WORK's Status remains DRAFT - its own Blockers 1-3 (below) are unaffected by this correction and still need resolution before READY.

## Completion Record

Not yet DONE. Status: DRAFT. Blockers 1-3 need human approval before this WORK can move to READY.
