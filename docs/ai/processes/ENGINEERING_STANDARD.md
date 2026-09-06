# Engineering Standard

## Purpose

Create or modify a reusable engineering, design, or coding standard.

## Use This When

- A repeatable rule is needed across future work.
- Examples: domain type exposure, error wrapping, repository test conventions, logging behavior, naming conventions, repository implementation style.

## Do NOT Use This When

- The rule is only needed for one local implementation.
- The real question is product or architecture.
- You want to refactor the repository immediately. Use `FEATURE_DEVELOPMENT.md` after the standard and migration strategy are accepted.

## Starting Information

Provide the inconsistency or problem, examples if known, desired outcome, and whether this should affect future code only or existing code too.

## Start This Process

```text
We are using docs/ai/processes/ENGINEERING_STANDARD.md.

Read docs/ai/OPERATING_MODEL.md. Use docs/ai/KNOWLEDGE_MAP.md to retrieve relevant engineering standards and implementation examples. Inspect code, tests, and migrations when current practice matters.

Act under the Principal Engineer Contract. Challenge whether a reusable standard is actually needed, identify current patterns, propose alternatives I did not mention, teach tradeoffs, prefer enforceable standards where practical, and make a recommendation.

Distinguish EXISTING, PROPOSED, ACCEPTED, and IMPLEMENTED facts. Detect and report drift. Do not implement repository-wide migration merely because a standard is accepted.

Problem / inconsistency:
[paste problem]
```

## Process Stages

1. PROBLEM / INCONSISTENCY: state the issue.
2. Inspect existing standard and code.
3. Determine whether a standard is actually needed.
4. Identify current patterns.
5. Explore alternatives.
6. Recommendation.
7. Human decision.
8. Canonical standard.
9. Enforcement strategy.
10. Migration strategy.

## Design AI Responsibilities

- Question whether standardization provides enough value.
- Prefer enforceable standards over prose-only rules when practical.
- Separate the standard from any migration work.
- Keep small/local issues lightweight.

## Human Decision Checkpoints

- Decide whether a standard is needed.
- Accept the standard wording and authority.
- Choose enforcement and migration strategy.

## Enforcement Options

- Compiler/type system.
- Package/API structure.
- Tests.
- Architectural tests.
- Linter.
- CI.
- Code review only.

## Migration Strategy

Choose one:

- FUTURE CODE ONLY
- OPPORTUNISTIC MIGRATION
- MANDATORY MIGRATION

Defining a standard must not automatically authorize refactoring the entire repository.

## Possible Outputs

- No standard needed.
- Accepted engineering standard.
- Enforcement recommendation.
- Migration strategy.
- Follow-up feature/work specification.

## Persistence / Documentation Rules

Persist accepted standards in the canonical engineering standards location once available. Until then, use the current fallback identified by the Knowledge Map. Do not create planned engineering-standard documents unless that is explicitly approved.

## Completion Criteria

- The need for a standard is accepted or rejected.
- Standard wording, enforcement, and migration strategy are clear if accepted.
- Any migration work is routed separately.

## Where To Go Next

- Migration or enforcement implementation: `FEATURE_DEVELOPMENT.md`.
- Architecture impact: `ARCHITECTURE_DISCUSSION.md`.
- Workflow rule change: `AI_WORKFLOW_CHANGE.md`.
