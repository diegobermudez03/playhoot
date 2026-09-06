# Guided Technical Exploration Protocol

Status: AGENT EXECUTION PROTOCOL

Use with the human-facing guide at `docs/ai/processes/GUIDED_TECHNICAL_EXPLORATION.md`. This protocol is primarily executed by a CONVERSATIONAL AI; inspecting or changing local repository evidence/persistent artifacts is a CODEBASE AGENT step.

## Scope

This protocol is AI-guided exploration for technical areas the human has not fully framed. It produces understanding and recommendations, not approved architecture, standards, product behavior, or implementation authority.

## HUMAN SURFACE

Teach the relevant concepts, map the problem space, identify adequate areas and gaps, compare realistic alternatives, classify recommendations, and suggest next process.

For non-trivial or multi-session work, prepare `docs/ai/workspaces/active/<exploration-topic>/HUMAN_REVIEW.md` content and persist it via a CODEBASE AGENT HANDOFF.

## SYSTEM EFFECTS

- Load relevant canonical and implementation context through the Knowledge Map.
- Inspect code, tests, migrations, and current-state docs when the current implementation matters.
- Identify concerns the human did not mention.
- Apply anti-overengineering and avoid generic best-practice recommendations without Playhoot evidence.
- Persist meaningful non-authoritative recommendations to `docs/engineering/ENGINEERING_RADAR.md` when useful.
- Route accepted outcomes to their proper canonical owner through the owning process.
- Do not update current-state docs to describe future possibilities.
- For non-trivial or multi-session workspaces, ensure the paired
  `docs/ai/workspaces/active/<exploration-topic>/AI_CONTEXT.md` is persisted
  alongside `HUMAN_REVIEW.md` via a CODEBASE AGENT HANDOFF, following
  `docs/ai/workspaces/README.md`.
- Before asking for a material decision, ensure the workspace has enough
  persisted state for a fresh agent to resume. When the human answers, update
  the active process state and/or durable owner before future continuation
  depends on it, keeping the human-facing checkpoint current where practical.

No material decision may exist only in `AI_CONTEXT.md`.

## Execution

1. Understand the Playhoot context and exploration goal.
2. Map the technical area and important concepts.
3. Assess current implementation where relevant.
4. Identify gaps, risks, adequate areas, and unnecessary sophistication.
5. Present alternatives and tradeoffs.
6. Classify recommendations as NOW, SOON, LATER, or NOT NEEDED where useful.
7. Route each actionable item to the next process.

## Completion

Complete when the human has enough understanding to choose next steps, recommendations are classified, and actionable items are routed.
