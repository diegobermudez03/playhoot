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
