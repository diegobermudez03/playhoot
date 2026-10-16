package sandbox

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/fastschema/qjs"
)

// RunAsWorkerIfRequested runs one worker request/response when the process was
// started in sandbox worker mode, and returns true. The caller must then exit,
// since the worker is a re-exec of the same binary.
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
	contextVal := ctx.ParseJSON(string(contextJSON))
	defer contextVal.Free()

	if req.Operation == workerOperationProject {
		return runProject(ctx, req, contextVal)
	}
	return runExecute(ctx, req, contextVal)
}

func runExecute(ctx *qjs.Context, req workerRequest, contextVal *qjs.Value) workerResponse {
	prevStateVal := ctx.ParseJSON(rawOrNull(req.PreviousState))
	defer prevStateVal.Free()
	eventVal := ctx.ParseJSON(rawOrNull(req.Event))
	defer eventVal.Free()

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

// runProject invokes the script's pure project(state, viewer, context); its
// return value, whatever its shape, becomes ClientState.
func runProject(ctx *qjs.Context, req workerRequest, contextVal *qjs.Value) workerResponse {
	stateVal := ctx.ParseJSON(rawOrNull(req.State))
	defer stateVal.Free()

	viewerJSON, err := json.Marshal(req.Viewer)
	if err != nil {
		reason := "encoding viewer for script: " + err.Error()
		return workerResponse{Fatal: &reason}
	}
	viewerVal := ctx.ParseJSON(string(viewerJSON))
	defer viewerVal.Free()

	global := ctx.Global()
	entry := global.GetPropertyStr("project")
	if !entry.IsFunction() {
		reason := "script does not define a global project function"
		return workerResponse{Rejected: &reason}
	}

	result, err := global.InvokeJS("project", stateVal, viewerVal, contextVal)
	if err != nil {
		reason := "project() threw: " + err.Error()
		return workerResponse{Rejected: &reason}
	}
	defer result.Free()

	resultJSON, err := result.JSONStringify()
	if err != nil {
		reason := "project() returned a value that cannot be serialized: " + err.Error()
		return workerResponse{Rejected: &reason}
	}
	if resultJSON == "" || resultJSON == "undefined" {
		reason := "project() did not return a value"
		return workerResponse{Rejected: &reason}
	}

	return workerResponse{ClientState: json.RawMessage(resultJSON)}
}
