# Playhoot patch of github.com/fastschema/qjs v0.0.6

Vendored and replaced in the root `go.mod` because upstream gives authored code
host capabilities that cannot be turned off through its options.

Changes, all in `runtime.go` (`New`):

- No host directory is mounted into the WebAssembly module (upstream mounts
  `Option.CWD` as `/`); `Option.CWD` is therefore ignored.
- `WithSysWalltime`, `WithSysNanotime` and `WithSysNanosleep` are not set, so
  clocks, sleep and randomness are wazero's deterministic fakes, not the host's.

Upstream tests and `test.sh` were removed: they exercised the filesystem the
patch takes away. Keep this file current with every change to the copy.
