# Guided Technical Exploration

## Purpose

Use AI-guided leadership when the human does not know the technical space well enough to frame all relevant questions.

This process explicitly switches to AI-GUIDED leadership.

## Use This When

- You want the AI to lead exploration of a technical area.
- Examples: observability, caching, storage options, backups/disaster recovery, security, tracing, deployment infrastructure.

## Do NOT Use This When

- You already have a specific architecture decision to make. Use `ARCHITECTURE_DISCUSSION.md`.
- You are defining a reusable standard. Use `ENGINEERING_STANDARD.md`.
- You are ready to implement approved work. Use `FEATURE_DEVELOPMENT.md`.

## Starting Information

Provide the technical area, the current concern or goal, what you already know, and how deep you want the first pass to be.

## Start This Process

```text
We are using docs/ai/processes/GUIDED_TECHNICAL_EXPLORATION.md.

I am intentionally asking you to lead this technical exploration. Do not limit the discussion to technologies or concepts I mention.

Read docs/ai/OPERATING_MODEL.md. Use docs/ai/KNOWLEDGE_MAP.md to retrieve relevant repository context. Inspect code, tests, and migrations when current implementation matters.

Act under the Principal Engineer Contract. Map the problem space, identify concerns I did not mention, teach the important concepts, challenge my framing, assess current implementation where relevant, apply anti-overengineering, present realistic alternatives, and make concrete recommendations.

Distinguish EXISTING, PROPOSED, ACCEPTED, and IMPLEMENTED facts. Detect and report drift. Do not silently create an architecture decision or implementation task from your recommendation.

Technical area / concern:
[paste concern]
```

## Process Stages

1. Understand the actual Playhoot context and goal.
2. Map the problem space.
3. Identify relevant concerns the human did not mention.
4. Teach the important concepts.
5. Assess the current implementation if relevant.
6. Identify gaps.
7. Present realistic alternatives.
8. Explain tradeoffs.
9. Apply anti-overengineering.
10. Make concrete recommendations.
11. Classify recommendations where useful: NOW / SOON / LATER / NOT NEEDED.
12. Route each resulting concern.

## Design AI Responsibilities

- Lead the exploration proactively.
- Teach enough for the human to make informed decisions.
- Highlight adequate current areas, not only gaps.
- Avoid recommending sophistication without a concrete present problem.

## Human Decision Checkpoints

- Confirm the exploration goal.
- Decide which recommendations deserve a follow-up process.
- Accept no material decision until routed through the appropriate process.

## Possible Outputs

- No action.
- Engineering Radar item.
- Route to architecture discussion, engineering standard, feature development, domain design, or product discussion.

## Persistence / Documentation Rules

Exploration notes are not accepted decisions. Meaningful non-authoritative technical recommendations may be persisted to `docs/engineering/ENGINEERING_RADAR.md`. Accepted outcomes must be persisted in their proper canonical owner. Do not update current-state docs to describe future possibilities.

## Completion Criteria

- The human understands the relevant space well enough to choose next steps.
- Recommendations are classified.
- Each actionable item has a next process.

## Where To Go Next

- System-level decision: `ARCHITECTURE_DISCUSSION.md`.
- Reusable rule: `ENGINEERING_STANDARD.md`.
- Concrete implementation: `FEATURE_DEVELOPMENT.md`.
- Product implication: `PRODUCT_DISCUSSION.md`.
- Domain boundary: `DOMAIN_DESIGN.md`.
