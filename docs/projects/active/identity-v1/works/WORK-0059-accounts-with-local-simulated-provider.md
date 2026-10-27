# WORK-0059: Accounts with a local simulated provider

Status: PLANNED
Created: 2026-10-25
Last status change: 2026-10-25

Related decisions:
- The decisions recorded by WORK-0056

Canonical context:
- `plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx` (§3 "Vínculo con el proveedor", §4, §9 "Proveedor simulado")

## Outcome

Registered accounts end to end without Google: a provider port with an explicit fictitious local provider (stable subjects, its own issuer, enabled only in local and test, startup fails if enabled in production), first sign-in creating the User, account and binding atomically, recurring sign-in resolving the same User, and the explicit identity switch from a guest. Everything except the external provider is the production implementation. With the frontend's counterpart, an account can create and administer its Session locally (plan §10 stage 3).

## Context

Concurrent first sign-ins for the same external binding must produce one User and one account; repeated callbacks must not leave orphan Users or unbounded families (§3). Signing in with an account while a guest is active revokes the guest family; participations are not transferred (§3 "Conversión futura"). To fix when designed (WORK-0056 review): what `login/complete` does with a `replaces_refresh_token` that is unknown, revoked or already consumed (proposal: never fails the login; a consumed one is reuse and revokes its family anyway).

## Scope

Not yet designed.

## Acceptance Criteria

Not yet designed. Owns at least "Cuenta recurrente", "Permisos", "Proveedor local" and the backend side of "Cambio de usuario" (§11).

## Blockers

- WORK-0056, WORK-0057.
