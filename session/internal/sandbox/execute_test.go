package sandbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testCrashWorkerArg is a second re-exec sentinel recognized only by this
// test binary's TestMain, used solely to prove worker-crash isolation
// (Execute must surface a clean *WorkerExecutionError, never crash its own
// process, when the spawned worker process itself misbehaves). It has no
// effect on the production RunAsWorkerIfRequested path.
const testCrashWorkerArg = "__playhoot_jsexecutor_test_crash_worker__"

func TestMain(m *testing.M) {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case workerModeArg:
			runWorker(os.Stdin, os.Stdout)
			os.Exit(0)
		case testCrashWorkerArg:
			os.Exit(137)
		}
	}
	os.Exit(m.Run())
}

func testExecutionContext() ExecutionContext {
	return ExecutionContext{
		LogicalTime: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
		RandomSeed:  424242,
		ActingActor: "actor-1",
	}
}

const counterScript = `
function execute(previousState, event, context) {
	const counter = (previousState && previousState.counter) || 0;
	return {
		newState: { counter: counter + event.amount, actingActor: context.actingActor, randomSeed: context.randomSeed },
		requestedCommands: [{ type: "noop", loggedAt: context.logicalTime }]
	};
}
`

func TestExecute_DeterministicRoundTrip(t *testing.T) {
	in := ExecutionInput{
		Script:        ResolvedScript{Source: counterScript},
		PreviousState: json.RawMessage(`{"counter": 5}`),
		Event:         json.RawMessage(`{"amount": 3}`),
		Context:       testExecutionContext(),
	}

	out1, err := Execute(context.Background(), in)
	require.NoError(t, err)

	out2, err := Execute(context.Background(), in)
	require.NoError(t, err)

	require.JSONEq(t, string(out1.NewState), string(out2.NewState))
	require.Equal(t, len(out1.RequestedCommands), len(out2.RequestedCommands))
	for i := range out1.RequestedCommands {
		require.JSONEq(t, string(out1.RequestedCommands[i]), string(out2.RequestedCommands[i]))
	}

	var state struct {
		Counter     int    `json:"counter"`
		ActingActor string `json:"actingActor"`
		RandomSeed  string `json:"randomSeed"`
	}
	require.NoError(t, json.Unmarshal(out1.NewState, &state))
	require.Equal(t, 8, state.Counter)
	require.Equal(t, "actor-1", state.ActingActor)
	require.Equal(t, "424242", state.RandomSeed)
}

func TestExecute_ScriptThrows_IsRejectedNotCrashed(t *testing.T) {
	in := ExecutionInput{
		Script:        ResolvedScript{Source: `function execute() { throw new Error("boom"); }`},
		PreviousState: json.RawMessage(`{}`),
		Event:         json.RawMessage(`{}`),
		Context:       testExecutionContext(),
	}

	_, err := Execute(context.Background(), in)
	require.Error(t, err)

	var rejected *ScriptRejectedError
	require.ErrorAs(t, err, &rejected)
}

func TestExecute_MalformedReturnShape_IsRejected(t *testing.T) {
	cases := map[string]string{
		"not_an_object":      `function execute() { return 42; }`,
		"missing_new_state":  `function execute() { return { requestedCommands: [] }; }`,
		"no_execute_defined": `const notExecute = function() { return {}; };`,
	}

	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			in := ExecutionInput{
				Script:        ResolvedScript{Source: script},
				PreviousState: json.RawMessage(`{}`),
				Event:         json.RawMessage(`{}`),
				Context:       testExecutionContext(),
			}

			_, err := Execute(context.Background(), in)
			require.Error(t, err)

			var rejected *ScriptRejectedError
			require.ErrorAsf(t, err, &rejected, "case %s: got %v", name, err)
		})
	}
}

