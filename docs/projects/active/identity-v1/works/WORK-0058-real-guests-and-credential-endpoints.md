# WORK-0058: Real guests and credential endpoints

Status: PLANNED
Created: 2026-10-25
Last status change: 2026-10-25

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

Not yet designed.

## Acceptance Criteria

Not yet designed. Owns at least the plan's "Dos invitados", "Alta repetida", "Persistencia", "Rotación", "Respuesta perdida" and "Logout" criteria (§11) on the backend side.

## Blockers

- WORK-0056, WORK-0057.
