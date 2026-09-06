# Implementation and Independent Review Protocol

Status: CANONICAL AI PROTOCOL

Use this protocol when a concrete WORK specification is READY for implementation or is already IMPLEMENTING.

This protocol assumes Feature Development has already produced a persistent WORK specification.

## Scope

Do not use this protocol to decide product behavior, decide architecture, decide domain ownership, define engineering standards, turn an idea into implementation scope, or approve DRAFT work. Those activities belong to their existing processes.

This protocol owns implementation-agent execution mechanics, implementation reports, independent-review mechanics, review verdict/finding semantics, the fix/re-review loop, operational closure, and minimal handoff prompts.

Actor roles and material decision boundaries are owned by `docs/ai/OPERATING_MODEL.md`. WORK statuses and lifecycle semantics are owned by `docs/work/README.md`. Feature Development remains the human-facing end-to-end process.

## Preconditions For Implementation

Before Codex changes implementation:

1. The referenced WORK file must exist under `docs/work/active/`.
2. Its status must be READY unless work is legitimately resuming from IMPLEMENTING.
3. A DRAFT WORK has no implementation authority.
4. A CANCELLED or DONE WORK must not be implemented.
5. Codex must read `AGENTS.md`, `docs/ai/OPERATING_MODEL.md`, `docs/ai/KNOWLEDGE_MAP.md`, and the referenced WORK specification.
6. Use the Knowledge Map to load only task-relevant canonical/domain/standard context.
7. Inspect actual code, tests, migrations, and relevant current-state documentation before making implementation assumptions.
8. Inspect the current Git/worktree state sufficiently to distinguish the work being implemented from unrelated pre-existing user changes.

Do not overwrite or silently absorb unrelated work.

## Starting Implementation

When implementation actually begins, transition the WORK file from READY to IMPLEMENTING by updating:

- Status
- Last status change

This transition does not require a new human decision because READY already contains explicit implementation authorization.

Codex must not modify the approved scope/design merely while performing this status transition.

## Implementation Agent Behavior

Codex should:

- implement the approved scope;
- follow relevant canonical architecture/domain knowledge;
- follow applicable engineering standards;
- write/update relevant tests;
- run relevant verification;
- make local implementation decisions within the Operating Model autonomy boundary;
- perform straightforward internal refactors needed by the approved work;
- update implementation/current-state documentation explicitly required by the WORK when the implementation now makes that documentation true;
- flag additional documentation implications rather than silently rewriting unrelated canonical knowledge.

Codex should not:

- expand product scope;
- change public behavior not authorized by the WORK;
- change domain ownership;
- redefine architecture;
- change persistence semantics materially;
- change consistency/concurrency/security/infrastructure semantics materially;
- introduce a material technology/dependency decision;
- change an engineering standard;
- rewrite the WORK to make an unapproved implementation deviation appear approved.

Do not escalate normal local engineering choices already inside the approved autonomy boundary. The goal is to escalate material decisions and implement local decisions.

## Discovery

Use the DISCOVERY format from `docs/ai/OPERATING_MODEL.md`. Do not define a competing escalation format.

When Codex discovers a material issue:

- stop implementation of the affected portion;
- report DISCOVERY;
- explain what work can continue safely without the decision;
- continue independent valid work when useful;
- do not invent the missing material decision.

A DISCOVERY does not automatically mean the entire WORK returns to DRAFT.

If the discovery can be resolved without materially changing the approved contract, implementation may continue after clarification.

If resolving it requires a material change to the approved WORK:

1. route the underlying issue through the appropriate Design AI/human process;
2. update the WORK;
3. return it to DRAFT;
4. obtain explicit human re-approval;
5. transition back to READY;
6. resume implementation of the changed contract.

When resuming after re-approval, Codex must re-read the current WORK rather than relying on conversation context.

## Reviewable Implementation

Finishing code does not change the WORK to DONE. The WORK remains IMPLEMENTING until independent review, required fixes, verification, documentation synchronization, and closure are complete.

Do not introduce additional WORK statuses such as REVIEWING, REVIEW_READY, AWAITING_REVIEW, or FIXING. "Awaiting review" is an operational condition, not a persisted WORK status.

Before requesting independent review, the implementation should normally be reviewable:

- approved implementation scope is complete;
- relevant tests exist;
- relevant verification has been attempted;
- explicitly required implementation/current-state documentation has been synchronized to implemented reality;
- known blockers/discoveries are reported.

Documentation may require another final synchronization after review fixes.

## Implementation Report

At the end of an implementation/fix pass, Codex should return:

```text
IMPLEMENTATION REPORT

Work:
`docs/work/active/WORK-NNNN-....md`

Work status:
IMPLEMENTING

Implemented:
- ...

Local implementation decisions:
- ...
- None, if none worth reporting.

Deviations from the approved WORK:
- None.
- Or only previously HUMAN-APPROVED material deviations.

Discoveries:
- None.
- Or reference unresolved/resolved DISCOVERY items.

Verification performed:
- <test/check>
- <result>
- <limitations if any>

Documentation synchronized:
- <path / summary>
- None, if none required.

Known limitations:
- None.
- Or explicit limitation.

Ready for independent review:
YES / NO
```

