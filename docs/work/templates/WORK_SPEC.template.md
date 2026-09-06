# Work Specification Template

This template contains authoring guidance. Template-only instructions must not be copied into the final WORK record.

Resulting record shape:

```text
# WORK-NNNN: <Title>

Status: DRAFT
Created: YYYY-MM-DD
Last status change: YYYY-MM-DD

Related decisions:
- <ADR/PDR or None>

Canonical context:
- <relevant canonical paths>

## Outcome

<Concrete result this work should achieve and why it matters.>

## Context

<Only context necessary to understand the implementation task.>

## Scope

### In Scope

- ...

### Out of Scope

- ...

## Approved Design

<Only material task-specific decisions needed to constrain implementation.>

Do NOT prescribe incidental code structure.

Do NOT duplicate complete architecture/domain documents.

If no special approved design beyond canonical context is required, say so concisely.

## Constraints and Invariants

- <task-relevant invariant/constraint>

Reference canonical sources when appropriate instead of copying them.

## Acceptance Criteria

- <observable/testable result>
- ...

Acceptance criteria describe required behavior/outcome, not implementation progress.

## Implementation Freedom

<Local choices intentionally left to Codex, or reference the normal Operating Model autonomy boundary when no special clarification is needed.>

## Verification

- <tests/checks/commands or verification expectations>

Do not invent exact commands before repository inspection when they are not known.

## Documentation Impact

### Accepted / Canonical Knowledge

- `<path>` - <impact>
- None, if no accepted knowledge changes.

### Current-State Documentation After Implementation

- `<path>` - <what actual implementation state will need synchronization>
- None, if no current-state docs change.

### Intentionally Unchanged

Optional. Include only when useful to prevent accidental broad documentation rewrites.

Current-state documentation must not be changed to future state before the implementation exists.

## Blockers

- None.

A DRAFT may list unresolved material blockers.

A READY spec MUST have no unresolved material blocker.

Questions that are genuinely inside Codex's implementation autonomy should not remain here as blockers.

## Completion Record

Leave this section unfilled while DRAFT/READY unless useful metadata is required.

When closing DONE, record concisely:

- implementation summary;
- verification actually performed and any limitations;
- required documentation synchronized;
- approved deviations from the original READY specification, if any;
- relevant follow-up work that remains outside scope.

For CANCELLED, record the concise cancellation reason instead.

Do not turn Completion Record into a commit/file dump. Git remains the detailed implementation history.
```

Optional sections may be added only when materially relevant, such as Data / Migration Impact, Compatibility, Rollout / Backfill, Operational Considerations, or Security Considerations.

Do not create empty boilerplate sections for concerns that do not matter.

If an optional concern becomes a material architecture/product/standard decision, route it through the corresponding decision process instead of silently deciding it in the work spec.
