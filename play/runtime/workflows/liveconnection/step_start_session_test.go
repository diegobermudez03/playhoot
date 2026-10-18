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
	"github.com/diegobermudez03/playhoot/play/runtime/internal/controller"
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
			return test{expectedOutcome: runtime.SessionStarted, expectedStarted: true}
		},
		"script_failing_once_is_retried_in_a_fresh_context": func(t *testing.T, deps dependencies) test {
			calls := append([]any{eligible(deps)}, scriptFails(deps, "context-1")...)
			calls = append(calls, gameStarts(deps, "context-2")...)
			gomock.InOrder(append(calls, commits(deps)...)...)
			return test{expectedOutcome: runtime.SessionStarted, expectedStarted: true}
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
			return test{expectedOutcome: runtime.StartAlreadyStarted, expectedStarted: true}
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
			manager := New(nil, nil, nil, deps.starts, deps.frontends, deps.sandbox)
			manager.seeds = func() uint64 { return seed }
			sessionController, ok, err := controller.NewRegistry().ControllerFor(ctx, sessionUUID, 1, func(context.Context) (controller.Manifest, error) {
				return controller.Manifest{MaxPlayers: 4}, nil
			})
			require.NoError(t, err)
			require.True(t, ok)
			first, _, admitted := sessionController.AdmitPlayer(ctx, playerA)
			require.True(t, admitted)
			second, _, admitted := sessionController.AdmitPlayer(ctx, playerB)
			require.True(t, admitted)

			result, err := manager.startSession(ctx, sessionController, sessionUUID, creatorUUID)

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
			_, _, admitted = sessionController.AdmitPlayer(ctx, "aaaaaaaa-aaaa-4aaa-8aaa-000000000003")
			require.Equal(t, tc.expectedLobby, admitted)
		})
	}
}
