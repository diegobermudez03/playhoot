# AI Workflow Change

## Purpose

Modify this AI-assisted development system itself.

## Use This When

- You want to change Codex autonomy, review requirements, documentation sync, process structure, AI prompts, or mandatory workflow artifacts.
- Examples: require threat modeling, add a new process, change runbook prompts, change bootstrap or knowledge routing.

## Do NOT Use This When

- You are changing product behavior.
- You are changing architecture or engineering standards for Playhoot itself.
- You are implementing a product feature.

## Starting Information

Provide the requested workflow change, the problem with the current workflow, desired outcome, and any affected runbooks or prompts.

## Start This Process

```text
We are using docs/ai/processes/AI_WORKFLOW_CHANGE.md.

Read docs/ai/OPERATING_MODEL.md. Use docs/ai/KNOWLEDGE_MAP.md to retrieve relevant AI workflow context. Inspect affected workflow files directly. Inspect code/tests/migrations only if current implementation matters to the workflow change.

Act under the Principal Engineer Contract. Challenge whether the workflow change is necessary, propose simpler or alternative workflow changes, teach tradeoffs, keep the change minimal and coherent, and make a recommendation.

Distinguish EXISTING, PROPOSED, ACCEPTED, and IMPLEMENTED facts. Detect and report drift. Do not modify product, architecture, domain, or engineering-standard rules merely because the AI workflow changes.

Requested workflow change:
[paste request]
```

## Process Stages

1. REQUESTED WORKFLOW CHANGE: state the requested change.
2. Desired outcome: clarify the behavior wanted.
3. Why current workflow is insufficient.
4. Identify affected workflow components.
5. Identify affected runbooks/prompts/templates/bootstrap/knowledge routing.
6. Design minimal coherent change.
7. Human approval.
8. Apply changes.
9. Workflow consistency check.
10. Dry-run scenarios.
11. Update workflow changelog once that mechanism exists.

## Design AI Responsibilities

- Keep the workflow self-modifying safely.
- Avoid broad rewrites when a targeted change works.
- Check for stale references, contradictory prompts, duplicated authority, and incomplete routing.
- Preserve human authority over material decisions.

## Human Decision Checkpoints

- Confirm current workflow insufficiency.
- Approve the minimal coherent change before implementation.
- Accept results after consistency check and dry run.

## Potentially Affected Artifacts

- `docs/ai/README.md`
- `docs/ai/OPERATING_MODEL.md`
- `docs/ai/KNOWLEDGE_MAP.md`
- `docs/ai/processes/*`
- `AGENTS.md`
- future AI templates
- future workflow `CHANGELOG`

## Consistency Check

After a workflow change, inspect all affected processes for stale references, contradictory prompts, duplicated authority, and incomplete routing.

## Dry Run

Test the changed workflow conceptually against at least:

- one normal feature;
- one architecture decision;
- one relevant edge case for the change.

## Possible Outputs

- No workflow change.
- Accepted workflow change.
- Updated workflow files.
- Follow-up workflow change proposal.
- Deferred changelog entry until the mechanism exists.

## Persistence / Documentation Rules

Workflow changes belong in AI workflow artifacts only. Do not modify product or architecture rules merely because the AI workflow changes.

## Completion Criteria

- Human approved the workflow change.
- Affected workflow artifacts are updated.
- Consistency check finds no unresolved contradictions.
- Dry-run scenarios remain coherent.
- No product, architecture, or engineering-standard decision was silently changed.

## Where To Go Next

- Product process affected: `PRODUCT_DISCUSSION.md`.
- Architecture process affected: `ARCHITECTURE_DISCUSSION.md`.
- Feature process affected: `FEATURE_DEVELOPMENT.md`.
- Standard process affected: `ENGINEERING_STANDARD.md`.
