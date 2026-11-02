package sandbox

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
)

// tapRaceScript is Tap Race built by the frontend's game tools
// (playhoot_frontend fixtures/sdk-games/tap-race): the real output of the SDK
// build under the sdk-v1 profile.
func tapRaceScript(t *testing.T) []byte {
	t.Helper()
	script, err := os.ReadFile("testdata/tap_race_sdk_v1.js")
	require.NoError(t, err)
	return script
}

func TestSDKProfileRunsABuiltGame(t *testing.T) {
	ctx := context.Background()
	script := tapRaceScript(t)
	require.Empty(t, scriptcheck.Validate(scriptcheck.ProfileSDKV1, script), "the built script must pass its profile")

	host := newTestHost(t)
	spec := ContextSpec{GameVersionUUID: "tap-race", Script: script, Profile: scriptcheck.ProfileSDKV1}
	contextID, err := host.CreateContext(ctx, spec)
	require.NoError(t, err)
	started, err := host.Start(ctx, contextID, StartInput{Participants: []string{"P1", "P2"}, RandomSeed: 7})
	require.NoError(t, err)
	require.Equal(t, ScriptSucceeded, started.Outcome, started.ScriptError)
	require.JSONEq(t, `{"taps":{"P1":0,"P2":0},"winner":null}`, string(started.State))

	// The handler changes the state it receives in place.
	tapped, err := host.Execute(ctx, contextID, Batch{Operations: []Operation{
		{SessionTimeMs: 100, Command: &Command{ParticipantID: "P1", Name: "tap", Payload: []byte(`{}`)}},
		{SessionTimeMs: 200, Command: &Command{ParticipantID: "P1", Name: "tap", Payload: []byte(`{}`)}},
		{SessionTimeMs: 200, Update: true},
	}})
	require.NoError(t, err)
	require.Equal(t, ScriptSucceeded, tapped.Outcome, tapped.ScriptError)
	require.JSONEq(t, `{"taps":{"P1":2,"P2":0},"winner":null}`, string(tapped.State))
	require.Len(t, tapped.CommandResponses, 2)
	require.JSONEq(t, `{"ok":true}`, string(tapped.CommandResponses[0]))
	require.False(t, tapped.Finished)

	view, err := host.Project(ctx, contextID, "P2")
	require.NoError(t, err)
	require.Equal(t, ScriptSucceeded, view.Outcome, view.ScriptError)
	require.JSONEq(t, `{"me":"P2","taps":{"P1":2,"P2":0},"goal":20,"secondsLeft":30,"winner":null}`, string(view.View))

	// A context restored from the checkpoint goes on the same way.
	restored, err := host.CreateContext(ctx, spec)
	require.NoError(t, err)
	restore, err := host.Restore(ctx, restored, Snapshot{State: tapped.State, PRNGState: tapped.PRNGState, ContextRevision: tapped.ContextRevision, SessionTimeMs: 200})
	require.NoError(t, err)
	require.Equal(t, ScriptSucceeded, restore.Outcome, restore.ScriptError)

	finished, err := host.Execute(ctx, restored, Batch{Operations: []Operation{{SessionTimeMs: 30000, Update: true}}})
	require.NoError(t, err)
	require.Equal(t, ScriptSucceeded, finished.Outcome, finished.ScriptError)
	require.True(t, finished.Finished)
	var state struct{ Winner string }
	require.NoError(t, json.Unmarshal(finished.State, &state))
	require.Equal(t, "P1", state.Winner)
}

func TestSDKProfileFreezesTopLevelValues(t *testing.T) {
	ctx := context.Background()
	const script = `"use strict";
		const ROUNDS = [1, 2];
		function start() { return { state: { n: 0 } }; }
		function command(state, cmd) { const rounds = ROUNDS; rounds.push(3); return { state: state, response: null }; }
		function project(state) { state.n = 99; return { n: state.n }; }
		function __playhoot_module_values() { return [ROUNDS]; }
	`

	t.Run("its arguments may change but its constants may not", func(t *testing.T) {
		host := newTestHost(t)
		contextID, err := host.CreateContext(ctx, ContextSpec{Script: []byte(script), Profile: scriptcheck.ProfileSDKV1})
		require.NoError(t, err)
		started, err := host.Start(ctx, contextID, StartInput{Participants: []string{"P1"}, RandomSeed: 1})
		require.NoError(t, err)
		require.Equal(t, ScriptSucceeded, started.Outcome, started.ScriptError)

		view, err := host.Project(ctx, contextID, "P1")
		require.NoError(t, err)
		require.Equal(t, ScriptSucceeded, view.Outcome, view.ScriptError)
		require.JSONEq(t, `{"n":99}`, string(view.View))

		result, err := host.Execute(ctx, contextID, Batch{Operations: []Operation{
			{SessionTimeMs: 1, Command: &Command{ParticipantID: "P1", Name: "go", Payload: []byte(`{}`)}},
		}})
		require.NoError(t, err)
		require.Equal(t, ScriptFailed, result.Outcome)
	})

	t.Run("a handwritten script keeps its arguments frozen", func(t *testing.T) {
		host := newTestHost(t)
		contextID, err := host.CreateContext(ctx, ContextSpec{Script: []byte(script), Profile: scriptcheck.ProfileHandwritten})
		require.NoError(t, err)
		started, err := host.Start(ctx, contextID, StartInput{Participants: []string{"P1"}, RandomSeed: 1})
		require.NoError(t, err)
		require.Equal(t, ScriptSucceeded, started.Outcome, started.ScriptError)
		view, err := host.Project(ctx, contextID, "P1")
		require.NoError(t, err)
		require.Equal(t, ScriptFailed, view.Outcome)
	})

	t.Run("an sdk-v1 script without its list of values fails to load", func(t *testing.T) {
		host := newTestHost(t)
		contextID, err := host.CreateContext(ctx, ContextSpec{Script: []byte(`function start() { return { state: {} }; } function project() { return {}; }`), Profile: scriptcheck.ProfileSDKV1})
		require.NoError(t, err)
		started, err := host.Start(ctx, contextID, StartInput{Participants: []string{"P1"}, RandomSeed: 1})
		require.NoError(t, err)
		require.Equal(t, ScriptFailed, started.Outcome)
		require.Contains(t, started.ScriptError, "__playhoot_module_values")
	})
}
