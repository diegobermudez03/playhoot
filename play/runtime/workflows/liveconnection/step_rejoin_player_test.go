package liveconnection

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
)

type attachAnswer struct {
	result      runtime.AttachPlayerResult
	participant bool
	err         error
}

// attachInBackground attaches userUUID to controller's running game without
// waiting for the game's answer.
func attachInBackground(controller *sessionController, userUUID string) <-chan attachAnswer {
	answers := make(chan attachAnswer, 1)
	go func() {
		result, participant, err := controller.attachParticipant(context.Background(), userUUID)
		answers <- attachAnswer{result: result, participant: participant, err: err}
	}()
	return answers
}

func receiveAttach(t *testing.T, answers <-chan attachAnswer) runtime.AttachPlayerResult {
	t.Helper()
	select {
	case answer := <-answers:
		require.NoError(t, answer.err)
		require.True(t, answer.participant)
		return answer.result
	case <-time.After(time.Second):
		t.Fatal("attach was not answered")
		return runtime.AttachPlayerResult{}
	}
}

// rejoinableGame is a running game of user-a (P1) and user-b (P2) whose
// script supports rejoining, with user-a disconnected by leaving and the game
// already told.
func rejoinableGame(t *testing.T) *sessionController {
	t.Helper()
	controller, players, _ := runningController(t, map[string][]byte{"P1": []byte(`{"away":true}`), "P2": []byte(`{}`)}, "user-a", "user-b")
	controller.lock().manifest.Rejoin = true
	controller.unlock()
	_, err := players["user-a"].Leave(context.Background())
	require.NoError(t, err)
	controller.takeBoundary()
	<-controller.flush
	return controller
}

func TestRejoin(t *testing.T) {
	type test struct {
		// decide answers the request waiting in work; nil leaves it waiting.
		decide func(t *testing.T, controller *sessionController, work boundary)

		expectedResult       runtime.AttachPlayerResult
		expectedDisconnected bool
	}

	ctx := context.Background()
	commit := func(accepted bool) func(t *testing.T, controller *sessionController, work boundary) {
		return func(t *testing.T, controller *sessionController, work boundary) {
			require.NoError(t, controller.commitBoundary(ctx, work, execution{
				game:    Game{ContextRevision: 2, State: []byte(`{}`)},
				views:   map[string][]byte{"P1": []byte(`{"away":false}`), "P2": []byte(`{}`)},
				rejoins: []bool{accepted},
			}))
		}
	}

	tests := map[string]func(t *testing.T) test{
		"accepted_participant_plays_again": func(t *testing.T) test {
			return test{decide: commit(true), expectedResult: runtime.AttachPlayerResult{Outcome: runtime.AttachAccepted}}
		},
		"rejected_participant_stays_disconnected": func(t *testing.T) test {
			return test{
				decide:               commit(false),
				expectedResult:       runtime.AttachPlayerResult{Outcome: runtime.AttachRejected, Rejection: runtime.RejectRejoinRejected},
				expectedDisconnected: true,
			}
		},
		"session_ending_first_answers_unavailable": func(t *testing.T) test {
			return test{
				decide: func(t *testing.T, controller *sessionController, _ boundary) {
					controller.retire()
				},
				expectedResult:       runtime.AttachPlayerResult{Outcome: runtime.AttachRejected, Rejection: runtime.RejectSessionUnavailable},
				expectedDisconnected: true,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller := rejoinableGame(t)

			answers := attachInBackground(controller, "user-a")
			waitForTimeline(t, controller, 1)
			requireClosed(t, controller.flush)
			work, _ := controller.takeBoundary()
			require.Equal(t, []Operation{{ParticipantID: "P1", Order: 2, Rejoin: work.operations[0].Rejoin}}, withoutTimes(work.operations))
			require.NotNil(t, work.operations[0].Rejoin)
			tc.decide(t, controller, work)

			result := receiveAttach(t, answers)

			connection := result.Connection
			result.Connection = runtime.PlayerConnection{}
			require.Equal(t, tc.expectedResult, result)
			require.Equal(t, tc.expectedDisconnected, controller.state.presence["P1"].disconnected)
			if result.Outcome != runtime.AttachAccepted {
				return
			}
			start, started := connection.Started(ctx)
			require.True(t, started)
			require.Equal(t, "P1", start.ParticipantID)
			view, ok := connection.NextView(ctx)
			require.True(t, ok)
			require.Equal(t, runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P1", Full: true, Revision: 1, State: []byte(`{"away":false}`)}}, view, "the view the rejoin left")
			require.True(t, controller.state.presence["P1"].attached())
		})
	}
}

func TestRejoinOfAGameWithoutIt(t *testing.T) {
	controller := rejoinableGame(t)
	controller.lock().manifest.Rejoin = false
	controller.unlock()

	result, participant, err := controller.attachParticipant(context.Background(), "user-a")

	require.NoError(t, err)
	require.True(t, participant)
	require.Equal(t, runtime.RejectParticipantDisconnected, result.Rejection)
	work, _ := controller.takeBoundary()
	require.Empty(t, work.operations, "the game is never asked")
}

func TestRejoinRequestsWhileOneWaitsShareIt(t *testing.T) {
	controller := rejoinableGame(t)
	ctx := context.Background()

	first := attachInBackground(controller, "user-a")
	waitForTimeline(t, controller, 1)
	second := attachInBackground(controller, "user-a")
	require.Eventually(t, func() bool { return len(rejoinsWaiting(controller)) == 1 }, time.Second, time.Millisecond)
	work, _ := controller.takeBoundary()
	require.Len(t, work.operations, 1, "one request reaches the game")
	require.NoError(t, controller.commitBoundary(ctx, work, execution{
		game: Game{ContextRevision: 2, State: []byte(`{}`)}, views: map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)}, rejoins: []bool{true},
	}))

	results := []runtime.AttachPlayerResult{receiveAttach(t, first), receiveAttach(t, second)}

	ends := 0
	for _, result := range results {
		require.Equal(t, runtime.AttachAccepted, result.Outcome)
		if endedWith(t, result.Connection.Ended) == runtime.EndReplaced {
			ends++
		}
	}
	require.Equal(t, 1, ends, "the later connection replaces the earlier")
}

