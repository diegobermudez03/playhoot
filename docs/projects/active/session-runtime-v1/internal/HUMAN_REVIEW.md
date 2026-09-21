# session-runtime-v1 — Project/WORK Migration Checkpoint (2026-09-20)

Process: repository workflow/documentation migration (Slice tracking -> Project + WORK, PLANNED state added) plus a full reconciliation of this Project's remaining scope. No production Session code was implemented.

## What changed

- The old numbered-Slice sequence (`internal/LEGACY_SLICE_PLAN.md`, superseded) is now `../PROJECT.md`.
- WORK-0001 through WORK-0007 moved into `../works/` with their IDs/statuses unchanged; WORK-0005/0006/0007 each got a small reconciliation note (see below), not a scope rewrite.
- Eleven new `PLANNED` WORK items (WORK-0008 through WORK-0018) were created to close previously-unowned required-capability gaps found during a Game-Language-to-Session-Runtime completeness audit. See `../PROJECT.md`'s Work table and Capability Coverage.

## Please confirm or correct

1. **Recommended execution order in `../PROJECT.md`.** It follows dependency evidence found in the actual codebase (e.g. Keyed Timers now ordered before Disconnect/Reconnect, since disconnect-grace timers are plausibly per-user/keyed) rather than the old plan's literal order. Nothing has been implemented against this order yet - it is a recommendation, not a commitment.

2. **WORK-0005's newly discovered Blocker 8 (host/Participant live-connection gap) is treated as a required compliance fix, not a new design question**, because `game/README.md` already states the accepted architecture ("Host and Participant are independent concepts... Creating a Session establishes a host but does not automatically make that host a gameplay participant") and the current live-transport implementation (`GET /ws` unconditionally calling `Join` first) contradicts it. If that reasoning is wrong - i.e. if you want the current behavior kept and the canonical architecture statement narrowed instead - say so; otherwise this stands as a real gap WORK-0005 must close before DONE. The exact fix mechanism (e.g. a host-specific connect path that does not create a Participant) is left as Implementation Freedom for whoever designs it.

3. **Two small scope questions surfaced but deliberately not decided** (see `../PROJECT.md`'s "Material Decisions Needing Human Input"):
   - Whether host "kick a participant" / "transfer host" is required for the Session Runtime V1 frontend, or explicitly out of scope. No WORK was created for it since it is not clearly a known-required outcome either way.
   - WORK-0007's already-known LOBBY_EXPIRED-while-connected gap becomes concretely reachable once WORK-0008 (live lobby bootstrap) lets clients connect before Start - worth resolving when WORK-0007 and WORK-0008 are jointly refined, not now.
   - Whether WORK-0018 (Game Language `UserDisconnected`/`UserReconnected` signal-schema support) should be tracked under this Project (as done here, since it directly gates WORK-0015) or as its own Game Language-domain initiative. Kept under this Project for now since Keyed Timers (WORK-0013) was already treated the same way in the prior plan.

## Next action

No implementation action is required as a result of this migration alone. When ready to resume implementation: WORK-0005 needs independent review (plus the Blocker 8 fix); WORK-0006/WORK-0007 need READY authorization once their remaining Blockers are resolved; any PLANNED WORK can move to DRAFT when a human decides to start designing it.
