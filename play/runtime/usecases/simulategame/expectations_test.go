package simulategame

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
)

func at(ms int64) *int64 { return &ms }

func TestSimulateHoldsAndExpectations(t *testing.T) {
	service := newService(t)

	t.Run("a_hold_sends_its_inputs_and_releases_them", func(t *testing.T) {
		result := simulateLog(t, service, runtime.Scenario{
			Players: 1, DurationMs: 900,
			Holds: []runtime.ScenarioHold{{FromMs: 200, ToMs: 600, Player: "P1", Inputs: json.RawMessage(`{"x": 5}`)}},
			Expect: []runtime.ScenarioExpectation{
				{AtMs: at(300), Of: "global", Path: "pos", Op: "eq", Value: json.RawMessage(`5`)},
				{AtMs: at(700), Of: "P1", Path: "pos", Op: "eq", Value: json.RawMessage(`5`)},
				{Of: "global", Path: "log", Op: "exists"},
			},
		})

		require.Equal(t, []string{"sample:P1@200", "sample:P1@600"}, samplesOf(stateLog(t, result.Final.State)))
		require.NotNil(t, result.Passed)
		require.False(t, *result.Passed)
		require.Len(t, result.Expectations, 3)
		require.True(t, result.Expectations[0].Passed)
		require.Equal(t, int64(300), result.Expectations[0].ObservedAtMs)
		require.False(t, result.Expectations[1].Passed, "released at 600")
		require.JSONEq(t, `0`, string(result.Expectations[1].Actual))
		require.True(t, result.Expectations[2].Passed)
		require.True(t, result.Expectations[2].Final)
	})

	t.Run("an_instant_after_the_game_finished_is_checked_on_the_final_state", func(t *testing.T) {
		result := simulateLog(t, service, runtime.Scenario{
			Players: 1, DurationMs: 900,
			Steps:  []runtime.ScenarioStep{{AtMs: 100, Player: "P1", Command: &runtime.ScenarioCommand{Name: "end"}}},
			Expect: []runtime.ScenarioExpectation{{AtMs: at(800), Of: "global", Path: "pos", Op: "eq", Value: json.RawMessage(`0`)}},
		})
		require.True(t, *result.Passed)
		require.True(t, result.Expectations[0].Final)
	})

	t.Run("an_instant_never_reached_fails_as_not_observed", func(t *testing.T) {
		result := simulateLog(t, service, runtime.Scenario{
			Players: 1, DurationMs: 900,
			Steps:  []runtime.ScenarioStep{{AtMs: 100, Player: "P1", Command: &runtime.ScenarioCommand{Name: "boom"}}},
			Expect: []runtime.ScenarioExpectation{{AtMs: at(500), Of: "global", Path: "pos", Op: "exists"}},
		})
		require.NotNil(t, result.ScriptError)
		require.False(t, *result.Passed)
		require.True(t, result.Expectations[0].NotObserved)
	})

	t.Run("holds_and_expectations_are_checked_against_the_game", func(t *testing.T) {
		result, err := service.Simulate(context.Background(), "v-1", []byte(logScript), string(scriptcheck.ProfileHandwritten), logManifest, runtime.Scenario{
			Players: 1, DurationMs: 900,
			Holds: []runtime.ScenarioHold{
				{FromMs: 100, ToMs: 500, Player: "P1", Inputs: json.RawMessage(`{"x": 1}`)},
				{FromMs: 400, ToMs: 950, Player: "P2", Inputs: json.RawMessage(`{"x": 1}`)},
				{FromMs: 300, ToMs: 600, Player: "P1", Inputs: json.RawMessage(`{"x": 2}`)},
			},
			Expect: []runtime.ScenarioExpectation{{AtMs: at(1000), Of: "P3", Path: "pos", Op: "exists"}},
		})
		require.NoError(t, err)
		require.Equal(t, runtime.SimulationInvalid, result.Outcome)
		fields := make([]string, len(result.Diagnostics))
		for i, diagnostic := range result.Diagnostics {
			fields[i] = diagnostic.Field
		}
		require.Equal(t, []string{"holds[1].player", "holds[1]", "holds[2]", "expect[0].of", "expect[0].at_ms"}, fields)
		require.Empty(t, result.ID, "nothing is kept for a scenario that did not run")
	})
}

