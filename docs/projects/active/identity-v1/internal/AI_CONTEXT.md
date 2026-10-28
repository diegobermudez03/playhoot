Process: Project execution (identity-v1), WORK per plan stage
Topic: Basic identity (real guests, Google accounts, own access/refresh credentials) per plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx
Current stage: WORK-0057 + WORK-0058 DONE 2026-10-27 as one slice with frontend identity-web-v1 WORK-0001/0002 (real guests end to end; credential redesign by the human: JWT access, stored refresh tokens, 5 h rotation, 5 min predecessor grace, stale just rejected; dev bypass test-user-1/2 via IDENTITY_DEV_TEST_USERS). Earlier: WORK-0056 DONE (2026-10-27, review APPROVED on re-review). Decided 2026-10-27 by the human: no browser-binding cookie for login attempts; ADR-0003 point 8 clarified; keeping the one-time code inside the browser is required in WORK-0060 and frontend WORK-0004. WORK-0062 DONE (2026-10-27, uncommitted). Next: frontend WORK-0006 (asked to the human) or WORK-0057
Current execution surface: CODEBASE AGENT
Related durable artifacts: docs/projects/active/identity-v1/PROJECT.md, plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx, identity/docs/decisions/IDENTITY-ADR-0001..0002, frontend Project identity-web-v1 (../playhoot_frontend/docs/projects/active/identity-web-v1/)
Blocked by: nothing (one-time code login completion accepted 2026-10-27; host experience pulled into frontend session-play-v1 P, with backend item in session-runtime-v1 CHECKLIST E: min/max players in the admin's lobby_update)
Next action: WORK-0059 (accounts with the local simulated provider; removes the dev bypass)
Last durable checkpoint: Project, WORK files, plan copied to plans/identity/, cross-references in session-runtime-v1 PROJECT.md and identity/CURRENT_STATE.md (2026-10-25)
Last updated: 2026-10-27

## Working agreement (the human, 2026-10-27)

Decisions are made in conversation; the agent writes code and the human reviews the code itself; code the human does not ask to change is approved. WORK files, ADRs, PROJECT.md and reviews are the agent's own memory, kept current silently: never ask the human to read or approve them, never present DRAFT/READY gates or HUMAN_REVIEW files. A decision in chat is the human approval the repository process needs (record it in the WORK/ADR yourself).

## Human decisions (2026-10-25)

- Identity goes now, before the SDK built-ins and game creation.
- The plan's decisions are accepted as a whole, except the refresh token location: if `localStorage` is more convenient for the mobile app, use it; otherwise a cookie. The agent's answer: yes, because one body-based backend contract then serves web and mobile (native apps keep it in secure storage either way); the human's rule therefore selects client-held refresh.
- Tracking: one WORK per plan stage, designed just in time.

## Human decisions (2026-10-27)

- Scope added: WORK-0062, the operation catalog with each operation's minimum authentication level (`anonymous < guest < account`), public `GET /v1/identity/operations`, own level from refresh/`me`; `Authorize` applies the same catalog. Distinct from future role capabilities. Frontend counterpart: identity-web-v1 WORK-0006.

## Context notes

- The plan's commits (backend `3608c45`, frontend `e3e76d6`) were HEAD of both repositories when the project was created; the plan's findings were spot-checked against code (createguest placeholder, idempotency key logged in `api/identity/create_guest.go`, frontend `browserCredentialStore.ts` in localStorage).
- The parent folder `../docs/` (outside both repositories) holds the other plans (02 audiences, 03 SDK, 04 creation flow); only plan 05 was copied here.
