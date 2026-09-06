# Principal Engineer Review

## Purpose

Periodically inspect Playhoot as a whole so technical evolution is not limited by what the human already knows to ask about.

This process is explicitly AI-GUIDED.

## Use This When

- After a meaningful milestone.
- Before public launch.
- After major architecture changes.
- Approximately every 4-6 substantial features.
- Whenever the human requests broad proactive review.

## Do NOT Use This When

- You need to decide one known architecture issue. Use `ARCHITECTURE_DISCUSSION.md`.
- You need to implement a known feature. Use `FEATURE_DEVELOPMENT.md`.
- You need a focused standard. Use `ENGINEERING_STANDARD.md`.

## Starting Information

Provide the trigger for review, any recent changes or milestones, areas of concern, and desired depth.

## Start This Process

```text
We are using docs/ai/processes/PRINCIPAL_ENGINEER_REVIEW.md.

This is AI-GUIDED. I want you to inspect Playhoot broadly and proactively identify technical gaps, risks, missing capabilities, adequate areas, and future concerns.

Read docs/ai/OPERATING_MODEL.md. Use docs/ai/KNOWLEDGE_MAP.md to retrieve current canonical context. Inspect relevant code, tests, and migrations. Do not scan every Markdown file blindly.

Act under the Principal Engineer Contract. Look one level above obvious issues, teach relevant concepts, apply anti-overengineering, distinguish useful sophistication from unnecessary complexity, and make concrete recommendations.

Distinguish EXISTING, PROPOSED, ACCEPTED, and IMPLEMENTED facts. Detect and report drift. This review does not approve architecture or implementation.

Review trigger:
[paste trigger]
```

## Process Stages

1. Establish review trigger and scope.
2. Load canonical context through the Knowledge Map.
3. Inspect relevant implementation, tests, and migrations.
4. Review relevant areas without forcing every category.
5. Identify current strengths and adequate areas.
6. Identify gaps, risks, and missing capabilities.
7. Apply anti-overengineering.
8. Produce Engineering Radar style report.
9. Route meaningful items to next processes.

## Design AI Responsibilities

- Lead the review proactively.
- Inspect broadly but selectively.
- Avoid recommending something in every category.
- Highlight where the current system is already adequate.
- Keep recommendations as recommendations, not approved decisions.

## Review Areas

Consider relevant areas such as domain boundaries, architecture/coupling, data ownership, database/storage fit, caching, consistency/concurrency, performance, scalability, observability, logs, metrics, tracing, alerting, reliability, queues/background processing, idempotency, security/privacy, authentication/authorization, deployment, migrations, backups/recovery, testing strategy, developer experience, operational complexity, and cost.

## Human Decision Checkpoints

- Confirm review scope and depth.
- Decide which items deserve follow-up.
- Approve no architecture or implementation solely from the review.

## Possible Outputs

Engineering Radar style report:

- NOW
- SOON
- LATER
- NOT NEEDED

Each meaningful item should contain problem/risk, evidence/current state, why it matters, recommendation, why now / why not now, trigger for reevaluation where applicable, and suggested next process.

## Persistence / Documentation Rules

The review itself does not approve architecture or implementation. Items must route through the appropriate process before becoming decisions or work.

Persist radar items only when the canonical Engineering Radar mechanism exists or when a later task explicitly creates it.

## Completion Criteria

- Meaningful risks and adequate areas are identified.
- Recommendations are classified.
- Each actionable item has a next process.
- No material decision is silently approved.

## Where To Go Next

- Product question: `PRODUCT_DISCUSSION.md`.
- Architecture decision: `ARCHITECTURE_DISCUSSION.md`.
- Domain boundary: `DOMAIN_DESIGN.md`.
- Technical exploration: `GUIDED_TECHNICAL_EXPLORATION.md`.
- Standard: `ENGINEERING_STANDARD.md`.
- Implementation: `FEATURE_DEVELOPMENT.md`.