An unapproved material deviation must not be reported after the fact merely as a "deviation". It should have triggered DISCOVERY and reapproval before implementation.

Do not mark the WORK DONE.

## Minimal Handoffs

Implementation:

```text
Implement the approved Playhoot work specification:

docs/work/active/WORK-NNNN-short-title.md

It is READY.

Follow:
docs/ai/protocols/IMPLEMENTATION_REVIEW.md
```

Independent review:

```text
Independently review the implementation of:

docs/work/active/WORK-NNNN-short-title.md

Follow:
docs/ai/protocols/IMPLEMENTATION_REVIEW.md

Do not modify files.
```

Required fixes:

```text
Address the REQUIRED_FIX findings from the independent review for:

docs/work/active/WORK-NNNN-short-title.md

Follow:
docs/ai/protocols/IMPLEMENTATION_REVIEW.md

Review report:
[paste review report]
```

Closure:

```text
Close the Playhoot work:

docs/work/active/WORK-NNNN-short-title.md

The independent review verdict is APPROVED.

Follow the closure rules in:
docs/ai/protocols/IMPLEMENTATION_REVIEW.md
```

Codex/reviewer loads the relevant context independently. Do not paste the entire WORK specification into handoff prompts.

## Independent Review Protocol

Independent review should preferably be performed by a fresh agent/session that did not implement the change.

The reviewer must reason from primary evidence:

1. approved WORK specification;
2. canonical knowledge relevant to the work;
3. engineering standards;
4. actual implementation/diff;
5. tests;
6. migrations;
7. documentation.

The implementation agent's summary may be useful supplementary context, but it is not authoritative evidence. Do not accept claims such as "tests pass", "this follows the architecture", or "there are no deviations" without inspecting/verifying them to a reasonable degree.

The independent reviewer must not modify implementation or documentation during the review unless the human explicitly asks for a combined review-and-fix task.

The normal Playhoot review is:

```text
review first
-> report findings
-> implementation agent fixes
-> independent re-review
```

The reviewer may run safe read/verification commands and tests.

Do not create a separate permanent review file. The review report is an execution/handoff artifact.

The reviewer should inspect the actual change relevant to the WORK rather than only reading final files in isolation. Where possible, use Git/diff history or current worktree information to understand what changed.

If the exact review change-set cannot be determined because unrelated changes are mixed together, report:

```text
REVIEW SCOPE AMBIGUITY
```

and explain what cannot be attributed confidently to the WORK. Do not silently review unrelated user work as though it were part of the specification.

Review proportionally to the change. Evaluate relevant concerns including WORK acceptance criteria, approved scope, correctness, architecture/domain boundaries, engineering standards, public contracts, persistence/data correctness, transaction/consistency/concurrency semantics when relevant, security/privacy when relevant, observability/error/failure behavior when relevant, tests and test quality, migrations when relevant, backward compatibility when relevant, unnecessary complexity/overengineering, documentation synchronization, and canonical-vs-implementation drift.

Do not mechanically invent findings for every category. APPROVED with no findings is a valid result.

Do not reopen an accepted product/architecture choice merely because the reviewer personally prefers an alternative. If the accepted design conflicts with canonical knowledge, is internally inconsistent, or exposes a material previously-unresolved risk, report that through the appropriate decision/drift path.

## Finding Dispositions

Every substantive finding must be classified as one of:

- REQUIRED_FIX
- DECISION_REQUIRED
- NON_BLOCKING

REQUIRED_FIX:

- A concrete issue that must be corrected before DONE and can be corrected within the already-approved WORK/design.
- Examples include a bug, violated acceptance criterion, violation of an accepted engineering standard, missing important error handling required by accepted behavior, incorrect persistence behavior, missing required test, incorrect current-state documentation, or unintended scope behavior.
- A REQUIRED_FIX does not require a new product/architecture decision merely because the reviewer found it.

DECISION_REQUIRED:

- The issue cannot be responsibly fixed without a material human decision or a material change to the approved WORK.
- Examples include unclear product behavior, domain ownership question, public API contract change, unapproved persistence semantics, concurrency/consistency guarantee requiring choice, material security/infrastructure/dependency choice, or conflict between READY WORK and canonical architecture.
- Route this to Design AI/human. Do not tell Codex to choose arbitrarily.

NON_BLOCKING:

- A suggestion or observation that does not need to be implemented for the current WORK to be considered correct and complete.
- Examples include optional cleanup, alternative local implementation, future optimization, possible future refactor, or broader concern outside approved scope.
- NON_BLOCKING findings do not automatically expand the current WORK.

Use concise severity labels where useful: CRITICAL, HIGH, MEDIUM, LOW. Disposition and severity are different concepts.

## Review Verdicts

The review has exactly three operational verdicts:

