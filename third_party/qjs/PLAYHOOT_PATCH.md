# Playhoot patch of github.com/fastschema/qjs v0.0.6

Vendored and replaced in the root `go.mod` because upstream gives authored code
host capabilities that cannot be turned off through its options.

Changes, all in `runtime.go` (`New`):

- No host directory is mounted into the WebAssembly module (upstream mounts
  `Option.CWD` as `/`); `Option.CWD` is therefore ignored.
- `WithSysWalltime`, `WithSysNanotime` and `WithSysNanosleep` are not set, so
  clocks, sleep and randomness are wazero's deterministic fakes, not the host's.

Memory fixes, in `value.go` (found 2026-10-18 when a game's state came back
as garbage):

- `JSONStringify` no longer calls the C helper `QJS_JSONStringify`, which frees
  the string before returning its address (a use-after-free: large results
  were read from reused memory). It calls `JSON.stringify` and reads the
  result through `String()`.
- `String()` releases the C string from `QJS_ToCString` with `JS_FreeCString`
  after copying it; upstream leaked it on every call.

Both are Go-side changes: `qjs.wasm` is upstream's, unchanged.

Upstream tests and `test.sh` were removed: they exercised the filesystem the
patch takes away. Keep this file current with every change to the copy.
