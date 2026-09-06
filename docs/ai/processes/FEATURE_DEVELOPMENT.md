# Feature Development

## Purpose

Take a concrete product or technical capability from design through implementation and review.

## Use This When

- A concrete feature or technical capability needs to be designed and implemented.
- Product and architecture blockers are either resolved or can be resolved during a short scope challenge.

## Do NOT Use This When

- The product decision is still open. Use `PRODUCT_DISCUSSION.md`.
- The main issue is system architecture. Use `ARCHITECTURE_DISCUSSION.md`.
- The domain boundary is unclear. Use `DOMAIN_DESIGN.md`.
- You are defining a reusable engineering rule. Use `ENGINEERING_STANDARD.md`.

## Starting Information

Provide the desired capability, user/business reason, known accepted decisions, constraints, out-of-scope items, and any relevant files or behavior.

## Start This Process

```text
We are using docs/ai/processes/FEATURE_DEVELOPMENT.md.

Read docs/ai/OPERATING_MODEL.md. Use docs/ai/KNOWLEDGE_MAP.md to retrieve relevant product, architecture, domain, and engineering-standard context. Inspect code, tests, and migrations when current implementation matters.

Act under the Principal Engineer Contract. Challenge my framing before optimizing inside the requested domain/package, look one level above the feature, propose alternatives I did not mention, teach relevant concepts and tradeoffs, apply anti-overengineering, and make concrete recommendations.

Distinguish EXISTING, PROPOSED, ACCEPTED, and IMPLEMENTED facts. Detect and report drift. Do not implement until we have an approved implementation specification and reach the Codex handoff stage.

Feature intent:
[paste intent]
```

## Process Stages

1. Feature intent: state the capability and product/business reason.
2. Context loading: use the Knowledge Map and inspect implementation where needed.
3. SCOPE CHALLENGE GATE.
4. Design discussion.
5. Important decisions.
6. Implementation specification.
7. Human approval / Definition of Ready.
8. Codex handoff.
9. Codex discovery/escalation loop.
10. Implementation completion.
11. Independent review.
12. Fix findings if necessary.
13. Documentation synchronization.
14. Definition of Done.
15. Close/archive work.

## Scope Challenge Gate

Before optimizing inside the requested domain/package, the Design AI must consider:

- Does this feature belong here?
- Does it expose a missing or incorrect domain boundary?
- Does it require unresolved product behavior?
- Does it require a new global architecture decision?
- Does it expose a missing engineering standard?

If yes, pause Feature Development and route to the appropriate process. Do not force a separate architecture discussion for trivial/local decisions.

## Design Discussion

Cover only relevant topics, such as behavior, public contracts, domain model, data/persistence, consistency, concurrency, errors, security, observability, failure modes, and tests.

Do not generate unnecessary sections when irrelevant.

The specification must explicitly distinguish:

- approved decisions;
- local implementation freedom;
- out of scope;
- unresolved blockers.

It must include Documentation Impact:

- canonical docs expected to change after implementation;
- current-state diagrams expected to change after implementation;
- docs that intentionally should NOT change when useful.

Current-state documentation must not be updated to future state before code is implemented.

## Design AI Responsibilities

- Keep scope proportional to risk.
- Challenge placement, unresolved product behavior, missing architecture decisions, and missing standards.
- Produce an implementation-ready specification only after material blockers are resolved.
- Do not grant implementation authority for material decisions the human has not accepted.

## Human Decision Checkpoints

- Approve the scope after the Scope Challenge Gate.
- Accept important product, architecture, domain, persistence, consistency, security, infrastructure, or standard decisions.
- Approve Definition of Ready before Codex handoff.
- Decide whether review findings require fixes.

## Definition of Ready

- Feature intent and business reason are clear.
- Material design blockers are resolved or explicitly out of scope.
- Approved decisions, local implementation freedom, and unresolved blockers are separated.
- Required verification and documentation impact are specified.

## Codex Handoff Prompt

```text
We are implementing an approved Feature Development specification for Playhoot.

Read AGENTS.md. Read docs/ai/OPERATING_MODEL.md. Use docs/ai/KNOWLEDGE_MAP.md to load only relevant context. Read the approved work specification below. Inspect the real implementation, tests, and migrations before changing code.

Follow the approved decisions. Retain autonomy for local implementation details within the specification. Escalate material discoveries using the DISCOVERY format from docs/ai/OPERATING_MODEL.md instead of silently redesigning product behavior, architecture, domain boundaries, persistence, consistency, security, infrastructure, or engineering standards.

Implement the feature and relevant tests. Run relevant verification. After implementation, update only documentation explicitly required by the specification. Flag additional documentation impacts instead of silently rewriting canonical knowledge.

Report implementation summary, local design decisions, deviations, discoveries, tests run, and documentation changes.

Approved specification:
[paste specification]
```

## Codex Discovery / Escalation Loop

If Codex discovers material drift or an unapproved material decision, pause that part of the work and report DISCOVERY. Continue only work that remains valid without the decision.

## Independent Review Prompt

```text
We are independently reviewing completed Playhoot implementation work.

Read AGENTS.md. Read docs/ai/OPERATING_MODEL.md. Use docs/ai/KNOWLEDGE_MAP.md to load relevant context. Read the approved specification and inspect the implementation, tests, migrations, and documentation changed by the work.

Review without relying on the implementation agent's assumptions. Check specification compliance, architecture/domain boundaries, engineering standards, public contracts, persistence/data correctness, consistency/concurrency where applicable, security, observability/error handling, failure modes, tests, unnecessary complexity, and documentation drift.

Prioritize findings by severity with file/line references where possible. Do not rewrite code unless asked. Distinguish EXISTING, PROPOSED, ACCEPTED, and IMPLEMENTED facts. Detect and report drift.

Approved specification:
[paste specification]
```

## Possible Outputs

- Approved implementation specification.
- Codex handoff.
- Implemented feature.
- Review findings.
- Required fixes.
- Documentation synchronization.
- Deferred or blocked work.

## Persistence / Documentation Rules

Update required canonical documentation only after implementation changes actual state. Do not update current-state diagrams to future state. Flag additional likely impacts rather than redefining canonical knowledge silently.

Do not require human line-by-line review of all code. Recommend deeper human review for critical areas such as concurrency, transactions, security, runtime/engine state, public contracts, and complex business invariants.

## Completion Criteria

- Definition of Ready was met before implementation.
- Implementation and tests are complete.
- Relevant verification has run or limitations are reported.
- Independent review is complete and required findings are fixed or consciously deferred.
- Required documentation synchronization is complete.
- No unresolved material drift was introduced by the task.

## Where To Go Next

- Product issue found: `PRODUCT_DISCUSSION.md`.
- Architecture issue found: `ARCHITECTURE_DISCUSSION.md`.
- Domain issue found: `DOMAIN_DESIGN.md`.
- Standard needed: `ENGINEERING_STANDARD.md`.
- Broad risk found: `PRINCIPAL_ENGINEER_REVIEW.md`.
