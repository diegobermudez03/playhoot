# Identity - Current State

Status: CURRENT IMPLEMENTATION

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| User Identity | NOT IMPLEMENTED | No Identity implementation, persistence, public capabilities, authentication/account behavior, or profile/display behavior is currently documented as implemented. |

## Placeholders (wired, but not real behavior)

- `identity/usecases/authorize` (`Authorize`) is called by the API's authorization middleware on every protected route, but it does not validate tokens: it only recognizes hardcoded test tokens (`test-user-1`, `test-user-2` -> fixed UserUUIDs, allowed; `test-guest` -> forbidden from the account-only operations, creating a Session and joining one as admin; anything else unauthenticated). Real behavior (validate the JWT, resolve the stable UserUUID, decide per operation; creating a Session and joining one as admin require an account, not a guest) is not implemented and is outside the current Session Runtime project.
- `identity/identity.go` declares the `Operation` and `AccessDecision` values that capability uses.

## Known Drift

- Identity has an accepted domain model, but no implementation exists yet.

## Current Gaps

- `Identity.User` and `UserUUID` are accepted canonical concepts but are not implemented.
- Guest-to-registered continuity is accepted domain behavior but is not implemented.
- Public create/resolve capabilities for `Identity.User` are not implemented.
