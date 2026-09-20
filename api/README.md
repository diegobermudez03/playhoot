# API

API is the external transport/application edge for Playhoot.

- Exposes user-facing transport endpoints.
- Translates transport requests and responses.
- May handle transport-level concerns.
- May perform BFF response shaping for frontend needs.
- May call a single-domain capability.
- May call Composer for cross-domain reads.
- May initiate Orchestrator workflows for cross-domain writes.
- Owns no business state.

BFF response shaping is not the same as cross-domain read composition. If a request requires combining multiple domain reads, that composition belongs to Composer.

This README does not define Identity or authorization ownership.

## WebSocket transport adapter

`api` hosts the WebSocket transport adapter for the Live Session Coordinator (`../play/`, see `../play/README.md`): one HTTP upgrade handshake and one read-pump/write-pump goroutine pair per connection, decoding/encoding wire JSON and forwarding decoded client commands to `play`. `Create`/`Join` stay ordinary HTTP request/response (`POST /sessions`, `POST /sessions/join`); only `Start`/`AnswerInteraction` and their resulting fan-out ride the WebSocket (`GET /ws`). `api` holds no Session/connection registry and no business state of its own, and depends only on `play`'s exported API - never on any `game` package.

See `../ARCHITECTURE.md` for global architecture rules.
