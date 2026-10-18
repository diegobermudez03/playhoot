package sandbox

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	runtimemetrics "runtime/metrics"

	"github.com/fastschema/qjs"
)

// RunAsWorkerIfRequested serves one context until its host closes stdin when
// the process was started in worker mode, and returns true; the caller must
// then exit. It must be the first thing main does.
func RunAsWorkerIfRequested() bool {
	if len(os.Args) < 2 || os.Args[1] != workerModeArg {
		return false
	}
	runWorker(os.Stdin, os.Stdout)
	return true
}

// runWorker answers each request line on in with one response line on out,
// until in ends.
func runWorker(in io.Reader, out io.Writer) {
	// Soft cap of this worker's whole Go heap, the JavaScript runtime's
	// memory included, so its footprint stays near the per-context budget.
	debug.SetMemoryLimit(workerGoMemoryLimitBytes)
	w := &worker{}
	defer w.close()
	reader := bufio.NewReader(in)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			return
		}
		resp := w.handle(line)
		resp.MemoryBytes = memoryBytes()
		encoded, marshalErr := json.Marshal(resp)
		if marshalErr != nil {
			encoded = []byte(fmt.Sprintf(`{"fatal":%q}`, "encoding worker response: "+marshalErr.Error()))
		}
		if _, err := out.Write(append(encoded, '\n')); err != nil {
			return
		}
	}
}

// memoryBytes is all the memory this process's Go runtime holds, the
// JavaScript runtime's included.
func memoryBytes() uint64 {
	sample := []runtimemetrics.Sample{{Name: "/memory/classes/total:bytes"}}
	runtimemetrics.Read(sample)
	if sample[0].Value.Kind() != runtimemetrics.KindUint64 {
		return 0
	}
	return sample[0].Value.Uint64()
}

// worker is one game's execution context: its JavaScript runtime with the
// script loaded, and the authoritative state its last execution produced.
type worker struct {
	runtime *qjs.Runtime
	// control is the determinism prelude's object, outside authored reach.
	control *qjs.Value
	// state is the authoritative game state as JSON; executions read a copy,
	// so a script can never keep a reference that changes it later.
	state    string
	started  bool
	revision int64
	// sessionTimeMs is the effective Session time state was reached at.
	sessionTimeMs int64
	// broken is set when a failure may have left the runtime unusable.
	broken bool
}

func (w *worker) close() {
	if w.control != nil {
		w.control.Free()
	}
	if w.runtime != nil {
		w.runtime.Close()
	}
}

func (w *worker) handle(line []byte) (resp workerResponse) {
	defer func() {
		// A trap inside the WebAssembly runtime (such as unbounded recursion)
		// panics; the authored code caused it, and the runtime is not reused.
		if r := recover(); r != nil {
			w.broken = true
			resp = scriptFailure(fmt.Sprintf("script crashed its runtime: %v", r))
		}
	}()

	var req workerRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return fatal("decoding request: " + err.Error())
	}
	if w.broken {
		return fatal("context is broken by an earlier failure")
	}
	switch req.Operation {
	case workerOperationLoad:
		return w.load(req)
	case workerOperationStart:
		return w.start(req)
	case workerOperationSamples:
		return w.samples(req)
	case workerOperationProject:
		return w.project(req)
	case workerOperationRestore:
		return w.restore(req)
	default:
		return fatal(fmt.Sprintf("unknown operation %q", req.Operation))
	}
}

func (w *worker) load(req workerRequest) workerResponse {
	if w.runtime != nil {
		return fatal("context already loaded")
	}
	// The patched runtime mounts no directory and gives no real clock: the
	// script has no host capability even through the qjs:std or qjs:os
	// modules.
	runtime, err := qjs.New(qjs.Option{
		MemoryLimit:  defaultMemoryLimitBytes,
		MaxStackSize: defaultMaxStackSizeBytes,
		Stdout:       io.Discard,
		Stderr:       io.Discard,
	})
	if err != nil {
		return fatal("creating sandbox runtime: " + err.Error())
	}
	w.runtime = runtime
	control, err := runtime.Context().Eval("determinism_prelude.js", qjs.Code(determinismPrelude))
	if err != nil {
		return fatal("installing determinism prelude: " + err.Error())
	}
	w.control = control
	if _, err := runtime.Context().Eval("script.js", qjs.Code(req.Script), qjs.FlagStrict()); err != nil {
		w.broken = true
		return scriptFailure("loading script: " + err.Error())
	}
	// Once loaded, nothing global can change: all game state lives in what
	// executions return, which the Runtime mirrors.
	if result, err := w.control.InvokeJS("lockdown"); err != nil {
		return fatal("locking the context down: " + err.Error())
	} else {
		result.Free()
	}
	return workerResponse{}
}

