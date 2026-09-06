# Architecture Discussion

Use this when you want to decide how Playhoot should be structured technically.

## When to use it

- A question affects system structure, ownership, coupling, persistence, infrastructure, or public contracts.
- You want help comparing architecture alternatives before implementation.
- The issue is broader than a local code detail.

## Step 1 - Start

Paste:

```text
I want to use the Playhoot Architecture Discussion process.

Architecture question:
[describe it]

Trigger or concern:
[why this came up]

Options I am considering:
[optional]
```

## Step 2 - What the AI will give you

For small questions, the AI may answer directly with a recommendation.

For material decisions, the AI will prepare `docs/ai/workspaces/active/<architecture-topic>/HUMAN_REVIEW.md` with the underlying problem, options, tradeoffs, recommendation, affected boundaries, and any decision needed from you.

## Step 3 - What you need to do

Review the checkpoint and respond with one of:

- approve an option;
- reject the recommendation;
- defer the decision;
- ask questions;
- request modifications;
- route to domain design, technical exploration, standard-setting, or feature development.

## Step 4 - What happens next

If you approve a material architecture decision, the AI will route any decision record, canonical architecture/domain documentation, or follow-up implementation work to the appropriate owner.

Approval of architecture does not mean the implementation already exists.

## Resume in a new session

```text
Resume the Playhoot Architecture Discussion process from:
docs/ai/workspaces/active/<process-topic>/
```
