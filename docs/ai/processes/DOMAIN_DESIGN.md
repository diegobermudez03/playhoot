# Domain Design

Use this when you want to decide Playhoot business/domain ownership.

## When to use it

- A new domain is proposed.
- A responsibility may belong in a different domain.
- Two domains may actually be one, or one domain may contain unrelated responsibilities.
- Ownership is unclear.

## Step 1 - Start

Paste:

```text
I want to use the Playhoot Domain Design process.

Domain question:
[describe it]

Responsibility or behavior involved:
[describe it]

Files, docs, or code that triggered this:
[optional]
```

## Step 2 - What the AI will give you

For small boundary questions, the AI may answer directly with a recommendation.

For material boundary decisions, the AI will prepare `docs/ai/workspaces/active/<domain-topic>/HUMAN_REVIEW.md` with the business responsibility, ownership options, invariants, lifecycle concerns, tradeoffs, recommendation, and migration implications.

## Step 3 - What you need to do

Review the checkpoint and respond with one of:

- accept a boundary;
- reject the proposed boundary change;
- defer the question;
- ask questions;
- request modifications;
- route implementation or migration work to Feature Development.

## Step 4 - What happens next

Accepted domain decisions update their canonical architecture/domain owners when appropriate.

Implementation and migration are separate; they require Feature Development and approved WORK before code changes.

## Resume in a new session

```text
Resume the Playhoot Domain Design process from:
docs/ai/workspaces/active/<process-topic>/
```
