package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

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
	host := NewHost()
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
		"no_host_capability_is_reachable": func(t *testing.T) test {
			return test{
				script: `function start() {
					let readHostFile = false;
					try { const f = std.open("../go.mod", "r"); if (f !== null) { readHostFile = true; f.close(); } } catch (e) {}
					const env = (typeof std !== "undefined" && typeof std.getenv === "function") ? std.getenv("PLAYHOOT_TEST_SECRET") : undefined;
					return { state: {
						ambientGlobals: typeof require !== "undefined" || typeof process !== "undefined" || typeof fetch !== "undefined",
						readHostFile: readHostFile,
						performance: typeof performance !== "undefined",
						os: typeof os !== "undefined",
						env: env === undefined ? null : env
					} };
				}`,
				expected: StartResult{
					Outcome:         ScriptSucceeded,
					State:           []byte(`{"ambientGlobals":false,"readHostFile":false,"performance":false,"os":false,"env":null}`),
					PRNGState:       seedState,
					ContextRevision: 1,
				},
			}
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

func TestProjectIsPure(t *testing.T) {
	host := newTestHost(t)
	contextID := startedContext(t, host, `
		function start() { return { state: { round: 1 } }; }
		function project(state, participant) { state.round = 99; return { round: state.round, roll: Math.random() }; }
	`)
	ctx := context.Background()

	first, err := host.Project(ctx, contextID, "P1")
	require.NoError(t, err)
	second, err := host.Project(ctx, contextID, "P1")
	require.NoError(t, err)

	require.Equal(t, ScriptSucceeded, first.Outcome, first.ScriptError)
	require.Equal(t, first, second, "a projection must neither change the state nor consume the game's randomness")
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
	pattern := filepath.Join(os.TempDir(), "playhoot-jsexecutor-*")
	before, err := filepath.Glob(pattern)
	require.NoError(t, err)
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
	after, err := filepath.Glob(pattern)
	require.NoError(t, err)
	require.Len(t, after, len(before), "a discarded or killed context must not leave its scratch directory behind")
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
		function project(state, participant) { return { round: state.round, roll: Math.random() }; }
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
		require.Equal(t, want, got, "the same state and PRNG state must project the same view")
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