// start runs the game's start(participants, context), which must return
// {state}, from the PRNG state the seed gives.
func (w *worker) start(req workerRequest) workerResponse {
	if w.runtime == nil {
		return fatal("context not loaded")
	}
	if w.started {
		return fatal("game already started")
	}
	if err := w.setRandomState(randomStateFromSeed(req.RandomSeed)); err != nil {
		return fatal(err.Error())
	}
	if err := w.setTime(0); err != nil {
		return fatal(err.Error())
	}

	participantsJSON, err := json.Marshal(req.Participants)
	if err != nil {
		return fatal("encoding participants: " + err.Error())
	}
	result, failure := w.invoke("start", string(participantsJSON), executionContextJSON(0, 0))
	if failure != nil {
		w.broken = true
		return *failure
	}
	state, failure := stateOf("start", result)
	if failure != nil {
		w.broken = true
		return *failure
	}

	randomState, err := w.randomState()
	if err != nil {
		return fatal(err.Error())
	}
	w.state = string(state)
	w.started = true
	w.revision++
	return workerResponse{State: state, PRNGState: encodeRandomState(randomState), ContextRevision: w.revision}
}

// samples runs the game's samples(state, transitions, context), which must
// return {state}, bringing the game to the batch's Session time.
func (w *worker) samples(req workerRequest) workerResponse {
	if !w.started {
		return fatal("game not started")
	}
	if req.SessionTimeMs < w.sessionTimeMs {
		return fatal("session time cannot go back")
	}
	if err := w.setTime(req.SessionTimeMs); err != nil {
		return fatal(err.Error())
	}
	transitionsJSON, err := json.Marshal(req.Transitions)
	if err != nil {
		return fatal("encoding transitions: " + err.Error())
	}
	result, failure := w.invoke("samples", w.state, string(transitionsJSON), executionContextJSON(req.SessionTimeMs, req.SessionTimeMs-w.sessionTimeMs))
	if failure != nil {
		w.broken = true
		return *failure
	}
	state, failure := stateOf("samples", result)
	if failure != nil {
		w.broken = true
		return *failure
	}
	randomState, err := w.randomState()
	if err != nil {
		return fatal(err.Error())
	}
	w.state = string(state)
	w.sessionTimeMs = req.SessionTimeMs
	w.revision++
	return workerResponse{State: state, PRNGState: encodeRandomState(randomState), ContextRevision: w.revision}
}