func TestRejoinRunsInTheGame(t *testing.T) {
	ctrl := gomock.NewController(t)
	sandboxMock, replayMock := NewMockgameSandbox(ctrl), NewMockreplayRecorder(ctrl)
	deps := testDependencies()
	deps.sandbox, deps.replay = sandboxMock, replayMock
	controller, player, now := runningGame(t, deps)
	controller.lock().manifest.Rejoin = true
	controller.unlock()
	anyCtx := gomock.Any()
	_, err := player.Leave(context.Background())
	require.NoError(t, err)
	controller.lock().timeline = nil
	controller.unlock()
	*now = now.Add(40 * time.Millisecond)
	gomock.InOrder(
		sandboxMock.EXPECT().Execute(anyCtx, "context-1", sandbox.Batch{Operations: []sandbox.Operation{{SessionTimeMs: 40, Rejoined: "P1"}}}).
			Return(sandbox.ExecutionResult{Outcome: sandbox.ScriptSucceeded, State: []byte(`{"x":2}`), PRNGState: []byte{1, 0, 0, 0}, ContextRevision: 2, RejoinsAccepted: []bool{true}}, nil),
		sandboxMock.EXPECT().Project(anyCtx, "context-1", "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{"x":2}`)}, nil),
		replayMock.EXPECT().Offer(anyCtx, replay.Capture{
			SessionUUID: boundarySessionUUID, GameVersionUUID: boundaryGameVersion, SessionTimeMs: 40, ContextRevision: 2,
			State: []byte(`{"x":2}`), Participants: []string{"P1"},
		}),
	)
	answers := attachInBackground(controller, "user-a")
	waitForTimeline(t, controller, 1)

	controller.runBoundary()

	result := receiveAttach(t, answers)
	require.Equal(t, runtime.AttachAccepted, result.Outcome)
	view, ok := result.Connection.NextView(context.Background())
	require.True(t, ok)
	require.Equal(t, []byte(`{"x":2}`), view.State)
}

// rejoinsWaiting returns the participants whose rejoin waits for the game.
func rejoinsWaiting(controller *sessionController) []string {
	s := controller.lock()
	defer controller.unlock()
	var waiting []string
	for participantID := range s.rejoins {
		waiting = append(waiting, participantID)
	}
	return waiting
}
