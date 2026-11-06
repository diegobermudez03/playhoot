# Launch Readiness

Status: ACTIVE TRACKING

Decided by the human on 2026-11-01: everything that needs an external integration or infrastructure is done in the last steps before the first deployment. Until then code is written against the existing interfaces (for example `monitoring/metrics`, which only logs today), and the first deployment is a single instance, so code may assume no other instance exists.

This list gathers what was deferred that way. Each item's history stays where it was recorded.

## Integrations

- Real Google sign-in: an OAuth client from Google Cloud Console (`GOOGLE_OAUTH_CLIENT_ID`, `GOOGLE_OAUTH_CLIENT_SECRET`, `GOOGLE_OAUTH_REDIRECT_URL`) and the manual check against Google (new login, recurring login, cancellation, refused wrong verifier). `docs/projects/completed/identity-v1/PROJECT.md`.
- Metrics provider for server and client metrics (`monitoring/metrics`), with alerts and dashboards defined on it. `docs/engineering/ENGINEERING_RADAR.md`.
- Draft asset uploads lost before a decision (`play_draft_asset_uploads_interrupted_total{cause=busy|read_timeout|client_gone|read_failed}`, with `play_draft_asset_upload_slot_wait_seconds` and `play_draft_asset_upload_read_seconds`): a dashboard and an alert on its rate, to size the 4 upload slots per instance and the 5-minute read deadline with real traffic (the human, 2026-11-02).

## Infrastructure

- Object storage for deployed environments, and the bucket CORS (`deploy/gcs-cors.json`). `deploy/README.md`.
- `PUBLIC_API_BASE_URL` for deployed environments: the API's public address, written into the links an AI gets for a game (required in production). Check that nothing in front of the service (load balancer, CDN) logs the AI routes' paths (`/ai/v1/{token}`), since the token is in them (the human, 2026-11-02).
- `PUBLIC_WEB_BASE_URL` for deployed environments: the web app's public address; the AI context page links the SDK guide at `<PUBLIC_WEB_BASE_URL>/docs/sdk/v1/index.md` (required in production). The web host must serve `apps/web/public/docs/sdk/v1/*.md` as-is, with a text content type and without the SPA fallback (2026-11-02).
- The game toolchain for deployed environments: package the frontend repository's game tools (`packages/game-tools` and the SDK packages it bundles, with Node) for the deployed image and set `GAME_TOOLCHAIN_DIR`; isolate the build worker (its own process or service with no application credentials, CPU and memory limits, a concurrency that never degrades running games), which `play/session/internal/gamebuild.Builder` already abstracts. Until then a build runs as a Node child process of the backend with a 60 s timeout and a 512 MiB heap (the human, 2026-11-02).
- Simulation caps (2026-11-06): every simulation is recorded in `play_simulation_results` for 7 days (its result cleared after 30 minutes), and `keepsimulationresults.CountSimulations(version, since)` counts them; decide the caps per version and window (and per User across games if needed) with real use, and wire the check before simulating.
- Build dashboards and alerts: `play_draft_builds_total{outcome}` (alert on `infrastructure`), `play_draft_build_duration_seconds`, `play_draft_build_queue_wait_seconds`, to size the workers per instance.
- MCP server and AI authorization (2026-11-03, CHECKLIST I): the human checks real AI clients (Claude and ChatGPT connectors) on the throwaway GCP VM with an ephemeral database: discovery from the 401, registration, consent, tools, refresh, revocation. The issuer and resource are `PUBLIC_API_BASE_URL` and `<PUBLIC_API_BASE_URL>/mcp`, the consent page `<PUBLIC_WEB_BASE_URL>/connect-ai`; both must be the public HTTPS addresses, and nothing in front of the service may cut `/mcp` bodies under 72 MiB or 5 minutes. Dashboard of `identity_ai_authorization_total{operation}` with an alert on `reuse_detected`.

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