// stateOf reads the state a mutating entrypoint returned as {state}.
func stateOf(entrypoint string, result string) (json.RawMessage, *workerResponse) {
	var shape struct {
		State json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal([]byte(result), &shape); err != nil || len(shape.State) == 0 || string(shape.State) == "null" {
		failure := scriptFailure(entrypoint + "() must return an object with a state")
		return nil, &failure
	}
	if len(shape.State) > defaultMaxOutputBytes {
		failure := scriptFailure(fmt.Sprintf("%s() returned a state of %d bytes, exceeding the %d-byte limit", entrypoint, len(shape.State), defaultMaxOutputBytes))
		return nil, &failure
	}
	return shape.State, nil
}

// restore sets the context at a running game's confirmed state, PRNG state
// and revision, without running start().
func (w *worker) restore(req workerRequest) workerResponse {
	if w.runtime == nil {
		return fatal("context not loaded")
	}
	if w.started {
		return fatal("game already started")
	}
	if !json.Valid(req.State) {
		return fatal("restored state is not JSON")
	}
	randomState, err := decodeRandomState(req.PRNGState)
	if err != nil {
		return fatal(err.Error())
	}
	if err := w.setRandomState(randomState); err != nil {
		return fatal(err.Error())
	}
	if err := w.setTime(req.SessionTimeMs); err != nil {
		return fatal(err.Error())
	}
	w.state = string(req.State)
	w.started = true
	w.revision = req.ContextRevision
	w.sessionTimeMs = req.SessionTimeMs
	return workerResponse{ContextRevision: w.revision}
}

// project runs the game's project(state, participant, context) on the frozen
// state. It has no randomness: Math.random throws while it runs.
func (w *worker) project(req workerRequest) workerResponse {
	if !w.started {
		return fatal("game not started")
	}
	participantJSON, err := json.Marshal(req.Participant)
	if err != nil {
		return fatal("encoding participant: " + err.Error())
	}
	if err := w.allowRandom(false); err != nil {
		return fatal(err.Error())
	}
	view, failure := w.invoke("project", w.state, string(participantJSON), executionContextJSON(w.sessionTimeMs, 0))
	if err := w.allowRandom(true); err != nil {
		return fatal(err.Error())
	}
	if failure != nil {
		return *failure
	}
	if len(view) > defaultMaxOutputBytes {
		return scriptFailure(fmt.Sprintf("project() returned %d bytes, exceeding the %d-byte limit", len(view), defaultMaxOutputBytes))
	}
	return workerResponse{View: json.RawMessage(view)}
}

// invoke calls the script's global function name with JSON arguments, each
// deep-frozen so authored code cannot change them, and returns its result as
// JSON, or the response the failure answers with.
func (w *worker) invoke(name string, argumentsJSON ...string) (string, *workerResponse) {
	ctx := w.runtime.Context()
	global := ctx.Global()
	if !global.GetPropertyStr(name).IsFunction() {
		failure := scriptFailure(fmt.Sprintf("script does not define a global %s function", name))
		return "", &failure
	}
	arguments := make([]*qjs.Value, len(argumentsJSON))
	for i, argument := range argumentsJSON {
		arguments[i] = ctx.ParseJSON(argument)
		defer arguments[i].Free()
		frozen, err := w.control.InvokeJS("freezeValue", arguments[i])
		if err != nil {
			failure := fatal("freezing an argument: " + err.Error())
			return "", &failure
		}
		frozen.Free()
	}
	result, err := global.InvokeJS(name, arguments...)
	if err != nil {
		failure := scriptFailure(fmt.Sprintf("%s() threw: %s", name, err.Error()))
		return "", &failure
	}
	defer result.Free()
	encoded, err := result.JSONStringify()
	if err != nil {
		failure := scriptFailure(fmt.Sprintf("%s() returned a value that cannot be serialized: %s", name, err.Error()))
		return "", &failure
	}
	if encoded == "" || encoded == "undefined" {
		failure := scriptFailure(fmt.Sprintf("%s() did not return a value", name))
		return "", &failure
	}
	return encoded, nil
}

func (w *worker) setTime(ms int64) error {
	value := w.runtime.Context().NewInt64(ms)
	defer value.Free()
	result, err := w.control.InvokeJS("setTime", value)
	if err != nil {
		return fmt.Errorf("setting logical time: %w", err)
	}
	result.Free()
	return nil
}

func (w *worker) allowRandom(allowed bool) error {
	value := w.runtime.Context().NewBool(allowed)
	defer value.Free()
	result, err := w.control.InvokeJS("allowRandom", value)
	if err != nil {
		return fmt.Errorf("switching randomness: %w", err)
	}
	result.Free()
	return nil
}

func (w *worker) randomState() (uint32, error) {
	result, err := w.control.InvokeJS("getRandomState")
	if err != nil {
		return 0, fmt.Errorf("reading random state: %w", err)
	}
	defer result.Free()
	return uint32(result.Int32()), nil
}

func (w *worker) setRandomState(state uint32) error {
	value := w.runtime.Context().NewInt32(int32(state))
	defer value.Free()
	result, err := w.control.InvokeJS("setRandomState", value)
	if err != nil {
		return fmt.Errorf("setting random state: %w", err)
	}
	result.Free()
	return nil
}

// executionContextJSON is the context argument every entrypoint receives:
// the effective Session time, which starts at zero when the game starts, in
// milliseconds, and how much of it passed since the previous execution.
func executionContextJSON(sessionTimeMs int64, elapsedMs int64) string {
	return fmt.Sprintf(`{"sessionTime":%d,"elapsed":%d}`, sessionTimeMs, elapsedMs)
}

func scriptFailure(reason string) workerResponse {
	return workerResponse{ScriptError: &reason}
}

func fatal(reason string) workerResponse {
	return workerResponse{Fatal: &reason}
}
