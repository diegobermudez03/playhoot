# WORK-0056: Identity contracts and decisions

Status: PLANNED
Created: 2026-10-25
Last status change: 2026-10-25

Related decisions:
- `identity/docs/decisions/IDENTITY-ADR-0001-identity-user-public-identity-boundary.md`
- `identity/docs/decisions/IDENTITY-ADR-0002-token-renewal-with-rotating-refresh-tokens.md`

Canonical context:
- `plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx` (§2-§8)
- `identity/README.md`, `identity/CURRENT_STATE.md`
- Frontend repository: `docs/decisions/architecture/ADR-0009-credential-renewal-in-the-transport.md`

## Outcome

The plan's decisions, accepted by the human on 2026-10-25, recorded in their owners before any code: the provider (Google only), the opaque access token, where the refresh token lives and how it travels, the retry-recovery semantics that make IDENTITY-ADR-0002's reuse rule precise, the explicit identity switch, and the client-facing contract (endpoints and DTOs) every later WORK and the frontend implement. Without it each WORK would decide contracts on its own.

## Context

The plan's own status note: its new choices are design recommendations to adopt when preparing the repository work, and the idempotent-retry exception must amend the rotation ADR explicitly, never silently.

Human decision 2026-10-25: accept the plan as a whole, except that the refresh token is kept by the client (web `localStorage`, mobile secure storage) and sent in the request body instead of an `HttpOnly` cookie, so web and mobile share one contract. Consequences to design here (agent proposal, see PROJECT.md decision 1): Google login completes with a one-time code exchanged by the client with a verifier; no same-origin requirement; the "Set-Cookie applied but body lost" recovery case disappears; the XSS exposure of a non-expiring guest refresh token is accepted and mitigated by rotation with reuse detection.

## Scope

Not yet designed. Expected: a new IDENTITY-ADR (provider, opaque access, client-held refresh, Google handoff), an amendment to IDENTITY-ADR-0002 (retry recovery; restated rationale), the endpoint and DTO contract, and the matching frontend decision records through the frontend's own WORK.

## Approved Design

Not yet designed.

## Acceptance Criteria

Not yet designed.

## Documentation Impact

Not yet designed. Known: `identity/CURRENT_STATE.md` mentions validating a JWT; the opaque format changes that sentence.

## Blockers

- The human's confirmation of the design derived from the client-held refresh token (PROJECT.md decision 1).
