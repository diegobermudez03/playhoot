# GAME-ADR-0021: Session Lobby Command Idempotency Token Semantics

Status: ACCEPTED
Created: 2026-09-16
Last status change: 2026-09-16
Supersedes: None
Superseded by: None

## Context

GAME-ADR-0004 already requires Create/Join/Leave/Start to accept an opaque idempotency key and states that "natural/domain idempotency, such as repeated Join when already actively joined, is useful additional protection but is not a substitute for the explicit idempotency contract." It does not, however, spell out what happens when a *new* idempotency token is used for a Join while the caller is already an active Participant — the wording it and `game/README.md` use ("repeated active Join is idempotent") reads as a state-based rule and does not distinguish a genuine retry of the same logical command from an independent new command that happens to arrive while the desired end state already holds.

`docs/engineering/standards/idempotency.md` (accepted the same day as this record) established the general reusable rule that a differently-tokened command is always a new logical command, evaluated against current state, and must never be treated as an automatic replay merely because the end state it would produce already exists. This record applies that general rule to Session lobby idempotency specifically, refining GAME-ADR-0004's Join wording. It does not reopen or change any other aspect of GAME-ADR-0004 (serialization, JoinCode ownership, pinned-definition resolution, Leave/Start semantics, lazy expiration) or of GAME-ADR-0003's actor/participant model.

## Decision

Session lobby command idempotency (Create, Join, Leave, Start) follows `docs/engineering/standards/idempotency.md`'s general token semantics. For Join specifically:

- **Initial Join** (`Join(user, token=A)`), when admission is valid: the Participant becomes active and the outcome is recorded as A's completed result.
- **Retry of the same command** (`Join(user, token=A)` again, with the same semantically meaningful fields — JoinCode, UserUUID, display-name snapshot): replay A's originally recorded outcome. Do not treat the caller's current "already joined" state as a new decision point.
- **Same token, conflicting fields**: reject as an idempotency conflict, per the already-accepted per-operation meaningful-fields comparison (GAME-ADR-0004 does not enumerate these; the meaningful fields for Join are JoinCode, UserUUID, and the display-name snapshot).
- **A different token while already an active Participant** (`Join(user, token=B)`, B distinct from any token previously used for this user's admission): this is not a retry. Session Runtime evaluates current state as an ordinary new command and rejects it with the domain's already-active-participant business error (`AlreadyJoined` or an equivalent sentinel; the exact exported Go name remains implementation-local, per GAME-ADR-0004's existing scope). Session Runtime must not silently return success merely because the caller happens to already be admitted.

This same different-token-means-new-command rule applies to Create, Leave, and Start: a different token is always evaluated as a new logical command against current state, using each operation's already-accepted business semantics (GAME-ADR-0004) — this record does not invent additional Create/Leave/Start behavior beyond that.

## Rationale

Distinguishing "same command retried" from "new command that happens to find the desired state already true" is necessary to detect a caller/frontend genuinely issuing a second Join while the user is already seated, as opposed to a client legitimately retrying after a dropped response. Silently returning success for a differently-tokened Join would hide that distinction and could mask a caller-side bug (for example, a UI that lost track of already having joined and is now attempting a fresh join for a different, unintended reason).

## Alternatives Considered

### Treat any Join by an already-active Participant as an idempotent success regardless of token

Rejected. This is the state-based reading GAME-ADR-0004's original wording invited. It cannot distinguish a genuine retry from an independent new Join attempt, and would let a differently-tokened command silently succeed on the strength of pre-existing state rather than being evaluated as its own command.

### Require identical tokens across a user's entire Session lifetime

Rejected. Idempotency tokens are per-logical-command by design (`idempotency.md`); requiring one token per user per Session for the operation's entire lifetime is a different, unrequested product/API contract and is not needed to resolve the ambiguity this record addresses.

## Consequences

- Session Runtime's Join implementation must distinguish "existing completed request for this exact token" (replay) from "no completed request for this token, but the user is already an active Participant" (new command, evaluate against current state, reject as already-joined).
- `game/README.md`'s Join description is updated to token-aware wording (see Canonical Knowledge Impact).
- GAME-ADR-0004's own text is not rewritten; this record is the current authority for the Join-retry-vs-new-command distinction, read together with GAME-ADR-0004 for everything else about Join.

## Canonical Knowledge Impact

- `game/README.md` — the Session Runtime Lobby Lifecycle Contract's Join paragraph is refined from a state-based "repeated active Join is idempotent" description to the token-aware rule above.

## Implementation Impact

`docs/work/active/WORK-0001-session-lobby-foundation.md` is updated to require this token-aware behavior and corresponding acceptance criteria/tests. No production code, tests, or migrations are authorized directly by this record.
