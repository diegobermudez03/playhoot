package sandbox

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// samplesGame records every transition it receives, with the time and a
// random draw, so tests can see order, time and randomness.
const samplesGame = `
function start(participants) { return { state: { log: [], rolls: [] } }; }
function samples(state, transitions, context) {
	const log = state.log.concat(transitions.map((t) => t.participant + "@" + t.sessionTime + ":" + JSON.stringify(t.inputs)));
	return { state: { log, at: context.sessionTime, elapsed: context.elapsed, now: Date.now(), rolls: state.rolls.concat([Math.random()]) } };
}
function project(state, participant, context) { return { log: state.log, at: context.sessionTime }; }
`

func TestSamples(t *testing.T) {
	type test struct {
		script string
		input  SamplesInput

		expectedState         string
		expectedRevision      int64
		expectedOutcome       ScriptOutcome
		expectedErrorContains string
		expectInfraError      bool
	}

	tests := map[string]func(t *testing.T) test{
		"transitions_of_every_participant_in_order": func(t *testing.T) test {
			return test{
				script: samplesGame,
				input: SamplesInput{SessionTimeMs: 100, Transitions: []Transition{
					{ParticipantID: "P2", Inputs: []byte(`{"move":[1,0]}`), SessionTimeMs: 20},
					{ParticipantID: "P1", Inputs: []byte(`{"move":[0,1]}`), SessionTimeMs: 35},
					{ParticipantID: "P2", Inputs: []byte(`{"move":[0,0]}`), SessionTimeMs: 90},
				}},
				expectedState:    `{"log":["P2@20:{\"move\":[1,0]}","P1@35:{\"move\":[0,1]}","P2@90:{\"move\":[0,0]}"],"at":100,"elapsed":100,"now":100}`,
				expectedRevision: 2,
				expectedOutcome:  ScriptSucceeded,
			}
		},
		"script_that_mutates_its_state": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: { log: [] } }; } function samples(state) { state.log.push(1); return { state }; } function project() { return {}; }`,
				input:                 SamplesInput{SessionTimeMs: 100, Transitions: []Transition{{ParticipantID: "P1", Inputs: []byte(`{}`), SessionTimeMs: 10}}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "samples() threw",
			}
		},
		"script_without_samples": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function project() { return {}; }`,
				input:                 SamplesInput{SessionTimeMs: 100},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "does not define a global samples function",
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			host := newTestHost(t)
			contextID := startedContext(t, host, tc.script)

			result, err := host.Samples(context.Background(), contextID, tc.input)

			require.NoError(t, err)
			require.Equal(t, tc.expectedOutcome, result.Outcome, result.ScriptError)
			require.Contains(t, result.ScriptError, tc.expectedErrorContains)
			if tc.expectedState != "" {
				var state map[string]any
				require.NoError(t, jsonUnmarshal(result.State, &state))
				delete(state, "rolls")
				require.JSONEq(t, tc.expectedState, jsonMarshal(t, state))
				require.Equal(t, tc.expectedRevision, result.ContextRevision)
				require.Len(t, result.PRNGState, 4)
			}
		})
	}
}

func TestSamplesTimeNeverGoesBack(t *testing.T) {
	host := newTestHost(t)
	contextID := startedContext(t, host, samplesGame)
	_, err := host.Samples(context.Background(), contextID, SamplesInput{SessionTimeMs: 200})
	require.NoError(t, err)

	_, err = host.Samples(context.Background(), contextID, SamplesInput{SessionTimeMs: 100})

	require.Error(t, err, "a batch behind the context's time is a Runtime bug, not the script's")
}

func TestRestoredContextContinuesTheSameGame(t *testing.T) {
	host := newTestHost(t)
	ctx := context.Background()
	original := startedContext(t, host, samplesGame)
	first, err := host.Samples(ctx, original, SamplesInput{SessionTimeMs: 100, Transitions: []Transition{{ParticipantID: "P1", Inputs: []byte(`{}`), SessionTimeMs: 50}}})
	require.NoError(t, err)
	restored, err := host.CreateContext(ctx, ContextSpec{Script: []byte(samplesGame)})
	require.NoError(t, err)
	_, err = host.Restore(ctx, restored, Snapshot{State: first.State, PRNGState: first.PRNGState, ContextRevision: first.ContextRevision, SessionTimeMs: 100})
	require.NoError(t, err)
	next := SamplesInput{SessionTimeMs: 250, Transitions: []Transition{{ParticipantID: "P2", Inputs: []byte(`{}`), SessionTimeMs: 180}}}

	want, err := host.Samples(ctx, original, next)
	require.NoError(t, err)
	got, err := host.Samples(ctx, restored, next)
	require.NoError(t, err)

	require.Equal(t, ScriptSucceeded, want.Outcome, want.ScriptError)
	require.Equal(t, want, got, "the same state, randomness, revision and time must give the same result")
}

func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

func jsonMarshal(t *testing.T, v any) string {
	t.Helper()
	encoded, err := json.Marshal(v)
	require.NoError(t, err)
	return string(encoded)
}
