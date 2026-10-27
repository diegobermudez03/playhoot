# WORK-0062: Operation authentication levels catalog

Status: DONE
Created: 2026-10-27
Last status change: 2026-10-27 (READY by the human in conversation, under the working agreement that decisions are made in chat and code not objected to is approved; IMPLEMENTING and DONE the same day)

Related decisions:
- `identity/docs/decisions/IDENTITY-ADR-0003-basic-identity-google-accounts-opaque-access-client-held-refresh.md` (point 10)
- WORK-0056's approved contract ("Operation catalog", `GET /v1/identity/operations`)

Canonical context:
- `identity/README.md`, `identity/CURRENT_STATE.md`, `identity/identity.go`, `identity/usecases/authorize`
- `api/README.md`, `api/identity/`, `api/internal/auth`
- Standards: `domain-logic-placement.md`, `data-integrity.md`, `testing.md`, `code-comments.md`

## Outcome

Every operation that goes through `Authorize` declares its minimum authentication level in one catalog owned by Identity. `Authorize` decides with it, and the public `GET /v1/identity/operations` publishes it, so the frontend can show operations above the caller's level and redirect to the most flexible authentication they allow (frontend `identity-web-v1` WORK-0006). It needs no persistence, so it ships against the current placeholders and unblocks the frontend's redirection pattern before real credentials exist.

## Context

Today the rule is a deny list inside the placeholder (`accountOnlyOperations` in `identity/usecases/authorize/service.go`): anything not listed is allowed, including an operation nobody declared. The human decided (2026-10-27) that this is an authentication level per operation, distinct from future role capabilities and from each domain's object-level permissions, and that the catalog is public while the caller's own level comes with its credentials (refresh and `me`, WORK-0058).

## Scope

### In Scope

- The level scale and the catalog in Identity, and its use by `Authorize`.
- `GET /v1/identity/operations`.
- The new operation `VIEW_OWN_IDENTITY` declared and listed (its endpoint `me` is WORK-0058).

### Out of Scope

- Real credentials, token lookup and the caller's level in responses (WORK-0057, WORK-0058); `Authorize` keeps recognizing only the fixed test tokens.
- Role capabilities; Play's creator and participation checks (unchanged).
- The frontend's consumption (its WORK-0006).

## Approved Design

- **Public contract (`identity/identity.go`):** `Level` with `LevelAnonymous`, `LevelGuest`, `LevelAccount` (wire values `anonymous`, `guest`, `account`); `OperationLevel{Operation, RequiredLevel}`; the constant `OperationViewOwnIdentity` (`VIEW_OWN_IDENTITY`).
- **Rule:** a pure business package in Identity holds the catalog and the comparison (a level satisfies a requirement when it is equal or higher on `anonymous < guest < account`). Starting values, from WORK-0056: `CREATE_SESSION` account, `JOIN_SESSION_AS_ADMIN` account, `JOIN_SESSION_AS_PLAYER` guest, `REPORT_CLIENT_TELEMETRY` guest, `VIEW_OWN_IDENTITY` guest.
- **`Authorize`:** resolves the token to a User and a level (placeholder: `test-user-1` and `test-user-2` account, `test-guest` guest) and allows the operation when the level satisfies the catalog's requirement, `AccessForbidden` otherwise. An operation missing from the catalog is a programming error: alert through the monitoring abstraction and answer `AccessForbidden` (fail closed; `data-integrity.md`). The decisions for today's four operations stay exactly as they are.
- **Use case:** listing the catalog is an independent Identity capability (`usecases/`), returning it in a stable order (by operation name).
- **Endpoint (`api/identity`):** `GET /v1/identity/operations`, no token, `200 {"operations":[{"operation":"…","required_level":"…"}]}`, `Cache-Control: public, max-age=300`. It goes through the same general request limit and REST CORS as every REST route; no new metric (the generic REST request metric covers it).

## Constraints and Invariants

- One source: the catalog the endpoint publishes is the one `Authorize` applies, never a copy.
- Every declared `identity.Operation` is in the catalog.
- Clients treat the catalog as guidance; the backend still decides every request.

## Acceptance Criteria

- `GET /v1/identity/operations` without a token answers `200` with the five operations and their levels, ordered by operation, and `Cache-Control: public, max-age=300`.
- A test fails if a declared operation is missing from the catalog.
- `Authorize` with the fixed tokens decides as today for `CREATE_SESSION`, `JOIN_SESSION_AS_ADMIN`, `JOIN_SESSION_AS_PLAYER` and `REPORT_CLIENT_TELEMETRY`; `test-guest` and the test users are allowed `VIEW_OWN_IDENTITY`; an operation outside the catalog is forbidden and alerts; an unknown token stays unauthenticated.
- The full backend test suite passes.

## Implementation Freedom

Package and file names, the business package's API shape, test layout (following `testing.md`), how the alert is worded.

## Verification

- Unit tests: the rule (level comparison, completeness), `Authorize` table, the use case, the handler (status, body, headers, no token required).
- `go build ./...`, `go vet ./...`, `go test ./...`; a manual `curl` against a local server.

## Documentation Impact

### Accepted / Canonical Knowledge

- None (decided by IDENTITY-ADR-0003).

### Current-State Documentation After Implementation

- `api/README.md` - the identity route group's new endpoint and its contract.
- `identity/CURRENT_STATE.md` - the catalog and the listing implemented; the `Authorize` placeholder now decides by level (still fixed test tokens).
- `docs/projects/active/session-runtime-v1/CHECKLIST.md` - the Identity authorization placeholder line (decides by level now).
- Frontend repository, through its own workflow: `docs/architecture/BACKEND_CONTRACT.md` gets the endpoint once it exists.

## Blockers

- None.

## Completion Record

- Implementation: `identity.Level`, `identity.OperationLevel`, `OperationViewOwnIdentity`; the catalog and level comparison in `identity/internal/business/operationlevel`; `Authorize` decides by it (test users account, `test-guest` guest; an uncataloged operation alerts and is forbidden); `identity/usecases/listoperations`; `GET /v1/identity/operations` in `api/identity` wired through `api.NewServer` and `main.go`.
- Verification: `go build ./...`, `go vet`, the full `go test ./...` with the local Postgres (all green); against a running server, the endpoint answered `200` with the five operations in order and `Cache-Control: public, max-age=300`, and `test-guest` creating a Session still got `403`.
- Independent review: CHANGES_REQUIRED for one record-only finding (this WORK did not record the human's approval), fixed here; no code finding. Taken from its non-blocking notes: the completeness test now fails explicitly on an Operation declared in a shape it cannot read; the `identity/CURRENT_STATE.md` status row reworded. Left as is: no test asserts the alert (`monitoring.Alert` has no seam, as elsewhere).
- Docs: `api/README.md`, `identity/CURRENT_STATE.md`, `session-runtime-v1` CHECKLIST placeholder line, frontend `BACKEND_CONTRACT.md` (Auth).
