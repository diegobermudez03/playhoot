# Domain Design

Use this when you want to decide Playhoot business/domain ownership.

Surface flow: CONVERSATIONAL AI <-> HUMAN, with CODEBASE AGENT for repository evidence/persistence when needed.

## When to use it

- A new domain is proposed.
- A responsibility may belong in a different domain.
- Two domains may actually be one, or one domain may contain unrelated responsibilities.
- Ownership is unclear.

## Step 1 - HUMAN

Paste into a CONVERSATIONAL AI:

```text
Repository:
https://github.com/diegobermudez03/playhoot

Repository ref:
<branch or commit>

Process:
Playhoot Domain Design

Domain question:
[describe it]

Responsibility or behavior involved:
[describe it]

Files, docs, or code that triggered this:
[optional]
```

## Step 2 - CONVERSATIONAL AI

For small boundary questions, the AI may answer directly with a recommendation.

For material boundary decisions, the AI will prepare `docs/ai/workspaces/active/<domain-topic>/HUMAN_REVIEW.md` with the business responsibility, ownership options, invariants, lifecycle concerns, tradeoffs, recommendation, and migration implications. If the workspace needs to be persisted and the Conversational AI cannot write to the repository, it will give you a CODEBASE AGENT HANDOFF.

## Step 3 - HUMAN

Review the checkpoint and respond with one of:

- accept a boundary;
- reject the proposed boundary change;
- defer the question;
- ask questions;
- request modifications;
- route implementation or migration work to Feature Development.

## Step 4 - CONVERSATIONAL AI / CODEBASE AGENT

Accepted domain decisions update their canonical architecture/domain owners when appropriate. Repository-side persistence of that outcome is a CODEBASE AGENT step, normally reached through a CODEBASE AGENT HANDOFF from the Conversational AI.

Implementation and migration are separate; they require Feature Development and approved WORK before code changes.

## Resume in a new session - CONVERSATIONAL AI

Paste into a CONVERSATIONAL AI:

```text
Repository:
https://github.com/diegobermudez03/playhoot

Repository ref:
<branch or commit>

Resume the Playhoot Domain Design process from:
docs/ai/workspaces/active/<process-topic>/
```
