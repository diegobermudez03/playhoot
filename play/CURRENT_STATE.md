# Session Runtime - Current State

Status: CURRENT IMPLEMENTATION

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| Session creation | IMPLEMENTED (unverified against a database) | `play/session/workflows/sessionlifecycle` (`Manager.CreateSession`): idempotent per user and key, pins the Game's current EXECUTABLE version, creates the Session in LOBBY with a join code. Unit-tested with mocks; its repository tests exist but have only been compiled, not run against Postgres. Called by the play-lifecycle orchestration behind `POST /sessions`. |
| Game versions | SCHEMA ONLY | `play_games` / `play_game_versions` exist and are read by Session creation, but nothing creates them yet, so every creation answers `game_unavailable` until they are seeded. |
| Live Session Runtime | NOT IMPLEMENTED | No controller, live connections, or game execution. The direction is the real-time runtime of `play/docs/decisions/SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md`. |
| Script execution helpers | HELPER ONLY | `play/runtime/sandbox` runs a JavaScript source in an isolated QuickJS-on-WebAssembly worker process. Not called by anything. |
| Object storage helpers | HELPER ONLY, DOES NOT BUILD | `play/session/internal/objectstore` still imports the old `session/` path in its `storetest` package. |
| Migrations | IMPLEMENTED | One migration creates the tables in `play/docs/DATA_MODEL.md`. |

## Placeholders (wired, but not real behavior)

- No flow registers Games or makes versions executable in `play_games` / `play_game_versions`; Session creation always answers `game_unavailable` until rows are seeded.

## Known Drift

- Repository docs (`AGENTS.md`, `docs/ai/KNOWLEDGE_MAP.md`, `play/README.md`, ADR index) still refer to the domain as `session/`; the code lives under `play/`.

## Evidence

- `play/session/workflows/sessionlifecycle/`, `play/session/internal/migrations/`, `play/runtime/sandbox/`, `play/session/internal/objectstore/`.
