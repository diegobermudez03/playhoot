# WORK-0058: Real guests and credential endpoints

Status: DONE
Created: 2026-10-25
Last status change: 2026-10-27 (decided by the human in conversation under the working agreement; implemented together with WORK-0057)

Related decisions:
- The decisions recorded by WORK-0056

Canonical context:
- `plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx` (§4 "Invitado", §6, §7)
- `api/README.md`, `api/identity/`

## Outcome

The API edge of real guests and of the credential lifecycle: `POST /v1/guests` creates a distinct persisted guest per creation attempt (idempotent per key, conflict on a reused key with another name), and the refresh, `me` and logout endpoints serve guests and accounts alike. With the frontend's counterpart, two guests from different browsers get different UserUUIDs, keep them across reloads and backend restarts, and can play the same Session (plan §10 stage 2).

## Context

Today `POST /v1/guests` returns the same UUID and `test-guest` for every call (`identity/usecases/createguest`), and the handler logs the idempotency key (`api/identity/create_guest.go`), which the plan treats as a bootstrap secret once real credentials exist: that logging must go before credentials are issued (§4). The fixed test tokens stop being an authorization bypass for the normal local server (§9).

## Scope

Credentials as redesigned by the human in conversation on 2026-10-27 (recorded in IDENTITY-ADR-0003's amendment): access tokens are HS256 JWTs (user, expiry, level, display name) verified with the key alone; refresh tokens are opaque and stored as-is in `identity_sessions`; refresh takes only the refresh token, returns a new access token and keeps the refresh token unless it was issued over 5 hours ago, when a new row replaces it and the old one is deactivated (never updated in place); the immediate predecessor deactivated under 5 minutes ago still answers with the current refresh token; anything else stale is just rejected. Executed as one vertical slice with WORK-0058 and the frontend's identity-web-v1 WORK-0001/0002 ("real guests end to end"), with `test-user-1`/`test-user-2` kept as a development-only bypass (`IDENTITY_DEV_TEST_USERS`, refused in production) until WORK-0059.

## Acceptance Criteria

Met on the backend side: "Dos invitados", "Alta repetida" (same key within 5 minutes answers the same guest; `idempotency_key_spent` after), "Persistencia" (reload and backend restart keep the guest), "Rotación" (5 h rotation, 5 min predecessor grace), "Respuesta perdida" (the predecessor grace), "Logout". Previously: The plan's "Respuesta perdida" case of a `Set-Cookie` applied with the body lost no longer exists under the body-based contract (WORK-0056). Owns at least the plan's "Dos invitados", "Alta repetida", "Persistencia", "Rotación", "Respuesta perdida" and "Logout" criteria (§11) on the backend side.

## Blockers

None.

## Completion Record

- Implementation: `api/identity/{credentials,create_guest,refresh,logout,me}.go` (no idempotency key logging), wired by `api.NewServer` and `main.go`.
- Verification: handler tests (`session_endpoints_test.go`, `create_guest_test.go`); against the running server with the frontend: two guests in separate browser contexts got different UserUUIDs and saw each other's names in the same Session, one reloaded and stayed the same guest without being asked its name, and the same guest was restored after a backend restart with a new random access key; the host walkthrough (development account `test-user-1`) saw the guest by its real name, started, watched, cancelled, joined as a player.
- Independent review: CHANGES_REQUIRED (frontend renewal overtaken by another tab, no retry of a failed renewal, fast device clock, spent guest key reused; backend logout racing a rotation, issue-time clock skew, missing endsession test, the guest-creation repo owning its transaction and idempotency decision); all fixed (the backend now also returns `access_expires_in`; guest creation's transaction and decision moved into the service, the repo only claims and writes). Re-review APPROVED. Left as non-blocking: raw SQL inserts, the logout race test relies on a sleep.
- Docs: `api/README.md` (identity route group), `identity/CURRENT_STATE.md`.
