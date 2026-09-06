# Domain Design

## Purpose

Create, remove, split, merge, or materially redefine a business/domain boundary.

## Use This When

- A new domain is proposed.
- Two domains may actually be one.
- One domain may contain unrelated responsibilities.
- Ownership is unclear.
- Responsibility is moving between domains.

## Do NOT Use This When

- A new package is needed but domain ownership is unchanged.
- The issue is only local code organization.
- The main question is product value. Use `PRODUCT_DISCUSSION.md`.
- The implementation work is already approved. Use `FEATURE_DEVELOPMENT.md`.

## Starting Information

Provide the proposed domain question, current responsibility pressure, related product behavior, and any code or docs that triggered the concern.

## Start This Process

```text
We are using docs/ai/processes/DOMAIN_DESIGN.md.

Read docs/ai/OPERATING_MODEL.md. Use docs/ai/KNOWLEDGE_MAP.md to retrieve relevant product, architecture, and domain context. Inspect code, tests, and migrations when current implementation matters.

Act under the Principal Engineer Contract. Challenge whether this is truly a domain issue, propose alternative boundaries I did not mention, teach relevant domain-design tradeoffs, consider artificial separation costs, and make a recommendation.

Distinguish EXISTING, PROPOSED, ACCEPTED, and IMPLEMENTED facts. Detect and report drift. Do not implement migration work in this process.

Domain question:
[paste question]
```

## Process Stages

1. DOMAIN QUESTION: state the proposed boundary change.
2. Product responsibility: identify the business responsibility.
3. Ubiquitous/business concepts: name concepts used by the product.
4. Owned state/data: identify what data would be owned.
5. Invariants: identify rules the domain must protect.
6. Lifecycle: compare creation, update, deletion, and versioning lifecycles.
7. Public capabilities: define what the domain exposes.
8. Explicit non-ownership: define what it must not own.
9. Independent evolution: assess whether it changes independently.
10. Dependency pressure: inspect coupling and dependency direction.
11. Cross-domain coordination consequences: identify orchestrator/composer pressure.
12. Alternative boundaries: include no domain change.
13. Recommendation.
14. Human decision.
15. Architecture/documentation impact.
16. Migration impact if existing code differs.

## Design AI Responsibilities

- Make clear that domain != package, folder, deployment, or service.
- Make clear that different lifecycle/process != automatically different domain.
- Question artificial separation when almost every operation would require orchestration only to cross the boundary.
- Also question oversized domains when responsibilities differ even if code volume is large or deployment is shared.

## Human Decision Checkpoints

- Confirm the product responsibility being modeled.
- Accept no-change, split, merge, remove, create, or redefine.
- Decide whether rationale should be persisted.

## Possible Outputs

- Do not create/change the domain.
- Accepted domain boundary change.
- Rejected proposal.
- Deferred boundary concern.
- Migration or implementation work to specify later.

## Persistence / Documentation Rules

Important domain changes normally result in architecture decision rationale and canonical architecture/domain documentation changes once those destinations exist.

Implementation and migration happen separately through `FEATURE_DEVELOPMENT.md` or an appropriate work specification.

## Completion Criteria

- Domain ownership is accepted, rejected, or deferred.
- Migration impact is identified if existing code differs.
- Next documentation or implementation process is routed.

## Where To Go Next

- Migration or code change: `FEATURE_DEVELOPMENT.md`.
- Broader system question: `ARCHITECTURE_DISCUSSION.md`.
- Product uncertainty: `PRODUCT_DISCUSSION.md`.
