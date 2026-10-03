# Archive: Documentation Before The Real-Time Pivot

Status: HISTORICAL ARCHIVE - NOT CURRENT TRUTH

**AI agents: do not read anything under this directory unless the user explicitly asks about the history of the project** (why something was once built a certain way, what a cancelled project or WORK contained, what an old decision said). It is not loaded by default, not routed to by the Knowledge Map for normal work, and not evidence of how Playhoot works now.

## What this is

Until 2026-10-03 Playhoot's Session Runtime was designed for discrete games: every interaction a durable, replayable turn, authored JavaScript loaded per interaction, and the JavaScript executor a separate gRPC service. That design was abandoned in favor of a real-time runtime that also supports continuous games (`session/docs/decisions/SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md`, `docs/decisions/product/PDR-0001-support-discrete-and-continuous-games.md`).

Everything here is the documentation of that previous design, frozen as it stood:

- Statuses (ACCEPTED, DONE, IMPLEMENTING, ...) are the statuses at the time of the pivot and no longer carry authority. Open projects and WORK were cancelled where they stood.
- Cross-references between archived files, and from them to files outside the archive, are not maintained and may be broken.
- WORK identifiers continue after the archive (the next new WORK is `WORK-0056`), and `SESSION-ADR-NNNN` identifiers continue after `SESSION-ADR-0027`; archived identifiers are never reused.

## Layout

The paths mirror the original repository locations.

| Path | Contents |
| --- | --- |
| `docs/projects/` | Active and completed Projects with their WORK (js-runtime-migration, session-runtime-v1, and two completed projects) |
| `docs/work/active/` | Standalone WORK specifications that were still open |
| `docs/decisions/architecture/` | ADR-0015 and ADR-0016 (marked SUPERSEDED) |
| `docs/decisions/LEGACY_ADR_ID_MAP.md` | Mapping of pre-2026-09 ADR identifiers |
| `docs/ai/workspaces/` | Temporary process workspaces that were open |
| `docs/engineering/`, `docs/product/` | Radar items and product ideas removed because they concerned the retired design |
| `game/docs/decisions/` | Game Language and JavaScript-execution decisions |
| `session/docs/` | Session Runtime data model, flows, persistence and artifact models, iframe contract, authoring constraints, and ADR-0001 through ADR-0027 |
| `session/CURRENT_STATE.md`, `session/README.md` | Session Runtime's domain model and state documents of the previous design |
| `session/internal/`, `session/jsexecutor/`, `session/workflows/` | Package-local contracts (`LOGICAL_CONTRACT.md`, the executor README) of removed code |
| `sessionv1_comments.md` | Loose review notes from the previous design |

## The code

The previous implementation is not copied here. It is in Git history: tag `pre-realtime-pivot` is the last commit before the pivot, and `refs/snapshots/pre-realtime-pivot` additionally holds the uncommitted working-tree changes that existed at that moment.

```text
git show pre-realtime-pivot:<path>
git show refs/snapshots/pre-realtime-pivot:<path>
```
