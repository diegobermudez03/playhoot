# WORK-0057: Persisted identity core

Status: DONE
Created: 2026-10-25
Last status change: 2026-10-27 (decided by the human in conversation under the working agreement; designed, implemented and verified the same day)

Related decisions:
- `identity/docs/decisions/IDENTITY-ADR-0001-identity-user-public-identity-boundary.md`
- `identity/docs/decisions/IDENTITY-ADR-0002-token-renewal-with-rotating-refresh-tokens.md`
- The decisions recorded by WORK-0056

Canonical context:
- `plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx` (§3, §5, §6, §8)
- `docs/engineering/standards/database-constraints.md`, idempotency standard

## Outcome

Identity's domain capabilities on real persistence: stable Users (guest and registered in the same entity), accounts and unique external identity bindings, credential families with access and refresh issuance, authorization of operations, atomic refresh rotation with reuse detection and short retry recovery, revocation, and guest-creation idempotency. Every endpoint of the later WORK builds on these; today Identity has no tables and no migrations wired at startup (plan §1).

## Context

The plan's starting model (§3 "Modelo de partida") lists the responsibilities: users, accounts, external identities, auth sessions (families), access tokens, refresh tokens, OIDC attempts and browser context, temporary results. Under IDENTITY-ADR-0003 the browser-context binding (a cookie) no longer exists: a login attempt is bound to the client by its verifier, and completes through a one-time code (WORK-0056). Names describe responsibilities; tables may be adjusted keeping uniqueness, atomicity and contracts. Per the repository standard, the database holds only keys, unique constraints, indexes and types; every other rule lives in the application and its transactions.

Authorization keeps the existing operation matrix (§8), but an unknown operation must be refused (the placeholder allows anything not on its deny list), and a database failure is a transient service error, never an invalid credential.

## Scope

Credentials as redesigned by the human in conversation on 2026-10-27 (recorded in IDENTITY-ADR-0003's amendment): access tokens are HS256 JWTs (user, expiry, level, display name) verified with the key alone; refresh tokens are opaque and stored as-is in `identity_sessions`; refresh takes only the refresh token, returns a new access token and keeps the refresh token unless it was issued over 5 hours ago, when a new row replaces it and the old one is deactivated (never updated in place); the immediate predecessor deactivated under 5 minutes ago still answers with the current refresh token; anything else stale is just rejected. Executed as one vertical slice with WORK-0058 and the frontend's identity-web-v1 WORK-0001/0002 ("real guests end to end"), with `test-user-1`/`test-user-2` kept as a development-only bypass (`IDENTITY_DEV_TEST_USERS`, refused in production) until WORK-0059. Accounts and their sign-in are WORK-0059; this WORK builds the guest half of the core plus the credential lifecycle shared by both.

## Acceptance Criteria

Met: persisted Users, sessions and guest idempotency claims (migration `20261029000000_identity_core`); issue, verify (strict expiry), refresh with rotation and predecessor grace under row locks, logout; concurrent refresh and concurrent guest creation covered by Postgres tests (`refreshcredentials`, `createguest` integration tests).

## Blockers

None.

## Completion Record

- Implementation: `identity/internal/business/{accesstoken,refreshtoken,displayname}`, `identity/internal/migrations`, `identity/migration`, `identity/usecases/{createguest,refreshcredentials,endsession}`, `Authorize` verifying JWTs (`identity/usecases/authorize`), `main.go` (`IDENTITY_ACCESS_TOKEN_KEY`, `IDENTITY_DEV_TEST_USERS`).
- Verification: `go build ./...`, `go vet`, identity and api tests with the local Postgres (no skips); browser walkthrough against the running server (see WORK-0058).
- Independent review: CHANGES_REQUIRED (frontend renewal overtaken by another tab, no retry of a failed renewal, fast device clock, spent guest key reused; backend logout racing a rotation, issue-time clock skew, missing endsession test, the guest-creation repo owning its transaction and idempotency decision); all fixed (the backend now also returns `access_expires_in`; guest creation's transaction and decision moved into the service, the repo only claims and writes). Re-review APPROVED. Left as non-blocking: raw SQL inserts, the logout race test relies on a sleep.
- Docs: `identity/CURRENT_STATE.md`, IDENTITY-ADR-0003 amendment, `api/README.md`, `session-runtime-v1` CHECKLIST (A4, Placeholders).