// capabilityProbeScript checks two different kinds of isolation: globals
// that bare QuickJS simply never defines (a vacuous check on its own — it
// would pass even with no sandboxing at all), and an actual attempted
// filesystem escape through QuickJS's own std module, which IS reachable
// unless the sandbox's mounted root is confined to an empty scratch
// directory. "go.mod" only exists at the real repository root, never inside
// a fresh per-invocation scratch directory, so successfully reading it would
// prove a real isolation failure, not merely the absence of unrelated
// Node/browser globals.
const capabilityProbeScript = `
function execute(previousState, event, context) {
	const noAmbientGlobals =
		typeof require === "undefined" &&
		typeof process === "undefined" &&
		typeof fetch === "undefined" &&
		typeof globalThis.Deno === "undefined";

	let escapedReadHostFile = false;
	try {
		const f = std.open("../go.mod", "r");
		if (f !== null) {
			escapedReadHostFile = true;
			f.close();
		}
	} catch (e) {
		escapedReadHostFile = false;
	}

	const isolated = noAmbientGlobals && !escapedReadHostFile;
	return { newState: { isolated: isolated, escapedReadHostFile: escapedReadHostFile }, requestedCommands: [] };
}
`

func TestExecute_NoHostCapabilitiesReachable(t *testing.T) {
	in := ExecutionInput{
		Script:        ResolvedScript{Source: capabilityProbeScript},
		PreviousState: json.RawMessage(`{}`),
		Event:         json.RawMessage(`{}`),
		Context:       testExecutionContext(),
	}

	out, err := Execute(context.Background(), in)
	require.NoError(t, err)

	var state struct {
		Isolated            bool `json:"isolated"`
		EscapedReadHostFile bool `json:"escapedReadHostFile"`
	}
	require.NoError(t, json.Unmarshal(out.NewState, &state))
	require.False(t, state.EscapedReadHostFile, "script must not be able to read a real file outside its sandbox scratch directory")
	require.True(t, state.Isolated, "script should not observe require/process/fetch/Deno globals or escape its sandbox")
}

const ambientNondeterminismScript = `
function execute(previousState, event, context) {
	return { newState: { dateNow: Date.now(), newDate: new Date().getTime(), mathRandom: Math.random() }, requestedCommands: [] };
}
`

func TestExecute_AmbientDateAndMathRandomAreNeutralized(t *testing.T) {
	execCtx := testExecutionContext()
	in := ExecutionInput{
		Script:        ResolvedScript{Source: ambientNondeterminismScript},
		PreviousState: json.RawMessage(`{}`),
		Event:         json.RawMessage(`{}`),
		Context:       execCtx,
	}

	out1, err := Execute(context.Background(), in)
	require.NoError(t, err)
	out2, err := Execute(context.Background(), in)
	require.NoError(t, err)

	var state1, state2 struct {
		DateNow    int64   `json:"dateNow"`
		NewDate    int64   `json:"newDate"`
		MathRandom float64 `json:"mathRandom"`
	}
	require.NoError(t, json.Unmarshal(out1.NewState, &state1))
	require.NoError(t, json.Unmarshal(out2.NewState, &state2))

	require.Equal(t, execCtx.LogicalTime.UnixMilli(), state1.DateNow, "Date.now() must reflect the supplied logical time, not the real host clock")
	require.Equal(t, state1.DateNow, state1.NewDate, "new Date().getTime() must agree with Date.now()")
	require.Equal(t, state1, state2, "identical context must produce byte-identical Date/Math.random output across separate executions")
}

func TestExecute_WorkerProcessCrash_IsCleanExecutionError(t *testing.T) {
	original := currentWorkerArg
	currentWorkerArg = testCrashWorkerArg
	t.Cleanup(func() { currentWorkerArg = original })

	in := ExecutionInput{
		Script:        ResolvedScript{Source: counterScript},
		PreviousState: json.RawMessage(`{}`),
		Event:         json.RawMessage(`{"amount": 1}`),
		Context:       testExecutionContext(),
	}

	_, err := Execute(context.Background(), in)
	require.Error(t, err)

	var workerErr *WorkerExecutionError
	require.ErrorAs(t, err, &workerErr)
}

const ambientCapabilityLeakScript = `
function execute(previousState, event, context) {
	const envValue = (typeof std !== "undefined" && typeof std.getenv === "function")
		? std.getenv("PLAYHOOT_TEST_SECRET")
		: undefined;
	return {
		newState: {
			performanceReachable: typeof performance !== "undefined",
			osReachable: typeof os !== "undefined",
			envValue: envValue === undefined ? null : envValue
		},
		requestedCommands: []
	};
}
`

