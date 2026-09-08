# Architecture Decision Record Template

This template contains authoring guidance. Template-only instructions must not be copied into the final ADR.

This one template covers both ADR families. Use identifier `ADR-NNNN` and location `docs/decisions/architecture/` for a global/cross-domain decision; use identifier `<DOMAIN>-ADR-NNNN` (for example `GAME-ADR-0001`, `IDENTITY-ADR-0001`) and location `<domain>/docs/decisions/` for a decision whose authority is contained within one bounded context. See `docs/decisions/README.md` for the scope-resolution rule. Do not create a second, domain-specific template merely because the location/identifier differs — the decision shape is identical.

Determine the next number from the existing records/index for that specific family (global, or the owning domain) — never from a different family's count. Only include `Legacy ID:` when this record was migrated from a prior identifier as an exceptional repository-maintenance operation; omit it for a newly authored record.

Resulting record shape:

```text
# ADR-NNNN: <Decision Title>
(or: # <DOMAIN>-ADR-NNNN: <Decision Title>)

Status: PROPOSED
Created: YYYY-MM-DD
Last status change: YYYY-MM-DD
Supersedes: None
Superseded by: None
Legacy ID: <optional, only when migrated from a prior identifier>

## Context

<What problem/pressure requires a decision?>

## Decision

<The proposed/accepted/rejected architecture outcome, appropriate to status.>

## Rationale

<Why this option was selected or why the proposal was rejected.>

## Alternatives Considered

### <Alternative>

<Material tradeoff/reason it was not selected.>

Only include meaningful alternatives.

## Consequences

<Important benefits, costs, constraints, risks, or future implications.>

## Canonical Knowledge Impact

- `<path>` - <what accepted fact changes>

For PROPOSED/REJECTED records where there is no canonical change, state None.

## Implementation Impact

<None, or concise description of downstream implementation/migration work.>

Do not place a complete implementation plan here.
```
