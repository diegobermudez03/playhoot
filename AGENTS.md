- The project attempts to be well documented, there will be a lot of .md files, some written by AI, some written by human devs, some written by both, any AI should always read these .md files first before any change.
- However, if the AI needs some implementation information, it should read the .md file but verify the information written there, there could happen that a .md file is outdated in its implementation information
- If some instruction is ambiguous then ask for verification
- If AI finds contradiction between .md files and/or prompt given then the information should be verified and the contradiction should be explained
- Every make sure to understand the business reason for any change, it can be inferred by the business explanation in .md files, but if not easily explained or inferred then ask for business reason confirmation

## Project Patterns

### Repository Queries

- For GORM raw-query reads into structs, prefer `Scan(&value)` and verify `RowsAffected` instead of scanning every selected column manually.
- Column aliases should match the destination struct fields through GORM's normal mapping rules.
- For repository code and repo tests, prefer raw SQL for reads, updates, and deletes.
- Inserts are the exception: prefer ORM `Create()` for inserts so field mapping is explicit and does not depend on fragile parameter ordering.

### Error Boundaries

- Wrapping errors with `%w` should be the exception, not the default.
- By default, add context with `%s` so logs contain the full traversed error message, but callers cannot assert lower-level errors across package/domain boundaries.
- If a service intentionally exposes a specific error to callers, define a clear sentinel error for that contract and wrap that sentinel with `%w`.
- When logging an error, log a contextual `fmt.Errorf(...)` value rather than the raw lower-level error. Each layer should add a message that makes the error path easy to read, even if that creates several wrapper messages.

### Data Integrity Alerts

- For the current project stage, use panics as alert triggers for data-integrity violations and impossible persisted states.
- Server-layer panic recovery will be handled later so these alerts do not shut down the process.
- Returning ordinary errors for integrity problems can be ignored by callers; panics should make those problems visible. For example, an invalid persisted business visibility value is a DB integrity issue and should panic.

### Unit Test Shape

- Prefer table tests unless the function being tested is meaningfully different from that pattern.
- For service tests with external calls, define interfaces and use test mocks.
- In service tests, `svcMocks` should hold the generated mock pointers for those collaborators, for example `gameSvc *MockGameService`, `repo *MockRepo`, and similar dependencies.
- Preferred service table shape:

```go
type test struct {
	// input params and expected outputs
}

type svcMocks struct {
	gameSvc *MockGameService
	repo    *MockRepo
	// other generated mocks required by the service
}

tests := map[string]func(t *testing.T, mocks *svcMocks) test{
	"case_name_with_underscores": func(t *testing.T, mocks *svcMocks) test {
		// configure mocks and expected calls
		return test{}
	},
}

for name, setup := range tests {
	t.Run(name, func(t *testing.T) {
		mocks := &svcMocks{}
		tc := setup(t, mocks)
		// create service/repo, call method, assert output
	})
}
```

- Test case names should use underscores, not spaces.
- Inside each table iteration, create the service/repo under test, create mocks, call the setup callback, make the actual call, and assert outputs.
- For mocked service dependencies, assert the expected mocked calls, parameters, and outputs.
- By default, do not assert error messages in tests. Most test cases should only express whether an error is expected, using fields such as `expectErr bool` and assertions such as `require.Error(t, err)` or `require.NoError(t, err)`.
- Only assert a specific error when the code intentionally exposes a wrapped sentinel error. In those cases, the test struct should carry a field of type `require.ErrorAssertionFunc`, and the test case should provide the callback that performs the sentinel assertion.
- For panic cases, assert only that the call panics with `require.Panics(t, func() { ... })`. Do not assert specific panic messages unless there is a very unusual reason.

### Repository Tests

- Repository tests should use a disposable test database rather than a mocked SQL driver.
- Repository tests in the same domain should share one disposable test DB helper for that domain package, rather than creating a fresh DB per `t.Run(...)`.
- That shared domain test DB should still be disposable: create it for the test process, run migrations, reuse it across repo tests in that domain, and dispose it when the process is done with it.
- The generic disposable test DB machinery should live in `utils` whenever possible, and each domain package should keep only a thin wrapper around that shared helper.
- In each repo test setup callback, seed the data required for that case.
- Repo test seeds should not depend on empty-table assumptions. The shared DB should accumulate data from multiple cases so queries are exercised against nontrivial existing data.
- Repo test seeds should follow the same SQL/ORM rule: use `Create()` for inserts, and use raw SQL for reads, updates, and deletes.
- For read operations, assert the returned value.
- For write/update/delete operations, assert the returned value and also fetch directly from the DB to verify persisted state changed as expected.
