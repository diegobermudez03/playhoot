package jsengine

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/fastschema/qjs"
)

// RunAsWorkerIfRequested checks whether the current process was invoked in
// jsengine worker mode (the sentinel argument Execute passes when spawning a
// worker). If so, it runs exactly one request/response cycle against stdin/
// stdout and returns true — the caller must exit the process immediately
// afterward without falling through to normal application startup, since the
// worker process is a plain re-exec of the same binary, not a distinct one.
func RunAsWorkerIfRequested() bool {
	if len(os.Args) < 2 || os.Args[1] != workerModeArg {
		return false
	}
	runWorker(os.Stdin, os.Stdout)
	return true
}

func runWorker(in io.Reader, out io.Writer) {
	resp := executeRequest(in)
	encoded, err := json.Marshal(resp)
	if err != nil {
		// Marshaling our own response struct should never fail; fall back
		// to a minimal hand-built payload rather than writing nothing.
		fallback := fmt.Sprintf(`{"fatal":%q}`, "encoding worker response: "+err.Error())
		_, _ = io.WriteString(out, fallback)
		return
	}
	_, _ = out.Write(encoded)
}

func executeRequest(in io.Reader) (resp workerResponse) {
	defer func() {
		if r := recover(); r != nil {
			reason := fmt.Sprintf("worker panic: %v", r)
			resp = workerResponse{Fatal: &reason}
		}
	}()

	reqBytes, err := io.ReadAll(in)
	if err != nil {
		reason := "reading request: " + err.Error()
		return workerResponse{Fatal: &reason}
	}

	var req workerRequest
	if err := json.Unmarshal(reqBytes, &req); err != nil {
		reason := "decoding request: " + err.Error()
		return workerResponse{Fatal: &reason}
	}

	return runScript(req)
}

func runScript(req workerRequest) workerResponse {
	if req.ScratchDir == "" {
		reason := "no sandbox scratch directory supplied"
		return workerResponse{Fatal: &reason}
	}

	rt, err := qjs.New(qjs.Option{
		CWD:              req.ScratchDir,
		MemoryLimit:      defaultMemoryLimitBytes,
		MaxStackSize:     defaultMaxStackSizeBytes,
		MaxExecutionTime: defaultMaxExecutionTimeMillis,
		Stdout:           io.Discard,
		Stderr:           io.Discard,
	})
	if err != nil {
		reason := "creating sandbox runtime: " + err.Error()
		return workerResponse{Fatal: &reason}
	}
	defer rt.Close()

	ctx := rt.Context()

	logicalTime, err := time.Parse(time.RFC3339Nano, req.Context.LogicalTime)
	if err != nil {
		reason := "invalid logical time in context: " + err.Error()
		return workerResponse{Fatal: &reason}
	}
	randomSeed, err := strconv.ParseUint(req.Context.RandomSeed, 10, 64)
	if err != nil {
		reason := "invalid random seed in context: " + err.Error()
		return workerResponse{Fatal: &reason}
	}

	prelude := buildDeterminismPrelude(logicalTime.UnixMilli(), randomSeed)
	if _, err := ctx.Eval("determinism_prelude.js", qjs.Code(prelude)); err != nil {
		reason := "installing determinism prelude: " + err.Error()
		return workerResponse{Fatal: &reason}
	}

	if _, err := ctx.Eval("script.js", qjs.Code(req.Script)); err != nil {
		reason := "loading script: " + err.Error()
		return workerResponse{Rejected: &reason}
	}

	contextJSON, err := json.Marshal(req.Context)
	if err != nil {
		reason := "encoding context for script: " + err.Error()
		return workerResponse{Fatal: &reason}
	}

	prevStateVal := ctx.ParseJSON(rawOrNull(req.PreviousState))
	defer prevStateVal.Free()
	eventVal := ctx.ParseJSON(rawOrNull(req.Event))
	defer eventVal.Free()
	contextVal := ctx.ParseJSON(string(contextJSON))
	defer contextVal.Free()

	global := ctx.Global()
	entry := global.GetPropertyStr("execute")
	if !entry.IsFunction() {
		reason := "script does not define a global execute function"
		return workerResponse{Rejected: &reason}
	}

	result, err := global.InvokeJS("execute", prevStateVal, eventVal, contextVal)
	if err != nil {
		reason := "execute() threw: " + err.Error()
		return workerResponse{Rejected: &reason}
	}
	defer result.Free()

	resultJSON, err := result.JSONStringify()
	if err != nil {
		reason := "execute() returned a value that cannot be serialized: " + err.Error()
		return workerResponse{Rejected: &reason}
	}

	var shape struct {
		NewState          json.RawMessage   `json:"newState"`
		RequestedCommands []json.RawMessage `json:"requestedCommands"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &shape); err != nil {
		reason := "execute() did not return the required {newState, requestedCommands} shape: " + err.Error()
		return workerResponse{Rejected: &reason}
	}
	if len(shape.NewState) == 0 {
		reason := "execute() did not return a newState"
		return workerResponse{Rejected: &reason}
	}

	return workerResponse{
		NewState:          shape.NewState,
		RequestedCommands: shape.RequestedCommands,
	}
}
