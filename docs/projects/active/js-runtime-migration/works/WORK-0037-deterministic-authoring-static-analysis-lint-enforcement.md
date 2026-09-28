# WORK-0037: Deterministic-Authoring Static Analysis / Lint Enforcement

Status: PLANNED
Created: 2026-09-27
Last status change: 2026-09-27

Related decisions:
- `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
- `game/docs/decisions/GAME-ADR-0028-javascript-execution-replaces-game-language.md`
- `session/docs/decisions/SESSION-ADR-0025-snapshot-based-session-runtime-persistence.md`

Canonical context:
- None beyond the ADRs above; this is a new capability with no current-implementation precedent.

## Outcome

Because sandboxed JavaScript's determinism cannot be guaranteed the way Game Language's closed DSL guaranteed it by construction, this WORK builds a required-before-acceptance validation pass that flags authored/generated backend script code calling known nondeterministic host APIs (wall-clock reads, `Math.random`, network, filesystem, and any other source the selected sandbox exposes) before that script is accepted for publish or use. This is a best-effort authoring-quality gate, explicitly not a substitute for the isolation boundary `WORK-0036` enforces at runtime — it exists because `SESSION-ADR-0025`'s snapshot-based persistence model no longer depends on perfect determinism for correctness, but reducing nondeterminism still matters for the best-effort session-reconstruction capability (`WORK-0051`) and for authoring quality generally.

## Context

Not yet designed. Depends on `WORK-0035`'s supported JavaScript profile (the exact subset/APIs available) being defined first — this WORK enforces a policy over that surface, it does not define the surface.

## Scope

Not yet designed.

## Approved Design

Not yet designed.

## Constraints and Invariants

- Must not be presented as a correctness guarantee — its own Documentation Impact must state plainly that it reduces, not eliminates, nondeterminism risk (per `ADR-0015`'s Rationale).
- Must run as part of script validation/publish (coordinating with `WORK-0033`/`WORK-0044`), not only as an optional authoring-time suggestion.

## Acceptance Criteria

Not yet designed.

## Implementation Freedom

Not yet designed.

## Verification

Not yet designed.

## Documentation Impact

### Accepted / Canonical Knowledge

- Authoring-facing documentation (successor to `game/language/v1/program/DEFINITION.md`'s "what is/isn't buildable" framing) stating which APIs are disallowed and why.

## Blockers

- Depends on `WORK-0035`'s supported-JavaScript-profile decision.

## Completion Record

Not yet started.
