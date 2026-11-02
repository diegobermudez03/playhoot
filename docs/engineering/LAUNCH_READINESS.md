# Launch Readiness

Status: ACTIVE TRACKING

Decided by the human on 2026-11-01: everything that needs an external integration or infrastructure is done in the last steps before the first deployment. Until then code is written against the existing interfaces (for example `monitoring/metrics`, which only logs today), and the first deployment is a single instance, so code may assume no other instance exists.

This list gathers what was deferred that way. Each item's history stays where it was recorded.

## Integrations

- Real Google sign-in: an OAuth client from Google Cloud Console (`GOOGLE_OAUTH_CLIENT_ID`, `GOOGLE_OAUTH_CLIENT_SECRET`, `GOOGLE_OAUTH_REDIRECT_URL`) and the manual check against Google (new login, recurring login, cancellation, refused wrong verifier). `docs/projects/completed/identity-v1/PROJECT.md`.
- Metrics provider for server and client metrics (`monitoring/metrics`), with alerts and dashboards defined on it. `docs/engineering/ENGINEERING_RADAR.md`.

## Infrastructure

- Object storage for deployed environments, and the bucket CORS (`deploy/gcs-cors.json`). `deploy/README.md`.

## From Session Runtime v1

Unfinished items of `docs/projects/completed/session-runtime-v1/CHECKLIST.md` marked `[→ launch]` (multi-instance ownership and routing, global limits, platform termination by the consumption policy, observability still missing):

- Owner abstraction exists: Resolve(sessionID), Claim(sessionID), Release(sessionID)
- Exactly one Runtime pod owns an active Session; two pods can never mutate the same Session concurrently
- Before more than one instance may own Sessions (separate gateway, or more than one instance of the single binary): shared registry, owner-renewed lease + heartbeat, self-fencing, epoch on owner-only durable writes, gateway-to-Runtime trusted identity, addressable-instance deployment. Instructions: `play/docs/decisions/SESSION-ADR-0029-live-session-ownership-and-owner-routing.md`
- Join code -> sessionUUID resolution -> owner resolve/claim -> route to pod
- Lobby pod loss: Session stays LOBBY, players can rejoin the lobby; no lobby checkpoint
- First reconnect: owner registry sees no valid owner -> claims new pod -> request routed
- Platform-forced termination wired to its triggers (limits, consumption): `ControllerResolver.TerminateSession` is ready and tested, nothing calls it yet (requested by the human 2026-10-18). 2026-10-19, decided by the human: no rate limit or resource control triggers it (a boundary beyond the rate is deferred, a misbehaving player is limited); consumption triggers remain open
- Global limits across instances: request limits are kept in memory per instance, which is right while one instance runs (decided by the human 2026-10-19); before scaling out, limit globally (for example in the infrastructure in front of the service)
- Bucket CORS for the web origin applied to the production bucket: configured as code 2026-10-25 (`deploy/gcs-cors.json`, `deploy/README.md`), applied at deploy; nothing is deployed yet (Engineering Radar)
- Metrics emission to the real backend implemented in `monitoring/metrics`
- Per-message logging on live connections follows the logging standard (each inbound message is its own entry point; trace correlated per connection)
- Alerts use the monitoring abstraction; internal inconsistencies follow `data-integrity.md`
- Open WebSockets by role (PLAYER/ADMIN); attach/detach rates; transport-grace expirations
- Inbound/outbound message and byte rates; outbound backpressure/slow-consumer events (network message counts belong in `api/internal/live`; per-Session aggregates such as messages per Session belong to the Runtime)
- Processing boundary duration, batch size, queue depth (pending timeline operations)
- Boundary lag vs target cadence (~100 ms); update() call interval and skipped/dropped updates
- SAMPLE: accepted vs deduped vs stale; pending-transition buffer depth
- Execution latency per entrypoint (start, samples, command, update, project, ...)
- Context creates/recreates/evictions; state size and output size
- Projection computation latency; projection/patch size; full-vs-patch send ratio
- Checkpoint success/failure, duration, size, age of latest successful checkpoint per active Session (alert on stale)
- Owner claim/release/lease-loss events; any detection of dual ownership (critical alert)
- Alerts defined for: dual ownership, sandbox retry-after-script-error, repeated session aborts, checkpoint staleness, replay drops, final replay failure, boundary lag, sandbox sidecar unhealthy, queue saturation
- Dashboards for: lifecycle funnel, live load (controllers/connections/boundary lag), sandbox health, recovery/checkpoint health, replay pipeline
- Per-Session log correlation: one trace per connection, Session/Participant identifiers present in logs (without sensitive data)
- Live Session ownership (`play/runtime/usecases/ownership`, in scope, see A1): `LocateOrClaimOwner` answers `owned_here` with epoch 1 for every live Session, without keeping a registry (since 2026-10-18 it claims only live Sessions: `checklivesession` answers LOBBY before its expiry or RUNNING, anything else is `session_unavailable`, so no controller is created for it). Since 2026-10-18 `ReleaseOwner(sessionUUID, epoch)` exists and every controller calls it when it ends, but it releases nothing. Real behavior: in-process registry with release; shared registry + lease/heartbeat before multi-instance (SESSION-ADR-0029)
- Forwarding to a remote owner (`api/play/owner_routing.go`, in scope, see A1): a Session owned by another instance is answered `503`; unreachable while single-instance. Real behavior: proxy the connection to the owner (gateway), decided with the multi-instance topology
- Platform termination (`ControllerResolver.TerminateSession`, in scope, see S): ready and tested, nothing calls it
- Metrics emission (`monitoring/metrics`, `record`): measurements are only written to the request log; nothing is sent to a metrics backend (see Y)
- 1. One Session, one live owner
