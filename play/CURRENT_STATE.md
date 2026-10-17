# Session Runtime - Current State

Status: CURRENT IMPLEMENTATION

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| Session creation | IMPLEMENTED | `play/session/workflows/sessionlifecycle` (`Manager.CreateSession`): idempotent per user and key, pins the Game's current EXECUTABLE version, creates the Session in LOBBY with a join code. Unit, repository and integration tests (incl. concurrent retries) pass, and `POST /sessions` was tested by hand locally. Flow: `play/docs/FLOWS.md`. |
| Game versions | SCHEMA ONLY | `play_games` / `play_game_versions` exist and are read by Session creation, but nothing creates them yet, so every creation answers `game_unavailable` until they are seeded. |
| Live Session Runtime | NOT IMPLEMENTED | No controller, live connections, or game execution. The direction is the real-time runtime of `play/docs/decisions/SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md`. |
| Live Session ownership | PLACEHOLDER | `play/runtime/usecases/ownership` (`LocateOrClaimOwner`) is called by the API before every live upgrade, but answers that this instance owns every Session. Design: `play/docs/decisions/SESSION-ADR-0029-live-session-ownership-and-owner-routing.md`. |
| Admin connections | PLACEHOLDER | `play/runtime/workflows/liveconnection` (`AttachAdmin`) is called by `GET /sessions/{sessionUUID}/admin` after the upgrade, but admits every caller: selecting a perspective is accepted but no view is ever sent, and cancelling only ends the connection. The API side (auth, owner routing, upgrade, admin messages and operations) is implemented and tested. |
| Script execution helpers | HELPER ONLY | `play/runtime/sandbox` runs a JavaScript source in an isolated QuickJS-on-WebAssembly worker process. Not called by anything. |
| Object storage helpers | HELPER ONLY, DOES NOT BUILD | `play/session/internal/objectstore` still imports the old `session/` path in its `storetest` package. |
| Migrations | IMPLEMENTED | One migration creates the tables in `play/docs/DATA_MODEL.md`. |

## Placeholders (wired, but not real behavior)

- No flow registers Games or makes versions executable in `play_games` / `play_game_versions`. Outside production, test-data migrations (`play/session/internal/migrations/seed_*.go`, applied by `main` via `PostgresSeed`) insert a playable Game `11111111-1111-4111-8111-111111111111` (EXECUTABLE current version plus a newer DRAFT) and an unavailable Game `22222222-2222-4222-8222-222222222222` (DRAFT only); anywhere else creation answers `game_unavailable`.

- `play/runtime/usecases/ownership.Service.LocateOrClaimOwner` answers `owned_here` with epoch 1 for any Session UUID. Real behavior: only Sessions that exist in LOBBY or RUNNING are owned; an in-process registry claims and releases ownership; before more than one instance may own Sessions, a shared registry with an owner-renewed lease and heartbeat (SESSION-ADR-0029).
- `play/runtime/workflows/liveconnection.Manager.AttachAdmin` admits every caller; SelectPerspective accepts any participant but no view is ever sent, CancelSession only ends the connection, and the connection is released on Disconnect. Real behavior: reject a stale epoch, ask the durable Session whether it exists, is connectable and was created by the caller, register an ADMIN logical connection on the controller, and send a rejection and close otherwise.

## Known Drift

- Repository docs (`AGENTS.md`, `docs/ai/KNOWLEDGE_MAP.md`, `play/README.md`, ADR index) still refer to the domain as `session/`; the code lives under `play/`.

## Evidence

- `play/session/workflows/sessionlifecycle/` (tests incl. `step_create_session_integration_test.go`), `play/session/internal/migrations/`, `play/runtime/sandbox/`, `play/session/internal/objectstore/`.
