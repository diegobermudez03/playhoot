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

Early product or architecture exploration does not require creating a work spec.

Concrete work is persisted using `docs/work/templates/WORK_SPEC.template.md`. The resulting file lives under `docs/work/active/`, starts as DRAFT, and becomes READY only after explicit human Definition of Ready approval.

The specification must explicitly distinguish:

- approved decisions;
- local implementation freedom;
- out of scope;
- unresolved blockers.

It must include Documentation Impact:

- accepted/canonical knowledge affected by approved decisions;
- current-state documentation expected to change after implementation;
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
- Decide material/decision-required review findings and any material changes to the approved WORK.

## Definition of Ready

- Feature intent and business reason are clear.
- Material design blockers are resolved or explicitly out of scope.
- Approved decisions, local implementation freedom, and unresolved blockers are separated.
- Required verification and documentation impact are specified.
- READY is explicit implementation authority.
- A DRAFT specification must not be handed to Codex for implementation.
- READY means material uncertainty is resolved; private implementation details do not all need to be predetermined.

## Codex Handoff Prompt

```text
Implement the approved Playhoot work specification:

docs/work/active/WORK-NNNN-short-title.md

It is READY.

Follow:
docs/ai/protocols/IMPLEMENTATION_REVIEW.md

Codex is responsible for loading AGENTS.md, the Operating Model, Knowledge Map, WORK spec, and relevant implementation context as defined by the protocol.
```

## Implementation / Review Protocol

The operational implementation, DISCOVERY, report, independent-review, fix, re-review, and closure protocol lives at `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`.

If Codex discovers material drift or an unapproved material decision, pause that part of the work and report DISCOVERY. Continue only work that remains valid without the decision.

When implementation begins, transition the work spec from READY to IMPLEMENTING.

If a material approved change is required before or during implementation, return the work spec to DRAFT and require explicit human re-approval before returning it to READY.

Implementation completion leaves the WORK in IMPLEMENTING. There is no REVIEWING, REVIEW_READY, or AWAITING_REVIEW WORK status; review verdicts are not WORK statuses.

REQUIRED_FIX findings that fit the approved design return to Codex for fixes without requiring a new material human decision. DECISION_REQUIRED findings return to Design AI/human. NON_BLOCKING suggestions do not automatically expand scope.

Required fixes receive independent re-review.

After implementation, APPROVED independent review, required fixes, verification, and documentation synchronization are complete, mark the work spec DONE and move the file to `docs/work/completed/`.

CANCELLED work also moves to `docs/work/completed/`.

## Independent Review Prompt

```text
Independently review the implementation of:

docs/work/active/WORK-NNNN-short-title.md

Follow:
docs/ai/protocols/IMPLEMENTATION_REVIEW.md

Do not modify files.
```

## Fix Handoff Prompt

```text
Address the REQUIRED_FIX findings from the independent review for:

docs/work/active/WORK-NNNN-short-title.md

Follow:
docs/ai/protocols/IMPLEMENTATION_REVIEW.md

Review report:
[paste review report]
```

## Closure Prompt

```text
Close the Playhoot work:

docs/work/active/WORK-NNNN-short-title.md

The independent review verdict is APPROVED.

Follow the closure rules in:
docs/ai/protocols/IMPLEMENTATION_REVIEW.md
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

Approved concrete implementation work is persisted as a WORK specification under `docs/work/active/`. See `docs/work/README.md`.

Accepted/canonical knowledge is synchronized when the corresponding product, architecture, domain, or standard decision becomes accepted, according to the owning process. Implementation does not need to exist before accepted knowledge can become canonical.

Current-state documentation is updated only after implementation actually changes system state or behavior. Do not update current-state diagrams to future state.

If implementation reveals additional canonical or documentation implications, flag them rather than silently redefining accepted knowledge.

Do not require human line-by-line review of all code. Recommend deeper human review for critical areas such as concurrency, transactions, security, runtime/engine state, public contracts, and complex business invariants.

## Completion Criteria

- Definition of Ready was met before implementation.
- Implementation and tests are complete.
- Relevant verification has run or limitations are reported.
- Independent review is complete and required findings are fixed or consciously deferred.
- Required documentation synchronization is complete.
- No unresolved material drift was introduced by the task.
- DONE or CANCELLED work specs are moved from `docs/work/active/` to `docs/work/completed/`.

## Where To Go Next

- Product issue found: `PRODUCT_DISCUSSION.md`.
- Architecture issue found: `ARCHITECTURE_DISCUSSION.md`.
- Domain issue found: `DOMAIN_DESIGN.md`.
- Standard needed: `ENGINEERING_STANDARD.md`.
- Broad risk found: `PRINCIPAL_ENGINEER_REVIEW.md`.
