# Session Runtime - Current State

Status: CURRENT IMPLEMENTATION

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| Session creation | IMPLEMENTED | `play/session/workflows/sessionlifecycle` (`Manager.CreateSession`): idempotent per user and key, pins the Game's current EXECUTABLE version, creates the Session in LOBBY with a join code. Unit, repository and integration tests (incl. concurrent retries) pass, and `POST /sessions` was tested by hand locally. Flow: `play/docs/FLOWS.md`. |
| Game versions | SCHEMA ONLY | `play_games` / `play_game_versions` exist and are read by Session creation, but nothing creates them yet, so every creation answers `game_unavailable` until they are seeded. |
| Live Session Runtime | NOT IMPLEMENTED | No controller, live connections, or game execution. The direction is the real-time runtime of `play/docs/decisions/SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md`. |
| Live Session ownership | PLACEHOLDER | `play/runtime/usecases/ownership` (`LocateOrClaimOwner`) is called by the API before every live upgrade, but answers that this instance owns every Session. Design: `play/docs/decisions/SESSION-ADR-0029-live-session-ownership-and-owner-routing.md`. |
| Admin connections | PARTIAL | `play/runtime/workflows/liveconnection` (`AttachAdmin`) asks the durable Session whether the caller may join as admin (`play/session/usecases/checkadminjoin`, real: RUNNING, or LOBBY before its expiry, and the caller is the creator), finds or creates the Session's controller (`play/runtime/internal/controller`) and registers the ADMIN connection; several may be attached at once. Tested, and tested by hand from Postman. Lobby updates are real, and so is `StartSession`'s orchestration (see Start Session). `SelectPerspective`, `CancelSession` and the admin's views are not implemented yet: they answer "not implemented" and the API closes the connection. |
| Player connections | PARTIAL | `play/runtime/workflows/liveconnection` (`AttachPlayer`) asks the durable Session whether it accepts players (`play/session/usecases/checkplayerjoin`: LOBBY before its expiry; RUNNING is `session_started`), then the controller admits into the lobby: up to the manifest's maximum, the same user's older connection replaced, every change sent to admins and players. Leaving or disconnecting takes the player out at once. Tested, run end to end, and tested by hand. Start moves the controller to running and sends each player `session_started` and a FULL first view; the operations of a running game are not built. |
| Start Session | PARTIAL | `play/runtime/workflows/liveconnection` (`startSession`, behind `AdminConnection.StartSession`): the controller freezes the lobby and numbers it P1..Pn, the durable Session validates, the Sandbox runs `start()` and the first projections (retried once in a fresh context on a script failure), the initial checkpoint is stored before the Session is committed RUNNING, then every connection gets `session_started` (roster and a signed, expiring URL to the game's frontend package, issued by the durable Session) and each player a FULL view. A second script failure aborts the Session (`start_failed`, connections end `session_aborted`); any other failure returns it to its lobby. Tested with the durable Session and the Sandbox mocked; both are failing stand-ins in `main` (see Placeholders), so a real start always fails today. |
| Runtime manifest | PARTIAL | `play_game_versions.manifest` (versioned JSON, `schema_version` 1) holds the player limits; `play/session/internal/business/manifest` validates it and `play/session/usecases/loadruntimemanifest` loads it when a Session's controller is created. Other manifest facts (inputs, commands, entrypoints, capabilities) are not declared yet. |
| Join code resolution | IMPLEMENTED | `play/session/usecases/resolvejoincode` (`ResolveJoinCode`): an unreleased, unexpired join code names its Session (matched case-insensitively); anything else is `not_found`. Called by the API before routing a player who joins by join code. Unit and repository tests. Flow: `play/docs/FLOWS.md`. |
| Script execution helpers | HELPER ONLY | `play/runtime/sandbox` runs a JavaScript source in an isolated QuickJS-on-WebAssembly worker process. Not called by anything. |
| Object storage helpers | HELPER ONLY, DOES NOT BUILD | `play/session/internal/objectstore` still imports the old `session/` path in its `storetest` package. |
| Migrations | IMPLEMENTED | One migration creates the tables in `play/docs/DATA_MODEL.md`. |

## Placeholders (wired, but not real behavior)

- No flow registers Games or makes versions executable in `play_games` / `play_game_versions`. Outside production, test-data migrations (`play/session/internal/migrations/seed_*.go`, applied by `main` via `PostgresSeed`) insert a playable Game `11111111-1111-4111-8111-111111111111` (EXECUTABLE current version plus a newer DRAFT) and an unavailable Game `22222222-2222-4222-8222-222222222222` (DRAFT only); anywhere else creation answers `game_unavailable`.

- `play/runtime/usecases/ownership.Service.LocateOrClaimOwner` answers `owned_here` with epoch 1 for any Session UUID. Real behavior: only Sessions that exist in LOBBY or RUNNING are owned; an in-process registry claims and releases ownership; before more than one instance may own Sessions, a shared registry with an owner-renewed lease and heartbeat (SESSION-ADR-0029).
- The admin connection's `SelectPerspective` and `CancelSession` (`play/runtime/internal/controller`) return "not implemented", and `NextView` never produces a view.
- The durable side of Start (`unwiredSessionStart` in `unwired.go`): `CheckStartEligibility`, `FrontendAccess`, `StoreCheckpoint`, `CommitStart` and `AbortStart` always fail, so starting a valid lobby fails and the admin connection closes with 1011. Real behavior: a `play/session` use case that validates Start, signs a short-lived URL to the version's frontend package (no frontend artifact is stored on game versions yet), stores the checkpoint durably, commits roster + checkpoint reference + RUNNING in one transaction, and marks ABORTED / GAME_START_FAILURE.
- The Sandbox's execution contexts (`unwiredSandbox` in `unwired.go`): `CreateContext`, `Start`, `Project` and `DiscardContext` always fail. Real behavior: stateful per-Session contexts (`play/runtime/sandbox` only has the stateless executor).
- During a Start attempt, a player's `Leave` returns "not implemented" (as in a running game).
- A player connection's `Sample` is dropped and `Resync` does nothing (`play/runtime/internal/controller`); before the game runs `Command` answers `session_not_running` (right), and in a running game `Command` and `Leave` return "not implemented". `NextView` only produces the first FULL view of a started game.
- Session controllers are never destroyed, so they accumulate for the process lifetime. Real behavior: destroyed, and ownership released, when the Session reaches a terminal state.

## Known Drift

- Repository docs (`AGENTS.md`, `docs/ai/KNOWLEDGE_MAP.md`, `play/README.md`, ADR index) still refer to the domain as `session/`; the code lives under `play/`.

## Evidence

- `play/session/workflows/sessionlifecycle/` (tests incl. `step_create_session_integration_test.go`), `play/session/internal/migrations/`, `play/runtime/sandbox/`, `play/session/internal/objectstore/`.
