package sandbox

import (
	"context"
	"io"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/fastschema/qjs"
	"github.com/stretchr/testify/require"
)

// testCrashWorkerArg starts a worker that dies at once, recognized only by
// this test binary, to prove a crashing worker never takes the host down.
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

// newTestHost returns a Host whose contexts are all stopped when the test ends.
func newTestHost(t *testing.T) *Host {
	t.Helper()
	host := NewHost(HostLimits{})
	t.Cleanup(func() { _ = host.Close() })
	return host
}

// startedContext creates a context for script and starts it with P1 and P2.
func startedContext(t *testing.T, host *Host, script string) string {
	t.Helper()
	ctx := context.Background()
	contextID, err := host.CreateContext(ctx, ContextSpec{GameVersionUUID: "version-1", Script: []byte(script)})
	require.NoError(t, err)
	result, err := host.Start(ctx, contextID, StartInput{Participants: []string{"P1", "P2"}, RandomSeed: 42})
	require.NoError(t, err)
	require.Equal(t, ScriptSucceeded, result.Outcome, result.ScriptError)
	return contextID
}

func TestStart(t *testing.T) {
	type test struct {
		script string
		// budget, when set, replaces the execution budget.
		budget time.Duration

		expected              StartResult
		expectedErrorContains string
	}

	seedState := encodeRandomState(randomStateFromSeed(42))
	tests := map[string]func(t *testing.T) test{
		"state_from_the_participants_at_session_time_zero": func(t *testing.T) test {
			return test{
				script: `function start(participants, context) {
					return { state: { players: participants, sessionTime: context.sessionTime, now: Date.now() } };
				}`,
				expected: StartResult{Outcome: ScriptSucceeded, State: []byte(`{"players":["P1","P2"],"sessionTime":0,"now":0}`), PRNGState: seedState, ContextRevision: 1},
			}
		},
		"script_that_throws": func(t *testing.T) test {
			return test{script: `function start() { throw new Error("boom"); }`, expected: StartResult{Outcome: ScriptFailed}, expectedErrorContains: "start() threw"}
		},
		"result_without_state": func(t *testing.T) test {
			return test{script: `function start() { return { players: [] }; }`, expected: StartResult{Outcome: ScriptFailed}, expectedErrorContains: "must return an object with a state"}
		},
		"result_that_is_not_an_object": func(t *testing.T) test {
			return test{script: `function start() { return 42; }`, expected: StartResult{Outcome: ScriptFailed}, expectedErrorContains: "must return an object with a state"}
		},
		"script_without_start": func(t *testing.T) test {
			return test{script: `function project() { return {}; }`, expected: StartResult{Outcome: ScriptFailed}, expectedErrorContains: "does not define a global start function"}
		},
		"script_that_does_not_load": func(t *testing.T) test {
			return test{script: `function start( {`, expected: StartResult{Outcome: ScriptFailed}, expectedErrorContains: "loading script"}
		},
		"oversized_state": func(t *testing.T) test {
			return test{script: `function start() { return { state: { big: "x".repeat(300000) } }; }`, expected: StartResult{Outcome: ScriptFailed}, expectedErrorContains: "byte limit"}
		},
		"memory_exhaustion_is_a_script_failure": func(t *testing.T) test {
			return test{script: `function start() { let s = "x"; while (true) { s = s + s; } }`, expected: StartResult{Outcome: ScriptFailed}, expectedErrorContains: "start() threw"}
		},
		"stack_overflow_is_a_script_failure": func(t *testing.T) test {
			return test{script: `function recurse(n) { return recurse(n + 1); } function start() { recurse(0); }`, expected: StartResult{Outcome: ScriptFailed}}
		},
		"runaway_loop_exceeds_the_execution_budget": func(t *testing.T) test {
			return test{script: `function start() { while (true) {} }`, budget: 300 * time.Millisecond, expected: StartResult{Outcome: ScriptFailed}, expectedErrorContains: "execution budget exceeded"}
		},
		"no_host_helper_is_reachable": func(t *testing.T) test {
			return test{
				script: `function start() {
					const helpers = ["std", "os", "bjson", "print", "console", "scriptArgs", "navigator", "performance",
						"setTimeout", "setInterval", "queueMicrotask", "require", "process", "fetch"];
					return { state: { reachable: helpers.filter((name) => typeof globalThis[name] !== "undefined") } };
				}`,
				expected: StartResult{Outcome: ScriptSucceeded, State: []byte(`{"reachable":[]}`), PRNGState: seedState, ContextRevision: 1},
			}
		},
		"input_is_read_only": func(t *testing.T) test {
			return test{script: `function start(participants) { participants.push("P9"); return { state: {} }; }`, expected: StartResult{Outcome: ScriptFailed}, expectedErrorContains: "start() threw"}
		},
		"global_state_cannot_be_kept": func(t *testing.T) test {
			return test{script: `var starts = 0; function start() { starts++; return { state: { starts: starts } }; }`, expected: StartResult{Outcome: ScriptFailed}, expectedErrorContains: "start() threw"}
		},
		"builtins_cannot_carry_state": func(t *testing.T) test {
			return test{script: `function start() { Math.cache = 1; return { state: {} }; }`, expected: StartResult{Outcome: ScriptFailed}, expectedErrorContains: "start() threw"}
		},
	}

	require.NoError(t, os.Setenv("PLAYHOOT_TEST_SECRET", "should-not-leak"))
	t.Cleanup(func() { _ = os.Unsetenv("PLAYHOOT_TEST_SECRET") })
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			host := newTestHost(t)
			if tc.budget > 0 {
				host.budget = tc.budget
			}
			ctx := context.Background()
			contextID, err := host.CreateContext(ctx, ContextSpec{GameVersionUUID: "version-1", Script: []byte(tc.script)})
			require.NoError(t, err)

			result, err := host.Start(ctx, contextID, StartInput{Participants: []string{"P1", "P2"}, RandomSeed: 42})

			require.NoError(t, err)
			require.Contains(t, result.ScriptError, tc.expectedErrorContains)
			result.ScriptError = ""
			if tc.expected.State != nil {
				require.JSONEq(t, string(tc.expected.State), string(result.State))
				result.State = tc.expected.State
			}
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestStartIsDeterministic(t *testing.T) {
	const script = `function start(participants) {
		return { state: { rolls: [Math.random(), Math.random()], order: participants } };
	}`
	host := newTestHost(t)
	ctx := context.Background()
	run := func(seed uint64) StartResult {
		contextID, err := host.CreateContext(ctx, ContextSpec{Script: []byte(script)})
		require.NoError(t, err)
		result, err := host.Start(ctx, contextID, StartInput{Participants: []string{"P1", "P2"}, RandomSeed: seed})
		require.NoError(t, err)
		require.Equal(t, ScriptSucceeded, result.Outcome, result.ScriptError)
		return result
	}

	first, second, other := run(7), run(7), run(8)

	require.Equal(t, first, second, "the same seed must give the same state and PRNG state")
	require.NotEqual(t, first.State, other.State, "another seed must give other rolls")
	require.NotEqual(t, encodeRandomState(randomStateFromSeed(7)), first.PRNGState, "drawing randomness must advance the exported PRNG state")
}

func TestProject(t *testing.T) {
	type test struct {
		script      string
		participant string

		expected              ProjectResult
		expectedErrorContains string
	}

	const base = `function start(participants) { return { state: { round: 1, scores: { P1: 3, P2: 5 } } }; }`
	tests := map[string]func(t *testing.T) test{
		"view_of_the_participant": func(t *testing.T) test {
			return test{
				script:      base + `function project(state, participant, context) { return { me: participant, score: state.scores[participant], at: context.sessionTime }; }`,
				participant: "P2",
				expected:    ProjectResult{Outcome: ScriptSucceeded, View: []byte(`{"me":"P2","score":5,"at":0}`)},
			}
		},
		"projection_that_throws": func(t *testing.T) test {
			return test{script: base + `function project() { throw new Error("hidden"); }`, participant: "P1", expected: ProjectResult{Outcome: ScriptFailed}, expectedErrorContains: "project() threw"}
		},
		"projection_without_a_value": func(t *testing.T) test {
			return test{script: base + `function project() {}`, participant: "P1", expected: ProjectResult{Outcome: ScriptFailed}, expectedErrorContains: "did not return a value"}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			host := newTestHost(t)
			contextID := startedContext(t, host, tc.script)

			result, err := host.Project(context.Background(), contextID, tc.participant)

			require.NoError(t, err)
			require.Contains(t, result.ScriptError, tc.expectedErrorContains)
			result.ScriptError = ""
			if tc.expected.View != nil {
				require.JSONEq(t, string(tc.expected.View), string(result.View))
				result.View = tc.expected.View
			}
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestProjectCannotChangeTheContext(t *testing.T) {
	type test struct {
		project string

		expectedErrorContains string
	}

	tests := map[string]func(t *testing.T) test{
		"mutating_the_state": func(t *testing.T) test {
			return test{project: `function project(state) { state.round = 99; return {}; }`, expectedErrorContains: "project() threw"}
		},
		"drawing_randomness": func(t *testing.T) test {
			return test{project: `function project() { return { roll: Math.random() }; }`, expectedErrorContains: "Math.random is not available"}
		},
		"writing_a_global": func(t *testing.T) test {
			return test{project: `function project() { globalThis.seen = true; return {}; }`, expectedErrorContains: "project() threw"}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			host := newTestHost(t)
			contextID := startedContext(t, host, `function start() { return { state: { round: 1, rolls: [Math.random()] } }; }`+tc.project)

			result, err := host.Project(context.Background(), contextID, "P1")

			require.NoError(t, err)
			require.Equal(t, ScriptFailed, result.Outcome)
			require.Contains(t, result.ScriptError, tc.expectedErrorContains)
		})
	}
}

func TestPatchedRuntimeHasNoHostCapabilities(t *testing.T) {
	// Without the determinism prelude and through the qjs:std and qjs:os
	// modules themselves: the guarantee is the runtime's, not the cleanup's.
	run := func() string {
		rt, err := qjs.New(qjs.Option{MemoryLimit: defaultMemoryLimitBytes, MaxStackSize: defaultMaxStackSizeBytes, Stdout: io.Discard, Stderr: io.Discard})
		require.NoError(t, err)
		defer rt.Close()
		_, err = rt.Context().Eval("probe.js", qjs.Code(`
			import * as std from "qjs:std";
			import * as os from "qjs:os";
			let wrote = false;
			try { const f = std.open("/probe.txt", "w"); wrote = f !== null; } catch (e) {}
			let listed = false;
			try { const [entries, err] = os.readdir("/"); listed = err === 0 && entries.length > 0; } catch (e) {}
			globalThis.result = JSON.stringify({ wrote: wrote, listed: listed, env: std.getenv("PATH") ?? null, now: Date.now() });
		`), qjs.TypeModule())
		require.NoError(t, err)
		result := rt.Context().Global().GetPropertyStr("result")
		defer result.Free()
		return result.String()
	}

	first, second := run(), run()

	require.Contains(t, first, `"wrote":false,"listed":false,"env":null`, "no directory is mounted and the environment is empty, whatever this process's is")
	require.Equal(t, first, second, "the clock is not the host's: two runs see the same time")
	require.NotEmpty(t, os.Getenv("PATH"), "the probe only proves something if the host has an environment")
}

func TestHostLimits(t *testing.T) {
	ctx := context.Background()
	const script = `function start() { return { state: {} }; } function project() { return {}; }`

	t.Run("contexts_beyond_the_limit_are_refused", func(t *testing.T) {
		host := NewHost(HostLimits{MaxContexts: 1})
		t.Cleanup(func() { _ = host.Close() })
		first, err := host.CreateContext(ctx, ContextSpec{Script: []byte(script)})
		require.NoError(t, err)

		_, err = host.CreateContext(ctx, ContextSpec{Script: []byte(script)})
		require.ErrorIs(t, err, ErrNoCapacity)

		require.NoError(t, host.DiscardContext(ctx, first))
		_, err = host.CreateContext(ctx, ContextSpec{Script: []byte(script)})
		require.NoError(t, err, "a discarded context frees its capacity")
	})
	t.Run("executions_beyond_the_limit_wait", func(t *testing.T) {
		host := NewHost(HostLimits{MaxConcurrentExecutions: 1})
		t.Cleanup(func() { _ = host.Close() })
		slow, err := host.CreateContext(ctx, ContextSpec{Script: []byte(`function start() { let n = 0; while (n < 3e7) { n++; } return { state: {} }; }`)})
		require.NoError(t, err)
		fast, err := host.CreateContext(ctx, ContextSpec{Script: []byte(script)})
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = host.Start(ctx, slow, StartInput{Participants: []string{"P1"}})
		}()
		time.Sleep(50 * time.Millisecond)
		waiting, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
		defer cancel()
		_, err = host.Start(waiting, fast, StartInput{Participants: []string{"P1"}})
		require.Error(t, err, "with the only slot taken, a call waits for it")
		wg.Wait()
	})
}

func TestHostInfrastructureFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("worker_that_crashes_is_an_error_not_a_host_crash", func(t *testing.T) {
		host := newTestHost(t)
		host.workerArg = testCrashWorkerArg
		_, err := host.CreateContext(ctx, ContextSpec{Script: []byte(`function start() {}`)})
		require.Error(t, err)
	})
	t.Run("abandoned_call_kills_the_worker", func(t *testing.T) {
		host := newTestHost(t)
		contextID, err := host.CreateContext(ctx, ContextSpec{Script: []byte(`function start() { while (true) {} }`)})
		require.NoError(t, err)
		cancelled, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()
		began := time.Now()
		_, err = host.Start(cancelled, contextID, StartInput{Participants: []string{"P1"}, RandomSeed: 1})
		require.Error(t, err)
		require.Less(t, time.Since(began), 3*time.Second, "the worker must be killed, not merely abandoned")
		_, err = host.Project(ctx, contextID, "P1")
		require.Error(t, err, "a killed context is unusable")
	})
	t.Run("unknown_context", func(t *testing.T) {
		host := newTestHost(t)
		_, err := host.Start(ctx, "nope", StartInput{})
		require.ErrorIs(t, err, ErrUnknownContext)
	})
}

func TestDiscardContext(t *testing.T) {
	host := newTestHost(t)
	contextID := startedContext(t, host, `function start() { return { state: {} }; } function project() { return {}; }`)
	runaway, err := host.CreateContext(context.Background(), ContextSpec{Script: []byte(`function start() { while (true) {} }`)})
	require.NoError(t, err)
	host.budget = 300 * time.Millisecond
	_, err = host.Start(context.Background(), runaway, StartInput{Participants: []string{"P1"}})
	require.NoError(t, err)

	require.NoError(t, host.DiscardContext(context.Background(), contextID))
	require.NoError(t, host.DiscardContext(context.Background(), runaway))
	require.NoError(t, host.DiscardContext(context.Background(), contextID), "discarding again does nothing")

	_, err = host.Project(context.Background(), contextID, "P1")
	require.ErrorIs(t, err, ErrUnknownContext)
}

func TestConcurrentContextsDoNotInterfere(t *testing.T) {
	const n = 6
	host := newTestHost(t)
	results := make(chan StartResult, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			ctx := context.Background()
			contextID, err := host.CreateContext(ctx, ContextSpec{Script: []byte(`function start(p) { return { state: { players: p } }; }`)})
			if err != nil {
				errs <- err
				return
			}
			result, err := host.Start(ctx, contextID, StartInput{Participants: []string{"P" + strconv.Itoa(i)}, RandomSeed: uint64(i)})
			errs <- err
			results <- result
		}()
	}

	seen := make(map[string]bool)
	for i := 0; i < n; i++ {
		require.NoError(t, <-errs)
		result := <-results
		require.Equal(t, ScriptSucceeded, result.Outcome, result.ScriptError)
		seen[string(result.State)] = true
	}
	require.Len(t, seen, n, "each context must keep its own participants")
}

