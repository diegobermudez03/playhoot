# SESSION-ADR-0035: Game Input Requirements And The Player Device Check

Status: ACCEPTED
Created: 2026-10-25
Last status change: 2026-10-25
Supersedes: None
Superseded by: None

## Context

The architecture plan requires the runtime manifest to declare a game's required input capabilities, admission to check a player's device and controls against them, and discovery to filter games by them (project checklist B and E). None of it existed: any device could join any game, and a game played with a keyboard would be unplayable on a phone with no way to say so.

How a game reads input is not settled yet. Today a game's frontend script reads the browser's events itself, so the platform cannot know which interactions it uses. Game authoring, publishing and a game directory do not exist; versions come from seed migrations.

The human decided the design on 2026-10-25, asked from the frontend repository.

## Decision

1. **Interactions of this first iteration.** A game interacts with the player through three interactions only: the keyboard (letters, digits, arrows and space), moving the pointer without pressing (hover), and pressing or dragging with the primary button (a finger on a touch device).
2. **Future: interaction through SDK built-ins (not built).** Games will receive input only through Playhoot SDK built-in functions, one per interaction. When a version is published, a static analysis of its frontend script will infer which interactions it uses from the built-ins it calls. Until both exist, a version's interactions come from seed data.
3. **Device classes and the device rule.** A client classifies itself as `desktop` or `mobile` (a tablet is whichever its browser says). A desktop has a keyboard and a mouse, so it can perform all three interactions. A touch device has no keyboard and can imitate the pointer for only one of hover or press, not both. So a version that uses no keyboard and at most one of hover or press can be played on `both`; any other version only on `desktop`. `mobile` (only touch devices) is a valid value reserved for the future; no game is mobile-only in this iteration, and the rule never produces it.
4. **Storage as queryable columns.** Each game version has one row in `play_game_version_input_requirements`: `uses_keyboard`, `uses_pointer_hover`, `uses_pointer_press` (booleans) and `devices` (`desktop`, `mobile` or `both`), with an index on `(devices, game_version_id)` for the future game directory. `devices` is a query optimization computed by whoever writes the row from the three booleans with the rule above; writers validate that it matches, readers that enumerate its values treat an unknown one as broken data. No CHECK constraint (`docs/engineering/standards/database-constraints.md`). An EXECUTABLE version without a row is broken data, like a missing aspect ratio.
5. **The check at every player attach.** Both player join endpoints take a query parameter `device` (`desktop` or `mobile`). The Session's live controller checks it on every player connection, before anything else changes for that connection: lobby join, reattach, rejoin and device switch, since a player may come back on another device. A missing or unknown value is an unsupported device. `desktop` is allowed when `devices` is `desktop` or `both`, `mobile` when it is `mobile` or `both`. Admins are not checked: they watch, they do not play.
6. **Wire.** A refused device gets the existing post-upgrade rejection with the game's requirements, `{"type":"rejected","reason":"device_not_supported","requirements":{"devices":...,"uses_keyboard":...,"uses_pointer_hover":...,"uses_pointer_press":...}}`, and the connection is closed `1008` `admission rejected`, like every other admission rejection. `device_not_supported` joins the client telemetry catalog's backend rejection kinds.
7. **Seed data.** Every seeded version (the placeholder game, Space Drift's three versions, Parqués, and the DRAFTs) is pressed or dragged only and playable on `both`: Space Drift's current controls accept touch, and its keyboard alternatives cannot be expressed before the SDK built-ins exist.
8. **Future: the game directory (not built).** A directory listing games will filter them by `devices` for the visitor's device class.

**Amended 2026-10-25 (decided by the human): order of the check.** Point 5's "before anything else" is replaced: whether the Session admits the player (it exists and is in a joinable state: the durable eligibility and availability checks, and for a running game whether the player is one of its participants) is checked first, and the device only once the Session is valid to join, still on every player attach (lobby join, reattach, rejoin, device switch) and still before anything changes for that connection (a lobby place, a replaced connection, a rejoin asked of the game). So a Session that ended answers its own reason rather than `device_not_supported`. The same day, refusals by Session UUID became readable after the upgrade (`invalid_session`, `session_ended` with `status` and `end_cause`, `session_unavailable`; `api/README.md`).

## Rationale

- Three interactions cover the games being built now and map onto what both device classes can physically do, so the rule is small and explainable to a game author.
- Columns rather than JSON inside the manifest let the future directory filter with an index, without parsing every manifest.
- Storing `devices` duplicates what the booleans imply, but makes the directory's filter one indexed comparison and keeps the rule in one place, the writer.
- Checking every attach, not only the lobby join, is what makes a device switch safe: a player cannot carry a running game to a device that cannot play it, and their working connection is not replaced by one that cannot.
- The requirements of a pinned version never change, so the controller reads them once with the manifest and checks each connection in memory.

## Alternatives Considered

### Requirements inside the manifest JSON

Rejected by the human: the game directory needs to filter on them, which wants queryable, indexed columns.

### Check only when joining the lobby

Rejected: a participant reattaching or switching device could then play a game their device cannot control.

### Compute the devices when reading instead of storing them

Rejected: the directory would have to repeat the rule in SQL, and an index could not serve the filter.

## Consequences

- A client that does not send `device` cannot join any game as a player. The frontend must classify itself and send it on both player endpoints.
- A version's interactions are only as accurate as the seed data until the SDK built-ins and the publishing static analysis exist.
- Changing the device rule later changes what writers compute; rows already stored keep the `devices` they were written with.

## Canonical Knowledge Impact

- `play/docs/DATA_MODEL.md`: `play_game_version_input_requirements`.
- `play/docs/FLOWS.md`: Runtime Manifest Load, Player Device Check, and the required SDK, publishing and directory behavior.
- `play/CURRENT_STATE.md`: Player connections, Runtime manifest, Game versions.
- `api/README.md`: the `device` query parameter, the rejection and its close.
