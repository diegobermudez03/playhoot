// Package bootstrap exposes the minimal process-entry hook the root binary
// needs to dispatch into Session Runtime's sandboxed JavaScript worker mode
// (game/session/internal/jsengine), without pulling that sandbox runtime's
// dependencies into game/session's own zero-dependency public contract
// package (see that package's own doc comment for why it must stay
// dependency-free).
package bootstrap

import "github.com/diegobermudez03/playhoot/game/session/internal/jsengine"

// RunJSWorkerIfRequested checks whether the current process invocation is a
// jsengine worker re-exec and, if so, runs it and returns true. The caller
// (main.go) must exit the process immediately afterward without falling
// through to normal application startup.
func RunJSWorkerIfRequested() bool {
	return jsengine.RunAsWorkerIfRequested()
}
