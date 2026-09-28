Checkpoint: WORK-0034 implemented, pending independent review (not a new human-decision gate — informational).
Date: 2026-09-28

**Update:** You authorized READY ("Nice, proceed") and the implementation described below is now complete. `go build`/`go vet`/`go test ./...` are clean except one pre-existing, unrelated test failure (verified via diff against the pre-move code, not caused by this WORK). Real-Postgres verification of the two new tables is outstanding only because this sandbox has no reachable database — the same limitation recorded throughout this repository's history. Next step is independent review per `docs/ai/protocols/IMPLEMENTATION_REVIEW.md`, then closure if APPROVED; no further decision is needed from you unless review surfaces a DECISION_REQUIRED finding. Full detail: WORK-0034's own Completion Record.

Original READY-authorization checkpoint, preserved below:

## What this WORK does

Session Runtime's `Create` stops resolving the Game's current version through Game Management and instead resolves it entirely from two new tables Session Runtime owns itself. Also moves `game/management`/`game/session` to independent top-level packages (`management/`, `session/`), per `ADR-0014`.

## Scope, in one paragraph

Only `Create` moves. The other five call sites that read a pinned Game version (`Join`, `Start`, `AnswerInteraction`, `SubmitUserIntent`, `CancelSession`, `ExpireTimer`) are explicitly left untouched — still reading Game Management, still running the old Game Language engine — because they can't execute the new JavaScript artifact until `WORK-0038` wires in the JS Executor, which is already `WORK-0038`'s own stated scope. This was a real correctness constraint found while drafting, not a preference.

## The two decisions you already made in this session

1. `Create` receives only `game_uuid` (already true today, no signature change) and resolves the current version purely from Session Runtime's own new tables — never from Game Management, never from data Composer hands it. Composer's own future design (`WORK-0032`) is therefore just a visibility gate with no data handoff.
2. This WORK moves only `Create`, per the scope constraint above.

## What's new (illustrative, not frozen — see WORK-0034's Implementation Freedom)

Two Session-owned tables: `session_games` (`game_uuid` -> `current_definition_uuid`) and `session_game_version_artifacts` (one row per immutable version: `definition_uuid`, `game_uuid`, `backend_script`, `frontend_script`, optional `game_contract`/`assets`/`platform_contract_version`). Real FK between them (same domain, no cross-domain FK). `Create`'s old `engineservice.Compile` validation is removed with no replacement — there's no equivalent JS validation designed yet (that's `WORK-0037`'s future job, at authoring/publish time, not at `Create`).

## What's explicitly NOT done by this WORK

- Game Management still depends on `program`/`gameservice`/`engine` (needed by the 5 untouched call sites).
- `game/language/v1` is not relocated into Session's internal tree yet.
- No publish path exists yet to populate the new tables for a real game (`WORK-0033`) — this WORK's own tests seed fixture rows directly.
- `ADR-0014`'s full goal is therefore only partially closed by this WORK; `WORK-0038` closes the rest.

## Unresolved blockers

None. Both blockers inherited from the cancelled WORK-0031 were resolved by the two decisions above.

## What READY authorizes

A Codebase Agent may implement: the two new tables/migration, `Create`'s new resolution logic, removing `gameCurrentVersionReader` from `Manager`/`New`, and the `game/management`→`management/` / `game/session`→`session/` package moves — nothing beyond what WORK-0034's own Scope section lists. It does not authorize touching the five other call sites, Game Management's use cases, or `game/language/v1`'s location.

## Recommendation

The design is internally consistent and the open questions that existed were resolved by you directly, not invented. Ready for READY authorization if you're satisfied with the table shape and scope split above — full detail is in `../works/WORK-0034-session-owned-executable-script-artifact-and-package-restructuring.md`.
