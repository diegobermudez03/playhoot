# Product Discussion

Use this when you want to decide what Playhoot should do.

## When to use it

- You are considering product scope, user value, creator/player experience, launch target, or priority.
- You want to accept, reject, defer, or reshape a product idea before implementation.
- The main question is "should Playhoot do this?", not "how should the system be structured?"

## Step 1 - Start

Paste:

```text
I want to use the Playhoot Product Discussion process.

Product question or idea:
[describe it]

Why it matters:
[user/business reason]

Known constraints or timing:
[optional]
```

## Step 2 - What the AI will give you

For simple questions, the AI may answer directly with a recommendation.

For non-trivial decisions, the AI will prepare `docs/ai/workspaces/active/<product-topic>/HUMAN_REVIEW.md` with the product problem, options, tradeoffs, recommendation, and any material decision needed from you.

## Step 3 - What you need to do

Review the recommendation and respond with one of:

- approve the product direction;
- reject it;
- defer it;
- ask questions;
- request modifications;
- route it to another process.

## Step 4 - What happens next

If you approve a material product decision, the AI will route any durable decision record, canonical product documentation, or follow-up implementation work to the appropriate owner.

Approval of product direction does not automatically implement anything.

## Resume in a new session

```text
Resume the Playhoot Product Discussion process from:
docs/ai/workspaces/active/<process-topic>/
```
