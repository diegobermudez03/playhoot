# WORK-0056: Identity contracts and decisions

Status: DONE
Created: 2026-10-25
Last status change: 2026-10-27 (READY, IMPLEMENTING and DONE the same day)

Related decisions:
- `identity/docs/decisions/IDENTITY-ADR-0001-identity-user-public-identity-boundary.md`
- `identity/docs/decisions/IDENTITY-ADR-0002-token-renewal-with-rotating-refresh-tokens.md`
- `identity/docs/decisions/IDENTITY-ADR-0003-basic-identity-google-accounts-opaque-access-client-held-refresh.md` (ACCEPTED 2026-10-27 through this WORK)

Canonical context:
- `plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx` (§2-§8)
- `identity/README.md`, `identity/CURRENT_STATE.md`, `api/README.md`, `api/internal/auth`
- Frontend repository: `docs/decisions/architecture/ADR-0009-credential-renewal-in-the-transport.md`

## Outcome

The plan's decisions recorded in their owners, and the client-facing contract every later WORK and the frontend implement, fixed before any code, so no WORK decides contracts on its own. Its implementation is documentation only: IDENTITY-ADR-0003 accepted, IDENTITY-ADR-0002 amended, canonical Identity knowledge synchronized. The contract below is the approved design that WORK-0057 to WORK-0062 implement and that `api/README.md` documents only once each endpoint exists.

## Context

Human decisions: the plan as a whole (2026-10-25) except the refresh token location, client-held and sent in the body (2026-10-25); the login completion with a one-time code and a client verifier (2026-10-27); the operation catalog with minimum authentication levels, public, with the caller's level in the refresh and `me` responses (2026-10-27).

Today's conventions the contract keeps: everything under `/v1`; errors are `{"code"?, "message"}` (`api/internal/httpx`); protected routes answer `401 token_required | token_malformed | token_invalid` and `403 operation_forbidden` (`api/internal/auth`); WebSockets carry the bearer in the `base64url.bearer.authorization.playhoot.<token>` subprotocol.

## Scope

### In Scope

