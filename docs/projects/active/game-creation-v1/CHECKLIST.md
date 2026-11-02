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

- [ ] C. Authoring project: projects, atomic revisions over `baseRevision`, change envelope, conflicts, rejected attempts recorded.
- [ ] D. Draft assets: catalog, reserve / signed upload / finalize, local storage provider.
- [ ] E. Build jobs: in-process queue, Go runs the Node toolchain, pinned inputs, structured diagnostics.
- [ ] F. AI access by project link and the creation API; context page readable by an AI.

## Test (plan delivery 2)

- [ ] G. Human preview: test sessions with synthetic participants and perspective switching.
- [ ] H. Automated scenarios with a virtual clock reusing the real scheduling.

## Publish (plan delivery 3)

- [ ] I. MCP server and OAuth for AIs (separate AI attempt/session tables in Identity).
- [ ] J. Publication: release, register and activate in play, catalog, real playability check, sessions pinned to the authorized version.
- [ ] K. Templates and remix.
