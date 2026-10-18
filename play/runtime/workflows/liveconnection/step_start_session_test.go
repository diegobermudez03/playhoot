package liveconnection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/checkpoint"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestStartSession(t *testing.T) {
	type dependencies struct {
		starts    *MocksessionStarter
		frontends *MockfrontendSigner
		sandbox   *MockgameSandbox
	}
	type test struct {
		expectedOutcome runtime.StartSessionOutcome
		expectedErr     bool
		// expectedStarted is whether the players' connections see the start.
		expectedStarted bool
		// expectedEnd is how the players' connections ended, if they did.
		expectedEnd runtime.EndReason
		// expectedLobby is whether the Session is back in its lobby.
		expectedLobby bool
		// expectedContextID is the sandbox context the game keeps.
		expectedContextID string
	}

	const (
		sessionUUID     = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
		creatorUUID     = "0f8fad5b-d9cb-469f-a165-70867728950e"
		playerA         = "aaaaaaaa-aaaa-4aaa-8aaa-000000000001"
		playerB         = "aaaaaaaa-aaaa-4aaa-8aaa-000000000002"
		gameVersionUUID = "c1d2e3f4-0000-4000-8000-000000000001"
		seed            = uint64(42)
	)
	ctx := context.Background()
	script := []byte(`function start() {}`)
	spec := sandbox.ContextSpec{GameVersionUUID: gameVersionUUID, Script: script}
	input := sandbox.StartInput{Participants: []string{"P1", "P2"}, RandomSeed: seed}
	initial := sandbox.StartResult{Outcome: sandbox.ScriptSucceeded, State: []byte(`{"round":1}`), PRNGState: []byte{9}, ContextRevision: 1}
	failed := sandbox.StartResult{Outcome: sandbox.ScriptFailed, ScriptError: "TypeError: x is undefined"}
	roster := []sessionpkg.StartParticipant{{UserUUID: playerA, ParticipantID: "P1"}, {UserUUID: playerB, ParticipantID: "P2"}}
	encoded, err := checkpoint.Encode(checkpoint.Checkpoint{
		GameVersionUUID: gameVersionUUID,
		ContextRevision: 1,
		State:           []byte(`{"round":1}`),
		PRNGState:       []byte{9},
		Participants:    []string{"P1", "P2"},
	})
	require.NoError(t, err)

	expiresAt := time.Date(2026, 10, 17, 21, 0, 0, 0, time.UTC)
	access := sessionpkg.FrontendAccess{URL: "https://storage.example/frontend?sig=abc", SHA256: "9f86d081", ExpiresAt: expiresAt}
	// eligible also signs the frontend, which follows every eligible check.
	eligible := func(deps dependencies) *gomock.Call {
		return deps.frontends.EXPECT().FrontendAccess(ctx, gameVersionUUID).Return(access, nil).
			After(deps.starts.EXPECT().CheckStartEligibility(ctx, sessionUUID, creatorUUID, 2).
				Return(sessionpkg.StartEligibility{Outcome: sessionpkg.StartEligible, GameVersionUUID: gameVersionUUID, Script: script}, nil))
	}
	projects := func(deps dependencies, contextID string) []any {
		return []any{
			deps.sandbox.EXPECT().Project(ctx, contextID, "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{"me":1}`)}, nil),
			deps.sandbox.EXPECT().Project(ctx, contextID, "P2").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{"me":2}`)}, nil),
		}
	}
	scriptFails := func(deps dependencies, contextID string) []any {
		return []any{
			deps.sandbox.EXPECT().CreateContext(ctx, spec).Return(contextID, nil),
			deps.sandbox.EXPECT().Start(ctx, contextID, input).Return(failed, nil),
			deps.sandbox.EXPECT().DiscardContext(ctx, contextID).Return(nil),
		}
	}
	gameStarts := func(deps dependencies, contextID string) []any {
		return append([]any{
			deps.sandbox.EXPECT().CreateContext(ctx, spec).Return(contextID, nil),
			deps.sandbox.EXPECT().Start(ctx, contextID, input).Return(initial, nil),
		}, projects(deps, contextID)...)
	}
	running := sessionpkg.RunningSession{
		GameVersionUUID: gameVersionUUID,
		Script:          script,
		Participants:    roster,
		Checkpoint:      sessionpkg.Checkpoint{ContextRevision: 1, Content: encoded},
	}
	snapshot := sandbox.Snapshot{State: []byte(`{"round":1}`), PRNGState: []byte{9}, ContextRevision: 1}
	commit := sessionpkg.StartCommit{Participants: roster, Checkpoint: sessionpkg.Checkpoint{ContextRevision: 1, Content: encoded}}
	commits := func(deps dependencies) []any {
		return []any{
			deps.starts.EXPECT().CommitStart(ctx, sessionUUID, creatorUUID, commit).Return(sessionpkg.CommitStartCommitted, nil),
		}
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"game_starts": func(t *testing.T, deps dependencies) test {
			calls := append([]any{eligible(deps)}, gameStarts(deps, "context-1")...)
			gomock.InOrder(append(calls, commits(deps)...)...)
			return test{expectedOutcome: runtime.SessionStarted, expectedStarted: true, expectedContextID: "context-1"}
		},
		"script_failing_once_is_retried_in_a_fresh_context": func(t *testing.T, deps dependencies) test {
			calls := append([]any{eligible(deps)}, scriptFails(deps, "context-1")...)
			calls = append(calls, gameStarts(deps, "context-2")...)
			gomock.InOrder(append(calls, commits(deps)...)...)
			return test{expectedOutcome: runtime.SessionStarted, expectedStarted: true, expectedContextID: "context-2"}
		},
		"script_failing_twice_aborts_the_session": func(t *testing.T, deps dependencies) test {
			calls := append([]any{eligible(deps)}, scriptFails(deps, "context-1")...)
			calls = append(calls, scriptFails(deps, "context-2")...)
			gomock.InOrder(append(calls, deps.starts.EXPECT().AbortStart(ctx, sessionUUID).Return(nil))...)
			return test{expectedOutcome: runtime.StartFailed, expectedEnd: runtime.EndSessionAborted}
		},
		"projection_failing_in_the_script_fails_the_start": func(t *testing.T, deps dependencies) test {
			projectionFails := func(contextID string) []any {
				return []any{
					deps.sandbox.EXPECT().CreateContext(ctx, spec).Return(contextID, nil),
					deps.sandbox.EXPECT().Start(ctx, contextID, input).Return(initial, nil),
					deps.sandbox.EXPECT().Project(ctx, contextID, "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptFailed, ScriptError: "boom"}, nil),
					deps.sandbox.EXPECT().DiscardContext(ctx, contextID).Return(nil),
				}
			}
			calls := append([]any{eligible(deps)}, projectionFails("context-1")...)
			calls = append(calls, projectionFails("context-2")...)
			gomock.InOrder(append(calls, deps.starts.EXPECT().AbortStart(ctx, sessionUUID).Return(nil))...)
			return test{expectedOutcome: runtime.StartFailed, expectedEnd: runtime.EndSessionAborted}
		},
		"aborting_fails": func(t *testing.T, deps dependencies) test {
			calls := append([]any{eligible(deps)}, scriptFails(deps, "context-1")...)
			calls = append(calls, scriptFails(deps, "context-2")...)
			gomock.InOrder(append(calls, deps.starts.EXPECT().AbortStart(ctx, sessionUUID).Return(errors.New("database down")))...)
			return test{expectedErr: true, expectedLobby: true}
		},
		"session_left_its_lobby": func(t *testing.T, deps dependencies) test {
			deps.starts.EXPECT().CheckStartEligibility(ctx, sessionUUID, creatorUUID, 2).Return(sessionpkg.StartEligibility{Outcome: sessionpkg.StartNotInLobby}, nil)
			return test{expectedOutcome: runtime.StartNotInLobby, expectedLobby: true}
		},
		"session_already_running_in_the_database_is_adopted": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(append([]any{
				deps.starts.EXPECT().CheckStartEligibility(ctx, sessionUUID, creatorUUID, 2).Return(sessionpkg.StartEligibility{Outcome: sessionpkg.StartAlreadyStarted}, nil),
				deps.starts.EXPECT().LoadRunningSession(ctx, sessionUUID).Return(running, nil),
				deps.frontends.EXPECT().FrontendAccess(ctx, gameVersionUUID).Return(access, nil),
				deps.sandbox.EXPECT().CreateContext(ctx, spec).Return("context-1", nil),
				deps.sandbox.EXPECT().Restore(ctx, "context-1", snapshot).Return(sandbox.RestoreResult{Outcome: sandbox.ScriptSucceeded}, nil),
			}, projects(deps, "context-1")...)...)
			return test{expectedOutcome: runtime.StartAlreadyStarted, expectedStarted: true, expectedContextID: "context-1"}
		},
		"running_session_that_ended_meanwhile": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.starts.EXPECT().CheckStartEligibility(ctx, sessionUUID, creatorUUID, 2).Return(sessionpkg.StartEligibility{Outcome: sessionpkg.StartAlreadyStarted}, nil),
				deps.starts.EXPECT().LoadRunningSession(ctx, sessionUUID).Return(sessionpkg.RunningSession{}, sessionpkg.ErrSessionNotRunning),
			)
			return test{expectedOutcome: runtime.StartNotInLobby, expectedLobby: true}
		},
		"restoring_the_running_game_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.starts.EXPECT().CheckStartEligibility(ctx, sessionUUID, creatorUUID, 2).Return(sessionpkg.StartEligibility{Outcome: sessionpkg.StartAlreadyStarted}, nil),
				deps.starts.EXPECT().LoadRunningSession(ctx, sessionUUID).Return(running, nil),
				deps.frontends.EXPECT().FrontendAccess(ctx, gameVersionUUID).Return(access, nil),
				deps.sandbox.EXPECT().CreateContext(ctx, spec).Return("context-1", nil),
				deps.sandbox.EXPECT().Restore(ctx, "context-1", snapshot).Return(sandbox.RestoreResult{}, errors.New("connection reset")),
				deps.sandbox.EXPECT().DiscardContext(ctx, "context-1").Return(nil),
			)
			return test{expectedErr: true, expectedLobby: true}
		},
		"game_version_no_longer_playable": func(t *testing.T, deps dependencies) test {
			deps.starts.EXPECT().CheckStartEligibility(ctx, sessionUUID, creatorUUID, 2).Return(sessionpkg.StartEligibility{Outcome: sessionpkg.StartGameUnavailable}, nil)
			return test{expectedOutcome: runtime.StartGameUnavailable, expectedLobby: true}
		},
		"player_count_the_session_rejects": func(t *testing.T, deps dependencies) test {
			deps.starts.EXPECT().CheckStartEligibility(ctx, sessionUUID, creatorUUID, 2).Return(sessionpkg.StartEligibility{Outcome: sessionpkg.StartInvalidPlayerCount}, nil)
			return test{expectedOutcome: runtime.StartInvalidPlayerCount, expectedLobby: true}
		},
		"caller_who_is_not_the_creator_is_unexpected": func(t *testing.T, deps dependencies) test {
			deps.starts.EXPECT().CheckStartEligibility(ctx, sessionUUID, creatorUUID, 2).Return(sessionpkg.StartEligibility{Outcome: sessionpkg.StartNotCreator}, nil)
			return test{expectedErr: true, expectedLobby: true}
		},
		"eligibility_check_fails": func(t *testing.T, deps dependencies) test {
			deps.starts.EXPECT().CheckStartEligibility(ctx, sessionUUID, creatorUUID, 2).Return(sessionpkg.StartEligibility{}, errors.New("database down"))
			return test{expectedErr: true, expectedLobby: true}
		},
		"signing_the_frontend_fails": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.starts.EXPECT().CheckStartEligibility(ctx, sessionUUID, creatorUUID, 2).
					Return(sessionpkg.StartEligibility{Outcome: sessionpkg.StartEligible, GameVersionUUID: gameVersionUUID, Script: script}, nil),
				deps.frontends.EXPECT().FrontendAccess(ctx, gameVersionUUID).Return(sessionpkg.FrontendAccess{}, errors.New("signing key unavailable")),
			)
			return test{expectedErr: true, expectedLobby: true}
		},
		"sandbox_without_capacity": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				eligible(deps),
				deps.sandbox.EXPECT().CreateContext(ctx, spec).Return("", sandbox.ErrNoCapacity),
			)
			return test{expectedOutcome: runtime.StartSandboxUnavailable, expectedLobby: true}
		},
		"sandbox_unavailable": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				eligible(deps),
				deps.sandbox.EXPECT().CreateContext(ctx, spec).Return("", errors.New("connection refused")),
			)
			return test{expectedErr: true, expectedLobby: true}
		},
		"start_call_fails_and_its_context_is_discarded": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				eligible(deps),
				deps.sandbox.EXPECT().CreateContext(ctx, spec).Return("context-1", nil),
				deps.sandbox.EXPECT().Start(ctx, "context-1", input).Return(sandbox.StartResult{}, errors.New("connection reset")),
				deps.sandbox.EXPECT().DiscardContext(ctx, "context-1").Return(errors.New("connection refused")),
			)
			return test{expectedErr: true, expectedLobby: true}
		},
		"session_left_its_lobby_before_the_commit": func(t *testing.T, deps dependencies) test {
			calls := append([]any{eligible(deps)}, gameStarts(deps, "context-1")...)
			gomock.InOrder(append(calls,
				deps.starts.EXPECT().CommitStart(ctx, sessionUUID, creatorUUID, commit).Return(sessionpkg.CommitStartNotInLobby, nil),
				deps.sandbox.EXPECT().DiscardContext(ctx, "context-1").Return(nil),
			)...)
			return test{expectedOutcome: runtime.StartNotInLobby, expectedLobby: true}
		},
		"commit_fails": func(t *testing.T, deps dependencies) test {
			calls := append([]any{eligible(deps)}, gameStarts(deps, "context-1")...)
			gomock.InOrder(append(calls,
				deps.starts.EXPECT().CommitStart(ctx, sessionUUID, creatorUUID, commit).Return(sessionpkg.CommitStartOutcome(""), errors.New("database down")),
				deps.sandbox.EXPECT().DiscardContext(ctx, "context-1").Return(nil),
			)...)
			return test{expectedErr: true, expectedLobby: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{starts: NewMocksessionStarter(ctrl), frontends: NewMockfrontendSigner(ctrl), sandbox: NewMockgameSandbox(ctrl)}
			tc := setup(t, deps)
			controllerDeps := testDependencies()
			controllerDeps.starts, controllerDeps.frontends, controllerDeps.sandbox = deps.starts, deps.frontends, deps.sandbox
			controllerDeps.seeds = func() uint64 { return seed }
			sessionController := withManifest(newSessionController(ctx, sessionUUID, 1, controllerDeps), Manifest{MaxPlayers: 4})
			first, _, admitted := sessionController.joinLobby(ctx, playerA)
			require.True(t, admitted)
			second, _, admitted := sessionController.joinLobby(ctx, playerB)
			require.True(t, admitted)

			result, err := sessionController.startSession(ctx, creatorUUID)

			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, runtime.StartSessionResult{Outcome: tc.expectedOutcome}, result)
			}
			waiting, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
			defer cancel()
			start, started := second.Started(waiting)
			require.Equal(t, tc.expectedStarted, started)
			if tc.expectedStarted {
				require.Equal(t, runtime.PlayerStart{
					SessionStart: runtime.SessionStart{
						Participants: []runtime.Participant{
							{ParticipantID: "P1", UserUUID: playerA},
							{ParticipantID: "P2", UserUUID: playerB},
						},
						Frontend: runtime.GameFrontend{URL: access.URL, SHA256: access.SHA256, ExpiresAt: expiresAt},
					},
					ParticipantID: "P2",
				}, start)
				view, ok := second.NextView(waiting)
				require.True(t, ok)
				require.Equal(t, runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P2", Full: true, State: []byte(`{"me":2}`)}}, view)
			}
			end, ended := first.Ended(waiting)
			if tc.expectedEnd != "" {
				require.True(t, ended)
				require.Equal(t, runtime.ConnectionEnd{Reason: tc.expectedEnd}, end)
			} else {
				require.False(t, ended)
			}
			_, _, admitted = sessionController.joinLobby(ctx, "aaaaaaaa-aaaa-4aaa-8aaa-000000000003")
			require.Equal(t, tc.expectedLobby, admitted)
			require.Equal(t, tc.expectedContextID, sessionController.contextID)
		})
	}
}

// newStartController returns a fresh controller whose game allows up to
// maxPlayers, with userUUIDs admitted in order.
func newStartController(t *testing.T, maxPlayers int, userUUIDs ...string) (*sessionController, map[string]runtime.PlayerConnection) {
	t.Helper()
	controller := newTestController(t, maxPlayers)
	players := make(map[string]runtime.PlayerConnection, len(userUUIDs))
	for _, userUUID := range userUUIDs {
		player, _, admitted := controller.joinLobby(context.Background(), userUUID)
		require.True(t, admitted)
		players[userUUID] = player
	}
	return controller, players
}

// startedWith returns what started reports within a short wait, or false.
func startedWith[S any](t *testing.T, started func(context.Context) (S, bool)) (S, bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	return started(ctx)
}

func TestStartTransitions(t *testing.T) {
	t.Run("begin", testBeginStart)
	t.Run("cancel", testCancelStart)
	t.Run("complete", testCompleteStart)
	t.Run("complete_with_the_committed_roster", testCompleteStartWithTheCommittedRoster)
	t.Run("abort", testAbortStart)
}

func testBeginStart(t *testing.T) {
	type test struct {
		players []string
		// before runs against the Controller before the call under test.
		before func(t *testing.T, controller *sessionController)

		expectedParticipants []runtime.Participant
		expectedOutcome      runtime.StartSessionOutcome
		expectedOK           bool
		expectedPhase        phase
	}

	tests := map[string]func(t *testing.T) test{
		"lobby_is_frozen_with_participants_in_join_order": func(t *testing.T) test {
			return test{
				players: []string{"user-b", "user-a"},
				expectedParticipants: []runtime.Participant{
					{ParticipantID: "P1", UserUUID: "user-b"},
					{ParticipantID: "P2", UserUUID: "user-a"},
				},
				expectedOK:    true,
				expectedPhase: phaseStarting,
			}
		},
		"attempt_already_in_progress": func(t *testing.T) test {
			return test{
				players: []string{"user-a"},
				before: func(t *testing.T, controller *sessionController) {
					_, _, ok := controller.beginStart()
					require.True(t, ok)
				},
				expectedOutcome: runtime.StartInProgress,
				expectedPhase:   phaseStarting,
			}
		},
		"running_game": func(t *testing.T) test {
			return test{
				players: []string{"user-a"},
				before: func(t *testing.T, controller *sessionController) {
					_, _, ok := controller.beginStart()
					require.True(t, ok)
					controller.completeStart([]runtime.Participant{{ParticipantID: "P1", UserUUID: "user-a"}}, Game{}, runtime.GameFrontend{}, nil)
				},
				expectedOutcome: runtime.StartAlreadyStarted,
				expectedPhase:   phaseRunning,
			}
		},
		"aborted_session": func(t *testing.T) test {
			return test{
				players: []string{"user-a"},
				before: func(t *testing.T, controller *sessionController) {
					_, _, ok := controller.beginStart()
					require.True(t, ok)
					controller.abortStart()
				},
				expectedOutcome: runtime.StartNotInLobby,
				expectedPhase:   phaseEnded,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, _ := newStartController(t, 8, tc.players...)
			if tc.before != nil {
				tc.before(t, controller)
			}

			participants, outcome, ok := controller.beginStart()

			require.Equal(t, tc.expectedOK, ok)
			require.Equal(t, tc.expectedOutcome, outcome)
			require.Equal(t, tc.expectedParticipants, participants)
			require.Equal(t, tc.expectedPhase, controller.currentPhase())
			_, rejection, admitted := controller.joinLobby(context.Background(), "late-user")
			switch tc.expectedPhase {
			case phaseLobby:
				require.True(t, admitted)
			case phaseEnded:
				require.False(t, admitted)
				require.Equal(t, runtime.RejectSessionUnavailable, rejection)
			default:
				require.False(t, admitted)
				require.Equal(t, runtime.RejectSessionStarted, rejection, "nobody joins once the lobby is frozen")
			}
		})
	}
}

func testCancelStart(t *testing.T) {
	type test struct {
		// during runs while the Start attempt is in progress.
		during func(t *testing.T, players map[string]runtime.PlayerConnection)

		expectedLobby      []string
		expectedAdminLobby []string
	}

	tests := map[string]func(t *testing.T) test{
		"session_returns_to_its_lobby": func(t *testing.T) test {
			return test{
				expectedLobby:      []string{"user-a", "user-b"},
				expectedAdminLobby: []string{"user-a", "user-b"},
			}
		},
		"players_who_disconnected_during_the_attempt_leave_the_lobby": func(t *testing.T) test {
			return test{
				during: func(t *testing.T, players map[string]runtime.PlayerConnection) {
					players["user-a"].Disconnect()
				},
				expectedLobby:      []string{"user-b"},
				expectedAdminLobby: []string{"user-b"},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players := newStartController(t, 8, "user-a", "user-b")
			admin := controller.registerAdmin("creator")
			_, _, ok := controller.beginStart()
			require.True(t, ok)
			if tc.during != nil {
				tc.during(t, players)
			}
			require.Equal(t, []string{"user-a", "user-b"}, controller.lobbySnapshot(), "the lobby stays frozen during the attempt")

			controller.cancelStart()

			require.Equal(t, phaseLobby, controller.currentPhase())
			require.Equal(t, tc.expectedLobby, controller.lobbySnapshot())
			require.Equal(t, tc.expectedAdminLobby, latestLobby(t, admin.NextLobbyUpdate))
			_, _, admitted := controller.joinLobby(context.Background(), "user-c")
			require.True(t, admitted, "the lobby admits players again")
		})
	}
}

func testCompleteStart(t *testing.T) {
	type test struct {
		// during runs while the Start attempt is in progress.
		during func(t *testing.T, players map[string]runtime.PlayerConnection)

		expectedStartedA bool
		expectedViewA    bool
	}

	game := Game{ContextRevision: 1, State: []byte(`{"round":1}`), PRNGState: []byte{7}}
	views := map[string][]byte{"P1": []byte(`{"me":"a"}`), "P2": []byte(`{"me":"b"}`)}
	frontend := runtime.GameFrontend{URL: "https://storage.example/frontend?sig=abc", SHA256: "9f86d081", ExpiresAt: time.Date(2026, 10, 17, 21, 0, 0, 0, time.UTC)}
	roster := runtime.SessionStart{
		Participants: []runtime.Participant{
			{ParticipantID: "P1", UserUUID: "user-a"},
			{ParticipantID: "P2", UserUUID: "user-b"},
		},
		Frontend: frontend,
	}

	tests := map[string]func(t *testing.T) test{
		"every_connection_is_told_who_plays_and_players_get_their_first_view": func(t *testing.T) test {
			return test{expectedStartedA: true, expectedViewA: true}
		},
		"player_who_disconnected_during_the_attempt_stays_a_participant": func(t *testing.T) test {
			return test{
				during: func(t *testing.T, players map[string]runtime.PlayerConnection) {
					players["user-a"].Disconnect()
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller, players := newStartController(t, 8, "user-a", "user-b")
			admin := controller.registerAdmin("creator")
			participants, _, ok := controller.beginStart()
			require.True(t, ok)
			if tc.during != nil {
				tc.during(t, players)
			}

			controller.completeStart(participants, game, frontend, views)

			require.Equal(t, phaseRunning, controller.currentPhase())
			require.Equal(t, game, controller.state.game)
			require.Equal(t, []string{"user-a", "user-b"}, latestLobby(t, admin.NextLobbyUpdate), "the pending lobby update still arrives")
			adminStart, started := startedWith(t, admin.Started)
			require.True(t, started)
			require.Equal(t, roster, adminStart)

			playerB := players["user-b"]
			require.Equal(t, []string{"user-a", "user-b"}, latestLobby(t, playerB.NextLobbyUpdate))
			start, started := startedWith(t, playerB.Started)
			require.True(t, started)
			require.Equal(t, runtime.PlayerStart{SessionStart: roster, ParticipantID: "P2"}, start)
			view, ok := playerB.NextView(context.Background())
			require.True(t, ok)
			require.Equal(t, runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P2", Full: true, State: []byte(`{"me":"b"}`)}}, view)

			_, started = startedWith(t, players["user-a"].Started)
			require.Equal(t, tc.expectedStartedA, started)
			if tc.expectedViewA {
				view, ok := players["user-a"].NextView(context.Background())
				require.True(t, ok)
				require.Equal(t, "P1", view.ParticipantID)
			}

			later := controller.registerAdmin("creator")
			_, more := later.NextLobbyUpdate(context.Background())
			require.False(t, more, "an admin attached to a running game gets no lobby")
			laterStart, started := startedWith(t, later.Started)
			require.True(t, started)
			require.Equal(t, roster, laterStart)
		})
	}
}

func testCompleteStartWithTheCommittedRoster(t *testing.T) {
	controller, players := newStartController(t, 8, "user-a", "user-b", "user-c")
	_, _, ok := controller.beginStart()
	require.True(t, ok)
	// The durable Session committed an earlier start, without user-c.
	committed := []runtime.Participant{{ParticipantID: "P1", UserUUID: "user-b"}, {ParticipantID: "P2", UserUUID: "user-a"}}

	controller.completeStart(committed, Game{}, runtime.GameFrontend{}, map[string][]byte{"P1": []byte(`{}`), "P2": []byte(`{}`)})

	start, started := startedWith(t, players["user-b"].Started)
	require.True(t, started)
	require.Equal(t, runtime.PlayerStart{SessionStart: runtime.SessionStart{Participants: committed}, ParticipantID: "P1"}, start, "the committed numbering wins over the lobby's")
	require.Equal(t, runtime.EndSessionEnded, endedWith(t, players["user-c"].Ended), "a lobby player outside the committed roster is out")
	_, started = startedWith(t, players["user-c"].Started)
	require.False(t, started)
}

func testAbortStart(t *testing.T) {
	controller, players := newStartController(t, 8, "user-a")
	admin := controller.registerAdmin("creator")
	_, _, ok := controller.beginStart()
	require.True(t, ok)

	controller.abortStart()

	require.Equal(t, phaseEnded, controller.currentPhase())
	require.Equal(t, runtime.EndSessionAborted, endedWith(t, admin.Ended))
	require.Equal(t, runtime.EndSessionAborted, endedWith(t, players["user-a"].Ended))
	_, started := startedWith(t, players["user-a"].Started)
	require.False(t, started)
	_, rejection, admitted := controller.joinLobby(context.Background(), "user-b")
	require.False(t, admitted)
	require.Equal(t, runtime.RejectSessionUnavailable, rejection)
	require.Equal(t, runtime.EndSessionEnded, endedWith(t, controller.registerAdmin("creator").Ended))
}