func TestExecute_NoRealTimingOrEnvironmentLeak(t *testing.T) {
	require.NoError(t, os.Setenv("PLAYHOOT_TEST_SECRET", "should-not-leak"))
	t.Cleanup(func() { _ = os.Unsetenv("PLAYHOOT_TEST_SECRET") })

	in := ExecutionInput{
		Script:        ResolvedScript{Source: ambientCapabilityLeakScript},
		PreviousState: json.RawMessage(`{}`),
		Event:         json.RawMessage(`{}`),
		Context:       testExecutionContext(),
	}

	out, err := Execute(context.Background(), in)
	require.NoError(t, err)

	var state struct {
		PerformanceReachable bool    `json:"performanceReachable"`
		OSReachable          bool    `json:"osReachable"`
		EnvValue             *string `json:"envValue"`
	}
	require.NoError(t, json.Unmarshal(out.NewState, &state))
	require.False(t, state.PerformanceReachable, "performance.now() would leak real elapsed time")
	require.False(t, state.OSReachable, "the os module would leak real timing and other host capability")
	if state.EnvValue != nil {
		require.NotEqual(t, "should-not-leak", *state.EnvValue, "the worker process must not inherit real host environment variables")
	}
}

func TestExecute_ScratchDirectoryNotLeakedAfterKill(t *testing.T) {
	pattern := filepath.Join(os.TempDir(), "playhoot-jsexecutor-*")
	before, err := filepath.Glob(pattern)
	require.NoError(t, err)

	in := ExecutionInput{
		Script:        ResolvedScript{Source: `function execute() { while (true) {} }`},
		PreviousState: json.RawMessage(`{}`),
		Event:         json.RawMessage(`{}`),
		Context:       testExecutionContext(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err = Execute(ctx, in)
	require.Error(t, err)

	after, err := filepath.Glob(pattern)
	require.NoError(t, err)
	require.Len(t, after, len(before), "killing a runaway worker must not leave its scratch directory behind on disk")
}

func TestExecute_CallerDeadline_KillsRunawayWorker(t *testing.T) {
	in := ExecutionInput{
		Script:        ResolvedScript{Source: `function execute() { while (true) {} }`},
		PreviousState: json.RawMessage(`{}`),
		Event:         json.RawMessage(`{}`),
		Context:       testExecutionContext(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := Execute(ctx, in)
	elapsed := time.Since(start)

	require.Error(t, err)
	var workerErr *WorkerExecutionError
	require.ErrorAs(t, err, &workerErr)
	require.Less(t, elapsed, 5*time.Second, "a caller deadline must actually kill a tight loop that never yields, not merely time out the read while the worker keeps running")
}

// TestExecute_OversizedOutput_IsRejected proves the output-size cap: a
// script that returns a combined NewState/RequestedCommands shape larger
// than defaultMaxOutputBytes is rejected as a business-level outcome, not
// silently accepted or truncated.
func TestExecute_OversizedOutput_IsRejected(t *testing.T) {
	in := ExecutionInput{
		Script: ResolvedScript{Source: `
			function execute(previousState, event, context) {
				return { newState: { big: "x".repeat(300000) }, requestedCommands: [] };
			}
		`},
		PreviousState: json.RawMessage(`{}`),
		Event:         json.RawMessage(`{}`),
		Context:       testExecutionContext(),
	}

	_, err := Execute(context.Background(), in)
	require.Error(t, err)

	var rejected *ScriptRejectedError
	require.ErrorAs(t, err, &rejected)
	require.Contains(t, rejected.Reason, "byte limit")
}

// TestExecute_ExcessiveCommandCount_IsRejected proves the command-count
// cap: a script that returns more than defaultMaxCommandCount requested
// commands is rejected as a business-level outcome.
func TestExecute_ExcessiveCommandCount_IsRejected(t *testing.T) {
	in := ExecutionInput{
		Script: ResolvedScript{Source: `
			function execute(previousState, event, context) {
				const commands = [];
				for (let i = 0; i < ` + strconv.Itoa(defaultMaxCommandCount+1) + `; i++) {
					commands.push({ type: "noop" });
				}
				return { newState: {}, requestedCommands: commands };
			}
		`},
		PreviousState: json.RawMessage(`{}`),
		Event:         json.RawMessage(`{}`),
		Context:       testExecutionContext(),
	}

	_, err := Execute(context.Background(), in)
	require.Error(t, err)

	var rejected *ScriptRejectedError
	require.ErrorAs(t, err, &rejected)
	require.Contains(t, rejected.Reason, "command limit")
}

// TestExecute_MemoryExhaustion_IsRejectedNotCrashed proves
// LOGICAL_CONTRACT.md's previously-untested claim: a script that allocates
// past qjs.Option.MemoryLimit is stopped and surfaced as a clean error, not
// a worker process crash and not a hang past the caller's deadline.
func TestExecute_MemoryExhaustion_IsRejectedNotCrashed(t *testing.T) {
	in := ExecutionInput{
		// Exponential string doubling blows past defaultMemoryLimitBytes
		// (64 MiB) within roughly 30 iterations - fast enough that this
		// proves the memory cap itself, not the execution-time cap.
		Script: ResolvedScript{Source: `
			function execute(previousState, event, context) {
				let s = "x";
				while (true) { s = s + s; }
			}
		`},
		PreviousState: json.RawMessage(`{}`),
		Event:         json.RawMessage(`{}`),
		Context:       testExecutionContext(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	start := time.Now()
	_, err := Execute(ctx, in)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Less(t, elapsed, 4*time.Second, "a memory-exhausting script must be stopped by its own memory cap, well before the execution-time/caller deadline")

	// Confirmed (2026-09-29): qjs's own memory limit throws a catchable JS
	// exception ("InternalError: out of memory"), which the existing
	// "execute() threw" path already surfaces as a clean, business-level
	// ScriptRejectedError - no worker crash.
	var rejected *ScriptRejectedError
	require.ErrorAsf(t, err, &rejected, "got %T: %v", err, err)
}

// TestExecute_StackOverflow_IsRejectedNotCrashed proves
// LOGICAL_CONTRACT.md's previously-untested claim: unbounded recursion past
// qjs.Option.MaxStackSize is stopped and surfaced as a clean error, not a
// worker process crash.
func TestExecute_StackOverflow_IsRejectedNotCrashed(t *testing.T) {
	in := ExecutionInput{
		Script: ResolvedScript{Source: `
			function recurse(n) { return recurse(n + 1); }
			function execute(previousState, event, context) {
				recurse(0);
			}
		`},
		PreviousState: json.RawMessage(`{}`),
		Event:         json.RawMessage(`{}`),
		Context:       testExecutionContext(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	start := time.Now()
	_, err := Execute(ctx, in)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Less(t, elapsed, 4*time.Second, "unbounded recursion must be stopped by its own stack cap, well before the execution-time/caller deadline")

	// Confirmed (2026-09-29), unlike memory exhaustion: unbounded recursion
	// does not throw a catchable JS exception - it is an infrastructure-level
	// failure inside the QuickJS-on-wazero binding itself (observed as a wasm
	// trap surfacing through the WASM runtime's own cleanup, recovered by
	// this package's top-level panic guard, or - if that guard's own
	// recovery cannot run - a hard worker-process crash the OS-process
	// boundary still safely contains). Either path is a clean
	// WorkerExecutionError to the caller, never a hang and never a crash of
	// the caller's own process - but callers must not expect the same
	// graceful ScriptRejectedError memory exhaustion gets.
	var workerErr *WorkerExecutionError
	require.ErrorAsf(t, err, &workerErr, "got %T: %v", err, err)
}

func TestExecute_ConcurrentSessionsDoNotInterfere(t *testing.T) {
	const n = 8
	errs := make(chan error, n)
	states := make(chan string, n)

	for i := 0; i < n; i++ {
		amount := i
		go func() {
			in := ExecutionInput{
				Script:        ResolvedScript{Source: counterScript},
				PreviousState: json.RawMessage(`{"counter": 100}`),
				Event:         json.RawMessage(`{"amount": ` + strconv.Itoa(amount) + `}`),
				Context:       testExecutionContext(),
			}
			out, err := Execute(context.Background(), in)
			if err != nil {
				errs <- err
				states <- ""
				return
			}
			errs <- nil
			states <- string(out.NewState)
		}()
	}

	seenCounters := make(map[int]bool)
	for i := 0; i < n; i++ {
		err := <-errs
		require.NoError(t, err)
		raw := <-states
		var state struct {
			Counter int `json:"counter"`
		}
		require.NoError(t, json.Unmarshal([]byte(raw), &state))
		require.False(t, seenCounters[state.Counter], "each concurrent Session's own amount should produce its own distinct counter, not a mixed-up result")
		seenCounters[state.Counter] = true
	}
	require.Len(t, seenCounters, n)
}
