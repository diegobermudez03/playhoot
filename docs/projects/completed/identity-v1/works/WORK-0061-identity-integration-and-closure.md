# WORK-0061: Identity integration and closure

Status: DONE
Created: 2026-10-25
Last status change: 2026-10-31 (the human answered the closure questions in conversation; closed the same day)

Related decisions:
- The decisions recorded by WORK-0056

Canonical context:
- `plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx` (§9 "Controles operativos mínimos", §10 stage 5, §11)

## Outcome

The project closed as one verified whole: network and concurrency cases, WebSocket handshakes with real credentials, user switches, secrets kept out of logs and telemetry, rate and concurrency limits on guest creation, login start and refresh (classrooms share a public IP), metrics with bounded labels, cleanup of expired results, documents and contracts synchronized in both repositories, and an independent review against the plan's acceptance criteria (§11).

## Context

The plan requires concurrency and lost-response tests because they decide whether a guest keeps their identity; verifying the successful login alone is not enough (§11 "Actualización de conocimiento").

## Scope

The closure decisions the human took on 2026-10-31 (recorded in IDENTITY-ADR-0003) and the evidence for each §11 criterion. Added: the guest creation cap (200 per client address per hour), Postgres tests of session lifetimes (an account unused 30 days must sign in again; rotation never passes 90 days from sign-in; a guest 400 days old still renews), and the audit of the game boundary and logs. Moved to Identity phase 2 (after the game creation flow; `identity/CURRENT_STATE.md`): the account screen, display name changes, active sessions with device data, guest retention, key rotation, guest sign-out, and the real Google integration.

## Acceptance Criteria

Evidence per §11 criterion:

- Dos invitados, Alta repetida, Persistencia: WORK-0058 (browser walkthrough with two contexts, reload, backend restart; idempotency tests).
- Cuenta recurrente, Proveedor local, Permisos: WORK-0059 (same subject resolves the same User, concurrent first sign-ins create one; the local provider only in development/local/test; a guest cannot create or administer, an unknown operation is forbidden).
- Vencimiento: `TestSessionLifetimesWithPostgres`, `refreshtoken` tests, access tokens strict at expiry.
- Rotación: one successor per rotation under the row lock (`concurrent_renewals_of_an_old_token_rotate_once`); earlier access tokens valid until they expire.
- Respuesta perdida, Replay y concurrencia: replaced by the redesign (5-minute grace of the previous token; tabs racing never revoke each other).
- Red o base de datos caída: a failure is `503`/unavailable, never `refresh_invalid`; the client keeps the credential and retries, never creating another guest (frontend tests).
- Cambio de usuario: a renewal started for an earlier person never applies (generation counter); the guest's session ends with the sign-in; and when another tab switches identity, the web rebuilds every page for the new person, closing the previous person's live connections (frontend `identityKey`, headless Edge `switch_flow`: a guest's tab in a lobby follows the other tab into an existing account and is back in the lobby as that account, the guest gone).
- Logout: local first, server told without waiting (closure decision).
- WebSocket: authorized only at the handshake, so an expired access token does not close an admitted connection; the client renews before expiry and when the page is shown again.
- Frontera del juego: the GameHost never receives tokens (artifacts and assets fetched with `credentials: 'omit'`), and no log or client telemetry records tokens, codes or verifiers (audited 2026-10-31).

Owns at least "Red o base de datos caída", "WebSocket", "Frontera del juego" and "Vencimiento" end to end (§11).

## Blockers

None.

## Completion Record

- Implementation: `api/identity/handler.go` (guest creation cap) and its test; `TestSessionLifetimesWithPostgres`; IDENTITY-ADR-0003 closure decisions; Identity phase 2 in `identity/CURRENT_STATE.md`.
- Verification: backend `go test` with Postgres; the local closure demonstrated in headless Edge (a Session created by a persisted account, joined by two real guests from separate contexts); the Google closure is the planned follow-up of WORK-0060.
- Independent review against §11: CHANGES_REQUIRED for one medium finding (another tab switching identity left this tab's live connections open as the previous person, part of §11 "Cambio de usuario"; fixed on the web, `identityKey`) and stale documents (synced), plus the guest cap duplicating the default limits (now built on them); re-review APPROVED. ADR-0003 points 3 and 6 marked superseded.
