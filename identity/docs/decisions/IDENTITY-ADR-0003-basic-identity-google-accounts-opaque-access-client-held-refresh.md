# IDENTITY-ADR-0003: Basic Identity With Google Accounts, Opaque Access Tokens, Client-Held Refresh Tokens and Operation Levels

Status: ACCEPTED
Created: 2026-10-27
Last status change: 2026-10-27 (accepted by the human)
Supersedes: None
Superseded by: None

## Context

Identity is a placeholder: `Authorize` accepts three fixed test tokens and `POST /v1/guests` returns the same guest for every call (`identity/CURRENT_STATE.md`), so two people cannot be told apart and no account exists. IDENTITY-ADR-0002 accepted the renewal direction (short access tokens, rotating refresh tokens, accounts that expire, guests that never do) and left open where the client keeps the refresh token, the exact durations and the sign-in flows.

The plan `plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx` ("plan") proposes how to build basic identity. The human accepted it on 2026-10-25 as a whole except the refresh token location (client-held, not an `HttpOnly` cookie, so the web and the future mobile app share one backend contract), accepted on 2026-10-27 the Google login completion that follows from it, and added on 2026-10-27 a catalog of operations with their minimum authentication level for the clients' sign-in redirection.

## Decision

1. **Three access situations.** Anonymous: no valid identity is presented (no User row). Guest: a real `User` whose only proof is a credential kept by the client; it is authenticated. Account: a `User` with a registered account recoverable through an accepted provider. Guest and account are the same public entity (IDENTITY-ADR-0001); a guest is created only when it is needed to use an operation that allows guests, never as an anonymous placeholder.
2. **Google is the only account provider of this phase.** Playhoot integrates Google directly with OpenID Connect (Authorization Code, client secret only on the server, `state`, `nonce` and PKCE S256 per attempt, scopes `openid profile email`, no offline access). It is not Identity Platform or another managed authentication product: Google proves an identity; Playhoot keeps users, permissions and credentials. An external identity is the normalized issuer plus the verified `sub`; email and name are observed data that never create, recover or merge an identity. A fictitious local provider, enabled only in local and test (startup fails otherwise), replaces Google in development; everything else is the production implementation.
3. **Opaque access tokens.** An access token is 256 random bits in a recognizable format, stored only as a SHA-256 hash, valid at most 15 minutes and never beyond its family's limits. Authorizing a REST request or a WebSocket handshake is one indexed lookup resolving token, family and User and checking type, expiry, revocation and the User's eligibility. There is no lookup per frame, SAMPLE or tick. Access and refresh tokens are distinct secrets: neither is accepted in the other's place.
4. **Client-held refresh tokens.** The refresh token is returned in the response body and sent in the request body; the client keeps it (web: `localStorage`; mobile, later: the platform's secure storage). The backend sets no credential cookie and needs neither same-origin deployment nor CORS with credentials. Accepted risk: a script injected into the web shell could read a guest's refresh token, which never expires by time; rotation with reuse detection (IDENTITY-ADR-0002) limits it, and game frontends stay isolated in their frame with no access to the shell's storage.
5. **Durations.** Access: 15 minutes. Account family: 30 days of inactivity since the last successful refresh and 90 days absolute since the login that opened it; renewal moves the first, never the second. Guest family: no time limit; it ends by logout, reuse detection or revocation. A family is one browser or device; one User may have several.
6. **Retry recovery (precision of IDENTITY-ADR-0002 point 2).** Each refresh carries a client-generated `request_id`. Consuming the current refresh token, issuing its successor and recording the result is one atomic transaction per family. Presenting the predecessor again with the same `request_id` within 30 seconds, while its successor is still current, returns the same result (kept encrypted with a key outside the database) without another rotation and without extending any limit, after re-checking the family and the User. A recognized duplicate whose successor was already superseded is answered `refresh_superseded`, without secrets and without revoking. Any other presentation of a consumed refresh token (another `request_id`, or outside the window) revokes the family, as IDENTITY-ADR-0002 decided. Knowing a `request_id` alone recovers nothing. Consumed refresh hashes are kept as long as their family can be active, so reuse stays detectable for guests.
7. **Guest creation.** `POST /v1/guests` creates a distinct User, family and credentials per idempotency key in one transaction; the name is neither unique nor a way to sign in. The same key and name within 5 minutes, while the family is active and its first refresh token not yet rotated, returns the same response (kept encrypted); after that a replay neither returns those secrets nor creates another User. The key is a bootstrap secret: never logged, traced or sent to telemetry.
8. **Login completion with a one-time code.** Starting a login, the client generates a random verifier, keeps it, and sends only its S256 hash. After the provider's callback succeeds, the backend redirects the browser to the client's sign-in completion route with a single-use code valid for about 60 seconds, never with tokens. The client exchanges code and verifier for the credentials in a response body. A code taken from the URL is useless without the verifier, and a login started by someone else cannot be completed in the victim's browser. The redirect target is chosen by the server from configuration (only the web now; the mobile app's deep link later, the RFC 8252 pattern).

