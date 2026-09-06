# Architecture Discussion

## Purpose

Discuss system-level technical decisions independently from a specific implementation task.

## Use This When

- The question affects system structure, ownership, coupling, persistence, infrastructure, or public contracts.
- Examples: one domain or two, orchestration or events, session state persistence, cross-domain concerns, subsystem boundaries, responsibility location, system-level technology fit.

## Do NOT Use This When

- The question is only about local implementation detail.
- The main concern is product value. Use `PRODUCT_DISCUSSION.md`.
- The question is specifically about domain ownership. Use `DOMAIN_DESIGN.md`.
- You need AI-guided teaching in an unfamiliar technical area before deciding. Use `GUIDED_TECHNICAL_EXPLORATION.md`.

## Starting Information

Provide the architecture question, the trigger for asking it, known constraints, and any options you are considering.

## Start This Process

```text
We are using docs/ai/processes/ARCHITECTURE_DISCUSSION.md.

Read docs/ai/OPERATING_MODEL.md. Use docs/ai/KNOWLEDGE_MAP.md to retrieve relevant product and architecture context. Inspect code, tests, and migrations when current implementation matters.

Act under the Principal Engineer Contract. Do not accept the boundary or solution implied by my question. Challenge the framing, look one level above the immediate question, propose alternatives I did not mention, teach relevant concepts and tradeoffs, apply anti-overengineering, and make a concrete recommendation.

Distinguish EXISTING, PROPOSED, ACCEPTED, and IMPLEMENTED facts. Detect and report drift. Do not implement anything unless a later approved implementation process starts.

Architecture question:
[paste question]
```

## Process Stages

1. QUESTION / CONCERN: state the decision under discussion.
2. Load current architecture: use the Knowledge Map, not a full Markdown scan.
3. Establish current state: inspect implementation if actual behavior matters.
4. Define underlying problem: identify the real pressure behind the question.
5. Identify constraints/invariants: include product, domain, data, and operational constraints where relevant.
6. Challenge framing/boundary: test whether the implied boundary, technology, or solution is correct.
7. Explore alternatives: include no change and unmentioned options.
8. System-wide impact analysis: discuss only relevant lenses.
9. Tradeoffs: compare benefits, complexity, cost, and risks.
10. Recommendation: make a concrete recommendation with confidence and caveats.
11. Human decision: accept, reject, defer, or request more exploration.
12. Determine whether ADR/canonical architecture changes are warranted.
13. Determine downstream work.

## Design AI Responsibilities

- Challenge the stated boundary or solution.
- Consider relevant lenses: product implications, domain ownership, dependencies, data ownership, consistency, concurrency, reliability, observability, security/privacy, performance, infrastructure, deployment/scaling, operational complexity, cost, and future evolution.
- Avoid mechanically discussing every lens.
- Apply the anti-overengineering contract.

## Human Decision Checkpoints

- Confirm the underlying problem.
- Choose among alternatives or request more exploration.
- Decide whether the decision is important enough for persistent rationale.

## Possible Outputs

- No change.
- Accepted architecture decision.
- Rejected proposal.
- Deferred concern.
- Route to `DOMAIN_DESIGN.md`, `GUIDED_TECHNICAL_EXPLORATION.md`, `ENGINEERING_STANDARD.md`, or `FEATURE_DEVELOPMENT.md`.
- Future Engineering Radar item.

## Persistence / Documentation Rules

Significant human-decided architecture outcomes may be persisted as ADRs under `docs/decisions/architecture/`.

Use the persistence threshold from `docs/decisions/README.md`. Do not create an ADR for every technical discussion.

The runbook may create a decision directly as ACCEPTED or REJECTED if the human decision occurred during the discussion.

An accepted ADR does not replace updating canonical architecture/domain documentation. A rejected ADR changes no canonical architecture.

Canonical architecture should reflect accepted current architecture. Future or unimplemented designs must not be shown as implemented current-state diagrams.

Implementation work remains a separate downstream process.

## Completion Criteria

- The architecture question has a recommendation and human decision, or a clear reason to defer.
- Accepted decisions are not confused with implementation state.
- Downstream work is routed.

## Where To Go Next

- Domain boundary decision: `DOMAIN_DESIGN.md`.
- Unfamiliar technical space: `GUIDED_TECHNICAL_EXPLORATION.md`.
- New reusable rule: `ENGINEERING_STANDARD.md`.
- Approved implementation work: `FEATURE_DEVELOPMENT.md`.
