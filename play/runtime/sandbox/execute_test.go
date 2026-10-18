package sandbox

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// batchGame logs every transition, samples() call and command it receives,
// with times and elapsed time, and draws a random number per samples() call,
// so tests can see order, time and randomness. A command named "nope" is
// rejected by the game's rules; "silent" answers nothing.
const batchGame = `
function start(participants) { return { state: { log: [], rolls: [] } }; }
function samples(state, transitions, context) {
	const log = state.log.concat(transitions.map((t) => t.participant + "@" + t.sessionTime + ":" + JSON.stringify(t.inputs)));
	return { state: { log: log.concat(["samples@" + context.sessionTime + "+" + context.elapsed]), rolls: state.rolls.concat([Math.random()]) } };
}
function command(state, cmd, context) {
	const log = state.log.concat([cmd.participant + "@" + cmd.sessionTime + ":" + cmd.name + JSON.stringify(cmd.payload) + "+" + context.elapsed]);
	if (cmd.name === "nope") {
		return { state: { log, rolls: state.rolls }, response: { ok: false, reason: "NOT_YOUR_TURN" } };
	}
	if (cmd.name === "silent") {
		return { state: { log, rolls: state.rolls } };
	}
	return { state: { log, rolls: state.rolls }, response: { ok: true, now: Date.now() } };
}
function project(state, participant, context) { return { log: state.log }; }
`

func transitionAt(participantID string, sessionTimeMs int64) Transition {
	return Transition{ParticipantID: participantID, Inputs: []byte(`{}`), SessionTimeMs: sessionTimeMs}
}

func commandAt(sessionTimeMs int64, participantID string, name string, payload string) Operation {
	return Operation{SessionTimeMs: sessionTimeMs, Command: &Command{ParticipantID: participantID, Name: name, Payload: []byte(payload)}}
}

func TestExecute(t *testing.T) {
	type test struct {
		script string
		batch  Batch

		expectedLog           []string
		expectedResponses     []string
		expectedOutcome       ScriptOutcome
		expectedErrorContains string
	}

	tests := map[string]func(t *testing.T) test{
		"transitions_of_every_participant_in_order": func(t *testing.T) test {
			return test{
				script: batchGame,
				batch: Batch{Operations: []Operation{{SessionTimeMs: 100, Transitions: []Transition{
					{ParticipantID: "P2", Inputs: []byte(`{"move":[1,0]}`), SessionTimeMs: 20},
					{ParticipantID: "P1", Inputs: []byte(`{"move":[0,1]}`), SessionTimeMs: 35},
				}}}},
				expectedLog:     []string{`P2@20:{"move":[1,0]}`, `P1@35:{"move":[0,1]}`, "samples@100+100"},
				expectedOutcome: ScriptSucceeded,
			}
		},
		"operations_run_in_order_each_at_its_time": func(t *testing.T) test {
			return test{
				script: batchGame,
				batch: Batch{Operations: []Operation{
					{SessionTimeMs: 30, Transitions: []Transition{transitionAt("P1", 20)}},
					commandAt(30, "P2", "move", `{"x":1}`),
					{SessionTimeMs: 100, Transitions: []Transition{transitionAt("P1", 60)}},
				}},
				expectedLog:       []string{"P1@20:{}", "samples@30+30", `P2@30:move{"x":1}+0`, "P1@60:{}", "samples@100+70"},
				expectedResponses: []string{`{"ok":true,"now":30}`},
				expectedOutcome:   ScriptSucceeded,
			}
		},
		"logical_rejection_is_a_normal_response": func(t *testing.T) test {
			return test{
				script:            batchGame,
				batch:             Batch{Operations: []Operation{commandAt(40, "P1", "nope", `{}`)}},
				expectedLog:       []string{"P1@40:nope{}+40"},
				expectedResponses: []string{`{"ok":false,"reason":"NOT_YOUR_TURN"}`},
				expectedOutcome:   ScriptSucceeded,
			}
		},
		"command_without_a_response_answers_null": func(t *testing.T) test {
			return test{
				script:            batchGame,
				batch:             Batch{Operations: []Operation{commandAt(40, "P1", "silent", `{}`), commandAt(50, "P2", "move", `{}`)}},
				expectedLog:       []string{"P1@40:silent{}+40", "P2@50:move{}+10"},
				expectedResponses: []string{`null`, `{"ok":true,"now":50}`},
				expectedOutcome:   ScriptSucceeded,
			}
		},
		"command_that_throws_fails_the_batch": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function command() { throw new Error("bug"); } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{commandAt(40, "P1", "move", `{}`)}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "command() threw",
			}
		},
		"script_without_command": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{commandAt(40, "P1", "move", `{}`)}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "does not define a global command function",
			}
		},
		"response_over_the_output_limit": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function command(state) { return { state, response: "x".repeat(300000) }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{commandAt(40, "P1", "move", `{}`)}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "exceeding",
			}
		},
		"script_that_mutates_its_state": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: { log: [] } }; } function samples(state) { state.log.push(1); return { state }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{{SessionTimeMs: 100, Transitions: []Transition{transitionAt("P1", 10)}}}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "samples() threw",
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			host := newTestHost(t)
			contextID := startedContext(t, host, tc.script)

			result, err := host.Execute(context.Background(), contextID, tc.batch)

			require.NoError(t, err)
			require.Equal(t, tc.expectedOutcome, result.Outcome, result.ScriptError)
			require.Contains(t, result.ScriptError, tc.expectedErrorContains)
			if tc.expectedOutcome != ScriptSucceeded {
				return
			}
			var state struct{ Log []string }
			require.NoError(t, json.Unmarshal(result.State, &state))
			require.Equal(t, tc.expectedLog, state.Log)
			require.Equal(t, int64(2), result.ContextRevision, "one revision per batch")
			require.Len(t, result.PRNGState, 4)
			responses := make([]string, len(result.CommandResponses))
			for i, response := range result.CommandResponses {
				responses[i] = string(response)
			}
			if tc.expectedResponses == nil {
				require.Empty(t, responses)
			} else {
				require.Equal(t, tc.expectedResponses, responses)
			}
		})
	}
}

