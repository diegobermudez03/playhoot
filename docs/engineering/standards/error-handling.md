# Error Handling Standard

Status: CANONICAL ENGINEERING STANDARD

## Expected Business Outcome vs. Error

A workflow/application operation that runs to completion and is able to evaluate its business rules produces one of two materially different kinds of result. They must not both collapse into the same Go `error` return.

**Expected business outcome**: the operation executed correctly and reached a definite, expected decision - positive or negative. A deterministic decline (for example: a lobby is already full; a lobby has already expired; a caller is already an active participant) is not an execution failure merely because the decision was "no". Model this as a value the caller receives alongside a `nil` error - for example, a result struct with an `Outcome` field (`docs/engineering/standards/function-signatures.md -> Return Values` already permits a result struct exactly when the result is a genuine cohesive concept). Do not force every anticipated business decision through the `error` return merely because it is not the happy path.

**Error**: the operation could not be correctly executed or evaluated at all. Use `error` for cases such as: an invalid command/protocol contract (a malformed required identifier, a required field missing); a missing/empty required idempotency token; the same idempotency token reused with a conflicting semantic request; an infrastructure failure (DB read/write, transaction BEGIN/COMMIT); a dependency failure; a corrupt/impossible persisted state; a violated internal invariant.

Whether declined outcomes share one Go type with the success outcome, or are represented as a small set of outcome-specific values, is a local implementation choice - the standard's constraint is only that the method contract must not route every anticipated business decision through `error`.

A later transport layer (HTTP status, WebSocket response, API error envelope) may still choose to map a declined outcome onto something that looks like an error at that boundary. That transport-level mapping decision is separate from, and must not distort, the workflow/application contract itself - the workflow returns its outcome as a value regardless of how a future transport chooses to represent it externally.

This interacts with idempotent replay: a completed logical command's outcome - including a deterministic decline - is exactly what a same-token retry replays. See `docs/engineering/standards/idempotency.md -> Completed Outcomes vs. Transient Failures`.

## Error Exposure

- Wrapping errors with `%w` is the exception rather than the default.
- By default, add lower-level context using `%s`.
- This keeps useful lower-level message/context visible in the resulting error or log output while deliberately not exposing the lower-level error in the unwrap chain or caller-visible error contract.

## Intentional Error Contracts

When a package or service intentionally exposes a specific error:

- define a clear sentinel error representing that contract;
- wrap that intentional sentinel using `%w`;
- callers and tests may then use `errors.Is` / `require.ErrorIs`.

Do not imply that arbitrary lower-level implementation errors should be wrapped.

## Error Logging Boundary

- Do not log every error at every layer.
- Returned errors are normally logged at an appropriate entry/boundary layer.
- Intermediate layers should usually add context and return.

Log locally when an error:

- will be ignored;
- swallowed;
- converted into a successful/non-error path;
- otherwise will not reach a caller expected to log it.

When contextual logging is needed, log a contextual error value rather than only the raw lower-level error.

This standard does not define a detailed global logging architecture.
