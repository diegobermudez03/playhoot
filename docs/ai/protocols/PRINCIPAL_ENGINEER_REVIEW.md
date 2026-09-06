# Principal Engineer Review Protocol

Status: AGENT EXECUTION PROTOCOL

Use with the human-facing guide at `docs/ai/processes/PRINCIPAL_ENGINEER_REVIEW.md`. This protocol is primarily executed/coordinated by a CONVERSATIONAL AI; repository-local evidence and Radar persistence are always CODEBASE AGENT steps.

## Scope

This is AI-guided proactive technical review. It identifies risks, gaps, adequate areas, recommendations, and future concerns. It does not approve architecture, standards, product behavior, WORK, or implementation.

## HUMAN SURFACE

Provide a Principal Engineer Review report with relevant findings and recommendations, usually grouped as NOW, SOON, LATER, and NOT NEEDED. Include adequate/no-action-needed areas when they are meaningful.

For non-trivial or multi-session reviews, prepare `docs/ai/workspaces/active/<review-topic>/HUMAN_REVIEW.md` content and persist it via a CODEBASE AGENT HANDOFF.

## SYSTEM EFFECTS

- Load canonical and current implementation context through the Knowledge Map.
- Load `docs/engineering/ENGINEERING_RADAR.md` as non-authoritative prior recommendation context.
- Inspect relevant code, tests, migrations, current-state docs, and standards.
- Challenge existing Radar items against current evidence.
- Synchronize meaningful current recommendations into the Radar.
- Do not persist every observation, adequate area, transient note, or generic best practice.
- Do not create work, decisions, or standards merely because a recommendation is NOW.
- For non-trivial or multi-session workspaces, ensure the paired
  `docs/ai/workspaces/active/<review-topic>/AI_CONTEXT.md` is persisted
  alongside `HUMAN_REVIEW.md` via a CODEBASE AGENT HANDOFF, following
  `docs/ai/workspaces/README.md`.
- Before asking for a material decision, ensure the workspace has enough
  persisted state for a fresh agent to resume. When the human answers, update
  the active process state and/or durable owner before future continuation
  depends on it, keeping the human-facing checkpoint current where practical.

No material decision may exist only in `AI_CONTEXT.md`.

## Execution

1. Establish trigger, scope, and depth.
2. Load relevant canonical/current context and existing Radar.
3. Inspect implementation evidence proportionally.
4. Identify adequate areas, risks, gaps, opportunities, future concerns, and unnecessary sophistication.
5. Reevaluate relevant Radar items in scope.
6. Classify meaningful recommendations.
7. Synchronize Radar updates when useful.
8. Route follow-up candidates to the appropriate process.

Consider relevant areas such as domain boundaries, architecture/coupling, data ownership, consistency/concurrency, reliability, observability, security/privacy, deployment, migrations, backup/recovery, testing strategy, developer experience, operational complexity, and cost.

## Completion

Complete when relevant risks/opportunities and adequate areas were considered, recommendations are classified, meaningful Radar updates are synchronized, actionable recommendations have next processes, and no material decision or implementation was silently authorized.