- APPROVED
- CHANGES_REQUIRED
- DECISION_REQUIRED

APPROVED:

- No unresolved REQUIRED_FIX findings.
- No unresolved DECISION_REQUIRED findings.
- NON_BLOCKING observations may exist.
- The implementation may proceed to closure.

CHANGES_REQUIRED:

- One or more REQUIRED_FIX findings exist.
- They can be resolved within the current approved WORK.
- The WORK remains IMPLEMENTING.

DECISION_REQUIRED:

- At least one unresolved finding requires a material decision.
- Other REQUIRED_FIX findings may also exist.
- Affected implementation must pause pending resolution.

These are review verdicts, not WORK statuses.

## Review Report

Use this output shape:

```text
INDEPENDENT REVIEW

Work:
docs/work/active/WORK-NNNN-....md

Verdict:
APPROVED | CHANGES_REQUIRED | DECISION_REQUIRED

Verification performed:
- ...
- Not run: <reason>, where applicable.

Findings:
- None.
or
- [HIGH] <finding>
  Disposition: DECISION_REQUIRED
  Evidence: <file/line/test/spec reference>
  Why a decision is required: ...
  Relevant accepted constraint: ...
- [HIGH] <finding>
  Disposition: REQUIRED_FIX
  Evidence: <file/line/test/spec reference>
  Violated requirement/standard/invariant: ...
  Required outcome: ...
- [LOW] <observation>
  Disposition: NON_BLOCKING
  Evidence: <file/line/test/spec reference>
  Why it may be worth considering: ...

Documentation/drift notes:
- None.
or
- ...
```

Keep findings concrete. Prefer file/line references where possible. Do not produce stylistic nitpicks unless an accepted standard or material maintainability concern exists.

## Fix And Re-Review Loop

If the verdict is CHANGES_REQUIRED, send REQUIRED_FIX findings back to the implementation agent.

The WORK stays IMPLEMENTING.

Codex:

- fixes REQUIRED_FIX findings;
- does not automatically implement NON_BLOCKING suggestions;
- escalates DECISION_REQUIRED findings;
- reruns relevant verification;
- synchronizes docs affected by fixes;
- returns a new IMPLEMENTATION REPORT.

Then perform independent review again. Re-review should verify the fixes and consider regressions/material effects, not merely check that lines changed.

## Decision Required / Reapproval Loop

If review or implementation reports DECISION_REQUIRED or DISCOVERY, route the material question to Design AI and the human decision maker.

If the resolution does not materially change the approved WORK, document/communicate the clarification and resume as appropriate.

If the resolution materially changes the approved WORK:

1. pause affected implementation;
2. return WORK to DRAFT;
3. synchronize any accepted canonical decision through its owning process;
4. update the WORK;
5. obtain explicit human approval;
6. set WORK to READY;
7. resume implementation;
8. transition it to IMPLEMENTING when implementation resumes.

Do not allow the reviewer or Codex to self-approve the revised design.

The human must decide DECISION_REQUIRED findings, material changes to the approved WORK, and whether to consciously accept an exceptional unresolved risk/deviation that would otherwise block DONE.

A concrete REQUIRED_FIX that brings implementation into compliance with already-approved scope/design normally does not require a new human product or architecture decision.

NON_BLOCKING suggestions do not become current scope automatically.

## Closure

Only close the WORK after an independent review verdict of APPROVED and after Feature Development completion criteria are satisfied.

Before closure verify:

- required implementation exists;
- required tests/verification were performed or limitations recorded;
- no unresolved REQUIRED_FIX exists;
- no unresolved DECISION_REQUIRED exists;
- required documentation is synchronized;
- no unapproved material deviation remains.

Then:

1. Fill the WORK Completion Record concisely.
2. Change `Status:` to DONE.
3. Update `Last status change`.
4. Move the same file from `docs/work/active/` to `docs/work/completed/`.

The Completion Record should include implementation summary, actual verification performed and limitations, independent review result, required documentation synchronized, approved material deviations if any, and relevant explicitly-out-of-scope follow-up when useful.

Do not paste the entire review report. Do not rewrite the historical approved sections merely to match the final implementation.

The closing agent must independently verify that closure preconditions appear satisfied. Do not treat the user's sentence "APPROVED" as permission to hide unresolved known findings.

## Cancellation

Codex/reviewer may recommend cancellation when work is no longer viable. They must not unilaterally decide that approved human work intent is CANCELLED.

Cancellation requires human authorization.

Once authorized:

- record a concise cancellation reason;
- set `Status:` to CANCELLED;
- update `Last status change`;
- move the same WORK file to `docs/work/completed/`.

Do not imply that cancelled behavior was implemented or accepted.

## Non-Goals

Do not create `docs/work/reviews/`, REVIEW-NNNN files, reviewer databases, or extra review lifecycle statuses.

Do not establish new standards for Git branching, commits, PR process, CI provider, code owners, merge policy, deployment, rollback, or release management.

Do not require human line-by-line code review.

Do not require reviewer findings just to prove the review was useful.
