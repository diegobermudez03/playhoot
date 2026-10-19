# Session Runtime Architecture Decisions

Status: DECISION INDEX (DOMAIN-SCOPED)

ADRs preserve architecture decision rationale. Current Session Runtime truth is owned by `session/README.md` and, where applicable, `session/CURRENT_STATE.md`. Load an individual record only when the rationale behind a current rule is needed.

Naming: `SESSION-ADR-NNNN`, with a sequence independent of the global and other domain families.

| ID | Title | Status | Created | Canonical Impact |
| --- | --- | --- | --- | --- |
| [SESSION-ADR-0028](SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md) | Real-Time Session Runtime Supersedes The Discrete-Turn Architecture | ACCEPTED | 2026-10-03 | `session/README.md`, `ARCHITECTURE.md` |
| [SESSION-ADR-0029](SESSION-ADR-0029-live-session-ownership-and-owner-routing.md) | Live Session Ownership And Owner Routing | ACCEPTED | 2026-10-15 | `play/README.md` |
| [SESSION-ADR-0030](SESSION-ADR-0030-the-session-controller-owns-its-live-execution.md) | The Session Controller Owns Its Live Execution | ACCEPTED | 2026-10-18 | `play/README.md` |
| [SESSION-ADR-0031](SESSION-ADR-0031-game-time-is-handled-by-update-without-platform-timers.md) | Game Time Is Handled By update(), Without Platform Timers | ACCEPTED | 2026-10-18 | `play/CURRENT_STATE.md` |

`SESSION-ADR-0001` through `SESSION-ADR-0027` record the previous, discrete-turn design. They are archived in `docs/archive/pre-realtime-pivot/session/docs/decisions/` and are history only (see `docs/archive/pre-realtime-pivot/README.md`). Their numbers are not reused.

Next Session ADR: `SESSION-ADR-0032`.

Whenever a Session ADR is created or its lifecycle status changes, update this index.
