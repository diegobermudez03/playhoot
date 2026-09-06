# Error Handling Standard

Status: CANONICAL ENGINEERING STANDARD

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
