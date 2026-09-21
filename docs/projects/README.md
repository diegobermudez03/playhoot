# Projects

Status: CANONICAL PROCESS REFERENCE

A Project groups, orders, and defines the completion condition of related WORK. WORK remains the only lifecycle-bearing unit of implementation/design work (see `docs/work/README.md`). A Project does not implement anything itself and does not have its own implementation lifecycle.

```text
PROJECT
   |
   +-- WORK
   +-- WORK
   +-- WORK
   ...
```

## Ownership

A Project owns:

- the project goal and what it explicitly excludes;
- grouping of the related WORK that together deliver the goal;
- ordering/dependencies between that WORK;
- the project-level completion criterion;
- a human-readable dashboard (`PROJECT.md`) summarizing current status.

A Project does not own:

- accepted architecture/domain/product truth (owned by ADRs/PDRs/canonical domain docs);
- a WORK's detailed design/implementation contract (owned by the WORK itself);
- a WORK's lifecycle/status (owned by the WORK's own `Status:` field, per `docs/work/README.md`);
- source-level progress or exact diffs (Git remains that history).

`PROJECT.md` references WORK; it does not duplicate a WORK's approved design or restate its Completion Record.

## Term "Slice"

"Slice" is not a tracking concept. It may still be used descriptively (for example, "a vertical slice of behavior") but it has no numbering, status, or authority of its own. A concrete unit of required future outcome is a WORK, tracked under a Project, never a numbered "Slice N".

## When To Create A Project

Create a Project when an initiative's accepted direction decomposes into multiple related, ordered WORK items that together deliver one coherent outcome. A single standalone WORK unrelated to any broader initiative does not need a Project; it may remain a WORK under `docs/work/active/`/`docs/work/completed/` (see `docs/work/README.md`).

Do not create a Project for a purely exploratory initiative that has not yet decided it needs concrete implementation WORK — that remains an `docs/ai/workspaces/active/<initiative>/` process (Architecture Discussion, Domain Design, etc.) per `docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md`. A Project is what that kind of initiative graduates into once it needs an ordered set of implementation WORK.

## Identifiers And Files

```text
docs/projects/active/<project-slug>/
├── PROJECT.md
├── works/
│   ├── WORK-NNNN-short-kebab-title.md
│   └── ...
└── internal/          # optional, agent continuity only
    ├── AI_CONTEXT.md
    └── HUMAN_REVIEW.md   # only while a checkpoint is pending

docs/projects/completed/<project-slug>/
```

`<project-slug>` is a stable kebab-case name (for example `session-runtime-v1`). Do not renumber or reuse it once created.

WORK filenames/IDs are unaffected by which directory holds them: `WORK-NNNN` remains part of one global, repository-wide, monotonically increasing sequence regardless of whether the file lives under a Project's `works/` or under the standalone `docs/work/active/`/`docs/work/completed/`. When allocating a new WORK ID, inspect `docs/work/active/`, `docs/work/completed/`, and every `docs/projects/*/*/works/` directory.

## PROJECT.md Contents

`PROJECT.md` is the primary human entry point for the project. A human should be able to open it and understand the project in roughly 30 seconds. At minimum it contains:

- project name;
- status;
- goal;
- explicitly excluded scope;
- current work (what is IMPLEMENTING/DRAFT/READY right now — more than one may be current, e.g. one WORK implementing while the next is being designed);
- a compact table of every WORK and its status:

  ```text
  | Order | Work | Status |
  |------:|------|--------|
  | 1 | WORK-XXXX — ... | DONE |
  | 2 | WORK-XXXX — ... | IMPLEMENTING |
  | 3 | WORK-XXXX — ... | DRAFT |
  | 4 | WORK-XXXX — ... | PLANNED |
  ```

- ordering/dependencies between that WORK;
- completion criteria for the project as a whole.

`PROJECT.md` does not duplicate the complete detailed specification of each WORK; it references the WORK, which owns its own proposal/design.

A capability/coverage matrix (mapping required outcomes to owning WORK) is a useful addition when a project's completion criterion depends on covering a defined set of required capabilities, but it does not replace WORK ownership of any individual capability.

## Tracking Invariants

**Invariant 1.** If a future implementation outcome is known to be required for an active Project, it MUST have a WORK under that Project, even if the WORK is only `PLANNED`.

**Invariant 2.** No committed future work may exist only in `PROJECT.md`, an `internal/AI_CONTEXT.md`, an ADR, a handoff, or free-form prose. Those artifacts may explain/order/reference work, but the work itself must have a corresponding WORK.

**Invariant 3.** `PROJECT.md` plus the Project's WORK must be sufficient for a human to understand what has been completed, what is active, what remains, what each future outcome is, and when the Project is complete.

**Invariant 4.** WORK is the sole implementation work driver. Project defines grouping, ordering/dependencies, the project outcome, and project completion criteria. WORK defines a concrete required outcome, its lifecycle state, its detailed design when DRAFT/READY, and the implementation contract when READY.

Agent-specific continuity files (`internal/AI_CONTEXT.md`) may continue to exist when useful but must not become a hidden work backlog — any outcome recorded there that is actually required must be promoted to a WORK per Invariant 1.

## Directory Semantics

`docs/projects/active/` holds a Project while any of its WORK is non-terminal or the project goal is not yet satisfied.

A completed WORK stays inside its Project's `works/` directory. Do not move an individual DONE WORK out of its Project into a separate global "completed" location — that would fragment the Project's history. A Project's `works/` directory normally contains a mix of DONE, IMPLEMENTING, DRAFT, READY, and PLANNED WORK simultaneously.

Only once the entire Project is complete (every required WORK is DONE, or explicitly out of scope, and the project's own completion criterion in `PROJECT.md` is satisfied) does the whole Project directory move:

```text
docs/projects/active/<project-slug>/  ->  docs/projects/completed/<project-slug>/
```

## Relationship To `docs/ai/workspaces/`

An initiative that is still exploratory — architecture/domain design not yet decided to require concrete implementation WORK — remains a temporary process workspace under `docs/ai/workspaces/active/<initiative>/`, per `docs/ai/workspaces/README.md`. Once that initiative's accepted direction decomposes into an ordered set of required implementation WORK, it graduates into a Project under `docs/projects/active/<project-slug>/` instead of continuing to track that decomposition as prose/a `PLAN.md` inside the workspace. See `docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md` for the routing/graduation mechanics.

## Relationship To WORK

A Project never redefines or shadows WORK lifecycle/status semantics; those remain fully owned by `docs/work/README.md`. A Project only groups, orders, and tracks completion across WORK it references.

## Template

Use `docs/projects/templates/PROJECT.template.md` when creating a new `PROJECT.md`.
