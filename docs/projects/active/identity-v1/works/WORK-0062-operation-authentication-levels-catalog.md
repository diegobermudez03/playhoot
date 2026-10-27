# WORK-0062: Operation authentication levels catalog

Status: PLANNED
Created: 2026-10-27
Last status change: 2026-10-27

Related decisions:
- The decisions recorded by WORK-0056

Canonical context:
- `identity/identity.go` (`Operation`), `identity/usecases/authorize`
- `plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx` (§8)

## Outcome

Every client-facing operation declares the minimum authentication level it requires, on the ordered scale `anonymous < guest < account`, in one catalog that is both the rule `Authorize` applies and a public endpoint the client reads at startup. The client then knows, before calling anything, which operations it may show and where to send the person to authenticate when they choose one (decided by the human on 2026-10-27: the frontend shows operations above the person's level and, when chosen, redirects to the most flexible authentication the operation allows, then continues to the intended path).

## Context

Human decisions 2026-10-27:

- This is the authentication level per operation, distinct from role capabilities (a future, separate answer listing what a role may do) and from Play's object-level permissions (creator, participation), which stay where they are.
- Contract: a public, cacheable `GET /v1/identity/operations` with every operation and its required level; the caller's own level comes with the credential (refresh and `me` responses), not from this endpoint.

The endpoint shape, the starting values and which operations are listed are fixed by WORK-0056's approved contract ("Operation catalog"), not restated here. An operation missing from the catalog is refused by `Authorize` (the placeholder allows anything not on its deny list).

## Scope

Not yet designed. Can be built against the current placeholders right after WORK-0056, so the frontend's redirection pattern does not wait for persistence.

## Blockers

- WORK-0056 (endpoint and DTO shape recorded with the rest of the contract).