- IDENTITY-ADR-0003 and the amendment of IDENTITY-ADR-0002 (on the human's approval).
- The client-facing contract below.
- Synchronizing `identity/README.md`, `identity/CURRENT_STATE.md` (the JWT expectation), `ARCHITECTURE.md` (Identity section) and the Identity ADR index.

### Out of Scope

- Any code, migration or endpoint (WORK-0057 to WORK-0062).
- The persistence layout (WORK-0057 designs it within ADR-0003).
- The frontend's records (its WORK-0001, through its own workflow).
- Rate-limit values (WORK-0061).

## Approved Design

### Shared shapes

Credentials, returned by every endpoint that opens or renews a family:

```json
{
  "user": { "user_uuid": "…", "display_name": "Ana", "level": "guest" },
  "access_token": "pha_…",
  "access_expires_at": "2026-10-27T15:04:05Z",
  "refresh_token": "phr_…"
}
```

- `level` is `guest` or `account` (an authenticated caller is never `anonymous`). Clients tolerate unknown values.
- Token formats: a recognizable prefix (`pha_` access, `phr_` refresh) plus 256 random bits, base64url. Only hashes are stored. The prefix helps people and secret scanners; the server decides by the stored record, never by the prefix.
- Responses carrying credentials send `Cache-Control: no-store`.

Levels: `anonymous < guest < account`.

### Endpoints

| Endpoint | Auth | Request | Success | Failures (code) |
|---|---|---|---|---|
| `GET /v1/identity/operations` | none | — | `200 {"operations":[{"operation":"CREATE_SESSION","required_level":"account"},…]}`, cacheable (`Cache-Control: public, max-age=300`) | — |
| `POST /v1/guests` | none | `{display_name, idempotency_key}` (UUID, as today) | `201` credentials, level `guest` | `400 invalid_display_name`; `409 idempotency_conflict` (same key, other name); `409 idempotency_key_spent` (replay after the 5-minute window or after the family moved on: no secrets, no new User) |
| `POST /v1/identity/refresh` | refresh token in body | `{refresh_token, request_id}` (UUID) | `200` credentials | `401 refresh_invalid` (unknown, malformed, revoked, expired, or reuse detected, which revokes the family); `409 refresh_superseded` (recognized duplicate already superseded: re-read the stored token and retry with a new `request_id`); `409 request_id_spent` (current token with a spent `request_id`: retry with a new one; nothing revoked) |
| `GET /v1/identity/me` | access (operation `VIEW_OWN_IDENTITY`, guest) | — | `200 {"user":{…}}` | the usual `401`/`403` |
| `POST /v1/identity/logout` | refresh token in body | `{refresh_token}` | `204`: the family of the presented token (current or an earlier one of the same family) is revoked; already revoked or unknown also `204` | — |
| `POST /v1/identity/login/start` | none | `{provider, code_challenge}` (`provider`: `google`, or `local` only where enabled; `code_challenge`: S256 of the client's verifier) | `200 {"authorization_url":…}`; the client navigates the top-level page to it | `400 provider_unavailable` |
| `GET /v1/identity/login/{provider}/callback` | none (provider redirect) | provider's `code`, `state` | `302` to the configured completion route with `?code=<one-time>` | `302` to the same route with `?error=cancelled\|failed\|unavailable` |
| `POST /v1/identity/login/complete` | none | `{code, code_verifier, replaces_refresh_token?}` | `200` credentials, level `account`; a presented `replaces_refresh_token` has its family revoked in the same transaction | `400 login_code_invalid` (unknown, used, expired or verifier mismatch: start the login again) |

Every endpoint above may answer `503 service_unavailable` for a transient failure (database, provider); a client never treats it as an invalid credential. Protected routes keep today's `401`/`403` codes; an access token past its expiry is `token_invalid`, which the client answers by refreshing (ADR-0009).

### Rules the contract fixes

- **Retry recovery:** within 30 s, the predecessor refresh token with the same `request_id` gets the same credentials (not rotated again, limits not extended), only while the successor is still current; ADR-0003 point 6.
- **Guest replay:** the same key and name within 5 minutes, while the family is active and its first refresh token not rotated, gets the same `201` body; ADR-0003 point 7.
- **Login attempt:** valid 5 minutes, consumed once; a repeated callback never exchanges the provider code twice nor opens a second family. The one-time code lives about 60 s and is single use.
- **Completion route:** the redirect target comes from server configuration (`WEB_SIGN_IN_COMPLETE_URL` or equivalent), never from the request; the client keeps its intended path itself.
- **Operation catalog:** starting values `CREATE_SESSION` account, `JOIN_SESSION_AS_ADMIN` account, `JOIN_SESSION_AS_PLAYER` guest, `REPORT_CLIENT_TELEMETRY` guest, `VIEW_OWN_IDENTITY` guest. Public endpoints (guest creation, login, refresh, logout) do not go through `Authorize` and are not listed.
- **Never logged or sent to telemetry:** access and refresh tokens, `idempotency_key`, `request_id` values, provider codes, one-time codes, verifiers, client secret.

### What the client-held refresh token simplified (relative to the plan)

- No cookies at all: neither a refresh cookie nor a login-attempt cookie. The verifier binds the completion to the tab that started it, which also covers login CSRF.
- No family/generation descriptor in responses: logout and identity switch present the refresh token itself, which names exactly one family, so a stale logout can never revoke a different identity.
- Retry recovery only needs the predecessor: the client cannot hold the successor without having received the body.

## Constraints and Invariants

- IDENTITY-ADR-0001: `UserUUID` stays the only public reference; no consumer sees provider identifiers, token records or account rows.
- Names and codes above are backend contract values: clients must tolerate unknown ones (frontend standard `backend-contract-values.md`).

## Acceptance Criteria

- IDENTITY-ADR-0003 is ACCEPTED and IDENTITY-ADR-0002 carries its amendment note.
- `identity/README.md`, `ARCHITECTURE.md` (Identity) and the ADR index state the accepted ownership; `identity/CURRENT_STATE.md` no longer expects JWT validation and still states nothing is implemented.
- Every later WORK of `identity-v1` references this contract instead of redefining it.

## Implementation Freedom

Wording and placement of the documentation changes.

## Verification

Review of the documents against the human's decisions; no code.

## Documentation Impact

### Accepted / Canonical Knowledge

- `identity/docs/decisions/IDENTITY-ADR-0003-…` - PROPOSED -> ACCEPTED.
- `identity/docs/decisions/IDENTITY-ADR-0002-…` - amendment: point 6 decided, point 2 made precise, rationale restated (by reference to ADR-0003).
- `identity/docs/decisions/INDEX.md`, `identity/README.md`, `ARCHITECTURE.md` (Identity section).

### Current-State Documentation After Implementation

- `identity/CURRENT_STATE.md` - only the JWT expectation sentence; implementation status unchanged.

### Intentionally Unchanged

- `api/README.md` and the frontend's `BACKEND_CONTRACT.md`: they describe what exists; each endpoint is added there by the WORK that builds it.

## Blockers

- None. The human approved the design, including the three simplifications and the minor choices listed in the review (prefixes, `VIEW_OWN_IDENTITY`, error codes, 5-minute catalog cache), on 2026-10-27.

## Completion Record

- Implementation (documentation only): IDENTITY-ADR-0003 ACCEPTED and indexed; IDENTITY-ADR-0002 carries a dated amendment note referencing it; `identity/README.md` (Owns / Does Not Own), `ARCHITECTURE.md` (Identity section and the ownership line of the API section) and the JWT expectation in `identity/CURRENT_STATE.md` synchronized; `api/README.md` intentionally unchanged.
- Verification: independent review (fresh read-only agent) CHANGES_REQUIRED with three findings (an `ARCHITECTURE.md` line still leaving authentication unowned; WORK-0062 restating the catalog differently; stale tracking), all fixed; re-review APPROVED with no findings. No code, so no tests.
- Routed, non-blocking: login-code leak mitigations (WORK-0060, frontend WORK-0004), `replaces_refresh_token` edge cases (WORK-0059), the removed Set-Cookie case (WORK-0058), the client's cross-tab serialization obligation (frontend WORK-0001). Open for the human: ADR-0003 point 8 says a code taken from the URL is useless without the verifier, true only for logins the victim started; the mitigations above cover the remaining case.
- No deviation from the approved design.
