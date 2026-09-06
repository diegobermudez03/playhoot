# Testing Standard

Status: CANONICAL ENGINEERING STANDARD

## General Unit / Service Test Shape

- Prefer table-driven tests unless the behavior is meaningfully unsuited to that pattern.
- Test case names use underscores rather than spaces.

## Service Dependencies / Mocks

- External/collaborator calls in service tests should be represented through interfaces and mocks.
- `svcMocks` holds generated collaborator mock pointers.
- Preserve the preferred service test setup pattern: tests map -> setup callback -> test struct.
- Each test iteration creates/configures the service and mocks, runs setup, invokes behavior, and asserts outputs.
- Expected mock calls, parameters, and outputs should be asserted.

## Error Assertions

- By default, do not assert complete error-message strings.
- Most cases should express whether an error is expected, using `expectErr` or an equivalent field.
- Assert a specific error only when the code intentionally exposes an error contract such as a sentinel.
- Use `require.ErrorAssertionFunc` as the preferred table-test mechanism for those specific assertions.
- Panic tests should normally assert only that a panic occurs.
- Do not assert panic text without an unusual reason.

## Repository Tests

- Use a disposable real test database rather than a mocked SQL driver.
- Repository tests within the same domain/package should use the shared disposable-domain test DB infrastructure rather than independently inventing DB setup.
- Generic disposable DB infrastructure belongs in `utils` when possible; domain/package-specific wrappers remain thin.
- Seed each repository test case with the data it requires.
- Do not rely on empty-table assumptions.
- Shared test DB data should be allowed to accumulate so queries operate against non-trivial existing state.
- Seed operations follow the repository SQL/Create convention: `Create()` for inserts; raw SQL for reads, updates, and deletes.
- Read operations assert the returned value.
- Write/update/delete operations assert both the operation result and actual persisted state by reading the DB.

This standard does not define coverage targets, integration/e2e strategy, race-test policy, or other testing policy not previously accepted.
