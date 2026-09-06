# Session Runtime

This package implements the Session Runtime capability inside the Game bounded context.

It provides the stateful execution layer around Game Language behavior.

This package owns:

- Persisting session state.
- Receiving events and matching them to persisted sessions for execution.

This package does not own:

- Transport/network protocol concerns such as WebSocket, gRPC, or TCP.

Completed-session history/archive ownership remains unresolved and is not defined here.

Canonical Game boundary documentation lives in `../README.md`.
