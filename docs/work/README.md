# Work Specifications

Status: CANONICAL PROCESS REFERENCE

A Playhoot work specification owns the approved concrete implementation change. It bridges accepted design and Codebase Agent implementation authority.

## Ownership

A work spec owns:

- implementation outcome;
- approved task scope;
- task-specific approved design needed to constrain implementation;
- constraints/invariants relevant to the task;
- acceptance criteria;
- implementation autonomy boundary;
- required verification;
- documentation impact;
- completion evidence.

A work spec does not replace PDRs, ADRs, canonical product docs, `ARCHITECTURE.md`, domain README files, engineering standards, code/tests/migrations, or current-state documentation.

A work spec may reference those sources. Do not duplicate their complete contents.

## When To Create A Work Spec

Do not create a work spec for every idea or discussion.

A work spec is appropriate when concrete implementation work is being prepared or approved, **or** when an implementation outcome is already known to be required (typically because it is required to complete an active Project, per `docs/projects/README.md` Invariant 1) even though its design is not yet known — that WORK starts life as PLANNED rather than DRAFT.

- Open product questions belong in `docs/ai/processes/PRODUCT_DISCUSSION.md`.
- Open architecture questions belong in `docs/ai/processes/ARCHITECTURE_DISCUSSION.md`.
- Open domain-boundary questions belong in `docs/ai/processes/DOMAIN_DESIGN.md`.
- Reusable engineering rules belong in `docs/ai/processes/ENGINEERING_STANDARD.md`.

Future ideas, backlog items, and radar concerns do not become PLANNED or DRAFT work specs merely because they may eventually be implemented — PLANNED is for an outcome already known to be required, not a speculative idea backlog.

Normally `FEATURE_DEVELOPMENT.md` creates/persists the work spec (as PLANNED, or directly as DRAFT when concrete design work is already starting) and moves it to DRAFT once the Scope Challenge Gate has passed and enough design has occurred to define concrete implementation work.

DRAFT is an implementation-design state, not a general idea backlog.

## Work And Project

A WORK is the sole lifecycle-bearing unit of implementation/design work. When several related WORK items together deliver one coherent outcome, they are grouped under a Project (`docs/projects/README.md`), which owns grouping, ordering/dependencies, and the project-level completion criterion — never a WORK's own status or design. A standalone WORK unrelated to any broader initiative does not need a Project.

"Slice" is not a tracking concept; a required future outcome is a WORK, not a numbered "Slice N" (see `docs/projects/README.md`).

## Identifiers And Files

Use one generic work-spec family:

`WORK-NNNN-short-kebab-title.md`

The same format covers product features, technical capabilities, migrations, refactors, engineering-standard enforcement/migration, and targeted infrastructure work.

Do not create separate FEATURE/MIGRATION/REFACTOR numbering systems now.

Numbers form one repository-wide monotonically increasing WORK sequence, regardless of whether the file lives under a Project's `works/` directory (`docs/projects/active|completed/<project-slug>/works/`) or under the standalone directories below. Determine the next ID by inspecting `docs/work/active/`, `docs/work/completed/`, and every `docs/projects/*/*/works/` directory.

Never reuse or renumber an existing WORK ID.

The filename remains stable as status changes and as the file moves between directories.

## Directory Semantics

A WORK grouped under an active Project lives in that Project's own `docs/projects/active/<project-slug>/works/` for its entire lifecycle, DONE included — do not move a completed Project WORK out to a separate global completed location; that would fragment the Project's history (see `docs/projects/README.md`). Only once the whole Project resolves does its entire directory move to `docs/projects/completed/<project-slug>/`.

A standalone WORK not grouped under any Project uses:

`docs/work/active/` — non-terminal work specs:

- PLANNED
- DRAFT
- READY
- IMPLEMENTING

`docs/work/completed/` — terminal/closed work specs:

- DONE
- CANCELLED

When such a WORK reaches DONE or CANCELLED, move the same file from `active/` to `completed/`. Do not create a replacement copy. The completed directory preserves closed work history, including CANCELLED work that was not implemented.

## Status Model

The only current work-spec statuses are:

- PLANNED
- DRAFT
- READY
- IMPLEMENTING
- DONE
- CANCELLED

PLANNED:

- We have decided this outcome is required (as part of an active Project, or otherwise known to be needed), but its design is not yet known.
- Material design questions may remain fully open; scope boundaries may still be provisional.
- Not authorized for implementation, and not yet under active design either — that transition is PLANNED -> DRAFT.
- Not a speculative idea backlog item: create a PLANNED WORK only when an implementation outcome is already known to be required, not merely possible or interesting. See `docs/projects/README.md` Invariant 1.

DRAFT:

