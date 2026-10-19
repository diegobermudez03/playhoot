package sandbox

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// batchGame logs every call it receives with times and elapsed time, and
// draws a random number per samples() call. Commands "nope", "silent",
// "blame", "finish" and "bad_finish" reject, answer nothing, blame P2 and P1,
// finish the game, and finish it wrongly; {"cheat":true} inputs blame everyone.
const batchGame = `
function start(participants) { return { state: { log: [], rolls: [] } }; }
function samples(state, transitions, context) {
	const log = state.log.concat(transitions.map((t) => t.participant + "@" + t.sessionTime + ":" + JSON.stringify(t.inputs)));
	const cheats = transitions.filter((t) => t.inputs.cheat);
	const errors = cheats.map((t) => ({ participant: t.participant, error: { code: "CHEATING", at: t.sessionTime } }));
	const events = cheats.map((t) => ({ event: { type: "CHEATER_CAUGHT", who: t.participant } }));
	return { state: { log: log.concat(["samples@" + context.sessionTime + "+" + context.elapsed]), rolls: state.rolls.concat([Math.random()]) }, errors, events };
}
function command(state, cmd, context) {
	const log = state.log.concat([cmd.participant + "@" + cmd.sessionTime + ":" + cmd.name + JSON.stringify(cmd.payload) + "+" + context.elapsed]);
	if (cmd.name === "nope") {
		return { state: { log, rolls: state.rolls }, response: { ok: false, reason: "NOT_YOUR_TURN" } };
	}
	if (cmd.name === "silent") {
		return { state: { log, rolls: state.rolls } };
	}
	if (cmd.name === "finish") {
		return { state: { log, rolls: state.rolls }, response: { ok: true }, events: [{ event: "GAME_OVER" }], finished: true };
	}
	if (cmd.name === "bad_finish") {
		return { state: { log, rolls: state.rolls }, finished: "yes" };
	}
	if (cmd.name === "blame") {
		return { state: { log, rolls: state.rolls }, response: { ok: true }, errors: [{ participant: "P2", error: "BLAMED" }], events: [{ event: "BLAMING", participants: ["P1"] }] };
	}
	return { state: { log, rolls: state.rolls }, response: { ok: true, now: Date.now() } };
}
function update(state, context) {
	return { state: { log: state.log.concat(["update@" + context.sessionTime + "+" + context.elapsed]), rolls: state.rolls } };
}
function playerRejoined(state, participant, context) {
	return { state: { log: state.log.concat(["back:" + participant + "@" + context.sessionTime]), rolls: state.rolls }, accepted: participant !== "P9" };
}
function playerDisconnected(state, participant, context) {
	return { state: { log: state.log.concat(["gone:" + participant + "@" + context.sessionTime + "+" + context.elapsed]), rolls: state.rolls }, events: [{ event: "GONE" }] };
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

		expectedLog       []string
		expectedResponses []string
		expectedErrors    []GameError
		expectedEvents    []GameEvent
		expectedRejoins   []bool
		// expectedFinished is the index of the operation that finished the
		// game, if any did.
		expectedFinished      *int
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
		"update_runs_after_the_operations_before_it": func(t *testing.T) test {
			return test{
				script: batchGame,
				batch: Batch{Operations: []Operation{
					{SessionTimeMs: 70, Transitions: []Transition{transitionAt("P1", 40), transitionAt("P2", 70)}},
					{SessionTimeMs: 100, Update: true},
				}},
				expectedLog:     []string{"P1@40:{}", "P2@70:{}", "samples@70+70", "update@100+30"},
				expectedOutcome: ScriptSucceeded,
			}
		},
		"update_alone": func(t *testing.T) test {
			return test{
				script:          batchGame,
				batch:           Batch{Operations: []Operation{{SessionTimeMs: 100, Update: true}}},
				expectedLog:     []string{"update@100+100"},
				expectedOutcome: ScriptSucceeded,
			}
		},
		"script_without_update": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{{SessionTimeMs: 100, Update: true}}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "does not define a global update function",
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
		"errors_of_every_operation_in_batch_order": func(t *testing.T) test {
			return test{
				script: batchGame,
				batch: Batch{Operations: []Operation{
					{SessionTimeMs: 30, Transitions: []Transition{{ParticipantID: "P1", Inputs: []byte(`{"cheat":true}`), SessionTimeMs: 20}, transitionAt("P2", 25)}},
					commandAt(40, "P1", "blame", `{}`),
					{SessionTimeMs: 100, Update: true},
				}},
				expectedLog:       []string{`P1@20:{"cheat":true}`, "P2@25:{}", "samples@30+30", "P1@40:blame{}+10", "update@100+60"},
				expectedResponses: []string{`{"ok":true}`},
				expectedErrors: []GameError{
					{Operation: 0, ParticipantID: "P1", Error: []byte(`{"code":"CHEATING","at":20}`)},
					{Operation: 1, ParticipantID: "P2", Error: []byte(`"BLAMED"`)},
				},
				expectedEvents: []GameEvent{
					{Operation: 0, Event: []byte(`{"type":"CHEATER_CAUGHT","who":"P1"}`)},
					{Operation: 1, Participants: []string{"P1"}, Event: []byte(`"BLAMING"`)},
				},
				expectedOutcome: ScriptSucceeded,
			}
		},
		"disconnection_runs_at_its_place_in_the_batch": func(t *testing.T) test {
			return test{
				script: batchGame,
				batch: Batch{Operations: []Operation{
					{SessionTimeMs: 30, Transitions: []Transition{transitionAt("P1", 30)}},
					{SessionTimeMs: 60, Disconnected: "P2"},
					{SessionTimeMs: 100, Update: true},
				}},
				expectedLog:     []string{"P1@30:{}", "samples@30+30", "gone:P2@60+30", "update@100+40"},
				expectedEvents:  []GameEvent{{Operation: 1, Event: []byte(`"GONE"`)}},
				expectedOutcome: ScriptSucceeded,
			}
		},
		"rejoins_are_decided_by_the_game_in_batch_order": func(t *testing.T) test {
			return test{
				script: batchGame,
				batch: Batch{Operations: []Operation{
					{SessionTimeMs: 20, Rejoined: "P1"},
					{SessionTimeMs: 30, Rejoined: "P9"},
				}},
				expectedLog:     []string{"back:P1@20", "back:P9@30"},
				expectedRejoins: []bool{true, false},
				expectedOutcome: ScriptSucceeded,
			}
		},
		"rejoin_without_a_boolean_decision": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function playerRejoined(state) { return { state, accepted: "yes" }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{{SessionTimeMs: 100, Rejoined: "P1"}}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "accepted as true or false",
			}
		},
		"rejoin_without_a_decision": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function playerRejoined(state) { return { state }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{{SessionTimeMs: 100, Rejoined: "P1"}}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "accepted as true or false",
			}
		},
		"script_without_player_disconnected": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{{SessionTimeMs: 100, Disconnected: "P1"}}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "does not define a global playerDisconnected function",
			}
		},
		"errors_that_are_not_an_array": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function update(state) { return { state, errors: { participant: "P1", error: 1 } }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{{SessionTimeMs: 100, Update: true}}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "update() must return errors as an array",
			}
		},
		"error_without_a_participant": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function update(state) { return { state, errors: [{ error: "LATE" }] }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{{SessionTimeMs: 100, Update: true}}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "without a participant or an error value",
			}
		},
		"error_without_a_value": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function update(state) { return { state, errors: [{ participant: "P1", error: null }] }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{{SessionTimeMs: 100, Update: true}}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "without a participant or an error value",
			}
		},
		"errors_over_the_output_limit": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function update(state) { return { state, errors: [{ participant: "P1", error: "x".repeat(300000) }] }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{{SessionTimeMs: 100, Update: true}}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "exceeding",
			}
		},
		"events_that_are_not_an_array": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function update(state) { return { state, events: { event: 1 } }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{{SessionTimeMs: 100, Update: true}}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "update() must return events as an array",
			}
		},
		"event_without_a_value": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function update(state) { return { state, events: [{ participants: ["P1"] }] }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{{SessionTimeMs: 100, Update: true}}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "without an event value",
			}
		},
		"event_for_nobody": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function update(state) { return { state, events: [{ event: "BOOM", participants: [] }] }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{{SessionTimeMs: 100, Update: true}}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "empty participant list",
			}
		},
		"events_over_the_output_limit": func(t *testing.T) test {
			return test{
				script:                `function start() { return { state: {} }; } function update(state) { return { state, events: [{ event: "x".repeat(300000) }] }; } function project() { return {}; }`,
				batch:                 Batch{Operations: []Operation{{SessionTimeMs: 100, Update: true}}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "exceeding",
			}
		},
		"finishing_operation_is_the_last_executed": func(t *testing.T) test {
			finishedAt := 1
			return test{
				script: batchGame,
				batch: Batch{Operations: []Operation{
					{SessionTimeMs: 30, Transitions: []Transition{transitionAt("P1", 20)}},
					commandAt(40, "P2", "finish", `{}`),
					commandAt(50, "P1", "move", `{}`),
					{SessionTimeMs: 100, Update: true},
				}},
				expectedLog:       []string{"P1@20:{}", "samples@30+30", "P2@40:finish{}+10"},
				expectedResponses: []string{`{"ok":true}`},
				expectedEvents:    []GameEvent{{Operation: 1, Event: []byte(`"GAME_OVER"`)}},
				expectedFinished:  &finishedAt,
				expectedOutcome:   ScriptSucceeded,
			}
		},
		"finished_that_is_not_a_boolean": func(t *testing.T) test {
			return test{
				script:                batchGame,
				batch:                 Batch{Operations: []Operation{commandAt(40, "P2", "bad_finish", `{}`)}},
				expectedOutcome:       ScriptFailed,
				expectedErrorContains: "finished as true or false",
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
			require.Equal(t, tc.expectedErrors, result.Errors)
			require.Equal(t, tc.expectedEvents, result.Events)
			require.Equal(t, tc.expectedRejoins, result.RejoinsAccepted)
			require.Equal(t, tc.expectedFinished != nil, result.Finished)
			if tc.expectedFinished != nil {
				require.Equal(t, *tc.expectedFinished, result.FinishedOperation)
			}
		})
	}
}

func TestExecuteRejectsMalformedBatches(t *testing.T) {
	tests := map[string]Batch{
		"empty_batch":                 {},
		"time_going_back":             {Operations: []Operation{commandAt(200, "P1", "move", `{}`), commandAt(100, "P1", "move", `{}`)}},
		"step_with_both_kinds":        {Operations: []Operation{{SessionTimeMs: 10, Transitions: []Transition{transitionAt("P1", 10)}, Command: &Command{ParticipantID: "P1", Name: "move", Payload: []byte(`{}`)}}}},
		"step_with_neither_kind":      {Operations: []Operation{{SessionTimeMs: 10}}},
		"update_with_a_command":       {Operations: []Operation{{SessionTimeMs: 10, Update: true, Command: &Command{ParticipantID: "P1", Name: "move", Payload: []byte(`{}`)}}}},
		"update_with_a_disconnection": {Operations: []Operation{{SessionTimeMs: 10, Update: true, Disconnected: "P1"}}},
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
