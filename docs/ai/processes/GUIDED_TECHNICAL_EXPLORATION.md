# Guided Technical Exploration

Use this when you want the AI to lead you through an unfamiliar technical area.

## When to use it

- You know the area matters but do not know the right questions yet.
- You want the AI to map concerns, explain tradeoffs, and recommend next steps.
- Examples include observability, caching, storage options, security, backups, tracing, or deployment infrastructure.

## Step 1 - Start

Paste:

```text
I want to use the Playhoot Guided Technical Exploration process.

Technical area or concern:
[describe it]

What I already know:
[optional]

How deep the first pass should be:
[optional]
```

## Step 2 - What the AI will give you

The AI will lead the exploration, explain relevant concepts, identify gaps and adequate areas, compare options, and classify recommendations.

For non-trivial exploration, the AI may prepare `docs/ai/workspaces/active/<exploration-topic>/HUMAN_REVIEW.md` with a clear recommendation and suggested next process.

## Step 3 - What you need to do

Choose what deserves follow-up:

- no action;
- add or update a non-authoritative radar recommendation;
- route to architecture discussion;
- route to domain design;
- route to engineering standard;
- route to feature development;
- ask for another exploration pass.

## Step 4 - What happens next

Exploration does not itself approve architecture, standards, product behavior, or implementation.

Actionable recommendations move into the relevant decision or implementation process only if you choose to continue.

## Resume in a new session

```text
Resume the Playhoot Guided Technical Exploration process from:
docs/ai/workspaces/active/<process-topic>/
```
