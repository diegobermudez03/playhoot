# Identity - Current State

Status: CURRENT IMPLEMENTATION

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| User Identity | NOT IMPLEMENTED | No Identity implementation, persistence, public capabilities, authentication/account behavior, or profile/display behavior is currently documented as implemented. |

## Placeholders (wired, but not real behavior)

- `identity/usecases/authorize` (`Authorize`) is called by the API's authorization middleware on every protected route, but it does not validate tokens: it only recognizes hardcoded test tokens (`test-user-1`, `test-user-2` -> fixed Users with the display names "Test user 1" and "Test user 2", allowed; `test-guest` -> a fixed User named "Guest", forbidden from the account-only operations, creating a Session and joining one as admin, and allowed to join one as a player; anything else unauthenticated). It returns `identity.User` (UserUUID and display name, 2026-10-20); every guest is still the same User, so two guests cannot share a Session. Real behavior (validate the JWT, resolve the stable UserUUID, decide per operation; creating a Session and joining one as admin require an account, not a guest) is not implemented and is outside the current Session Runtime project.
- `identity/identity.go` declares the `Operation` and `AccessDecision` values that capability uses. `REPORT_CLIENT_TELEMETRY` (2026-10-21, for `POST /v1/client-telemetry`) is allowed to guests.

- `identity/usecases/createguest` (`CreateGuest`) is called by `POST /v1/guests`, but returns the same hardcoded test guest (token `test-guest`) for every call and stores nothing. Real behavior: create a guest User with the display name given, idempotently per key (a reused key with another name is `identity.ErrIdempotencyConflict`), validate the name (`invalid_display_name`), and issue a real credential that `Authorize` accepts.

## Known Drift

- Identity has an accepted domain model, but no implementation exists yet.

## Current Gaps

- Real authentication (access and refresh tokens) is not built. Its accepted direction is `identity/docs/decisions/IDENTITY-ADR-0002-token-renewal-with-rotating-refresh-tokens.md` (2026-10-25): short access tokens, rotating refresh tokens renewed on demand, accounts that expire by inactivity and an absolute maximum, guests that never expire; built by the Project `docs/projects/active/identity-v1/` (created 2026-10-25, before the SDK built-ins).

- `Identity.User` and `UserUUID` are accepted canonical concepts but are not implemented.
- Guest-to-registered continuity is accepted domain behavior but is not implemented.
- Public create/resolve capabilities for `Identity.User` are not implemented.
