package simulategame

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestMain(m *testing.M) {
	if sandbox.RunAsWorkerIfRequested() {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// logScript records, in its state, every call the Runtime makes and when.
const logScript = `
function start(participants, context) { return { state: { log: [], pos: 0, roll: Math.random() } }; }
function samples(state, transitions, context) {
	var log = state.log.slice(); var pos = state.pos;
	for (var i = 0; i < transitions.length; i++) { log.push("sample:" + transitions[i].participant + "@" + transitions[i].sessionTime); pos = transitions[i].inputs.x; }
	return { state: { log: log, pos: pos, roll: state.roll } };
}
function command(state, cmd, context) {
	if (cmd.name === "boom") { throw new Error("boom"); }
	var next = { log: state.log.concat(["command:" + cmd.name + "@" + cmd.sessionTime + ":pos=" + state.pos]), pos: state.pos, roll: state.roll };
	var answer = { state: next, response: { seen: state.pos }, events: [{ event: { said: cmd.name } }] };
	if (cmd.name === "end") { answer.finished = true; }
	return answer;
}
function update(state, context) { return { state: { log: state.log.concat(["update@" + context.sessionTime]), pos: state.pos, roll: state.roll } }; }
function playerDisconnected(state, participant, context) { return { state: state }; }
function project(state, participant, context) { return { me: participant, pos: state.pos, calls: state.log.length }; }
`

var logManifest = sessionpkg.RuntimeManifest{
	MinPlayers: 1,
	MaxPlayers: 2,
	Inputs:     []sessionpkg.RuntimeInput{{Name: "x", Type: "number", Default: json.RawMessage(`0`)}},
	Commands:   []sessionpkg.RuntimeCommand{{Name: "go"}, {Name: "end"}, {Name: "boom"}},
	Update:     true,
}

func newService(t *testing.T) *Service {
	t.Helper()
	host := sandbox.NewHost(sandbox.HostLimits{})
	t.Cleanup(func() { _ = host.Close() })
	return New(host)
}

func simulateLog(t *testing.T, service *Service, scenario runtime.Scenario) runtime.SimulationResult {
	t.Helper()
	result, err := service.Simulate(context.Background(), []byte(logScript), string(scriptcheck.ProfileHandwritten), logManifest, scenario)
	require.NoError(t, err)
	return result
}

func stateLog(t *testing.T, state json.RawMessage) []string {
	t.Helper()
	var decoded struct{ Log []string }
	require.NoError(t, json.Unmarshal(state, &decoded))
	return decoded.Log
}

func TestSimulateSchedulesLikeALiveSession(t *testing.T) {
	service := newService(t)

	t.Run("a command runs its own boundary at its arrival, after the samples before it, ending with update", func(t *testing.T) {
		result := simulateLog(t, service, runtime.Scenario{
			Players: 2, Seed: 1, DurationMs: 300,
			Steps: []runtime.ScenarioStep{
				{AtMs: 150, Player: "P1", Command: &runtime.ScenarioCommand{Name: "go"}},
				{AtMs: 120, Player: "P2", Inputs: json.RawMessage(`{"x":5}`)},
			},
			ObserveAtMs: []int64{130, 0},
		})
		require.Equal(t, runtime.SimulationRan, result.Outcome)
		require.Equal(t, []string{"update@100", "sample:P2@120", "command:go@150:pos=5", "update@150", "update@200", "update@300"}, stateLog(t, result.Final.State))
		require.Equal(t, runtime.SimulationStoppedDuration, result.Stopped)
		require.False(t, result.Finished)
		require.Len(t, result.Commands, 1)
		require.Equal(t, int64(150), result.Commands[0].AtMs)
		require.JSONEq(t, `{"seen":5}`, string(result.Commands[0].Response))
		require.Equal(t, []runtime.SimulatedEvent{{AtMs: 150, Event: json.RawMessage(`{"said":"go"}`)}}, result.Events)
		// Observations are taken at the first tick at or after the instant.
		require.Len(t, result.Observations, 2)
		require.Equal(t, int64(0), result.Observations[0].AtMs)
		require.Equal(t, int64(130), result.Observations[1].RequestedAtMs)
		require.Equal(t, int64(200), result.Observations[1].AtMs)
		require.JSONEq(t, `{"me":"P2","pos":5,"calls":5}`, string(result.Observations[1].Views["P2"]))
		require.Equal(t, 4, result.Boundaries)
	})

	t.Run("finishing cuts the batch at the operation that finished", func(t *testing.T) {
		result := simulateLog(t, service, runtime.Scenario{
			Players: 1, DurationMs: 1000,
			Steps: []runtime.ScenarioStep{
				{AtMs: 150, Player: "P1", Command: &runtime.ScenarioCommand{Name: "end"}},
				{AtMs: 170, Player: "P1", Inputs: json.RawMessage(`{"x":9}`)},
			},
		})
		require.True(t, result.Finished)
		require.Equal(t, int64(150), *result.FinishedAtMs)
		require.Equal(t, runtime.SimulationStoppedFinished, result.Stopped)
		require.Equal(t, []string{"update@100", "command:end@150:pos=0"}, stateLog(t, result.Final.State))
		require.Equal(t, int64(150), result.Final.AtMs)
	})

	t.Run("a game without update simulates up to the duration even when its ticks run nothing", func(t *testing.T) {
		withoutUpdate := logManifest
		withoutUpdate.Update = false
		result, err := service.Simulate(context.Background(), []byte(logScript), string(scriptcheck.ProfileHandwritten), withoutUpdate, runtime.Scenario{
			Players: 1, DurationMs: 500,
			Steps:       []runtime.ScenarioStep{{AtMs: 150, Player: "P1", Command: &runtime.ScenarioCommand{Name: "go"}}},
			ObserveAtMs: []int64{450},
		})
		require.NoError(t, err)
		require.Equal(t, runtime.SimulationStoppedDuration, result.Stopped)
		require.Equal(t, int64(500), result.SimulatedMs, "the clock reached the duration")
		require.Equal(t, int64(500), result.Final.AtMs)
		require.Equal(t, int64(500), result.Observations[0].AtMs)
		require.Equal(t, []string{"command:go@150:pos=0"}, stateLog(t, result.Final.State))
		require.Equal(t, 1, result.Boundaries, "only the command's boundary ran")
	})

	t.Run("the same scenario and seed give the same result, another seed another", func(t *testing.T) {
		scenario := runtime.Scenario{Players: 1, Seed: 7, DurationMs: 200}
		first := simulateLog(t, service, scenario)
		second := simulateLog(t, service, scenario)
		require.Equal(t, first, second)
		scenario.Seed = 8
		other := simulateLog(t, service, scenario)
		require.NotEqual(t, string(first.Final.State), string(other.Final.State))
	})
}

func TestSimulateAdmitsLikeALiveSession(t *testing.T) {
	service := newService(t)

	t.Run("invalid steps and refreshes never reach the game", func(t *testing.T) {
		result := simulateLog(t, service, runtime.Scenario{
			Players: 1, DurationMs: 200,
			Steps: []runtime.ScenarioStep{
				{AtMs: 10, Player: "P1", Command: &runtime.ScenarioCommand{Name: "fly"}},
				{AtMs: 20, Player: "P1", Inputs: json.RawMessage(`{"y":1}`)},
				{AtMs: 30, Player: "P1", Inputs: json.RawMessage(`{"x":0}`)},
			},
		})
		require.Len(t, result.Rejected, 2)
		require.Equal(t, "command", result.Rejected[0].Kind)
		require.Equal(t, reasonInvalid, result.Rejected[0].Reason)
		require.Equal(t, "inputs", result.Rejected[1].Kind)
		require.Equal(t, []string{"update@100", "update@200"}, stateLog(t, result.Final.State), "the inputs at their defaults are a refresh, not a transition")
	})

	t.Run("a participant's commands beyond the live rate are refused", func(t *testing.T) {
		steps := make([]runtime.ScenarioStep, 15)
		for i := range steps {
			steps[i] = runtime.ScenarioStep{AtMs: 50, Player: "P1", Command: &runtime.ScenarioCommand{Name: "go"}}
		}
		result := simulateLog(t, service, runtime.Scenario{Players: 1, DurationMs: 100, Steps: steps})
		require.Len(t, result.Commands, 10)
		require.Len(t, result.Rejected, 5)
		for _, rejected := range result.Rejected {
			require.Equal(t, "command_participant_rate", rejected.Reason)
		}
	})

	t.Run("a scenario the game cannot play is invalid", func(t *testing.T) {
		result := simulateLog(t, service, runtime.Scenario{
			Players: 3, DurationMs: MaxDurationMs + 1,
			Steps: []runtime.ScenarioStep{{AtMs: 5, Player: "P9", Command: &runtime.ScenarioCommand{Name: "go"}}},
		})
		require.Equal(t, runtime.SimulationInvalid, result.Outcome)
		fields := make([]string, len(result.Diagnostics))
		for i, d := range result.Diagnostics {
			fields[i] = d.Field
		}
		require.ElementsMatch(t, []string{"players", "duration_ms", "steps[0].player"}, fields)
	})
}

func TestSimulateStops(t *testing.T) {
	t.Run("a script failure is a result with where it failed", func(t *testing.T) {
		result := simulateLog(t, newService(t), runtime.Scenario{
			Players: 1, DurationMs: 500,
			Steps: []runtime.ScenarioStep{{AtMs: 250, Player: "P1", Command: &runtime.ScenarioCommand{Name: "boom"}}},
		})
		require.Equal(t, runtime.SimulationStoppedScriptError, result.Stopped)
		require.NotNil(t, result.ScriptError)
		require.Equal(t, int64(250), result.ScriptError.AtMs)
		require.Contains(t, result.ScriptError.Message, "boom")
		require.Equal(t, []string{"update@100", "update@200"}, stateLog(t, result.Final.State), "the state confirmed before the failure")
	})

	t.Run("out of real time it stops with what it simulated", func(t *testing.T) {
		service := newService(t)
		clock := time.Unix(0, 0)
		service.now = func() time.Time {
			clock = clock.Add(time.Second)
			return clock
		}
		result := simulateLog(t, service, runtime.Scenario{Players: 1, DurationMs: MaxDurationMs})
		require.Equal(t, runtime.SimulationStoppedBudget, result.Stopped)
		require.Less(t, result.SimulatedMs, int64(MaxDurationMs))
		require.NotEmpty(t, result.Final.State)
	})

	t.Run("a full instance answers busy", func(t *testing.T) {
		service := newService(t)
		for range maxConcurrentSimulations {
			service.slots <- struct{}{}
		}
		_, err := service.Simulate(context.Background(), []byte(logScript), string(scriptcheck.ProfileHandwritten), logManifest, runtime.Scenario{Players: 1, DurationMs: 100})
		require.ErrorIs(t, err, runtime.ErrSimulationsBusy)
	})
}

func TestSimulateABuiltGame(t *testing.T) {
	script, err := os.ReadFile("../../sandbox/testdata/tap_race_sdk_v1.js")
	require.NoError(t, err)
	manifest := sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 8, Commands: []sessionpkg.RuntimeCommand{{Name: "tap"}}, Update: true}
	steps := make([]runtime.ScenarioStep, 20)
	for i := range steps {
		steps[i] = runtime.ScenarioStep{AtMs: int64(150 * (i + 1)), Player: "P1", Command: &runtime.ScenarioCommand{Name: "tap"}}
	}

	result, err := newService(t).Simulate(context.Background(), script, string(scriptcheck.ProfileSDKV1), manifest, runtime.Scenario{Players: 2, DurationMs: 31000, Steps: steps, ObserveAtMs: []int64{1000}})

	require.NoError(t, err)
	require.True(t, result.Finished)
	require.Equal(t, int64(3000), *result.FinishedAtMs)
	var state struct {
		Taps   map[string]int
		Winner string
	}
	require.NoError(t, json.Unmarshal(result.Final.State, &state))
	require.Equal(t, "P1", state.Winner)
	require.Equal(t, 20, state.Taps["P1"])
	require.Contains(t, string(result.Observations[0].Views["P2"]), `"me":"P2"`)
}
