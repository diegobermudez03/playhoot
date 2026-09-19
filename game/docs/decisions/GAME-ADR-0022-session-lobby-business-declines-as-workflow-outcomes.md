# GAME-ADR-0022: Session Lobby Business Declines As Workflow Outcomes

Status: ACCEPTED
Created: 2026-09-17
Last status change: 2026-09-17
Supersedes: None
Superseded by: None

## Context

GAME-ADR-0004 accepts Create/Join/Leave/Start's deterministic business declines (for example: an already-active Participant, lobby capacity exceeded, a lobby discovered already expired, Leave outside `LOBBY`) as part of the Session lifecycle's normal behavior. GAME-ADR-0021 refines Join's token semantics specifically: a differently-tokened Join while already an active Participant is a new command, evaluated against current state, and rejected with the already-active-participant business decision rather than silently replayed or treated as success.

Neither record decided *how* a deterministic business decline is represented in the Session lifecycle workflow's own Go contract. The first Slice 1 implementation pass represented every such decline as a returned Go `error` (a package-level sentinel, e.g. `ErrAlreadyJoined`/`ErrLobbyFull`/`ErrLobbyExpired`), including cases where the decline itself is the operation's durably completed, replayable logical outcome. `docs/engineering/standards/error-handling.md` (refined the same day as this record — see its "Expected Business Outcome vs. Error" section) established the general reusable rule that an expected, correctly-evaluated business decision — positive or negative — is a normal result value, not automatically an `error`; only cases where the operation could not be correctly executed/evaluated at all are errors. This record applies that general rule to the Session lifecycle's already-accepted Join/Create/Leave business declines specifically, refining (not superseding) GAME-ADR-0021's and GAME-ADR-0004's wording, which predates and does not contradict this record but did not itself settle the outcome-vs-error question.

## Decision

The Session lifecycle workflow (Create/Join/Leave, and later Start) represents its already-accepted deterministic business declines as valid result/outcome values, not as returned Go `error`s, per `docs/engineering/standards/error-handling.md -> Expected Business Outcome vs. Error`.

Concretely, for Join: `JOINED`, `LOBBY_EXPIRED`, `LOBBY_FULL`, and `ALREADY_JOINED` are all possible outcomes of a correctly evaluated Join, returned as a result value (conceptually a `JoinResult{Outcome: ...}`) alongside a `nil` error. This is a representation change only — no new domain semantics beyond what GAME-ADR-0004/GAME-ADR-0021 already accept:

- `JOINED` — admission succeeded (GAME-ADR-0004).
- `LOBBY_EXPIRED` — the lobby was discovered already expired and materialized `TERMINAL` (GAME-ADR-0004).
- `LOBBY_FULL` — admission would exceed the pinned Definition's `players.max` (GAME-ADR-0004).
- `ALREADY_JOINED` — a differently-tokened Join while already an active Participant, evaluated as a new command against current state (GAME-ADR-0021).

The same representation applies to Create's and Leave's own already-accepted deterministic declines (for example, Leave outside `LOBBY`), using each operation's own existing accepted semantics — this record does not invent new Create/Leave business behavior.

Errors remain reserved for cases the operation could not correctly execute/evaluate at all: an invalid command/protocol contract (a missing/empty idempotency token, a malformed identifier); the same idempotency token reused with a conflicting semantic request; infrastructure/dependency failure; corrupt/impossible persisted state; a violated internal invariant. `docs/engineering/standards/error-handling.md` owns this general classification; this record only confirms the Session lifecycle's already-accepted business declines fall on the outcome side of it.

Idempotency replay is unaffected in substance: a same-token retry still replays the exact previously completed logical outcome (GAME-ADR-0021, `docs/engineering/standards/idempotency.md`). What changes is only that the replayed value is a normal result rather than a recreated `error` — the persisted `session_requests.outcome` already recorded which outcome occurred; replay returns that outcome as a value.

Token semantics, the retry-vs-new-command distinction, and every other GAME-ADR-0021/GAME-ADR-0004 decision are unchanged and are not reopened by this record.

## Rationale

Routing every anticipated, correctly-evaluated business decision through Go's `error` return conflates "the operation could not be executed" with "the operation ran and declined" — two different things a caller (and a future transport layer) needs to distinguish differently. It also motivated a parallel, ad hoc error channel in the first implementation pass (a closure-captured `bizErr` alongside the transaction callback's own `error` return) purely to let a durable outcome commit while still reporting a decline — mechanical complexity that a plain result value avoids entirely: a declined outcome commits through the ordinary `(result, nil)` success path, and the transaction helper's commit/rollback rule stays exactly "commit on nil callback error, roll back otherwise," per `docs/engineering/standards/repositories.md -> Transaction Ownership`.

## Alternatives Considered

### Keep every business decline as a returned `error`, and add a documented convention for which errors may still commit durable state

Rejected. This still requires some out-of-band mechanism (a second error-like channel, or a documented exception to "non-nil error rolls back") for exactly the cases where a decline's durable outcome must survive — the same complexity this record removes, without the benefit of a plain result value at the caller's contract.

### Represent every operation's result as `(bool success, error)` with the error itself carrying outcome detail

Rejected. This does not resolve the underlying conflation - a caller/transport still cannot tell "declined but fully evaluated" from "could not be evaluated" without inspecting error content, and a boolean success flag adds no information a properly-typed outcome value doesn't already carry more precisely.

## Consequences

- Session lifecycle result types (`JoinResult`, and the equivalent Create/Leave result types) carry an `Outcome`-shaped field distinguishing success from each already-accepted decline; exact Go naming/shape is implementation-local, per `docs/work/active/WORK-0001-session-lobby-foundation.md`.
- The Session lifecycle workflow's transaction callback contract simplifies to the standard shape in `docs/engineering/standards/repositories.md -> Transaction Ownership`: `(result, nil)` commits, `(zeroResult, err)` rolls back, with no separate business-error channel.
- A future transport layer remains free to map a declined outcome onto an HTTP status/WebSocket response/error envelope; that mapping is separate from, and does not reopen, this record's workflow-level contract (`error-handling.md -> Expected Business Outcome vs. Error`).
- `game/README.md`'s Join paragraph is refined to describe `AlreadyJoined` (and the lobby's other declines) as workflow outcomes rather than as an application "business error" (see Canonical Knowledge Impact).

## Canonical Knowledge Impact

- `game/README.md` — the Session Runtime Lobby Lifecycle Contract's Join paragraph is refined so its already-accepted `AlreadyJoined` decline (GAME-ADR-0021) is described as a returned/replayed workflow outcome rather than as an application-level "business error."

## Implementation Impact

`docs/work/active/WORK-0001-session-lobby-foundation.md` is revised to require Join/Create/Leave's already-accepted deterministic declines to be represented as result-value outcomes rather than returned `error`s, and to remove the transitional `bizErr`-style closure-mutation pattern accordingly. No production code, tests, or migrations are authorized directly by this record.