- Concrete implementation work is being actively designed/scoped.
- Has no implementation authority.
- May contain unresolved blockers.
- May change materially.

READY:

- Human explicitly approved implementation.
- Definition of Ready is satisfied.
- Material product/architecture/domain/design decisions required for the work are resolved.
- Scope, constraints, acceptance criteria, verification, and documentation impact are sufficiently clear.
- A Codebase Agent may implement within the documented autonomy boundary.

IMPLEMENTING:

- Approved implementation is actively being executed.
- The READY approval remains the authority for scope/design.

DONE:

- Implementation is complete.
- Required tests/verification have run or limitations are explicitly recorded.
- Required independent review/fixes are complete according to Feature Development.
- Required documentation synchronization is complete.
- The work spec has been closed and moved to `docs/work/completed/`.

CANCELLED:

- The work will not continue under this specification.
- Preserve the file as closed historical work.
- Cancellation does not make proposed behavior canonical or implemented.

## Normal Transitions

- PLANNED -> DRAFT
- DRAFT -> READY
- READY -> IMPLEMENTING
- IMPLEMENTING -> DONE

Cancellation may occur from any non-terminal state:

- PLANNED -> CANCELLED
- DRAFT -> CANCELLED
- READY -> CANCELLED
- IMPLEMENTING -> CANCELLED

DONE and CANCELLED are terminal.

## Material Change / Reapproval

A READY specification is an approved implementation contract.

If a material change is required before or during implementation, pause the affected work, route the underlying decision to the correct process when necessary, update the work spec, return its status to DRAFT, and require explicit human re-approval before READY and implementation resume.

Material changes include changes to approved scope, product behavior, domain ownership, architecture, public contracts, persistence/data semantics, consistency/concurrency semantics, security/privacy boundaries, infrastructure/deployment, material dependency/technology, accepted engineering standards, acceptance criteria, or another approved design constraint.

A minor clarification or local implementation choice inside the already approved autonomy boundary does not require returning to DRAFT.

## Human Approval

Only the human decision maker may authorize DRAFT -> READY.

A Conversational AI may prepare/revise the specification and recommend READY.

A Codebase Agent must never self-approve a DRAFT specification.

Starting implementation may transition READY -> IMPLEMENTING according to the implementation workflow.

DONE is only reached after the Feature Development completion criteria are satisfied; implementation completion alone is not enough.

## Definition Of Ready

A work spec may become READY only when:

1. Outcome / reason for the work is clear.
2. In-scope and out-of-scope boundaries are clear.
3. Required material product decisions are accepted.
4. Required architecture/domain decisions are accepted.
5. Required engineering-standard decisions are accepted.
6. Important task-specific design decisions that constrain implementation are approved.
7. Constraints/invariants are explicit where relevant.
8. Acceptance criteria are observable/testable enough to verify completion.
9. Required verification is identified.
10. Documentation impact is identified.
11. No unresolved MATERIAL blocker remains.
12. Local implementation choices intentionally left to the Codebase Agent are distinguished from unresolved human decisions.

READY means material uncertainty is resolved, not that the specification is pseudocode. Do not require every private implementation detail to be predetermined.

## Implementation Autonomy

Use the autonomy boundary defined by `docs/ai/OPERATING_MODEL.md`.

A work spec should constrain material implementation decisions while leaving local engineering choices to the Codebase Agent, including private helper decomposition, private types, local naming, test fixtures, mock organization, straightforward internal refactors, and detailed implementation choices already bounded by accepted design and standards.

Do not duplicate the complete Operating Model decision-boundary list into every actual WORK file.

## Work Spec Vs Decision Record

A work spec may contain task-specific approved design, but it must not become a substitute for ADR/PDR rationale.

If a material product/architecture/domain decision meets the decision-record persistence threshold, persist its rationale in the appropriate ADR/PDR and reference that decision from the WORK spec.

Trivial/local implementation decisions do not need ADRs merely because they appear in a work spec.

## Work Spec Vs Canonical Knowledge

A work spec does not override canonical product, architecture, domain, or engineering-standard knowledge.

If a READY work spec unexpectedly conflicts with canonical knowledge, report the conflict/drift and resolve it before implementing the conflicting portion.

Accepted decisions that intentionally change canonical knowledge should have their canonical owners synchronized according to the appropriate decision process.

Acceptance of architecture/product design does not mean implementation exists. Current-state docs continue to reflect implementation reality only.

## Completed Work Immutability

A DONE or CANCELLED work spec is historical work memory.

Do not later rewrite its approved scope/design merely to match the newest implementation.

Typos, broken links, and factual metadata may be corrected without changing historical meaning.

If later work changes the implementation materially, create a new WORK specification.
