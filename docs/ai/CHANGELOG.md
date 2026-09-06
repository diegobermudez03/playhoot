# AI Workflow Changelog

Status: HISTORICAL WORKFLOW LOG

This file records APPLIED MATERIAL changes to Playhoot's AI-assisted development workflow.

It answers:

"What materially changed in the workflow, when, and why?"

It does NOT define the current workflow.

Current workflow truth lives in:

- `docs/ai/OPERATING_MODEL.md`;
- `docs/ai/KNOWLEDGE_MAP.md`;
- current processes;
- current protocols;
- current templates;
- other current workflow process references.

Do not reconstruct current behavior from the changelog when current workflow artifacts are available.

## Entry Shape

```text
## YYYY-MM-DD - <Short change title>

**Change**
- <what materially changed>

**Reason**
- <problem/outcome>

**Affected workflow artifacts**
- `<path>`

**Compatibility / migration**
- <impact on existing persistent artifacts, or None>

**Notes**
- <important consequence / limitation, when materially useful>
```

Do not include full diffs, commit dumps, every implementation detail, or copied runbook contents. Git remains the detailed change history.

## When To Add Entries

Add an entry when an APPLIED workflow change materially changes semantics.

Examples:

- role/authority change;
- lifecycle/status change;
- new/removed mandatory artifact;
- material process change;
- new protocol;
- new persistent workflow mechanism;
- changed review gate;
- changed knowledge routing semantics;
- material persistent-template semantics.

Do not add entries for typo corrections, formatting, broken links, purely editorial wording, or normal product/architecture/code changes.

The changelog is for APPLIED changes. Do not use it as a proposal backlog, rejected-proposal register, or future roadmap.

## History Semantics

Applied changelog entries are historical.

Do not substantively rewrite old entries to make them match later workflow.

Typo/link corrections are allowed if historical meaning is unchanged.

If a workflow change is later reversed:

- apply a new workflow change;
- update current workflow artifacts;
- append a new changelog entry describing the reversal.

Do not delete the original historical entry.

Do not introduce Workflow v1/v2, semantic version numbers, WFC IDs, workflow decision record IDs, or separate workflow proposal files.

Current Git history plus this changelog is sufficient. If stable workflow-change identity becomes useful later, propose it through `AI_WORKFLOW_CHANGE.md`.

## 2026-09-06 - Established governed workflow evolution

**Change**
- Formalized material-vs-editorial workflow changes.
- Added workflow impact and artifact-compatibility analysis.
- Required explicit human approval for material workflow changes.
- Formalized consistency validation and conceptual dry runs.
- Created this persistent AI workflow changelog.

**Reason**
- Playhoot's AI-assisted development workflow needs a safe evolution path so material changes are not applied by editing one document in isolation while leaving contradictory semantics elsewhere.

**Affected workflow artifacts**
- `docs/ai/processes/AI_WORKFLOW_CHANGE.md`
- `docs/ai/KNOWLEDGE_MAP.md`
- `docs/ai/CHANGELOG.md`

**Compatibility / migration**
- No existing WORK, ADR, PDR, Radar, domain-documentation, code, test, or migration artifact migration is required by this workflow mechanism.

## 2026-09-06 - Graduated canonical knowledge routing

**Change**
- Completed the end-to-end knowledge/workflow consistency audit.
- Removed transitional routing semantics from the Knowledge Map.
- Graduated `docs/ai/KNOWLEDGE_MAP.md` to the stable canonical routing map.
- Clarified that DOMAIN-DEPENDENT routing applies only to accepted domains.

**Reason**
- AI agents can reconstruct Playhoot's accepted knowledge, current implementation state, non-authoritative sources, historical rationale, engineering standards, work authority, review flow, Radar persistence, and workflow evolution from repository artifacts without relying on historical chat context.

**Affected workflow artifacts**
- `docs/ai/KNOWLEDGE_MAP.md`
- `docs/ai/CHANGELOG.md`

**Compatibility / migration**
- No existing ADR, PDR, WORK, Radar, domain-documentation, code, test, or migration artifact migration is required. Unresolved business/domain questions remain unresolved rather than being implicitly accepted.

## 2026-09-06 - Separated human workflow surfaces from agent execution

**Change**
- Made `docs/ai/processes/*` human-facing process guides.
- Moved internal execution semantics into matching agent protocols under `docs/ai/protocols/*`.
- Introduced temporary resumable process workspaces under `docs/ai/workspaces/active/<process-topic>/`.
- Separated `HUMAN_REVIEW.md` from `AI_CONTEXT.md`.
- Established that no material decision may be hidden only in agent context.
- Made workspace promotion and cleanup explicit.

**Reason**
- Human process pages should explain how to start, review, and decide without exposing internal execution mechanics. Agents still need durable protocol instructions to preserve Playhoot's authority, routing, persistence, compatibility, and validation semantics across sessions.

**Affected workflow artifacts**
- `AGENTS.md`
- `docs/ai/README.md`
- `docs/ai/OPERATING_MODEL.md`
- `docs/ai/KNOWLEDGE_MAP.md`
- `docs/ai/CHANGELOG.md`
- `docs/ai/processes/*`
- `docs/ai/protocols/*`
- `docs/ai/workspaces/README.md`

**Compatibility / migration**
- Existing canonical product, architecture, domain, decision, work, and radar artifacts require no semantic migration.
- `docs/work/active/WORK-0001-create-game-draft.md` remains DRAFT and unchanged.
- Future and resumed process execution uses the new workspace model.