func TestRestore(t *testing.T) {
	const script = `
		function start(participants) { return { state: { round: 3, rolls: [Math.random()] } }; }
		function project(state, participant) { return { round: state.round, roll: state.rolls[0], me: participant }; }
	`
	ctx := context.Background()

	t.Run("restored_context_continues_from_the_snapshot", func(t *testing.T) {
		host := newTestHost(t)
		original, err := host.CreateContext(ctx, ContextSpec{Script: []byte(script)})
		require.NoError(t, err)
		started, err := host.Start(ctx, original, StartInput{Participants: []string{"P1", "P2"}, RandomSeed: 42})
		require.NoError(t, err)
		snapshot := Snapshot{State: started.State, PRNGState: started.PRNGState, ContextRevision: started.ContextRevision}
		restored, err := host.CreateContext(ctx, ContextSpec{Script: []byte(script)})
		require.NoError(t, err)

		result, err := host.Restore(ctx, restored, snapshot)

		require.NoError(t, err)
		require.Equal(t, RestoreResult{Outcome: ScriptSucceeded}, result)
		want, err := host.Project(ctx, original, "P1")
		require.NoError(t, err)
		got, err := host.Project(ctx, restored, "P1")
		require.NoError(t, err)
		require.Equal(t, ScriptSucceeded, want.Outcome, want.ScriptError)
		require.Equal(t, want, got, "the same state must project the same view")
	})
	t.Run("script_that_does_not_load", func(t *testing.T) {
		host := newTestHost(t)
		contextID, err := host.CreateContext(ctx, ContextSpec{Script: []byte(`function start( {`)})
		require.NoError(t, err)

		result, err := host.Restore(ctx, contextID, Snapshot{State: []byte(`{}`), PRNGState: encodeRandomState(1), ContextRevision: 1})

		require.NoError(t, err)
		require.Equal(t, ScriptFailed, result.Outcome)
		require.Contains(t, result.ScriptError, "loading script")
	})
	t.Run("malformed_prng_state", func(t *testing.T) {
		host := newTestHost(t)
		contextID, err := host.CreateContext(ctx, ContextSpec{Script: []byte(script)})
		require.NoError(t, err)

		_, err = host.Restore(ctx, contextID, Snapshot{State: []byte(`{}`), PRNGState: []byte{1}, ContextRevision: 1})

		require.Error(t, err)
	})
}
