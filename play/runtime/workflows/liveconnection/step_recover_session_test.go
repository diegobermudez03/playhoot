package liveconnection

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/checkpoint"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// recoverySetup is a crashed running Session as the database keeps it: P1 was
// playing and P2 had disconnected, with a checkpoint at revision 6.
type recoverySetup struct {
	sessionUUID string
	now         time.Time
	limits      sessionpkg.SessionLimits
	running     sessionpkg.RunningSession
	spec        sandbox.ContextSpec
	snapshot    sandbox.Snapshot
	access      sessionpkg.FrontendAccess
}

func newRecoverySetup(t *testing.T) recoverySetup {
	t.Helper()
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	encoded, err := checkpoint.Encode(checkpoint.Checkpoint{
		GameVersionUUID: boundaryGameVersion, ContextRevision: 6, ExecutedThroughSessionTimeMs: 4200,
		State: []byte(`{"round":3}`), PRNGState: []byte{7, 0, 0, 0}, Participants: []string{"P1", "P2"},
		ProcessedCommandTokens: map[string][]string{"P1": {"t1"}}, Disconnected: []string{"P2"},
	})
	require.NoError(t, err)
	return recoverySetup{
		sessionUUID: boundarySessionUUID,
		now:         now,
		limits:      sessionpkg.SessionLimits{LobbyExpiresAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)},
		running: sessionpkg.RunningSession{
			GameVersionUUID: boundaryGameVersion,
			Script:          boundaryScript,
			Participants:    []sessionpkg.StartParticipant{{UserUUID: "user-a", ParticipantID: "P1"}, {UserUUID: "user-b", ParticipantID: "P2"}},
			Checkpoint:      sessionpkg.Checkpoint{ContextRevision: 6, Content: encoded},
		},
		spec:     sandbox.ContextSpec{GameVersionUUID: boundaryGameVersion, Script: boundaryScript},
		snapshot: sandbox.Snapshot{State: []byte(`{"round":3}`), PRNGState: []byte{7, 0, 0, 0}, ContextRevision: 6, SessionTimeMs: 4200},
		access:   sessionpkg.FrontendAccess{URL: "https://storage.example/frontend", SHA256: "9f86d081", ExpiresAt: now.Add(15 * time.Minute)},
	}
}

// recoveryMocks are a recovering controller's durable and sandbox sides.
type recoveryMocks struct {
	limits    *MocklimitsLoader
	starts    *MocksessionStarter
	ends      *MocksessionEnder
	frontends *MockfrontendSigner
	sandbox   *MockgameSandbox
	replay    *MockreplayRecorder
}

func newRecoveryMocks(ctrl *gomock.Controller) recoveryMocks {
	return recoveryMocks{
		limits: NewMocklimitsLoader(ctrl), starts: NewMocksessionStarter(ctrl), ends: NewMocksessionEnder(ctrl),
		frontends: NewMockfrontendSigner(ctrl), sandbox: NewMockgameSandbox(ctrl), replay: NewMockreplayRecorder(ctrl),
	}
}

func (m recoveryMocks) dependencies(now func() time.Time) *controllerDependencies {
	deps := testDependencies()
	deps.limits, deps.starts, deps.ends, deps.frontends, deps.sandbox, deps.replay = m.limits, m.starts, m.ends, m.frontends, m.sandbox, m.replay
	deps.now = now
	return deps
}