func TestKeptResults(t *testing.T) {
	service := newService(t)
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	service.results.(*fakeStore).now = func() time.Time { return now }
	result := simulateLog(t, service, runtime.Scenario{
		Players: 1, DurationMs: 900, ObserveAtMs: []int64{300},
		Steps: []runtime.ScenarioStep{{AtMs: 200, Player: "P1", Inputs: json.RawMessage(`{"x": 4}`)}},
	})
	ctx := context.Background()

	kept, err := service.Result(ctx, result.ID, "v-1")
	require.NoError(t, err)
	require.Equal(t, result.ID, kept.ID)
	require.True(t, now.Add(ResultTTL).Equal(kept.ExpiresAt))
	_, err = service.Result(ctx, result.ID, "another-version")
	require.ErrorIs(t, err, runtime.ErrSimulationResultNotFound)

	read, err := service.ReadState(ctx, result.ID, "v-1", at(300), "global", "pos")
	require.NoError(t, err)
	require.Equal(t, runtime.StateReadFound, read.Outcome)
	require.JSONEq(t, `4`, string(read.Value))
	view, err := service.ReadState(ctx, result.ID, "v-1", nil, "P1", "")
	require.NoError(t, err)
	require.True(t, view.Final)
	require.Contains(t, string(view.Value), `"me"`)
	missing, err := service.ReadState(ctx, result.ID, "v-1", at(300), "global", "nothing.here")
	require.NoError(t, err)
	require.Equal(t, runtime.StateReadPathNotFound, missing.Outcome)
	notObserved, err := service.ReadState(ctx, result.ID, "v-1", at(500), "global", "pos")
	require.NoError(t, err)
	require.Equal(t, runtime.StateReadNotObserved, notObserved.Outcome)
	require.Equal(t, []int64{300}, notObserved.Observed)
	invalid, err := service.ReadState(ctx, result.ID, "v-1", nil, "P9", "")
	require.NoError(t, err)
	require.Equal(t, runtime.StateReadInvalid, invalid.Outcome)

	now = now.Add(ResultTTL)
	_, err = service.Result(ctx, result.ID, "v-1")
	require.ErrorIs(t, err, runtime.ErrSimulationResultNotFound, "expired")
}

func TestKeepingFailsTheSimulation(t *testing.T) {
	host := sandbox.NewHost(sandbox.HostLimits{})
	t.Cleanup(func() { _ = host.Close() })
	store := newFakeStore()
	store.fail = errors.New("database down")
	service := New(host, store)

	_, err := service.Simulate(context.Background(), "v-1", []byte(logScript), string(scriptcheck.ProfileHandwritten), logManifest, runtime.Scenario{Players: 1, DurationMs: 200})

	require.Error(t, err, "a result that cannot be kept is not answered without its id")
}

func TestAStoredResultReadsBackTheSame(t *testing.T) {
	service := newService(t)
	result := simulateLog(t, service, runtime.Scenario{
		Players: 1, DurationMs: 300,
		Steps:  []runtime.ScenarioStep{{AtMs: 100, Player: "P1", Command: &runtime.ScenarioCommand{Name: "boom"}}},
		Expect: []runtime.ScenarioExpectation{{Of: "global", Path: "pos", Op: "exists"}},
	})

	kept, err := service.Result(context.Background(), result.ID, "v-1")

	require.NoError(t, err)
	require.Equal(t, result.ID, kept.ID)
	require.Equal(t, result.ScriptError, kept.ScriptError)
	require.Equal(t, result.Expectations[0].Passed, kept.Expectations[0].Passed)
	require.JSONEq(t, string(result.Final.State), string(kept.Final.State))
}

func TestReadStateTooLarge(t *testing.T) {
	big := make(map[string]string, 3)
	for _, key := range []string{"b", "a", "c"} {
		big[key] = string(make([]byte, MaxStateReadBytes/2))
	}
	state, err := json.Marshal(big)
	require.NoError(t, err)
	result := runtime.SimulationResult{Players: []string{"P1"}, Final: runtime.SimulationObservation{State: state}}

	read := readState(result, nil, "global", "")

	require.True(t, read.TooLarge)
	require.Nil(t, read.Value)
	require.Equal(t, []string{"a", "b", "c"}, read.Keys)
	require.Greater(t, read.Size, MaxStateReadBytes)
}

// samplesOf is the sample entries of a log.
func samplesOf(log []string) []string {
	var samples []string
	for _, entry := range log {
		if len(entry) > 7 && entry[:7] == "sample:" {
			samples = append(samples, entry)
		}
	}
	return samples
}