**Clarified 2026-10-27 (decided by the human).** "A code taken from the URL is useless without the verifier" holds for logins the person started. A login started by an attacker (with their own verifier) and completed by a victim leaves the code in the victim's browser; it is exploitable only if it leaves that browser within its validity. The human decided not to bind the login attempt to the browser with a cookie, and instead requires that the code never leaves it: the completion route removes it from the URL before loading anything else, sends no referrer, loads no third-party resources, and its query strings are kept out of every log.
9. **Explicit identity switch.** Signing in with an account while holding a guest is a switch, not a conversion: the completion may present the guest's refresh token, and that family is revoked in the same transaction that opens the account's family; the guest's participations are not transferred. Failing or cancelling keeps the guest. Upgrade and reconciliation stay deferred (IDENTITY-ADR-0001).
10. **Operation authentication levels.** Every operation a client calls through `Authorize` has a minimum level on the ordered scale `anonymous < guest < account`, kept in one catalog. `Authorize` decides with it (an operation outside the catalog is refused), and the same catalog is published to clients so they can show operations above the caller's level and redirect to the most flexible authentication the operation allows. It states authentication only: role capabilities (a future, separate answer) and each domain's object-level permissions (for example, Play's creator check) are unchanged. Clients treat it as guidance, never as authorization.
11. **Failures are not invalid credentials.** A database or provider failure while authorizing or renewing is a transient service error, never `token_invalid` or a revocation.

**Amended 2026-10-27 (decided by the human): credentials redesigned.** This replaces points 3 and 6, the token storage of points 4 and 7, and the reuse rule of IDENTITY-ADR-0002 point 2:

- **Refresh tokens are stored as they are** (random, not an encrypted payload) in `identity_sessions`, one row per token with its expiry; a row is active until it is deactivated.
- **Renewing does not rotate the refresh token.** A refresh request presents only the refresh token; while its row is active and unexpired it gets a new access token and keeps its refresh token. Only when the active refresh token is more than 5 hours old is it rotated: a new row is created and the old one is deactivated (never updated in place), so a session writes at most one row every 5 hours. An account's inactivity window therefore moves forward only when its refresh token rotates.
- **The previous token keeps working for 5 minutes.** A refresh token that is not active but is the one just replaced, deactivated less than 5 minutes ago, gets a new access token and the active refresh token. Any other inactive, expired or unknown refresh token is refused, without closing the session.
- **Access tokens are signed JWTs**, never stored: the user's identifier, level and display name, and the expiry (15 minutes). Every authorized request verifies the signature with the server's key and reads nothing else, so all of them are valid until their expiry; signing out deactivates the session (no further renewal) but an access token already issued keeps working until it expires.
- **No retry-recovery mechanism**: a lost renewal answer is harmless while nothing rotated, and the 5-minute window covers a lost rotation; guest creation replays the same session's active refresh token for the same key and name within 5 minutes.
- With rotations this rare and the previous token accepted, browser tabs renewing at the same time cannot revoke each other: the clients need no cross-tab lock, only to read the latest stored refresh token before renewing.

## Rationale

- One stable `User` with accounts and external identities attached keeps every `UserUUID` other domains stored valid, and leaves room for a later guest upgrade without changing it.
- Google alone avoids passwords, email verification and recovery now while still letting a person recover their account on another device; it costs no per-login fee in the direct integration.
- Opaque access tokens make revocation effective on the next request and avoid key distribution for signed access tokens; the price is one indexed read per request or handshake, to be measured. IDENTITY-ADR-0002's rationale for short access tokens (avoiding a revocation check per request) no longer applies as written: with opaque tokens revocation is checked anyway, and the 15-minute lifetime now bounds what a token copied from a log, a proxy or a device is worth and keeps the token table small.
- A client-held refresh token gives the web and the mobile app the same contract and removes the cookie's same-origin, CSRF and late-`Set-Cookie` concerns, at the cost of the XSS exposure accepted in point 4.
- The one-time code with a client verifier is the standard way to finish a redirect login without putting tokens in a URL or a cookie, and it works unchanged for a native app.
- One catalog for authorization and for the clients keeps what the UI promises and what the server enforces from drifting apart.

## Alternatives Considered

### Refresh token in an `HttpOnly` cookie (the plan's proposal)

Rejected by the human: safer against injected scripts, but it needs same-origin deployment or credentialed CORS, CSRF checks, and a second transport for the mobile app.

### Signed (JWT) access tokens

Not chosen now: no read per request, but revocation waits for expiry unless a revocation list is checked, and signing keys must be managed. Reconsider if the per-request read becomes a measured problem.

### Identity Platform or another managed authentication service

Not needed: a different product with its own pricing; Playhoot keeps users and credentials itself.

### Tokens in the redirect URL after login

Rejected: URLs end up in history, logs, `Referer` and extensions.

### Passwords or magic links as the first account method

Postponed: email delivery, verification and recovery are not needed to unblock the current tests.

## Consequences

- Identity owns accounts, external identities, credential families, access and refresh tokens, login attempts, the operation catalog and their persistence.
- The fixed test tokens disappear as an authorization bypass of the normal server.
- Revocation does not close WebSockets already admitted, nor end running operations; an expiring access token does not end an open connection.
- Guests that lose their stored refresh token lose that identity for good in this phase; the UI says so on logout.
- A future cross-origin web deployment needs nothing new from Identity; a future mobile app adds a redirect target and its own storage.

## Canonical Knowledge Impact

- `identity/README.md` - Identity owns accounts, external identities, credentials and the operation catalog; the "Does Not Own" limits on authentication/provider details and authorization policy are narrowed accordingly.
- `identity/docs/decisions/IDENTITY-ADR-0002-token-renewal-with-rotating-refresh-tokens.md` - amended: point 6 decided by this record; point 2 made precise by point 6 above; rationale of short access tokens restated.
- `identity/docs/decisions/INDEX.md` - this record.
- `ARCHITECTURE.md` - the Identity section no longer leaves authentication/account/provider systems and authorization policy unresolved.
- `identity/CURRENT_STATE.md` - its expectation of JWT validation is replaced by the opaque format.
- Frontend repository: ADR-0009 (where the refresh token lives, login completion), through its own workflow.

## Implementation Impact

Built by the Project `docs/projects/active/identity-v1/` (WORK-0057 to WORK-0062); the client-facing contract is WORK-0056's approved design.
