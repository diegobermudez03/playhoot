# WORK-0020: Role-Aware Live Connections / Host Administration Channel

Status: DRAFT
Created: 2026-09-20
Last status change: 2026-09-20

Related decisions:
- GAME-ADR-0025 (Role-Aware Live Connections - the accepted direction this WORK implements)
- GAME-ADR-0002 (Live Session Coordinator responsibility boundary - refined, not superseded, by GAME-ADR-0025)
- GAME-ADR-0005 (public/internal identity boundary - refined by GAME-ADR-0025's ADMIN validation)

Canonical context:
- `game/docs/decisions/GAME-ADR-0025-role-aware-live-connections.md` (the accepted model this WORK implements)
- `game/README.md` (Session Runtime Actor and Lifecycle Model - "Host and Participant are independent concepts", the invariant this WORK's transport finally complies with)
- `docs/projects/active/session-runtime-v1/works/WORK-0005-thin-live-coordinator.md` ("Host Connection Design Revision" section - the original proposal this WORK supersedes and formally owns going forward; WORK-0005's already-implemented Create/Join/AnswerInteraction/Deliver path and participant registry are unchanged foundations this WORK builds on, not replaced. WORK-0005's own Blocker 10 (2026-09-21) removed `Start` from the live wire entirely rather than leave it on the Join-coupled connection this WORK's own Blocker 8 already knew was wrong - so this WORK is not replacing a working `Start`-over-wire path, it is adding the first one, on the `ADMIN` connection it builds.)
- `play/coordinator.go`, `play/sessionruntime/` (the existing Coordinator/SessionRuntime-port implementation this WORK extends)

## Outcome

The live transport distinguishes exactly two connection roles, `ADMIN` and `PARTICIPANT`, bound to structurally separate Coordinator registry entries. A Session host obtains an `ADMIN` connection without `Join` and without consuming a Participant slot, validated against `sessions.host_actor_id`. A host may independently also complete the ordinary `PARTICIPANT` `Join`-then-connect handshake, holding both connections at once with independent lifecycles. Host-only commands (`Start` today; future `Cancel`, spectator-target selection) are reachable only from an `ADMIN` connection; player-authored commands (`AnswerInteraction` today; future `UserIntent`, `Leave`) are reachable only from a `PARTICIPANT` connection. `sessionlifecycle.Manager`'s own authoritative per-command checks remain the sole actual authorization; the connection-role boundary is a routing/defense-in-depth layer on top of them, not a replacement.

This WORK formally supersedes and implements WORK-0005's own "Host Connection Design Revision (2026-09-20, DRAFT PROPOSAL)" - that section's design intent (a distinct host-connect handshake, a `ResolveHost` read capability, a structurally separate host registry) is the starting point for this WORK's own Approved Design, not a competing proposal; WORK-0005 itself no longer owns implementing it (see WORK-0005's revised Blocker 8 and Status Correction).

## Context

WORK-0005's Blocker 8 discovered that the originally-implemented live transport forced a host through `Join` merely to obtain a connection to send `Start`, contradicting `game/README.md`'s already-accepted Host/Participant independence invariant. WORK-0005's own DRAFT proposal sketched a fix (a second registry, a `ResolveHost` capability), but the reconciliation that produced this WORK broadened the requirement beyond that one gap: Session Runtime V1 also needs role-specific lobby projections (WORK-0008), a host spectator view during RUNNING (WORK-0021), terminal notification reaching every connection regardless of role (WORK-0007), and future host-only commands (`Cancel`, WORK-0011) to be structurally distinguishable from player commands (`UserIntent`, WORK-0010) rather than accumulating ad hoc per-handler checks. This WORK is the single place that ownership now lives, so the connection-role model is designed once, coherently, rather than iteratively patched by each downstream WORK that happens to need a piece of it.

## Scope

### In Scope

- A host-connect handshake (no `Join`, no `session_participants` row) that validates the caller against `sessions.host_actor_id` before upgrading - reusing WORK-0005's own already-proposed `ResolveHost`-shaped read capability and mechanism (mirroring `Start`'s existing host check, restated read-only rather than duplicated as a new authorization rule).
- A second, structurally separate Coordinator registry (`ADMIN` connections), alongside the existing participant registry, so participant-addressed fan-out can never reach an `ADMIN` connection and vice versa.
- Command-surface separation at the transport layer: an `ADMIN` connection's read pump dispatches only host-only commands (`START` today); a `PARTICIPANT` connection's read pump dispatches only player commands (`ANSWER_INTERACTION` today), each rejecting the other's commands transport-side with an `ERROR` reply, never forwarding them to Coordinator/Manager.
- Support for the same `UserUUID` holding one `ADMIN` and one `PARTICIPANT` connection simultaneously for the same Session, each with an independent bind/unbind lifecycle.
- Whatever minimal wiring is needed so WORK-0007's terminal broadcast already reaches both registries (this WORK does not itself change WORK-0007's own broadcast Acceptance Criteria, only ensures the registry shape it broadcasts against already includes both).

### Out of Scope

- Role-specific lobby projection payloads (roster, participant count, join/leave events) - WORK-0008's own scope, which depends on this WORK's connection model existing first.
- The host spectator view itself (subscription/selection API, Presentation mirroring) - WORK-0021's own scope, which depends on this WORK's `ADMIN` connection existing first.
- Manual cancellation's own command/business logic - WORK-0011's scope; this WORK only ensures `Cancel` has an `ADMIN`-only place to be dispatched from once WORK-0011 adds it.
- Any change to `sessionlifecycle.Manager`'s existing business logic, persistence model, or method signatures for `Start`/`AnswerInteraction` - unchanged, exactly as WORK-0005 already established.
- Reconnect/resync mechanics for either connection role beyond what already exists (bind-replaces-prior-connection semantics) - WORK-0015's own scope, informed by this WORK's role separation (a physical `ADMIN` connection loss never produces `UserDisconnected`) but not implemented here.
- Real credential verification/Identity implementation - continues WORK-0005's already-established trusted-`UserUUID` stance.

## Approved Design

GAME-ADR-0025 already settles the direction (two structurally separate registries, layered authorization, host-as-Participant via two independent connections). WORK-0005's own "Host Connection Design Revision" section already sketches a concrete mechanism (`GET /ws/host` or equivalent, `ResolveHost`, `Coordinator.BindHost`, a `hosts map[SessionUUID]Conn` registry) consistent with that direction - this WORK adopts that mechanism as its starting Approved Design rather than re-deriving it, subject to the Blockers below (carried forward from WORK-0005, now owned here).

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
- The existing Create -> Join -> Start -> receive-interaction -> answer -> receive-resolution end-to-end proof (WORK-0005) continues to pass with the host connecting via the host handshake (never joining) and a separately-joined Participant receiving/answering the opened interaction.
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

- `play/README.md` - describe the two-registry connection model and the `ADMIN`/`PARTICIPANT` command-surface separation, superseding its current single-connection-kind description.
- `api/README.md` - describe the host-connect handshake alongside the existing participant Join-then-connect handshake.
- `game/CURRENT_STATE.md`, `game/docs/FLOWS.md` - describe role-aware connections as now-implemented.

### Intentionally Unchanged

- WORK-0001 through WORK-0004 (completed historical WORKs) - not reopened.
- WORK-0005's own already-implemented Create/Join/Start/AnswerInteraction/Deliver path and participant registry - this WORK extends, does not replace, that foundation.
- `sessionlifecycle.Manager` - no change to its business logic, persistence model, or method signatures.

## Completion Record

Not yet DONE. Status: DRAFT. Blockers 1-3 need human approval before this WORK can move to READY.
