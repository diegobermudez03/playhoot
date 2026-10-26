Process: Project execution (identity-v1), WORK per plan stage
Topic: Basic identity (real guests, Google accounts, own access/refresh credentials) per plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx
Current stage: Project created 2026-10-25 with WORK-0056..0061 PLANNED (one per plan stage 0-5, the human's choice of tracking); next is WORK-0056 PLANNED -> DRAFT
Current execution surface: CODEBASE AGENT
Related durable artifacts: docs/projects/active/identity-v1/PROJECT.md, plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx, identity/docs/decisions/IDENTITY-ADR-0001..0002, frontend Project identity-web-v1 (../playhoot_frontend/docs/projects/active/identity-web-v1/)
Blocked by: the human's confirmation of the design derived from the client-held refresh token (PROJECT.md decision 1)
Next action: design WORK-0056 (DRAFT): new IDENTITY-ADR-0003, IDENTITY-ADR-0002 amendment, endpoint/DTO contract; then the frontend's WORK-0001
Last durable checkpoint: Project, WORK files, plan copied to plans/identity/, cross-references in session-runtime-v1 PROJECT.md and identity/CURRENT_STATE.md (2026-10-25)
Last updated: 2026-10-25

## Human decisions (2026-10-25)

- Identity goes now, before the SDK built-ins and game creation.
- The plan's decisions are accepted as a whole, except the refresh token location: if `localStorage` is more convenient for the mobile app, use it; otherwise a cookie. The agent's answer: yes, because one body-based backend contract then serves web and mobile (native apps keep it in secure storage either way); the human's rule therefore selects client-held refresh.
- Tracking: one WORK per plan stage, designed just in time.

## Context notes

- The plan's commits (backend `3608c45`, frontend `e3e76d6`) were HEAD of both repositories when the project was created; the plan's findings were spot-checked against code (createguest placeholder, idempotency key logged in `api/identity/create_guest.go`, frontend `browserCredentialStore.ts` in localStorage).
- The parent folder `../docs/` (outside both repositories) holds the other plans (02 audiences, 03 SDK, 04 creation flow); only plan 05 was copied here.