// recovers expects the whole recovery of setup, in order.
func (m recoveryMocks) recovers(setup recoverySetup) []any {
	anyCtx := gomock.Any()
	return []any{
		m.limits.EXPECT().LoadSessionLimits(anyCtx, setup.sessionUUID).Return(setup.limits, nil),
		m.starts.EXPECT().LoadRunningSession(anyCtx, setup.sessionUUID).Return(setup.running, nil),
		m.frontends.EXPECT().FrontendAccess(anyCtx, boundaryGameVersion).Return(setup.access, nil),
		m.sandbox.EXPECT().CreateContext(anyCtx, setup.spec).Return("context-1", nil),
		m.sandbox.EXPECT().Restore(anyCtx, "context-1", setup.snapshot).Return(sandbox.RestoreResult{Outcome: sandbox.ScriptSucceeded}, nil),
		m.sandbox.EXPECT().Project(anyCtx, "context-1", "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{"me":1}`)}, nil),
		m.sandbox.EXPECT().Project(anyCtx, "context-1", "P2").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{"me":2}`)}, nil),
		// A new recording starts where the checkpoint was.
		m.replay.EXPECT().StartRecording(setup.sessionUUID).Return(testRecordingID),
		m.replay.EXPECT().Offer(anyCtx, testRecordingID, replay.Capture{
			SessionUUID: setup.sessionUUID, GameVersionUUID: boundaryGameVersion, SessionTimeMs: 4200, ContextRevision: 6,
			State: []byte(`{"round":3}`), Participants: []string{"P1", "P2"},
		}),
	}
}

func TestRecoverSession(t *testing.T) {
	type test struct {
		// before runs against the fresh controller before recovering.
		before func(t *testing.T, controller *sessionController)

		expectedOutcome   string
		expectedErr       bool
		expectedPhase     phase
		expectedContextID string
	}

	setup := newRecoverySetup(t)
	anyCtx := gomock.Any()

	tests := map[string]func(t *testing.T, mocks recoveryMocks) test{
		"running_session_is_rebuilt_from_its_checkpoint": func(t *testing.T, mocks recoveryMocks) test {
			gomock.InOrder(mocks.recovers(setup)...)
			return test{expectedOutcome: recoveryRecovered, expectedPhase: phaseRunning, expectedContextID: "context-1"}
		},
		"session_past_its_lifetime_ends_instead": func(t *testing.T, mocks recoveryMocks) test {
			expired := setup.limits
			expired.ExpiresAt = setup.now
			gomock.InOrder(
				mocks.limits.EXPECT().LoadSessionLimits(anyCtx, setup.sessionUUID).Return(expired, nil),
				mocks.ends.EXPECT().EndSession(anyCtx, setup.sessionUUID, sessionpkg.SessionEnd{Cause: sessionpkg.EndMaxLifetime}).
					Return(sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "EXPIRED", EndCause: "MAX_LIFETIME"}, nil),
			)
			return test{expectedOutcome: recoveryEnded, expectedPhase: phaseEnded}
		},
		"session_the_database_no_longer_runs_needs_nothing": func(t *testing.T, mocks recoveryMocks) test {
			gomock.InOrder(
				mocks.limits.EXPECT().LoadSessionLimits(anyCtx, setup.sessionUUID).Return(setup.limits, nil),
				mocks.starts.EXPECT().LoadRunningSession(anyCtx, setup.sessionUUID).Return(sessionpkg.RunningSession{}, sessionpkg.ErrSessionNotRunning),
			)
			return test{expectedOutcome: recoveryNotNeeded, expectedPhase: phaseLobby}
		},
		"game_already_running_here_needs_nothing": func(t *testing.T, mocks recoveryMocks) test {
			return test{
				before: func(t *testing.T, controller *sessionController) {
					_, _, ok := controller.beginStart()
					require.True(t, ok)
					controller.completeStart(nil, Game{}, runtime.GameFrontend{}, nil, nil)
				},
				expectedOutcome: recoveryNotNeeded,
				expectedPhase:   phaseRunning,
			}
		},
		"restoring_fails_and_the_next_connection_tries_again": func(t *testing.T, mocks recoveryMocks) test {
			gomock.InOrder(
				mocks.limits.EXPECT().LoadSessionLimits(anyCtx, setup.sessionUUID).Return(setup.limits, nil),
				mocks.starts.EXPECT().LoadRunningSession(anyCtx, setup.sessionUUID).Return(setup.running, nil),
				mocks.frontends.EXPECT().FrontendAccess(anyCtx, boundaryGameVersion).Return(setup.access, nil),
				mocks.sandbox.EXPECT().CreateContext(anyCtx, setup.spec).Return("context-1", nil),
				mocks.sandbox.EXPECT().Restore(anyCtx, "context-1", setup.snapshot).Return(sandbox.RestoreResult{}, errors.New("worker crashed")),
				mocks.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil),
			)
			return test{expectedErr: true, expectedPhase: phaseLobby}
		},
		"sandbox_without_capacity_fails": func(t *testing.T, mocks recoveryMocks) test {
			gomock.InOrder(
				mocks.limits.EXPECT().LoadSessionLimits(anyCtx, setup.sessionUUID).Return(setup.limits, nil),
				mocks.starts.EXPECT().LoadRunningSession(anyCtx, setup.sessionUUID).Return(setup.running, nil),
				mocks.frontends.EXPECT().FrontendAccess(anyCtx, boundaryGameVersion).Return(setup.access, nil),
				mocks.sandbox.EXPECT().CreateContext(anyCtx, setup.spec).Return("", sandbox.ErrNoCapacity),
			)
			return test{expectedErr: true, expectedPhase: phaseLobby}
		},
		"reading_the_limits_fails": func(t *testing.T, mocks recoveryMocks) test {
			mocks.limits.EXPECT().LoadSessionLimits(anyCtx, setup.sessionUUID).Return(sessionpkg.SessionLimits{}, errors.New("database down"))
			return test{expectedErr: true, expectedPhase: phaseLobby}
		},
	}

	for name, build := range tests {
		t.Run(name, func(t *testing.T) {
			mocks := newRecoveryMocks(gomock.NewController(t))
			tc := build(t, mocks)
			controller := withManifest(newSessionController(context.Background(), setup.sessionUUID, 1, mocks.dependencies(func() time.Time { return setup.now })), Manifest{MaxPlayers: 4})
			if tc.before != nil {
				tc.before(t, controller)
			}

			outcome, err := controller.recoverSession(context.Background())

			if tc.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedOutcome, outcome)
			}
			require.Equal(t, tc.expectedPhase, controller.currentPhase())
			require.Equal(t, tc.expectedContextID, controller.contextID)
		})
	}
}

func TestRecoveredGameResumesWhereItsCheckpointWas(t *testing.T) {
	setup := newRecoverySetup(t)
	clock := &testClock{now: setup.now}
	mocks := newRecoveryMocks(gomock.NewController(t))
	gomock.InOrder(mocks.recovers(setup)...)
	controller := withManifest(newSessionController(context.Background(), setup.sessionUUID, 1, mocks.dependencies(clock.Now)), Manifest{MaxPlayers: 4})

	outcome, err := controller.recoverSession(context.Background())
	require.NoError(t, err)
	require.Equal(t, recoveryRecovered, outcome)

	s := controller.lock()
	require.Equal(t, int64(4200), s.sessionTime(), "game time resumes from the checkpoint, the outage not counted")
	require.Equal(t, int64(6), s.game.ContextRevision)
	require.True(t, s.commands["P1"].processed["t1"], "processed COMMAND tokens survive")
	require.True(t, s.presence["P2"].disconnected, "who was disconnected stays so")
	require.False(t, s.presence["P1"].disconnected)
	require.Equal(t, setup.now.Add(30*time.Second), s.presence["P1"].graceEndsAt, "who was playing gets the recovery window")
	controller.unlock()

	// P2 comes back only by rejoining, which this game does not support.
	rejected, participant, disconnected := controller.reattachPlayer(context.Background(), "user-b")
	require.True(t, participant)
	require.True(t, disconnected)
	require.Equal(t, runtime.AttachPlayerResult{}, rejected)
}

func TestRecoveryWindow(t *testing.T) {
	type test struct {
		// elapsed is how much platform time passes after the recovery.
		elapsed time.Duration
		// returns attaches P1 before the time passes.
		returns bool

		expectedDisconnected bool
	}

	tests := map[string]test{
		"participant_back_within_the_window_keeps_playing":    {elapsed: time.Minute, returns: true},
		"participant_away_inside_the_window_is_still_playing": {elapsed: 29 * time.Second},
		"participant_who_never_came_back_is_disconnected":     {elapsed: 30 * time.Second, expectedDisconnected: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			setup := newRecoverySetup(t)
			clock := &testClock{now: setup.now}
			mocks := newRecoveryMocks(gomock.NewController(t))
			gomock.InOrder(mocks.recovers(setup)...)
			controller := withManifest(newSessionController(context.Background(), setup.sessionUUID, 1, mocks.dependencies(clock.Now)), Manifest{MaxPlayers: 4})
			_, err := controller.recoverSession(context.Background())
			require.NoError(t, err)
			if tc.returns {
				_, participant, disconnected := controller.reattachPlayer(context.Background(), "user-a")
				require.True(t, participant)
				require.False(t, disconnected)
			}
			clock.Advance(tc.elapsed)

			work, ok := controller.takeBoundary()

			require.True(t, ok)
			require.Equal(t, tc.expectedDisconnected, controller.state.presence["P1"].disconnected)
			told := false
			for _, operation := range work.operations {
				told = told || (operation.Disconnected && operation.ParticipantID == "P1")
			}
			require.Equal(t, tc.expectedDisconnected, told, "playerDisconnected() reaches the game")
		})
	}
}

func TestConcurrentReconnectionsRecoverOnce(t *testing.T) {
	setup := newRecoverySetup(t)
	ctrl := gomock.NewController(t)
	mocks := newRecoveryMocks(ctrl)
	players, manifests := NewMockplayerJoinChecker(ctrl), NewMockmanifestLoader(ctrl)
	anyCtx := gomock.Any()
	// Each connection arriving before the recovery ended checks the durable
	// Session, one arriving after skips it as a participant's; only one
	// recovers it.
	players.EXPECT().EligibleToJoinAsPlayer(anyCtx, setup.sessionUUID).Return(sessionpkg.PlayerJoinSessionStarted, nil).MinTimes(1).MaxTimes(2)
	manifests.EXPECT().LoadRuntimeManifest(anyCtx, setup.sessionUUID).Return(sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 4}, nil)
	gomock.InOrder(mocks.recovers(setup)...)
	resolver := New(nil, players, manifests, mocks.limits, mocks.starts, mocks.ends, nil, heartbeatsRecorded(t, setup.sessionUUID), mocks.frontends, mocks.sandbox, mocks.replay, ownersReleasing(t, setup.sessionUUID, 1))
	resolver.deps.now = func() time.Time { return setup.now }
	t.Cleanup(resolver.Close)
	// The recovered context and recording end when the resolver stops.
	mocks.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil).MaxTimes(1)
	mocks.replay.EXPECT().StopRecording(testRecordingID).MaxTimes(1)

	var wg sync.WaitGroup
	results := make([]runtime.AttachPlayerResult, 2)
	errs := make([]error, 2)
	for i, userUUID := range []string{"user-a", "user-b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = resolver.AttachPlayer(context.Background(), setup.sessionUUID, userUUID, 1)
		}()
	}
	wg.Wait()

	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	require.Equal(t, runtime.AttachAccepted, results[0].Outcome, "who was playing comes back")
	require.Equal(t, runtime.AttachRejected, results[1].Outcome)
	require.Equal(t, runtime.RejectParticipantDisconnected, results[1].Rejection, "who was disconnected must rejoin")
	start, ok := startedWith(t, results[0].Connection.Started)
	require.True(t, ok)
	require.Equal(t, "P1", start.ParticipantID)
	view, ok := results[0].Connection.NextView(context.Background())
	require.True(t, ok)
	require.Equal(t, runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P1", Full: true, Revision: 1, State: []byte(`{"me":1}`)}}, view)
}

func TestAdminReconnectionRecovers(t *testing.T) {
	setup := newRecoverySetup(t)
	ctrl := gomock.NewController(t)
	mocks := newRecoveryMocks(ctrl)
	admins, manifests := NewMockadminJoinChecker(ctrl), NewMockmanifestLoader(ctrl)
	anyCtx := gomock.Any()
	gomock.InOrder(append([]any{
		admins.EXPECT().EligibleToJoinAsAdmin(anyCtx, setup.sessionUUID, "creator").Return(sessionpkg.AdminJoinEligibleRunning, nil),
		manifests.EXPECT().LoadRuntimeManifest(anyCtx, setup.sessionUUID).Return(sessionpkg.RuntimeManifest{MinPlayers: 1, MaxPlayers: 4}, nil),
	}, mocks.recovers(setup)...)...)
	resolver := New(admins, nil, manifests, mocks.limits, mocks.starts, mocks.ends, nil, heartbeatsRecorded(t, setup.sessionUUID), mocks.frontends, mocks.sandbox, mocks.replay, ownersReleasing(t, setup.sessionUUID, 1))
	resolver.deps.now = func() time.Time { return setup.now }
	t.Cleanup(resolver.Close)
	mocks.sandbox.EXPECT().DiscardContext(anyCtx, "context-1").Return(nil).MaxTimes(1)
	mocks.replay.EXPECT().StopRecording(testRecordingID).MaxTimes(1)

	result, err := resolver.AttachAdmin(context.Background(), setup.sessionUUID, "creator", 1)

	require.NoError(t, err)
	require.Equal(t, runtime.AttachAccepted, result.Outcome)
	start, ok := startedWith(t, result.Connection.Started)
	require.True(t, ok, "the admin sees the recovered game as started")
	require.Equal(t, []runtime.Participant{{ParticipantID: "P1", UserUUID: "user-a"}, {ParticipantID: "P2", UserUUID: "user-b"}}, start.Participants)
}
