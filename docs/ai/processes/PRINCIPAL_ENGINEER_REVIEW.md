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

Load the existing `docs/engineering/ENGINEERING_RADAR.md` as previous AI recommendation context, not as canonical product/architecture truth. Challenge existing Radar items against current evidence.

## Start This Process

```text
We are using docs/ai/processes/PRINCIPAL_ENGINEER_REVIEW.md.

This is AI-GUIDED. I want you to inspect Playhoot broadly and proactively identify technical gaps, risks, missing capabilities, adequate areas, and future concerns.

Read docs/ai/OPERATING_MODEL.md. Use docs/ai/KNOWLEDGE_MAP.md to retrieve current canonical context. Load docs/engineering/ENGINEERING_RADAR.md as non-authoritative previous recommendation context. Inspect relevant code, tests, and migrations. Do not scan every Markdown file blindly.

Act under the Principal Engineer Contract. Look one level above obvious issues, teach relevant concepts, apply anti-overengineering, distinguish useful sophistication from unnecessary complexity, and make concrete recommendations.

Distinguish EXISTING, PROPOSED, ACCEPTED, and IMPLEMENTED facts. Detect and report drift. This review does not approve architecture or implementation.

Review trigger:
[paste trigger]
```

## Process Stages

1. Establish review trigger, scope, and depth.
2. Load relevant canonical and current implementation context through the Knowledge Map.
3. Load relevant existing Engineering Radar recommendations.
4. Inspect relevant implementation evidence.
5. Identify adequate areas.
6. Discover risks, gaps, opportunities, future concerns, and deliberately unnecessary sophistication.
7. Reevaluate relevant existing Radar items.
8. Apply anti-overengineering.
9. Produce the human-facing Principal Engineer Review report.
10. Synchronize meaningful current recommendations into `docs/engineering/ENGINEERING_RADAR.md`.
11. Route follow-up candidates to appropriate processes.

## Design AI Responsibilities

- Lead the review proactively.
- Inspect broadly but selectively.
- Avoid recommending something in every category.
- Highlight where the current system is already adequate.
- Keep recommendations as recommendations, not approved decisions.
- Do not blindly trust existing Radar items.

## Review Areas

Consider relevant areas such as domain boundaries, architecture/coupling, data ownership, database/storage fit, caching, consistency/concurrency, performance, scalability, observability, logs, metrics, tracing, alerting, reliability, queues/background processing, idempotency, security/privacy, authentication/authorization, deployment, migrations, backups/recovery, testing strategy, developer experience, operational complexity, and cost.

## Human Decision Checkpoints

- Confirm review scope and depth.
- Decide which items deserve follow-up.
- Approve no architecture or implementation solely from the review.
- The human does not need to approve the existence of each non-authoritative Radar item before it can be persisted.
- Material product, architecture, domain, standard, and work decisions still require the existing human approval boundaries.

## Possible Outputs

Engineering Radar style report:

- NOW
- SOON
- LATER
- NOT NEEDED

Each meaningful item should contain problem/risk, evidence/current state, why it matters, recommendation, why now / why not now, trigger for reevaluation where applicable, and suggested next process.

Include a concise Adequate / No Action Needed section when meaningful, but do not mechanically list every reviewed category.

## Persistence / Documentation Rules

The review itself does not approve architecture or implementation. Items must route through the appropriate process before becoming decisions or work.

Synchronize meaningful persistent recommendations to `docs/engineering/ENGINEERING_RADAR.md`.

This synchronization is allowed without turning recommendations into accepted decisions because the Radar is explicitly non-authoritative.

Do not persist:

- every observation;
- every adequate area;
- transient review commentary;
- speculative generic best practice;
- duplicate versions of an existing concern.

Do not create work, decisions, or standards merely because an item is NOW.

## Completion Criteria

- Relevant risks/opportunities and adequate areas were considered.
- Recommendations are classified.
- Meaningful persisted recommendations have been synchronized with the Radar.
- Stale/reconsidered Radar items in scope were updated/removed as needed.
- Actionable recommendations have a suggested next process.
- No material decision was silently accepted.
- No implementation was silently authorized.

## Where To Go Next

- Product question: `PRODUCT_DISCUSSION.md`.
- Architecture decision: `ARCHITECTURE_DISCUSSION.md`.
- Domain boundary: `DOMAIN_DESIGN.md`.
- Technical exploration: `GUIDED_TECHNICAL_EXPLORATION.md`.
- Standard: `ENGINEERING_STANDARD.md`.
- Implementation: `FEATURE_DEVELOPMENT.md`.
