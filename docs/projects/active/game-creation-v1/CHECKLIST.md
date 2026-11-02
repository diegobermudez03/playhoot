# Game Creation v1 - Checklist

Order decided by the human on 2026-11-01. Frontend items live in the frontend's `game-creation-web-v1`.

## Build (plan delivery 0)

### B. SDK compile profile `sdk-v1` (with frontend A)

- [x] scriptcheck profile `sdk-v1`: top-level only `function` declarations and `const` bindings whose initializers are literals, nested object/array literals, functions, or calls to the SDK definition functions; no `let`/`var`, no reassignment; escape rules kept (`eval`, `Function`, `globalThis`, `Reflect`, `WebAssembly`, `constructor`, `__proto__`, `prototype`, async, generators); parameter-mutation rules not applied. Legacy scripts keep the current profile.
- [x] Worker: for `sdk-v1` scripts, arguments are not frozen, and the top-level constants listed by the bundle's generated `__playhoot_module_values()` are deep-frozen once after load.
- [x] A game version records its script profile (`play_game_versions.script_profile`, default `handwritten` for existing rows and earlier seeds); start and recovery treat an unknown one as broken data and pass it to the sandbox.
- [x] Test with the real SDK bundle in the real sandbox (`play/runtime/sandbox/sdk_profile_test.go`, Tap Race from the frontend build in `testdata/`): start, commands changing the state in place, project, restore, finish; mutation of a frozen constant fails; handwritten scripts keep frozen arguments. 2026-11-01.
- [x] Tap Race seeded (`seed_20261101000000_tap_race`, Game `55555555-5555-4555-8555-555555555555`) and played end to end against a running backend on a temporary database: two guests, start, 20 COMMANDs, `GAME_FINISHED` with the right winner, the signed frontend's hash equal to the build's. 2026-11-01.

## Drafts (plan delivery 1)

- [x] C. Authoring project (2026-11-01): Game Management `CreateGame` (unpublished game + first edition) and `FindEditableVersion`; Play `versiondrafting` (DRAFT version with revisions, starter template, optimistic lock on the base revision, required idempotency key, every attempt recorded, restore); `orchestrator/gameauthoring` and `composer/gameauthoring`; `api/games` (`POST /v1/games`, `GET /draft`, `POST /draft/changes`, `GET /draft/revisions`, `POST /draft/restore`); Identity `CREATE_GAME` and `EDIT_GAME`. The change envelope's fields are snake_case (`base_revision`, `idempotency_key`). Smoke-tested against a running backend. 2026-11-02, from the human's review: no literal values in SQL (constants bound as parameters, inserts through GORM `Create()`); a version Play does not have yet (creation stopped between the domains) is created on demand by the orchestrator on the next edit or read, Game Management unaware, no background job; a change that changes nothing answers `unchanged` without a revision.
- [x] D. Draft assets (2026-11-02): declared in `game.project.json` (`assets`: type, description, optional); upload through the backend (`PUT /draft/assets/{key}`, redesigned by the human the same day from a reserve/signed-PUT/finalize flow, standard `docs/engineering/standards/object-storage.md`: streamed and cut at the type's limit, hashed while read, real type sniffed from the bytes, registered before being stored content-addressed, bound to the key in a new revision on the current one, no base revision; at most 4 uploads in memory per instance), remove; reads list each asset's status with a signed preview link; attempts record their kind. `api/games` endpoints under `/draft/assets`. Decided by the human 2026-11-02: publishing deletes uploaded objects no published version uses, keeping their record (`play_asset_objects.deleted_at`); a new edition keeps everything until its own publication; a retention limit for unpublished drafts is post-launch. That cleanup belongs to J.
- [x] E. Build jobs (2026-11-02): every revision built automatically in the background, coalesced (older queued builds `SUPERSEDED`), reused when sources, assets and toolchain match an earlier succeeded build; `play/session/workflows/versionbuilding` worker (one per instance, woken on queue, requeues builds left running) behind the `gamebuild.Builder` port (Node process of the game tools at `GAME_TOOLCHAIN_DIR`); output checked with scriptcheck `sdk-v1` and the manifest parser, artifacts registered then stored content-addressed; diagnostics by stage with `infrastructure` never blamed on the game; builds in `GET /draft`, edit answers and the long poll `GET /draft/build?revision&wait`.
- [ ] F. AI access by project link and the creation API; context page readable by an AI.

## Test (plan delivery 2)

- [ ] G. Human preview: test sessions with synthetic participants and perspective switching.
- [ ] H. Automated scenarios with a virtual clock reusing the real scheduling.

## Publish (plan delivery 3)

- [ ] I. MCP server and OAuth for AIs (separate AI attempt/session tables in Identity).
- [ ] J. Publication: release, register and activate in play, catalog, real playability check, sessions pinned to the authorized version; required assets present; delete the uploaded asset objects no published version uses (record kept in play_asset_objects.deleted_at).
- [ ] K. Templates and remix.
