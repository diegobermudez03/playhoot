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

Do not treat "the desired state already exists" as evidence that a differently-tokened command was a retry. A new token while the desired end state already holds must be evaluated as a genuinely new command against current state (which may legitimately reject it, e.g. as an already-satisfied-precondition business outcome), not silently treated as success merely because the end state happens to already be true. A deterministic rejection reached this way is itself the command's completed logical outcome; whether it is surfaced as a declined result value or as an `error` is governed by `error-handling.md -> Expected Business Outcome vs. Error`, not by this standard.

## Idempotency Policy Ownership

Repository/persistence code may store: the claim/request row; the request payload/fingerprint/explicit semantic fields; the completed outcome; the response data.

Repository/persistence code must not decide: which fields define semantic equivalence; replay vs. current-business-state handling; what the logical outcome means; or any other business-level implication of a replay/conflict. Those are workflow/application semantics — see `domain-logic-placement.md`'s responsibility-ownership guidance for the corresponding workflow/repository split.

## Claim Is A Concurrency Mechanism, Not A Substitute For It

The ability to atomically claim an idempotency identity must not be removed. A different serialization mechanism the domain already has (for example a row lock on an already-existing aggregate) does not replace request idempotency for an operation that has no pre-existing row to lock (for example, a create operation before its target exists). The persistence layer needs a concurrency-safe way to ensure two concurrent requests sharing the same request identity produce exactly one logical operation — for example, an insert against a unique constraint on the identity.

## Completed Outcomes vs. Transient Failures

A completed, deterministic logical outcome may be recorded and replayed. An infrastructure failure or a rolled-back transaction is not a completed logical command and must never become a permanently replayable outcome. If a command is rejected deterministically and that rejection is itself considered the command's completed logical outcome, a same-token retry should replay that rejection consistently — it must not be reinterpreted as a fresh attempt against a different business-state snapshot.

Replaying a stored outcome means returning it as a normal result value, not necessarily recreating a Go `error`. A completed logical outcome that was itself a business decline (per `error-handling.md -> Expected Business Outcome vs. Error`) is persisted and replayed as that decline's outcome value; the workflow does not need a separate mechanism to "recreate" an error merely because the original decision was negative. Only genuinely invalid idempotency usage (a missing/empty token, a same-token conflicting request, corrupt/unreadable persisted idempotency state, a persistence failure) remains an `error`.

## Claim Mechanism Contract

The shared claim/replay mechanism's contract should avoid a redundant boolean alongside a nullable existing-request value. Prefer a two-outcome shape:

```text
(requestID, existingRequest, error)

requestID != 0 && existingRequest == nil  => caller acquired a fresh claim
requestID == 0 && existingRequest != nil  => the request identity already existed
```

Both non-zero or both zero/nil (outside of an `error` return) is an internal invariant failure, not a third normal case. Do not expose a separate `claimed bool` alongside `existingRequest` when the two values are already mutually exclusive.

Name the persisted claim/request shape returned to workflow/application code for what it is to that caller - a request/claim, not a storage row (e.g. `idempotency.Request`, not `idempotency.Row` - see `repositories.md -> Naming Exposed To Workflow Code`). A private GORM model type used only inside the persistence implementation may still be named as a row/model.

On PostgreSQL, prefer a non-error conflict path for the claim insert - conceptually `INSERT ... ON CONFLICT (user_uuid, operation, idempotency_key) DO NOTHING RETURNING id` - over deliberately provoking a unique-constraint error and then continuing to query through the same transaction afterward. If a row is returned, the claim is fresh; if no row is returned, read the existing request. An equivalent PostgreSQL-safe mechanism providing the same guarantee is acceptable; the constraint is not causing an expected, normal conflict path to look like an unhandled error inside the claiming transaction.

## Organizational Guidance

Idempotency claim/replay mechanics are a shared cross-cutting protocol under `repositories.md -> Sharing Rule` when more than one behavior needs the same claim/replay mechanism, in the same sense as per-aggregate locking. This does not require a large, general-purpose idempotency subsystem: the mechanism can remain small and can live locally within the owning workflow's package when only that workflow uses it. Do not build a generic canonical-JSON-diffing framework for request-equivalence comparison; model comparison narrowly per operation, using each operation's own explicit meaningful-fields struct.

The shared mechanism (claim/read/complete) may be consumed directly by the workflow step that needs it, the same way a workflow calls a domain type's method - see `repositories.md -> Sharing Rule`'s guidance against adding a repository forwarding method with no persistence responsibility of its own. What must stay local to the owning step, even though the mechanism itself is shared, is the command-specific interpretation: the meaningful-fields payload struct, semantic-equality comparison, and replay/conflict/rejection-outcome mapping for that one command. Do not centralize every operation's payload struct and interpretation function inside one generic workflow-level file (e.g. a catch-all `idempotency.go` holding every step's payload/interpretation together) merely because they all call the same shared mechanism - place each operation's payload type and interpretation function next to the step that owns that command (e.g. inside `step_join.go`), per `domain-logic-placement.md`'s behavior-locality principle. A small, genuinely mechanical helper (for example, JSON-marshaling a payload) may remain shared only when it has no command-specific interpretation of its own.

## Enforcement

Code review and this standard. No architecture linter or other automated enforcement tool is introduced by this standard.

During code review, flag in particular:

- an operation with a durable idempotent-retry contract that accepts no idempotency token, or infers retry intent from current state alone;
- a repository/persistence layer deciding what a replay/conflict means instead of the workflow/application layer;
- a claim mechanism removed in favor of relying solely on an aggregate lock, for an operation with no pre-existing row to lock;
- a transient infrastructure failure that leaves behind a permanently replayable "completed" outcome;
- a generic canonical-JSON-diffing idempotency-comparison framework introduced where a narrow per-operation comparison would do;
- a redundant `claimed bool` alongside an already-mutually-exclusive `existingRequest` value (see Claim Mechanism Contract above);
- a PostgreSQL claim implementation that deliberately provokes a unique-constraint violation and then continues querying through the same transaction, instead of a non-error conflict path (e.g. `ON CONFLICT ... DO NOTHING RETURNING id`);
- a repository method that only forwards to the shared claim/complete mechanism with no persistence responsibility of its own;
- per-operation payload/interpretation code centralized in one generic workflow-level bucket file instead of living next to the step that owns that command (see Organizational Guidance above).
