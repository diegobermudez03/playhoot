# Identity - Current State

Status: CURRENT IMPLEMENTATION

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| Operation authentication levels | IMPLEMENTED | 2026-10-27: the catalog `Authorize` decides with (`identity/internal/business/operationlevel`, `anonymous < guest < account`; an uncataloged operation is forbidden and alerts), published by `GET /v1/identity/operations`. |
| Guest Users | IMPLEMENTED | 2026-10-27: `POST /v1/guests` (`identity/usecases/createguest`) persists a distinct guest per idempotency key with the normalized display name and opens its credential session; a reused key with another name is `identity.ErrIdempotencyConflict`; a key answers again only within 5 minutes (`idempotency_key_spent` after). |
| Credentials | IMPLEMENTED | 2026-10-27 (IDENTITY-ADR-0003 as amended the same day): access tokens are HS256 JWTs (`identity/internal/business/accesstoken`, 15 minutes, claims: subject UserUUID, expiry, level, display name) verified by `Authorize` with the key alone, reading nothing; refresh tokens are opaque (`phr_` + 32 random bytes) stored as-is in `identity_sessions`. |
| Refresh | IMPLEMENTED | 2026-10-27: `POST /v1/identity/refresh` (`identity/usecases/refreshcredentials`) takes only the refresh token. An active, unexpired session gets a new access token and keeps its refresh token, unless it was issued over 5 hours ago: then a new session row replaces it (`replaced_by_id`) and the old one is deactivated, never updated in place. The immediate predecessor deactivated less than 5 minutes ago answers with a new access token and the current refresh token; anything else is `refresh_invalid` (401). Guests never expire; account sessions expire after 30 days unused and 90 days from sign-in (`identity/internal/business/refreshtoken`). |
| Logout | IMPLEMENTED | 2026-10-27: `POST /v1/identity/logout` (`identity/usecases/endsession`) deactivates the refresh token's session; idempotent, always 204. Access tokens already issued stay valid until they expire. |
| Own identity | IMPLEMENTED | 2026-10-27: `GET /v1/identity/me` returns the caller's User from its access token (`VIEW_OWN_IDENTITY`, guest). |
| Accounts | NOT IMPLEMENTED | `identity_accounts` exists; no sign-in creates one yet (local provider in WORK-0059, Google in WORK-0060). |

## Persistence

Migration `identity/internal/migrations/20261029000000_identity_core.go`, registered by `migrations.go` through `identity/migration`: `identity_users`, `identity_accounts`, `identity_sessions` (one row per issued refresh token; a partial unique index keeps one active row per `session_uuid`), `identity_guest_requests` (idempotency claims). Only keys, unique constraints and indexes, per the database-constraints standard.

## Placeholders (wired, but not real behavior)

- Development bypass: with `IDENTITY_DEV_TEST_USERS=true` (refused at startup when `ENVIRONMENT=production`), `Authorize` also accepts the fixed tokens `test-user-1` and `test-user-2` as the accounts "Test user 1" and "Test user 2". It is the only way to act as an account until WORK-0059; it goes away with it. The former `test-guest` token no longer exists.
- `IDENTITY_ACCESS_TOKEN_KEY` (at least 32 bytes) is required in production; elsewhere an unset key is replaced by a random one per process, so access tokens die with the process and clients renew them with their refresh token.

## Current Gaps

- Accounts and their sign-in (local simulated provider, then Google), guest-to-account continuity, and identity reconciliation are not implemented.
- Revocation does not reach access tokens already issued nor open WebSockets (out of scope of identity-v1).
