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

## 2026-09-06 - Introduced explicit AI execution surfaces

**Change**
- Replaced product-specific AI role names (ChatGPT, Design AI, Codex, Implementation Agent) with capability-based execution surfaces: HUMAN, CONVERSATIONAL AI, CODEBASE AGENT, and INDEPENDENT REVIEWER.
- Made every human-facing process step under `docs/ai/processes/*` explicitly label which surface it requires.
- Added a repository URL/ref block to Conversational AI starter and resume prompts so a fresh conversational session knows what repository state it can reason about.
- Added a repository bootstrap instruction to that same block: a fresh Conversational AI opens the supplied repository/ref and reads `/AGENTS.md` first, then follows its routing instructions rather than scanning documentation indiscriminately. Made explicit that a repository URL does not by itself imply browsing/repository access; the AI must report inaccessible repository/ref/file state instead of assuming its contents.
- Formalized the CODEBASE AGENT HANDOFF concept in `docs/ai/OPERATING_MODEL.md`: a temporary, human-visible prompt a Conversational AI produces when a repository-local action is needed that it cannot perform itself.
- Defined independent implementation review as normally fulfilled by a fresh Codebase Agent operating read-only.
- Preserved human material-decision authority; a Codebase Agent persisting a workspace or Radar update on behalf of a handoff does not gain authority over the decision being recorded.

**Reason**
- The workflow previously named roles after specific products (ChatGPT, Codex), which forced the human to guess which commercial tool a given step belonged to, and did not distinguish a tool's conversational capability from its repository-mutation capability.

**Affected workflow artifacts**
- `AGENTS.md`
- `docs/ai/README.md`
- `docs/ai/OPERATING_MODEL.md`
- `docs/ai/KNOWLEDGE_MAP.md`
- `docs/ai/CHANGELOG.md`
- `docs/ai/processes/*`
- `docs/ai/protocols/*`
- `docs/ai/workspaces/README.md`
- `docs/work/README.md`
- `docs/work/templates/WORK_SPEC.template.md`
- `docs/engineering/ENGINEERING_RADAR.md`

**Compatibility / migration**
- Existing canonical product, architecture, domain, decision, work, and radar artifacts require no semantic migration.
- `docs/work/active/WORK-0001-create-game-draft.md` remains unchanged; its historical references to "Codex" are compatible historical wording interpreted under current terminology as Codebase Agent.
- Current normative workflow documentation uses the new capability terminology going forward.

**Notes**
- A single product may still fulfill more than one surface when it genuinely has the capability; the abstraction is capability-based, not a mandate to switch tools.

## 2026-09-06 - Made repository mutation exclusively a Codebase Agent surface

**Change**
- Clarified that execution surface is action-scoped: while acting as CONVERSATIONAL AI, an agent may converse, reason, and inspect remotely accessible repository state, but does not create, modify, delete, or persist a repository file, does not persist/update/remove a workspace, does not persist a decision/canonical document, does not change WORK state, does not update Radar, does not write code/tests, and does not run checkout-local commands or otherwise depend on local/unpushed checkout state.
- Made explicit that any such action is a CODEBASE AGENT step regardless of whether the underlying commercial product could technically perform it — the same product may switch surfaces between actions, but a repository mutation performed by a conversational product does not become a CONVERSATIONAL AI action.
- Removed conditional phrasing across `docs/ai/OPERATING_MODEL.md`, `docs/ai/workspaces/README.md`, `docs/ai/README.md`, `docs/engineering/ENGINEERING_RADAR.md`, all `docs/ai/processes/*` guides, and all `docs/ai/protocols/*` that made repository persistence/mutation sound conditional on "the Conversational AI cannot write" or "lacks write access", or that described a Conversational AI as maintaining/persisting/synchronizing repository files itself.
- Normalized the CODEBASE AGENT HANDOFF concept in `docs/ai/OPERATING_MODEL.md`: a Conversational AI produces a handoff whenever a repository-local mutation or checkout-local execution is needed, not only when it happens to lack write capability.
- Clarified workspace ownership: the Conversational AI prepares the semantic content of `HUMAN_REVIEW.md`/`AI_CONTEXT.md`; persisting, updating, or removing those files is always a Codebase Agent action reached via a Codebase Agent Handoff.
- Confirmed remote reading remains allowed: a Conversational AI may still inspect the supplied repository/ref, code, docs, tests, and migrations when its tool capabilities allow.

**Reason**
- Several human-facing process guides said a Conversational AI would prepare, maintain, persist, or synchronize repository/workspace files, "if it cannot write" to the repository — implying it might otherwise mutate the repository directly while remaining in the Conversational AI surface. That contradicted the capability-based execution-surface model, where the surface is determined by the action (repository mutation is always Codebase Agent), not by which product is being used or what it happens to be technically capable of.

**Affected workflow artifacts**
- `docs/ai/OPERATING_MODEL.md`
- `docs/ai/README.md`
- `docs/ai/workspaces/README.md`
- `docs/ai/CHANGELOG.md`
- `docs/ai/processes/PRODUCT_DISCUSSION.md`
- `docs/ai/processes/ARCHITECTURE_DISCUSSION.md`
- `docs/ai/processes/GUIDED_TECHNICAL_EXPLORATION.md`
- `docs/ai/processes/DOMAIN_DESIGN.md`
- `docs/ai/processes/FEATURE_DEVELOPMENT.md`
- `docs/ai/processes/ENGINEERING_STANDARD.md`
- `docs/ai/processes/PRINCIPAL_ENGINEER_REVIEW.md`
- `docs/ai/processes/AI_WORKFLOW_CHANGE.md`
- `docs/ai/protocols/PRODUCT_DISCUSSION.md`
- `docs/ai/protocols/ARCHITECTURE_DISCUSSION.md`
- `docs/ai/protocols/GUIDED_TECHNICAL_EXPLORATION.md`
- `docs/ai/protocols/DOMAIN_DESIGN.md`
- `docs/ai/protocols/FEATURE_DEVELOPMENT.md`
- `docs/ai/protocols/ENGINEERING_STANDARD.md`
- `docs/ai/protocols/PRINCIPAL_ENGINEER_REVIEW.md`
- `docs/ai/protocols/AI_WORKFLOW_CHANGE.md`
- `docs/engineering/ENGINEERING_RADAR.md`

**Compatibility / migration**
- Existing product, architecture, domain, standard, and decision-record artifacts are unchanged.
- `docs/work/active/WORK-0001-create-game-draft.md` remains READY and unchanged; this is a wording/actor-consistency correction, not an implementation or lifecycle change.

**Notes**
- Human material-decision authority, Codebase Agent implementation autonomy, Independent Reviewer semantics, workspace persistence/durability requirements, decision thresholds, WORK lifecycle, and review verdict/finding semantics are unchanged by this correction.
