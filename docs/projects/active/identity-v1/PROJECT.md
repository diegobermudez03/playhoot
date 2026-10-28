# Project: Identity v1 (basic identity)

Status: ACTIVE
Created: 2026-10-25
Last updated: 2026-10-27

## Goal

Replace Identity's placeholders (fixed test tokens, one shared test guest) with real, persisted identity, as planned in `plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx` (the "plan"; `§N` points to its sections): distinct persisted guests, registered accounts through Google, Playhoot's own access and refresh credentials with renewal, rotation, expiry and logout, and the existing permission matrix enforced for real. An account can create a Session, two different guests can join it and keep their identity across reloads, and an account that signs in again resolves the same User (plan, "Resultado de esta fase").

Decided by the human on 2026-10-25: this project goes before the SDK built-ins and game creation (plans 03 and 04 in the parent `docs/`); `session-runtime-v1` stays active meanwhile. The frontend counterpart is the frontend repository's Project `identity-web-v1`, which tracks its own WORK.

## Explicitly Out Of Scope

- Passwords, magic links, providers other than Google, MFA (plan §2).
- Guest-to-account upgrade and identity reconciliation/merge (plan §3 "Conversión futura"): the schema must allow them later keeping the UserUUID, but no flow is built.
- Full profiles, editable avatar, device/session management screens, guest recovery (plan intro, §5).
- Configurable roles, organization permissions, permissions of the future game editor, delegated authorization for agents (plan §1).
- Remote eviction of already open WebSockets on revocation (plan §5 "Revocación").

## Current Work

Created 2026-10-25 from the plan. The human accepted the plan's decisions as a whole with one change: the refresh token is kept by the client (web `localStorage`, mobile secure storage) and travels in the request body, not in an `HttpOnly` cookie, because one body-based contract then serves the web and the future mobile app. The consequences of that change on the plan (§4 Google handoff, §6 retry recovery, §7 transport) are designed in WORK-0056, the next step. 2026-10-27: the human approved WORK-0056 (READY) with the full client-facing contract; IDENTITY-ADR-0003 is ACCEPTED and IDENTITY-ADR-0002 amended; WORK-0056 DONE the same day after an independent review (approved on re-review). WORK-0062 (operation catalog, `GET /v1/identity/operations`) DONE the same day. Frontend WORK-0006 done against the test tokens. Then, the same day, the human redesigned the credentials in conversation (JWT access tokens, refresh tokens stored as-is with 5 h rotation and a 5 min predecessor grace; see IDENTITY-ADR-0003's amendment) and WORK-0057 and WORK-0058 were done as one vertical slice with the frontend's WORK-0001/0002: real guests end to end, accounts only through the development bypass. Next: WORK-0059.

## Work

| Order | Work | Status |
|------:|------|--------|
| 0 | [WORK-0056](works/WORK-0056-identity-contracts-and-decisions.md) — Identity contracts and decisions | DONE |
| 0b | [WORK-0062](works/WORK-0062-operation-authentication-levels-catalog.md) — Operation authentication levels catalog | DONE |
| 1 | [WORK-0057](works/WORK-0057-persisted-identity-core.md) — Persisted identity core | DONE |
| 2 | [WORK-0058](works/WORK-0058-real-guests-and-credential-endpoints.md) — Real guests and credential endpoints | DONE |
| 3 | [WORK-0059](works/WORK-0059-accounts-with-local-simulated-provider.md) — Accounts with a local simulated provider | PLANNED |
| 4 | [WORK-0060](works/WORK-0060-google-oidc-provider.md) — Google OIDC provider | PLANNED |
| 5 | [WORK-0061](works/WORK-0061-identity-integration-and-closure.md) — Integration and closure | PLANNED |

## Ordering / Dependencies

The plan's delivery order (§10):

- WORK-0056 first: every later WORK implements the contracts and decisions it records (ADRs and DTOs), and the frontend's WORK depend on the same contracts.
- WORK-0062 (operation catalog with each operation's minimum authentication level, added by the human on 2026-10-27) right after WORK-0056: it needs no persistence, so the frontend's redirection pattern can start against the placeholders; `Authorize` applies the same catalog.
- WORK-0057 (persistence, issuance, refresh, revocation as domain capabilities) before any other endpoint.
- WORK-0058 (guests) and WORK-0059 (accounts with the simulated provider) together close the local goal: an account creates and administers a Session, two real guests play it. Each needs its frontend counterpart in `identity-web-v1` to be usable.
- WORK-0060 (real Google) needs WORK-0059's provider port; it can run in parallel with the frontend's local work.
- WORK-0061 last: network and concurrency cases, WebSocket, limits, documents and contracts synchronized, independent review against the plan's acceptance criteria (§11).

## Capability Coverage

The plan's acceptance criteria (§11 "Evidencia requerida") are the coverage list; each WORK names the criteria it owns once it moves to DRAFT. Already-known overlaps with `session-runtime-v1`: its CHECKLIST section A4 (identity integration) and its Identity placeholders (guest creation, authorization) are owned by this project.

Added to the scope by the human on 2026-10-27: the operation catalog (WORK-0062) behind the frontend's global sign-in pattern (its `identity-web-v1` WORK-0006: corner sign-in/profile button, definitive `/sign-in` route, guest page offering the account option, return to the intended path, a minimal "Create Session" requiring an account).

## Material Decisions Needing Human Input

1. **Design derived from keeping the refresh token on the client**: ACCEPTED by the human on 2026-10-27, recorded in WORK-0056: the Google callback cannot leave a session in a cookie, so it redirects to the web with a one-time code (about 60 s) that the client exchanges, together with a verifier it generated when starting the login, for access and refresh in the response body (the native-app pattern of RFC 8252, reusable by the mobile app); same-origin deployment is no longer required and the existing REST CORS without credentials stays; the plan's "Set-Cookie applied but body lost" recovery case disappears.
2. **Rationale of IDENTITY-ADR-0002**: resolved 2026-10-27: restated by IDENTITY-ADR-0003 and the amendment note in IDENTITY-ADR-0002.

## Completion Criteria

The project is complete when every WORK above is DONE (or explicitly cancelled by the human), the plan's acceptance criteria (§11) are verified, the local closure (a Session created by a persisted account and joined by two real guests from separate browser contexts) and the Google closure (first and recurring login against the real provider) have been demonstrated, and durable knowledge is promoted to its owners (`identity/README.md`, `identity/CURRENT_STATE.md`, Identity ADRs, `api/README.md`, data model and flows).
