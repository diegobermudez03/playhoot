# Product Discussion

## Purpose

Discuss product questions without prematurely turning them into technical implementation work.

PRODUCT QUESTION: "What should Playhoot do?"

TECHNICAL QUESTION: "How should Playhoot be structured to do it?"

## Use This When

- You are deciding whether Playhoot should do something.
- You are discussing user interaction, product scope, positioning, flows, or priorities.
- Examples: remixing public games, templates vs free prompt, classrooms, first-release value, educator focus.

## Do NOT Use This When

- The product decision is already accepted and ready to implement. Use `FEATURE_DEVELOPMENT.md`.
- The main question is a system boundary or technical structure. Use `ARCHITECTURE_DISCUSSION.md` or `DOMAIN_DESIGN.md`.
- You want the AI to lead an unfamiliar technical exploration. Use `GUIDED_TECHNICAL_EXPLORATION.md`.

## Starting Information

Provide the product idea or question, the user or creator problem you think it addresses, any constraints, and whether this is for now, later, or open exploration.

## Start This Process

```text
We are using docs/ai/processes/PRODUCT_DISCUSSION.md.

Read docs/ai/OPERATING_MODEL.md. Use docs/ai/KNOWLEDGE_MAP.md to retrieve only the relevant repository context. Inspect code, tests, and migrations only if current implementation matters.

Act under the Principal Engineer Contract. Challenge my framing, consider whether the problem exists one level above the proposed scope, propose product approaches I did not mention, teach relevant concepts and tradeoffs, distinguish useful sophistication from unnecessary complexity, and make a recommendation.

Distinguish EXISTING, PROPOSED, ACCEPTED, and IMPLEMENTED facts. Detect and report drift if repository documentation and implementation conflict. Do not implement anything. Do not turn this product idea into architecture unless we explicitly decide to route there.

Product question:
[paste question]
```

## Process Stages

1. IDEA / QUESTION: state the product question clearly.
2. Problem clarification: identify the user problem and desired outcome.
3. User/persona/use-case analysis: identify who benefits and in what situation.
4. Challenge the proposed solution: test whether the stated solution is the right level.
5. Alternative product approaches: include options the human did not mention.
6. Value / complexity / risk discussion: compare expected value, cost, uncertainty, and risks.
7. MVP vs future: separate first useful version from later expansion.
8. Recommendation: recommend do it, do a different solution, defer it, reject it, or run an experiment first.
9. Human decision: record what, if anything, is accepted.
10. Impact classification: identify product docs, architecture questions, domain questions, or implementation readiness.
11. Persistence / next process: decide whether any repository update or next runbook is needed.

## Design AI Responsibilities

- Keep the discussion product-first.
- Challenge assumptions about users, value, timing, and scope.
- Explain tradeoffs without inflating process for small decisions.
- Avoid converting proposals into accepted decisions.

## Human Decision Checkpoints

- Approve the problem statement before comparing solutions.
- Accept, reject, defer, or request an experiment after recommendation.
- Decide whether the outcome needs persistence.

## Possible Outputs

- No accepted change.
- Accepted product decision.
- Rejected proposal.
- Deferred idea.
- Experiment proposal.
- Route to architecture, domain design, or feature development.

## Persistence / Documentation Rules

A casual discussion that produces no accepted decision may require no repository change.

Significant human-decided product outcomes may be persisted as PDRs under `docs/decisions/product/`.

Use the persistence threshold from `docs/decisions/README.md`. Not every product discussion requires a PDR.

ACCEPTED product decisions update the relevant canonical product knowledge. REJECTED product decisions do not change canonical product state.

Deferred ideas normally remain in `docs/product/IDEAS.md` or another appropriate non-authoritative location rather than introducing a new decision status.

Implementation work remains separate.

## Completion Criteria

- The product question is answered or explicitly deferred.
- Accepted decisions are clearly separated from proposals.
- Next process, if any, is identified.

## Where To Go Next

- Product decision creates architecture questions: `ARCHITECTURE_DISCUSSION.md` or `DOMAIN_DESIGN.md`.
- Implementation-ready without unresolved architecture: `FEATURE_DEVELOPMENT.md`.
- Reusable engineering rule needed: `ENGINEERING_STANDARD.md`.
