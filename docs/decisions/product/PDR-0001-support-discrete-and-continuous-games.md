# PDR-0001: Support Both Discrete And Continuous Games

Status: ACCEPTED
Created: 2026-10-03
Last status change: 2026-10-03
Supersedes: None
Superseded by: None

## Product Context

Playhoot's runtime was designed only for discrete games, whose interactions are individual fixed events. Moving game rules to authored JavaScript scripts removed the main technical reason to restrict games to that shape.

## Decision

Playhoot supports continuous games in addition to discrete games. In a continuous game an interaction is not only a single event: it can be a continuous stream, such as movement. Discrete games remain supported.

## Rationale

Authored scripts make the platform flexible enough to host both kinds, and continuous games widen what a creator can build without changing the creator experience.

## Alternatives Considered

### Discrete games only

Rejected: it keeps the earlier runtime but leaves out a class of games the platform can now host.

## Consequences

- The runtime architecture changes materially (`session/docs/decisions/SESSION-ADR-0028-real-time-session-runtime-supersedes-discrete-turn-architecture.md`).
- Which continuous genres are in scope for the initial launch is not decided. The "not initial priorities" list in `PROJECT_OVERVIEW.md` predates this decision and must be reviewed.

## Canonical Knowledge Impact

- `PROJECT_OVERVIEW.md` - the initial product boundary no longer excludes continuous interaction as such.

## Follow-up

A product discussion to settle the continuous genres in scope and the resulting expectations for the initial launch.
