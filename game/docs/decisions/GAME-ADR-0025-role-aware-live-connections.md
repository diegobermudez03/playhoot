# GAME-ADR-0025: Role-Aware Live Connections — Admin and Participant

Status: ACCEPTED
Created: 2026-09-20
Last status change: 2026-09-20
Supersedes: None
Refines: GAME-ADR-0002 (adds an explicit connection-role distinction inside the already-accepted Live Session Coordinator boundary; does not change the Coordinator's non-authoritative-truth boundary or V1 single-process scaling stance), GAME-ADR-0005 (adds Admin-role host validation as a second identity-boundary check alongside the existing Participant validation, without changing the public/internal identity boundary itself)
Superseded by: None
Legacy ID: None

## Context

`game/README.md`'s Session Runtime Actor and Lifecycle Model already accepts that Host and Participant are independent concepts: creating a Session establishes a host without making that host a gameplay Participant. WORK-0005 implemented the first live transport without a role distinction at the connection level — a single connection kind, bound to `(SessionUUID, UserUUID)` after `Join`, carried both ordinary player commands and (as later discovered, WORK-0005's Blocker 8) the host's own `Start` command, forcing a host with no independent desire to play to go through `Join` merely to obtain a connection. WORK-0005's own DRAFT proposal for fixing this (a distinct host-connect handshake and a second registry) is subsumed by this broader record and by WORK-0020, which owns implementing it.

Beyond the immediate host-connection gap, Session Runtime V1 also needs: a host to observe lobby state and administer the Session (Start, future Cancel) without playing; a host to optionally also join as a Participant, holding both connections at once; a host to spectate a Participant's live visual experience during RUNNING without gaining that Participant's answer authority; and future host-only commands (cancellation, spectator selection) to be structurally distinguishable from player-only commands (AnswerInteraction, UserIntent) rather than differentiated only by ad hoc per-handler checks.

## Decision

### Two independent connection roles

The live transport (`play`/`api`) distinguishes exactly two connection roles, `ADMIN` and `PARTICIPANT`, bound to structurally separate registry entries even for the same `UserUUID` on the same Session. The same person may simultaneously hold one `ADMIN` connection and one `PARTICIPANT` connection for the same Session — never one connection wearing two hats, and never a connection role inferred from a client-supplied claim alone.

- **ADMIN**: obtained without `Join` and without consuming a Participant slot, validated against `sessions.host_actor_id` (mirroring `Start`'s existing host check as a read-only capability, per WORK-0005's original `ResolveHost` proposal, now owned by WORK-0020). Carries host-only commands: `Start` (already existing), future `Cancel` (WORK-0011), future spectator-target selection (WORK-0021), and any other host control this Project explicitly accepts later. Receives ADMIN-shaped lobby/RUNNING/terminal projections.
- **PARTICIPANT**: obtained through `Join` (already implemented by WORK-0005), tied to gameplay participation. Carries player-authored commands: `AnswerInteraction` (already existing), future `UserIntent` (WORK-0010), and `Leave` where phase-appropriate. Receives PARTICIPANT-shaped lobby/RUNNING/terminal projections.

A connection-role claim made at connect time is never trusted as command authorization by itself. `sessionlifecycle.Manager`'s own per-command authoritative checks (`host_actor_id` for `Start`, recipient identity for `AnswerInteraction`, and equivalent future checks for `Cancel`/`UserIntent`) remain the sole authoritative enforcement; the connection-role boundary is a routing/UX/future-delivery-targeting layer, defense-in-depth against a client that never had legitimate access to the role's command surface, not a replacement for Manager's authority.

### Host-as-Participant

A host may independently complete the ordinary PARTICIPANT `Join`-then-connect handshake, exactly as any other player would, producing a second, independent connection/registry entry alongside their separate ADMIN connection. The two connections have independent lifecycles (either may disconnect/reconnect without affecting the other) and independent command surfaces. This is not a third role — it is one person holding two role-scoped connections at once, each already fully specified above.

### Role-specific lobby projections

