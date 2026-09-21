# Project Dashboard Template

This template contains authoring guidance. Template-only instructions must not be copied into the final `PROJECT.md`.

Resulting record shape:

```text
# Project: <Name>

Status: ACTIVE | COMPLETED
Created: YYYY-MM-DD
Last updated: YYYY-MM-DD

## Goal

<What this project achieves and why it matters, in a few sentences.>

## Explicitly Out Of Scope

- <capability/concern intentionally excluded from this project, and why>
- ...

## Current Work

<What is IMPLEMENTING/DRAFT/READY right now. More than one may be current
(e.g. one WORK implementing while the next is being designed).>

## Work

| Order | Work | Status |
|------:|------|--------|
| 1 | WORK-XXXX — <title> | DONE |
| 2 | WORK-XXXX — <title> | IMPLEMENTING |
| 3 | WORK-XXXX — <title> | DRAFT |
| 4 | WORK-XXXX — <title> | PLANNED |

<One short line per WORK is enough here - the WORK file itself owns the detail.>

## Ordering / Dependencies

<Why the table above is ordered the way it is - the real dependencies, not
just narrative sequencing. Note any WORK that can proceed in parallel.>

## Capability Coverage

<Optional. Include when project completion depends on covering a defined set
of required capabilities. Map each capability to the WORK that owns it (DONE/
active/PLANNED) or to an explicit "outside this project's scope" note. There
must be no capability that is required but owned by nothing.>

## Material Decisions Needing Human Input

<Optional. Genuinely open product/architecture/domain questions discovered
while planning this project that must not be silently decided. Remove this
section once nothing is pending.>

## Completion Criteria

<What "this project is done" means, precisely enough that reaching it is
unambiguous.>
```

Do not duplicate a WORK's own Outcome/Scope/Approved Design/Acceptance Criteria here — reference the WORK file instead.

Keep the `Work` table current as WORK status changes; do not let it drift from the actual `Status:` field of each referenced WORK file.
