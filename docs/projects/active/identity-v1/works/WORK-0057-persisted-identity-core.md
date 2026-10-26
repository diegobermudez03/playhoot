# WORK-0057: Persisted identity core

Status: PLANNED
Created: 2026-10-25
Last status change: 2026-10-25

Related decisions:
- `identity/docs/decisions/IDENTITY-ADR-0001-identity-user-public-identity-boundary.md`
- `identity/docs/decisions/IDENTITY-ADR-0002-token-renewal-with-rotating-refresh-tokens.md`
- The decisions recorded by WORK-0056

Canonical context:
- `plans/identity/05_Playhoot_Plan_de_Identidad_Basica.docx` (§3, §5, §6, §8)
- `docs/engineering/standards/database-constraints.md`, idempotency standard

## Outcome

Identity's domain capabilities on real persistence: stable Users (guest and registered in the same entity), accounts and unique external identity bindings, credential families with access and refresh issuance, authorization of operations, atomic refresh rotation with reuse detection and short retry recovery, revocation, and guest-creation idempotency. Every endpoint of the later WORK builds on these; today Identity has no tables and no migrations wired at startup (plan §1).

## Context

The plan's starting model (§3 "Modelo de partida") lists the responsibilities: users, accounts, external identities, auth sessions (families), access tokens, refresh tokens, OIDC attempts and browser context, temporary results. Names describe responsibilities; tables may be adjusted keeping uniqueness, atomicity and contracts. Per the repository standard, the database holds only keys, unique constraints, indexes and types; every other rule lives in the application and its transactions.

Authorization keeps the existing operation matrix (§8), but an unknown operation must be refused (the placeholder allows anything not on its deny list), and a database failure is a transient service error, never an invalid credential.

## Scope

Not yet designed.

## Acceptance Criteria

Not yet designed. Includes the plan's transactional concurrency tests (§10 stage 1).

## Blockers

- WORK-0056.