During `LOBBY`, ADMIN and PARTICIPANT receive different projections of the same underlying lobby state, per the already-accepted principle that a client only ever receives what its role needs (`game/README.md`'s public/internal identity boundary, GAME-ADR-0005):

- ADMIN receives enough to render a host lobby view: current participant roster (whatever per-participant metadata WORK-0008 finalizes), participant count, join/leave events, lobby phase/deadline, and whatever state its accepted host controls need (at minimum, Start-eligibility).
- PARTICIPANT receives at minimum a live participant count and count updates on join/leave, plus lobby phase/deadline as needed to render a waiting-room UI. Full roster/participant identities are not exposed to an ordinary Participant merely because ADMIN needs them — this is a role-specific projection, not a shared one with fields hidden client-side.

The exact wire shape of either projection is owned by WORK-0008, not this record.

### Host spectator view (RUNNING)

An ADMIN connection may select a Participant to spectate and later switch the selection. This is Coordinator/UI delivery state, ephemeral and non-authoritative — it is never persisted as Session Runtime truth, never affects the observed Participant's runtime state or Game Language semantics, and does not require its own durable design here. The spectator stream is read-only visual mirroring only: current/updated Presentation state and presentation-only Effects for the selected Participant, never that Participant's actionable `interaction_id` or answer authority. ADMIN cannot answer or otherwise act on the observed Participant's behalf through this path. The concrete mechanism (subscription/selection API, initial-state reconstruction reuse) is owned by WORK-0021, not this record.

### Terminal notification reaches every bound connection, of either role

WORK-0007's terminal-notification broadcast (Session becomes `TERMINAL`) must reach every currently-bound connection for the Session, ADMIN and PARTICIPANT alike, including both of a host-as-Participant's two connections independently. A Session-wide broadcast is not filtered by role or recipient the way `Deliver`'s per-recipient `Event` fan-out is.

### Reconnect is per connection role

A physical ADMIN connection loss must never itself produce a Game Language `UserDisconnected` signal — that signal is a PARTICIPANT/runtime-membership concept (GAME-ADR-0011), and an admin console losing its socket is not a player leaving the game. Semantic disconnect/reconnect (GAME-ADR-0010/GAME-ADR-0015/GAME-ADR-0016) applies to the PARTICIPANT role only. If the same person holds both connections, losing one does not imply losing the other. Full reconnect design for each role remains WORK-0015's, informed by this record's role separation rather than assuming one undifferentiated "the user reconnected" event.

## Rationale

Making the role distinction structural (two registries, two command surfaces) rather than a single connection with an internal role tag is what makes it mechanically impossible for participant-addressed fan-out to reach an admin connection or vice versa, and gives host-only commands and future host-targeted delivery (spectator view, cancellation) a natural place to land without a later registry redesign. Treating layered authorization (connection-role boundary plus Manager's own authoritative per-command checks) as the model, rather than trusting the connection role alone, keeps the actual security boundary where it already correctly lives (`sessionlifecycle.Manager`), consistent with how WORK-0005 already reasoned about its original host-connection proposal.

Keeping the spectator selection ephemeral and read-only-visual-only avoids creating a second way to mutate or observe authoritative interaction state outside the already-accepted Participant/Coordinator delivery model, and avoids conflating "who is allowed to watch" with "who is allowed to act."

## Alternatives Considered

### One connection kind, an internal boolean/role field determines behavior

Rejected. A shared registry entry with a role tag makes it easy for a future change to accidentally leak participant-addressed fan-out to an admin connection (or the reverse) since both live in the same map/lookup path; a structurally separate registry makes that class of bug impossible by construction.

### Let a host's ADMIN connection double as a Participant connection when the host also plays

Rejected. This reintroduces exactly the coupling WORK-0005's Blocker 8 discovered as a compliance gap against `game/README.md`'s Host/Participant independence invariant — a host who also plays needs a real Participant connection with real Participant command authority, not an ADMIN connection pretending to be one.

### Trust a client-supplied role claim as sufficient command authorization

Rejected. This would let an arbitrary caller claim `ADMIN` and reach host-only commands merely by asserting it; Manager's own authoritative checks must remain the actual enforcement, with the connection-role boundary as a UX/routing layer on top.

## Consequences

- WORK-0020 (Role-Aware Live Connections / Host Administration Channel) owns implementing the two-registry connection model, the ADMIN handshake/validation, and the ADMIN/PARTICIPANT command-authorization boundary, superseding WORK-0005's own Blocker 8 proposal.
- WORK-0008 (Live Lobby / Session Bootstrap) depends on this record for its role-specific lobby projections.
- WORK-0021 (Host Participant Spectator View) depends on this record for the ADMIN connection its spectator selection attaches to.
- WORK-0007, WORK-0010, WORK-0011, WORK-0015 are each updated to reflect the ADMIN/PARTICIPANT distinction in their own scope.
- No migration, production code, or implementation is authorized by this record; it is design/architecture acceptance only.

## Canonical Knowledge Impact

- `game/README.md` — Session Runtime Actor and Lifecycle Model gains a forward reference to this record for the connection-role distinction (the Host/Participant independence invariant itself is unchanged, only now given a live-transport-level mechanism).
- `docs/projects/active/session-runtime-v1/PROJECT.md` — new WORK-0020 and WORK-0021 added to the roadmap.

## Implementation Impact

Future implementation work must align the live transport with this model, per WORK-0020/WORK-0021 once each reaches READY. No migration, production code, or WORK beyond WORK-0020/WORK-0021's own creation is authorized by this record.
