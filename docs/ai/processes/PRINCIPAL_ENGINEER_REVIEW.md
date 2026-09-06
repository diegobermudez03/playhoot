# Principal Engineer Review

Use this when you want the AI to proactively inspect Playhoot for risks, gaps, adequate areas, and next-step recommendations.

## When to use it

- After a meaningful milestone.
- Before public launch.
- After major architecture changes.
- Approximately every 4-6 substantial features.
- Whenever you want broad Principal Engineer judgment rather than a narrow answer.

## Step 1 - Start

Paste:

```text
I want to use the Playhoot Principal Engineer Review process.

Review trigger:
[milestone, concern, or timing]

Areas I especially care about:
[optional]

Desired depth:
[optional]
```

## Step 2 - What the AI will give you

The AI will produce a Principal Engineer Review covering relevant risks, opportunities, adequate areas, and recommendations. Recommendations are usually classified as NOW, SOON, LATER, or NOT NEEDED.

For non-trivial reviews, the AI may prepare `docs/ai/workspaces/active/<review-topic>/HUMAN_REVIEW.md` with the review summary and proposed Radar updates.

## Step 3 - What you need to do

Decide which recommendations deserve follow-up.

You do not need to approve every non-authoritative Radar update before it can be persisted, but material product, architecture, domain, standard, and implementation decisions still require their normal processes.

## Step 4 - What happens next

Meaningful recommendations may be synchronized into `docs/engineering/ENGINEERING_RADAR.md`.

Radar items are not approved architecture, standards, product roadmap, or implementation authority.

## Resume in a new session

```text
Resume the Playhoot Principal Engineer Review process from:
docs/ai/workspaces/active/<process-topic>/
```
