# WORK-0059: Accounts with a local simulated provider

Status: DONE
Created: 2026-10-25
Last status change: 2026-10-27 (the human said to continue in conversation, under the working agreement; designed, implemented and verified the same day)

Related decisions:
- The decisions recorded by WORK-0056

Canonical context:
- `plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx` (§3 "Vínculo con el proveedor", §4, §9 "Proveedor simulado")

## Outcome

Registered accounts end to end without Google: a provider port with an explicit fictitious local provider (stable subjects, its own issuer, enabled only in local and test, startup fails if enabled in production), first sign-in creating the User, account and binding atomically, recurring sign-in resolving the same User, and the explicit identity switch from a guest. Everything except the external provider is the production implementation. With the frontend's counterpart, an account can create and administer its Session locally (plan §10 stage 3).

## Context

Concurrent first sign-ins for the same external binding must produce one User and one account; repeated callbacks must not leave orphan Users or unbounded families (§3). Signing in with an account while a guest is active revokes the guest family; participations are not transferred (§3 "Conversión futura"). To fix when designed (WORK-0056 review): what `login/complete` does with a `replaces_refresh_token` that is unknown, revoked or already consumed (proposal: never fails the login; a consumed one is reuse and revokes its family anyway).

## Scope

The contract of WORK-0056 for login start, provider callback and completion, with a fictitious local provider behind a provider port (`identity/loginproviders`): its page lets the person pick a name (Ana, Beto, Carla, or any), the same name being the same subject of its own issuer. Enabled by `IDENTITY_LOCAL_PROVIDER=true`, only in `development`, `local` or `test` environments. The fixed test tokens of the development bypass are removed. On the open question of this WORK's Context, with the redesigned credentials: an unknown or ended `replaces_refresh_token` never fails the login (there is no reuse detection anymore); a given one has its session ended in the login's transaction.

## Acceptance Criteria

Met on the backend side: "Cuenta recurrente" (the same name resolves the same User), "Permisos" (an account creates and administers a Session; a guest cannot), "Proveedor local" (refused outside development/local/test), "Cambio de usuario" (the guest's session ends with the sign-in; cancelling keeps it). Plus: concurrent first sign-ins create one User; a repeated callback never exchanges twice; a code works once and a wrong verifier uses it up. Previously: Owns at least "Cuenta recurrente", "Permisos", "Proveedor local" and the backend side of "Cambio de usuario" (§11).

## Blockers

None.

## Amendment (2026-10-29, decided by the human in conversation)

- The login is a workflow, not three use cases: `identity/workflows/accountlogin` (`Manager` with `StartLogin`, `HandleCallback`, `CompleteLogin`, one file per step, per the domain-logic-placement standard); `identity/usecases/{startlogin,logincallback,completelogin}` were removed.
- History: every step of a login attempt is an event (`identity_login_attempt_events`), written with the change; external identities no longer overwrite what the provider said (it is kept per login in the `code_issued` event).
- Display name: an account is created without one (no name derived from the provider); the person chooses it after signing in, and clients ask for it whenever the account has none (`PUT /v1/identity/me/display-name`, `identity/usecases/setdisplayname`, with `identity_display_name_changes`).
- Migration `20261029020000_identity_trails`.
- Verification: workflow unit, repository and Postgres integration tests (attempt histories for completion, verifier mismatch, reuse, cancellation, invalid answers; observed data kept per login); the display name use case and endpoint tests; headless Edge: a new account asked its name (still asked after a reload), host flow 28/28, guest flow 11/11.

## Completion Record

- Implementation: `identity/identity.go` (provider and login types), migration `20261029010000_identity_logins`, `identity/internal/business/login`, `identity/loginproviders`, `identity/usecases/{startlogin,logincallback,completelogin}`, `authorize` without the bypass, `api/identity/login.go` (four routes), `api.Logins` through `api.NewServer`, `main.go` (`IDENTITY_LOCAL_PROVIDER`, `WEB_SIGN_IN_COMPLETE_URL`).
- Verification: unit, repository and Postgres integration tests (`completelogin/service_integration_test.go`: sign-in and recurring sign-in, single-use code, guest handover, concurrent first sign-ins, repeated callback); against the running server, the whole flow over HTTP; with the frontend in headless Edge: a guest switching to an account (its session refused afterwards), cancelling, recurring sign-in in another browser, reload, and the host flow signing in through the local provider's page.
- Independent review: APPROVED, with low findings: the router read the one-time code before it left the URL (fixed: the capture is the first module the web app evaluates); the local provider was refused only in production while IDENTITY-ADR-0003 point 2 says local and test only (fixed: only `development`, `local` or `test` environments); repository tests missing for the callback and completion repos (added); the sign-in button could stay loading after Back from the provider (fixed in the frontend). Accepted as is: raw SQL inserts ("prefer" `Create()`), and a lost completion response: the server already ended the guest's session and opened the account's, so the client, which never got the account, ends signed out and signs in again (a single-use code cannot be replayed, by design).
- Docs: `identity/CURRENT_STATE.md`, `api/README.md`, `.env.example`, `session-runtime-v1` CHECKLIST (A4, Placeholders).
