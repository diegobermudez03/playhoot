# Session Runtime

This package implements the Session Runtime capability inside the Game bounded context.

It provides the stateful execution layer around Game Language behavior.

This package owns:

- Persisting session state.
- Receiving events and matching them to persisted sessions for execution.

This package does not own:

- Transport/network protocol concerns such as WebSocket, gRPC, or TCP.

Completed-session archival direction is accepted (same-database PostgreSQL JSONB compaction, `game/docs/decisions/GAME-ADR-0024-replay-first-session-runtime-persistence.md`) but not yet implemented (`docs/projects/active/session-runtime-v1/works/WORK-0017-archival.md`); this package does not itself define the archive schema.

Canonical Game boundary documentation lives in `../README.md`.
