# Product Ideas

Status: NON-AUTHORITATIVE

This file preserves ideas and hypotheses. Nothing in this file is approved, planned, or required merely because it is listed here.

## Monetization

- Published-game limits.
- Monthly session/room usage limits.
- Paid higher limits.
- School/group/organization accounts.
- Advertising during play.

## Audience / Content Directions

- Couples-oriented experiences.
- General social/party experiences.
- Streamer or community-host experiences.
- Event-organizer experiences.
- Family adaptations of traditional games.

## Future Creator / Product Capabilities

- Creator collaboration.
- Analytics/dashboard.
- Community remixing.
- Persistent progression across sessions.
- Advanced import/inspection workflows.
- Advanced discovery ranking, recommendations, and curation.

## Session Re-Entry After Complete Client-State Loss

Post-launch / later iteration. Not V1. Not a roadmap commitment. Not approved implementation. Exact identity/security/UX semantics deferred.

A player should eventually be able to recover an active Session even when the browser/tab/app lost all frontend state.

**Registered account.** Future UX may expose something like "Current Sessions," where a registered User could see Sessions they are still associated with and attempt to resume one when the Session still exists/allows resumption and authored game semantics still permit that player to reconnect/rejoin gameplay. The existing durable `UserUUID -> SessionActor` relationship is expected to make this feasible, but no concrete UX/API is approved merely by listing this idea.

**Guest.** A guest should eventually have a recovery flow associated with the same active Session/JoinCode. The idea is that a guest could return using the same JoinCode and the same prior username/display name, and recover their former participation if the game still allows reconnection.

This is "a guest can recover their previous participation after complete client-state loss" - it is explicitly not "matching a username is permanently accepted authentication." Do not promote "same username" to an accepted identity/security mechanism. Guest identity recovery must be designed safely later; possible future approaches may involve a guest resume credential/token, a device/session credential, an explicit reclaim flow, or another secure mechanism.

## Minimize Persisted RuntimeTurn History

Raised 2026-09-20, alongside WORK-0006 (broaden live fan-out) confirming that engine execution is fully deterministic given (compiled Program, a starting Snapshot, a sequence of driving Signals) - no wall-clock, no OS randomness, nothing outside that triple.

Today, `session_runtime_turns` persists a full `snapshot_payload` (the entire resulting Snapshot) after every committed Turn. Given full determinism, that is more than strictly necessary: in principle only the *inputs* (the driving Signal per Turn, plus the one-time Start `Seed`) need to be durable - every Snapshot along the way, including the final one, is mechanically re-derivable by replaying those inputs through the same compiled Program from the beginning. Storing every intermediate Snapshot is redundant with storing the inputs that produced it.

Not approved, not scoped, not V1. Explicitly kept as full-snapshot persistence for now (simpler, already implemented, no replay-reconstruction code needed to serve any current read path). Revisit only if/when storage cost or a concrete need for storage minimization makes it worth trading for replay-reconstruction complexity. Depends on the same input-capture gaps a "replay" feature would need to close (see this file's determinism note and `docs/work/active/WORK-0006-broaden-live-fanout-effects-presentations.md`'s own persistence discussion) - the Start Turn's `Seed` is not currently captured anywhere durably, and later Turns' driving Signals are only reconstructable via a join against `session_interactions`, not stored as a first-class value.

## Personalized End-of-Game Screen

Raised 2026-09-20, alongside `docs/work/active/WORK-0007-session-termination-live-notification.md`.

V1's session-termination notification (fatal failure or natural game completion) sends one generic "session ended" signal and every client renders the same fixed screen, regardless of cause or outcome (winner/loser, score, etc.). A richer, personalized end-of-game screen - for example showing each player whether they won/lost, a final score, or game-specific results - is a real, plausible future need, but requires its own design (how a game defines what a "results" view looks like per player, how that's computed from the root workflow's `WorkflowOutcome.Result`, and how it interacts with the Presentation/View authoring model already used for in-game UI). Not approved, not scoped, not V1.