func TestExecuteRejectsMalformedBatches(t *testing.T) {
	tests := map[string]Batch{
		"empty_batch":            {},
		"time_going_back":        {Operations: []Operation{commandAt(200, "P1", "move", `{}`), commandAt(100, "P1", "move", `{}`)}},
		"step_with_both_kinds":   {Operations: []Operation{{SessionTimeMs: 10, Transitions: []Transition{transitionAt("P1", 10)}, Command: &Command{ParticipantID: "P1", Name: "move", Payload: []byte(`{}`)}}}},
		"step_with_neither_kind": {Operations: []Operation{{SessionTimeMs: 10}}},
	}

	for name, batch := range tests {
		t.Run(name, func(t *testing.T) {
			host := newTestHost(t)
			contextID := startedContext(t, host, batchGame)

			_, err := host.Execute(context.Background(), contextID, batch)

			require.Error(t, err, "a malformed batch is a Runtime bug, not the script's")
		})
	}
}

func TestExecuteTimeNeverGoesBack(t *testing.T) {
	host := newTestHost(t)
	contextID := startedContext(t, host, batchGame)
	_, err := host.Execute(context.Background(), contextID, Batch{Operations: []Operation{commandAt(200, "P1", "move", `{}`)}})
	require.NoError(t, err)

	_, err = host.Execute(context.Background(), contextID, Batch{Operations: []Operation{commandAt(100, "P1", "move", `{}`)}})

	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "cannot go back"), err.Error())
}

func TestRestoredContextContinuesTheSameGame(t *testing.T) {
	host := newTestHost(t)
	ctx := context.Background()
	original := startedContext(t, host, batchGame)
	first, err := host.Execute(ctx, original, Batch{Operations: []Operation{{SessionTimeMs: 100, Transitions: []Transition{transitionAt("P1", 50)}}}})
	require.NoError(t, err)
	restored, err := host.CreateContext(ctx, ContextSpec{Script: []byte(batchGame)})
	require.NoError(t, err)
	_, err = host.Restore(ctx, restored, Snapshot{State: first.State, PRNGState: first.PRNGState, ContextRevision: first.ContextRevision, SessionTimeMs: 100})
	require.NoError(t, err)
	next := Batch{Operations: []Operation{
		{SessionTimeMs: 180, Transitions: []Transition{transitionAt("P2", 150)}},
		commandAt(180, "P2", "move", `{"x":1}`),
		{SessionTimeMs: 250, Transitions: []Transition{transitionAt("P1", 200)}},
	}}

	want, err := host.Execute(ctx, original, next)
	require.NoError(t, err)
	got, err := host.Execute(ctx, restored, next)
	require.NoError(t, err)

	require.Equal(t, ScriptSucceeded, want.Outcome, want.ScriptError)
	require.Equal(t, want, got, "the same state, randomness, revision and time must give the same result")
}
