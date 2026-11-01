# WORK-0060: Google OIDC provider

Status: DONE
Created: 2026-10-25
Last status change: 2026-10-31 (DONE by the human's decision in conversation, with the real Google integration deferred to the end as planned follow-up)

Related decisions:
- The decisions recorded by WORK-0056

Canonical context:
- `plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx` (§2, §4 "Google mediante redirección", §9 "Google real")

## Outcome

The real Google adapter behind WORK-0059's provider port: Authorization Code with the client secret only on the server, state, nonce and PKCE S256 per attempt, ID token validation (signature, algorithm, issuer, audience, expiry, subject, nonce) by the adapter's own policy, scopes limited to `openid profile email`, environment configuration with secrets outside the repository. Declaring Google login available requires the real provider configured and tested; the simulated provider alone does not count (plan §9).

## Context

Google is identified by its normalized issuer and verified `sub`; email and name never cause another identity or an automatic binding (§3). No offline access and no dependency on a Google refresh token (§4). Test with fixtures for signatures and claims, and manually against a real Web client: new login, recurring login, cancellation, wrong PKCE verifier refused (§9).

## Scope

Built 2026-10-31: `identity/loginproviders/google.go`, behind the same `Provider` port as the local provider. Authorization URL with `client_id`, the registered `redirect_uri`, `response_type=code`, scopes `openid email profile`, `state`, `nonce`, PKCE S256 and `prompt=select_account`; the PKCE verifier toward Google is the server's own (`login.NewProviderVerifier`), kept on the attempt (`provider_code_verifier`, migration `20261031000000_identity_provider_pkce`, written once) and sent on the exchange, so the `Provider` port now carries the challenge and the verifier (the local provider ignores them). Exchange: the token endpoint server to server with the client secret, then the ID token verified by our own policy: RS256 only, signature against Google's keys (cached an hour, fetched again at once for an unknown key id, under a mutex), issuer `https://accounts.google.com` or `accounts.google.com`, audience our client id, expiry and issue time with a minute of skew, subject present, nonce equal to the attempt's. Stored issuer `https://accounts.google.com`; email recorded only when `email_verified`; the name is observed only. A code Google refuses or a token that fails is `ErrInvalidAnswer` (`?error=failed`); a 5xx, a network failure or a timeout (10 s) is unavailable (`?error=unavailable`). Configuration `GOOGLE_OAUTH_CLIENT_ID`, `GOOGLE_OAUTH_CLIENT_SECRET`, `GOOGLE_OAUTH_REDIRECT_URL` (all three or none; the redirect ends in `/v1/identity/login/google/callback`), secrets outside the repository. `GET /v1/identity/login/providers` lists the enabled providers so clients show only those. The login completes towards the client with WORK-0056's one-time code exchange.

Required (decided by the human 2026-10-27, IDENTITY-ADR-0003 point 8 clarification: no browser-binding cookie; the one-time code must never leave the victim's browser). Background from the WORK-0056 independent review: without the plan's browser-binding cookie, an attacker can start a login with their own verifier and send the victim the authorization URL; if Google lets the victim through without interaction and the returned one-time code then leaks within its ~60 s (`Referer`, logs of the web host or a CDN, history, extensions), the attacker can exchange it. Mitigations (the backend's part: no-referrer header on the redirect, no logging of the callback or completion query strings; the client's part is frontend WORK-0004): deliver or strip the code before the completion page loads anything (or use the URL fragment), `Referrer-Policy: no-referrer` on the completion route, keep that route's query strings out of logs, consider `prompt=select_account`.

## Acceptance Criteria

Met with a fake Google (`google_test.go`): the identity of a valid token; refused for another nonce, another audience, another issuer, an expired token, no subject, another signing key, a code Google refuses; unavailable when Google fails or cannot be reached; keys reused and refetched when Google rotates them; the token request carries the code, client, secret, redirect and verifier. The attempt keeps the verifier and Google gets its S256 (`TestStartSendsTheProviderAChallengeOfTheKeptVerifier`). In headless Edge with a fictitious client: the button enabled because the backend offers Google, and Google's authorization URL with our client, callback, scopes, state, nonce and S256 challenge.

- Independent review (2026-10-31): CHANGES_REQUIRED for one medium finding (every refusal from Google's token endpoint was an invalid answer and nothing was logged, so a misconfigured client would fail silently) and three low ones (the providers list was never retried, both provider buttons could start at once, a refetch failure discarded cached keys); all fixed: only `invalid_grant` is an invalid answer, 5xx/429 are unavailable, any other 4xx is `ErrMisconfigured` (logged, alerted, ended unavailable with reason `provider_misconfigured`), refused answers are logged without secrets, the web retries the providers list with backoff, the Google button is disabled while any sign-in starts, cached keys under 24 hours verify while Google's keys cannot be fetched, and RS256-only is locked by tests. Re-review APPROVED.

Deferred by the human on 2026-10-31 (planned, accepted): the real Google integration (an OAuth client from Google Cloud Console in `GOOGLE_OAUTH_CLIENT_ID`, `GOOGLE_OAUTH_CLIENT_SECRET`, `GOOGLE_OAUTH_REDIRECT_URL`, and the manual check against Google: new login, recurring login, cancellation, refused wrong verifier) is done at the end, before Google sign-in is declared available to people. Until then the code is complete and verified with a fake Google, and production simply does not offer Google.

The deferred check (plan §9 "Google real"): against a real Web client from the human's Google Cloud Console, a new login, a recurring login (same User), cancellation, and a refused wrong verifier.

## Blockers

- WORK-0056, WORK-0059.
- A Google Cloud project and OAuth Web client configured by the human (support info, branding, domains, privacy page; plan §2 "Preparación para Google real").
