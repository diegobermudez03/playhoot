package liveconnection

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

type commandAnswer struct {
	result runtime.CommandResult
	err    error
}

// sendCommand runs player's Command on its own goroutine, since it waits for
// the command's boundary.
func sendCommand(player runtime.PlayerConnection, command runtime.Command) <-chan commandAnswer {
	answers := make(chan commandAnswer, 1)
	go func() {
		result, err := player.Command(context.Background(), command)
		answers <- commandAnswer{result: result, err: err}
	}()
	return answers
}

func receive(t *testing.T, answers <-chan commandAnswer) runtime.CommandResult {
	t.Helper()
	select {
	case answer := <-answers:
		require.NoError(t, answer.err)
		return answer.result
	case <-time.After(time.Second):
		t.Fatal("command was not answered")
		return runtime.CommandResult{}
	}
}

// waitForTimeline waits until the timeline holds n operations.
func waitForTimeline(t *testing.T, controller *sessionController, n int) {
	t.Helper()
	require.Eventually(t, func() bool {
		s := controller.lock()
		defer controller.unlock()
		return len(s.timeline) == n
	}, time.Second, time.Millisecond)
}

// timelineCommands describes the commands waiting in the timeline as
// participant:token:name:payload@time.
func timelineCommands(controller *sessionController) []string {
	s := controller.lock()
	defer controller.unlock()
	var described []string
	for _, operation := range s.timeline {
		if command := operation.Command; command != nil {
			described = append(described, fmt.Sprintf("%s:%s:%s:%s@%d", operation.ParticipantID, command.token, command.name, command.payload, operation.SessionTimeMs))
		}
	}
	return described
}

// commitCommands commits the next boundary as if the game answered its
// commands with responses.
func commitCommands(t *testing.T, controller *sessionController, responses ...string) {
	t.Helper()
	work, ok := controller.takeBoundary()
	require.True(t, ok)
	executed := execution{game: Game{ContextRevision: work.game.ContextRevision + 1, State: []byte(`{}`)}, views: map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}}
	for _, response := range responses {
		executed.responses = append(executed.responses, []byte(response))
	}
	require.NoError(t, controller.commitBoundary(context.Background(), work, executed))
}

