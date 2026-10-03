# Session Runtime - Current State

Status: CURRENT IMPLEMENTATION

## Capability Status

| Area | Status | Notes |
| --- | --- | --- |
| Session Runtime | NOT IMPLEMENTED | The previous implementation was removed when Session Runtime was redesigned as a real-time runtime (`session/docs/decisions/SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md`). No sessions, tables, endpoints or workflows exist. |
| Script execution helpers | HELPER ONLY | `session/internal/sandbox` runs a JavaScript source in an isolated QuickJS-on-WebAssembly worker process. Tested, not called by anything. |
| Object storage helpers | HELPER ONLY | `session/internal/objectstore` (GCS and in-memory stores, verifying cache). Tested, not called by anything. |
| Migrations | STRUCTURE ONLY | `session/migration` runs an empty migration registry. |

## Known Drift

- None currently identified.

## Evidence

- `session/internal/sandbox/`, `session/internal/objectstore/`, `session/internal/storage/migrations/migration.go`.
