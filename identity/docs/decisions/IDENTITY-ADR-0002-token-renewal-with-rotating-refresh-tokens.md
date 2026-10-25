# IDENTITY-ADR-0002: Token Renewal With Short Access Tokens and Rotating Refresh Tokens

Status: ACCEPTED
Created: 2026-10-25
Last status change: 2026-10-25
Supersedes: None
Superseded by: None

## Context

Identity's authorization is a placeholder: `Authorize` accepts fixed test tokens and `POST /v1/guests` returns a fixed guest (`identity/CURRENT_STATE.md`). Real authentication is a project of its own, planned by the human right after the Session lifecycle and the Playhoot SDK built-ins. The web frontend already needs to know how a refused credential is handled: the human decided that a person never sees an authorization error; the client renews its credential in the background and only goes back to sign-in when renewal is impossible. Playing does not require an account: a guest user plays like any other user.

## Decision

Accepted by the human on 2026-10-25, from the frontend agent's proposal, as the direction the authentication project implements:

1. **Short access token.** The token sent on every request and WebSocket handshake lives about 15 minutes.
2. **Rotating refresh token.** Renewing returns a new access token and a new refresh token; the used refresh token stops working. Presenting an already used refresh token is treated as theft: the whole token family is revoked.
3. **Renew on demand.** The client renews when the backend refuses the access token as expired, or just before opening a live connection with a token about to expire (a refused WebSocket handshake cannot be read by a browser). Not on every page navigation: renewing while the person is active already extends the session.
4. **Accounts expire.** An account's refresh token has an inactivity window (starting value: 30 days without use) and an absolute maximum (starting value: 90 days). Past either, the person signs in again.
5. **Guests do not expire.** A guest's refresh token has no expiry: the guest lives in that browser or device as long as its storage is kept. Upgrading a guest to an account (future product flow) starts the account's expiry rules.
6. **Open for the authentication project:** where the client keeps the refresh token (an `httpOnly` cookie is safer against script access than browser storage, but changes CORS with credentials and how the mobile app sends it), the exact durations, and the sign-in screen and flows.

## Rationale

- Short access tokens limit what a leaked token can do without checking a revocation list on every request.
- Rotation makes a stolen refresh token detectable the moment either party uses it again.
- On-demand renewal gives the "stay signed in while active" behavior without extra requests per navigation.
- Guests must never lose their identity to an expiry they cannot recover from: they have no password to sign in again.

## Alternatives Considered

### Renewing the refresh token on every route change

Rejected: on-demand renewal already extends an active session; renewing per navigation adds requests and races between tabs for no gain.

### Long-lived access tokens without refresh

Rejected: a leaked token stays valid for its whole life, and revoking it needs a check on every request.

## Consequences

- Identity owns access/refresh token issuance, rotation, revocation and their persistence (token families, last use).
- Every API and live handshake keeps validating the access token; the frontend's transport renews and retries transparently (frontend ADR-0009).
- The frontend already has the renewal middleware with a placeholder that never renews, so today a refused credential is forgotten as before.

## Canonical Knowledge Impact

- `identity/CURRENT_STATE.md`: the accepted direction for real authentication.
- Frontend repository: ADR-0009 (client-side renewal) and `docs/architecture/BACKEND_CONTRACT.md` once the endpoints exist.

## Implementation Impact

None now. The authentication project builds the refresh endpoint, token persistence and rotation, the guest and account rules above, and decides the open points.
