# Idempotency Standard

Status: CANONICAL ENGINEERING STANDARD

This standard owns the reusable distinction between natural and request/command idempotency, required-token semantics, replay/conflict rules, and idempotency responsibility ownership. It does not decide any specific domain's operation list or per-operation payload fields — those belong to the owning domain's canonical documentation/decision records (for example Session lifecycle idempotency: `game/README.md`, `game/docs/decisions/`).

## Natural Idempotency vs Request/Command Idempotency

**Natural idempotency**: repeating the operation naturally produces an acceptable equivalent state/result without needing to identify a particular prior request. A request ledger/token is not necessarily required.

**Request/command idempotency**: the system promises that a specific logical command is recognized across retries and that its completed outcome can be replayed. This requires identifying which prior request a retry corresponds to — natural idempotency alone cannot provide this guarantee.

Do not infer retry intent only from current domain state. "The desired state already exists" is not evidence that a differently-identified command was a retry of a prior one — see Token Semantics below.

## Required Token

If an operation relies on request-level idempotency, its public/application contract must explicitly require an idempotency token. A missing/empty token is invalid input. The token must appear explicitly in the method signature (per `function-signatures.md`) unless it is represented by a genuinely semantic command concept.

## Token Semantics

For a given idempotency identity:

- same token + same semantic request parameters => same logical command => replay the previously completed logical outcome.
- same token + different semantic request parameters => idempotency conflict; reject, do not silently apply the new request or silently return the old result.
- different token => a new logical command; evaluate the domain's current business state normally, as an ordinary new request.

Do not treat "the desired state already exists" as evidence that a differently-tokened command was a retry. A new token while the desired end state already holds must be evaluated as a genuinely new command against current state (which may legitimately reject it, e.g. as an already-satisfied-precondition business error), not silently treated as success merely because the end state happens to already be true.

## Idempotency Policy Ownership

Repository/persistence code may store: the claim/request row; the request payload/fingerprint/explicit semantic fields; the completed outcome; the response data.

Repository/persistence code must not decide: which fields define semantic equivalence; replay vs. current-business-state handling; what the logical outcome means; or any other business-level implication of a replay/conflict. Those are workflow/application semantics — see `domain-logic-placement.md`'s responsibility-ownership guidance for the corresponding workflow/repository split.

## Claim Is A Concurrency Mechanism, Not A Substitute For It

The ability to atomically claim an idempotency identity must not be removed. A different serialization mechanism the domain already has (for example a row lock on an already-existing aggregate) does not replace request idempotency for an operation that has no pre-existing row to lock (for example, a create operation before its target exists). The persistence layer needs a concurrency-safe way to ensure two concurrent requests sharing the same request identity produce exactly one logical operation — for example, an insert against a unique constraint on the identity.

## Completed Outcomes vs. Transient Failures

A completed, deterministic logical outcome may be recorded and replayed. An infrastructure failure or a rolled-back transaction is not a completed logical command and must never become a permanently replayable outcome. If a command is rejected deterministically and that rejection is itself considered the command's completed logical outcome, a same-token retry should replay that rejection consistently — it must not be reinterpreted as a fresh attempt against a different business-state snapshot.

## Organizational Guidance

Idempotency claim/replay mechanics are a shared cross-cutting protocol under `repositories.md -> Sharing Rule` when more than one behavior needs the same claim/replay mechanism, in the same sense as per-aggregate locking. This does not require a large, general-purpose idempotency subsystem: the mechanism can remain small and can live locally within the owning workflow's package when only that workflow uses it. Do not build a generic canonical-JSON-diffing framework for request-equivalence comparison; model comparison narrowly per operation, using each operation's own explicit meaningful-fields struct.

## Enforcement

Code review and this standard. No architecture linter or other automated enforcement tool is introduced by this standard.

During code review, flag in particular:

- an operation with a durable idempotent-retry contract that accepts no idempotency token, or infers retry intent from current state alone;
- a repository/persistence layer deciding what a replay/conflict means instead of the workflow/application layer;
- a claim mechanism removed in favor of relying solely on an aggregate lock, for an operation with no pre-existing row to lock;
- a transient infrastructure failure that leaves behind a permanently replayable "completed" outcome;
- a generic canonical-JSON-diffing idempotency-comparison framework introduced where a narrow per-operation comparison would do.
