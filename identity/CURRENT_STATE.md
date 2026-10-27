# Identity - Current State

Status: CURRENT IMPLEMENTATION

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| Operation authentication levels | IMPLEMENTED | 2026-10-27: the catalog `Authorize` decides with, published by `GET /v1/identity/operations` (see Placeholders for the credentials it is applied to). |
| User Identity | NOT IMPLEMENTED | No persistence, real Users, accounts, real credentials or profile/display behavior is implemented. |

## Placeholders (wired, but not real behavior)

- `identity/usecases/authorize` (`Authorize`) is called by the API's authorization middleware on every protected route, but it does not validate tokens: it only recognizes hardcoded test tokens (`test-user-1`, `test-user-2` -> fixed Users with the display names "Test user 1" and "Test user 2", allowed; `test-guest` -> a fixed User named "Guest", level `guest`, forbidden from the operations that require an account, creating a Session and joining one as admin, and allowed the ones that require a guest; anything else unauthenticated). It returns `identity.User` (UserUUID and display name, 2026-10-20); every guest is still the same User, so two guests cannot share a Session. Real behavior (look up the opaque access token, resolve the stable UserUUID, decide per operation by its minimum authentication level, IDENTITY-ADR-0003; creating a Session and joining one as admin require an account, not a guest) is not implemented and is outside the current Session Runtime project.
- `identity/identity.go` declares the `Operation`, `Level` and `AccessDecision` values that capability uses. Since 2026-10-27 (IDENTITY-ADR-0003) `Authorize` decides by the operation catalog (`identity/internal/business/operationlevel`: each operation's minimum level on `anonymous < guest < account`; the test users are `account`, `test-guest` is `guest`; an operation outside the catalog is forbidden and alerts), and `identity/usecases/listoperations` lists it for `GET /v1/identity/operations`. The catalog is real; the credentials it is applied to are still the fixed test tokens. `VIEW_OWN_IDENTITY` is declared (guest) for the future `me` endpoint.

- `identity/usecases/createguest` (`CreateGuest`) is called by `POST /v1/guests`, but returns the same hardcoded test guest (token `test-guest`) for every call and stores nothing. Real behavior: create a guest User with the display name given, idempotently per key (a reused key with another name is `identity.ErrIdempotencyConflict`), validate the name (`invalid_display_name`), and issue a real credential that `Authorize` accepts.

## Known Drift

- Identity has an accepted domain model, but no implementation exists yet.

## Current Gaps

- Real authentication (access and refresh tokens) is not built. Its accepted direction is `identity/docs/decisions/IDENTITY-ADR-0002-token-renewal-with-rotating-refresh-tokens.md` (2026-10-25): short access tokens, rotating refresh tokens renewed on demand, accounts that expire by inactivity and an absolute maximum, guests that never expire; built by the Project `docs/projects/active/identity-v1/` (created 2026-10-25, before the SDK built-ins).

- `Identity.User` and `UserUUID` are accepted canonical concepts but are not implemented.
- Guest-to-registered continuity is accepted domain behavior but is not implemented.
- Public create/resolve capabilities for `Identity.User` are not implemented.
