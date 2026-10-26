# WORK-0060: Google OIDC provider

Status: PLANNED
Created: 2026-10-25
Last status change: 2026-10-25

Related decisions:
- The decisions recorded by WORK-0056

Canonical context:
- `plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx` (§2, §4 "Google mediante redirección", §9 "Google real")

## Outcome

The real Google adapter behind WORK-0059's provider port: Authorization Code with the client secret only on the server, state, nonce and PKCE S256 per attempt, ID token validation (signature, algorithm, issuer, audience, expiry, subject, nonce) by the adapter's own policy, scopes limited to `openid profile email`, environment configuration with secrets outside the repository. Declaring Google login available requires the real provider configured and tested; the simulated provider alone does not count (plan §9).

## Context

Google is identified by its normalized issuer and verified `sub`; email and name never cause another identity or an automatic binding (§3). No offline access and no dependency on a Google refresh token (§4). Test with fixtures for signatures and claims, and manually against a real Web client: new login, recurring login, cancellation, wrong PKCE verifier refused (§9).

## Scope

Not yet designed. How the login completes towards the client depends on WORK-0056 (one-time code exchange instead of a cookie).

## Acceptance Criteria

Not yet designed.

## Blockers

- WORK-0056, WORK-0059.
- A Google Cloud project and OAuth Web client configured by the human (support info, branding, domains, privacy page; plan §2 "Preparación para Google real").
