# Product Decision Record Template

This template contains authoring guidance. Template-only instructions must not be copied into the final PDR.

Resulting record shape:

```text
# PDR-NNNN: <Decision Title>

Status: PROPOSED
Created: YYYY-MM-DD
Last status change: YYYY-MM-DD
Supersedes: None
Superseded by: None

## Product Context

<What user/product problem or product choice requires a decision?>

## Decision

<The proposed/accepted/rejected product outcome.>

## Rationale

<Why this is the right product decision at this point.>

## Alternatives Considered

### <Alternative>

<Material tradeoff/reason it was not selected.>

## Consequences

<Important product value, scope, limitations, risks, or assumptions.>

## Canonical Knowledge Impact

- `<path>` - <what accepted product fact changes>

For PROPOSED/REJECTED records with no canonical change, state None.

## Follow-up

<Experiment, architecture question, domain question, implementation work, or None.>

Do not turn the PDR into an implementation specification.
```