func TestAcceptCommand(t *testing.T) {
	type test struct {
		// processed are the tokens each participant executed before.
		processed map[string][]string
		// limits changes the Session's limits.
		limits func(*liveLimits)
		// run sends commands to the running game and returns their answers.
		run func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) []runtime.CommandResult

		expectedResults []runtime.CommandResult
		// expectedPending are the commands left in the timeline.
		expectedPending []string
	}

	ctx := context.Background()
	tests := map[string]func(t *testing.T) test{
		"command_waits_for_its_boundary_and_gets_its_response": func(t *testing.T) test {
			return test{
				run: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) []runtime.CommandResult {
					clock.Advance(30 * time.Millisecond)
					answers := sendCommand(players["user-a"], runtime.Command{Token: "t1", Name: "move_to", Payload: []byte(`{"x":3}`)})
					waitForTimeline(t, controller, 1)
					require.Equal(t, []string{`P1:t1:move_to:{"x":3}@30`}, timelineCommands(controller))
					requireClosed(t, controller.flush)
					commitCommands(t, controller, `{"ok":true}`)
					return []runtime.CommandResult{receive(t, answers)}
				},
				expectedResults: []runtime.CommandResult{{Token: "t1", Outcome: runtime.CommandExecuted, Response: []byte(`{"ok":true}`)}},
			}
		},
		"payload_is_completed_with_its_defaults_before_the_game": func(t *testing.T) test {
			return test{
				run: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) []runtime.CommandResult {
					sendCommand(players["user-a"], runtime.Command{Token: "t1", Name: "move_to"})
					waitForTimeline(t, controller, 1)
					return nil
				},
				expectedPending: []string{`P1:t1:move_to:{"x":0}@0`},
			}
		},
		"executed_token_is_already_processed": func(t *testing.T) test {
			return test{
				run: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) []runtime.CommandResult {
					answers := sendCommand(players["user-a"], runtime.Command{Token: "t1", Name: "pass"})
					waitForTimeline(t, controller, 1)
					commitCommands(t, controller, `"NOT_YOUR_TURN"`)
					first := receive(t, answers)
					retried, err := players["user-a"].Command(ctx, runtime.Command{Token: "t1", Name: "pass"})
					require.NoError(t, err)
					return []runtime.CommandResult{first, retried}
				},
				expectedResults: []runtime.CommandResult{
					{Token: "t1", Outcome: runtime.CommandExecuted, Response: []byte(`"NOT_YOUR_TURN"`)},
					{Token: "t1", Outcome: runtime.CommandAlreadyProcessed},
				},
			}
		},
		"tokens_are_scoped_to_their_participant": func(t *testing.T) test {
			return test{
				processed: map[string][]string{"P1": {"t1"}},
				run: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) []runtime.CommandResult {
					mine, err := players["user-a"].Command(ctx, runtime.Command{Token: "t1", Name: "pass"})
					require.NoError(t, err)
					sendCommand(players["user-b"], runtime.Command{Token: "t1", Name: "pass"})
					waitForTimeline(t, controller, 1)
					return []runtime.CommandResult{mine}
				},
				expectedResults: []runtime.CommandResult{{Token: "t1", Outcome: runtime.CommandAlreadyProcessed}},
				expectedPending: []string{"P2:t1:pass:{}@0"},
			}
		},
		"same_token_while_pending_is_enqueued_once": func(t *testing.T) test {
			return test{
				run: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) []runtime.CommandResult {
					original := sendCommand(players["user-a"], runtime.Command{Token: "t1", Name: "pass"})
					waitForTimeline(t, controller, 1)
					gone, cancel := context.WithCancel(ctx)
					cancel()
					_, err := players["user-a"].Command(gone, runtime.Command{Token: "t1", Name: "pass"})
					require.ErrorIs(t, err, context.Canceled, "the duplicate waits for the original")
					duplicate := sendCommand(players["user-a"], runtime.Command{Token: "t1", Name: "pass"})
					require.Len(t, timelineCommands(controller), 1)
					commitCommands(t, controller, `{"ok":true}`)
					return []runtime.CommandResult{receive(t, original), receive(t, duplicate)}
				},
				expectedResults: []runtime.CommandResult{
					{Token: "t1", Outcome: runtime.CommandExecuted, Response: []byte(`{"ok":true}`)},
					{Token: "t1", Outcome: runtime.CommandAlreadyProcessed},
				},
			}
		},
		"session_ending_answers_pending_commands_as_not_running": func(t *testing.T) test {
			return test{
				run: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) []runtime.CommandResult {
					answers := sendCommand(players["user-a"], runtime.Command{Token: "t1", Name: "pass"})
					waitForTimeline(t, controller, 1)
					controller.retire()
					return []runtime.CommandResult{receive(t, answers)}
				},
				expectedResults: []runtime.CommandResult{{Token: "t1", Outcome: runtime.CommandNotRunning}},
			}
		},
		"commands_beyond_the_pending_limit_are_rate_limited_and_their_token_stays_unused": func(t *testing.T) test {
			return test{
				limits: func(limits *liveLimits) { limits.pendingCommands = 1 },
				run: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) []runtime.CommandResult {
					first := sendCommand(players["user-a"], runtime.Command{Token: "t1", Name: "pass"})
					waitForTimeline(t, controller, 1)
					refused, err := players["user-a"].Command(ctx, runtime.Command{Token: "t2", Name: "pass"})
					require.NoError(t, err)
					commitCommands(t, controller, `{"ok":1}`)
					retried := sendCommand(players["user-a"], runtime.Command{Token: "t2", Name: "pass"})
					waitForTimeline(t, controller, 1)
					commitCommands(t, controller, `{"ok":2}`)
					return []runtime.CommandResult{refused, receive(t, first), receive(t, retried)}
				},
				expectedResults: []runtime.CommandResult{
					{Token: "t2", Outcome: runtime.CommandRateLimited},
					{Token: "t1", Outcome: runtime.CommandExecuted, Response: []byte(`{"ok":1}`)},
					{Token: "t2", Outcome: runtime.CommandExecuted, Response: []byte(`{"ok":2}`)},
				},
			}
		},
		"participant_sending_commands_beyond_its_rate_is_limited_but_not_the_others": func(t *testing.T) test {
			return test{
				limits: func(limits *liveLimits) {
					limits.participantCommands = []ratelimit.Limit{{Count: 1, Per: time.Second}}
				},
				run: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) []runtime.CommandResult {
					sendCommand(players["user-a"], runtime.Command{Token: "t1", Name: "pass"})
					waitForTimeline(t, controller, 1)
					refused, err := players["user-a"].Command(ctx, runtime.Command{Token: "t2", Name: "pass"})
					require.NoError(t, err)
					sendCommand(players["user-b"], runtime.Command{Token: "t1", Name: "pass"})
					waitForTimeline(t, controller, 2)
					return []runtime.CommandResult{refused}
				},
				expectedResults: []runtime.CommandResult{{Token: "t2", Outcome: runtime.CommandRateLimited}},
				expectedPending: []string{"P1:t1:pass:{}@0", "P2:t1:pass:{}@0"},
			}
		},
		"session_receiving_commands_beyond_its_rate_limits_every_participant": func(t *testing.T) test {
			return test{
				limits: func(limits *liveLimits) {
					limits.sessionCommands = []ratelimit.Limit{{Count: 1, Per: time.Second}, {Count: 2, Per: time.Minute}}
				},
				run: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) []runtime.CommandResult {
					sendCommand(players["user-a"], runtime.Command{Token: "t1", Name: "pass"})
					waitForTimeline(t, controller, 1)
					tooSoon, err := players["user-b"].Command(ctx, runtime.Command{Token: "t1", Name: "pass"})
					require.NoError(t, err)
					clock.Advance(time.Second)
					sendCommand(players["user-b"], runtime.Command{Token: "t2", Name: "pass"})
					waitForTimeline(t, controller, 2)
					clock.Advance(time.Second)
					tooMany, err := players["user-b"].Command(ctx, runtime.Command{Token: "t3", Name: "pass"})
					require.NoError(t, err)
					return []runtime.CommandResult{tooSoon, tooMany}
				},
				expectedResults: []runtime.CommandResult{
					{Token: "t1", Outcome: runtime.CommandRateLimited},
					{Token: "t3", Outcome: runtime.CommandRateLimited},
				},
				expectedPending: []string{"P1:t1:pass:{}@0", "P2:t2:pass:{}@1000"},
			}
		},
		"commands_and_transitions_share_the_timeline_in_arrival_order": func(t *testing.T) test {
			return test{
				run: func(t *testing.T, controller *sessionController, players map[string]runtime.PlayerConnection, clock *testClock) []runtime.CommandResult {
					require.NoError(t, players["user-a"].Sample(ctx, sample(1, `{"move":[1,0]}`)))
					clock.Advance(10 * time.Millisecond)
					sendCommand(players["user-b"], runtime.Command{Token: "t1", Name: "pass"})
					waitForTimeline(t, controller, 2)
					clock.Advance(10 * time.Millisecond)
					require.NoError(t, players["user-a"].Sample(ctx, sample(2, `{"move":[0,0]}`)))
					work, _ := controller.takeBoundary()
					require.Equal(t, []uint64{1, 2, 3}, []uint64{work.operations[0].Order, work.operations[1].Order, work.operations[2].Order})
					require.NotNil(t, work.operations[1].Command)
					controller.requeue(work)
					return nil
				},
				expectedPending: []string{"P2:t1:pass:{}@10"},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players, clock := runningControllerWithTokens(t, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, tc.processed, tc.limits, "user-a", "user-b")

			results := tc.run(t, controller, players, clock)

			require.Equal(t, tc.expectedResults, results)
			require.Equal(t, tc.expectedPending, timelineCommands(controller))
		})
	}
}

func TestCommandBeforeTheGameRuns(t *testing.T) {
	controller := newTestController(t, 8)
	player, _, ok := controller.joinLobby(context.Background(), runtime.Player{UserUUID: "user-a"})
	require.True(t, ok)

	result, err := player.Command(context.Background(), runtime.Command{Token: "t1", Name: "pass"})

	require.NoError(t, err)
	require.Equal(t, runtime.CommandResult{Token: "t1", Outcome: runtime.CommandNotRunning}, result)
}
