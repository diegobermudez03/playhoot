package liveconnection

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/internal/business/checkpoint"
	"github.com/diegobermudez03/playhoot/play/runtime/replay"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// testDependencies are a controller's dependencies with nothing to call, a
// fixed seed, and a tick and limits check that never come.
func testDependencies() *controllerDependencies {
	return &controllerDependencies{
		now:                time.Now,
		seeds:              func() uint64 { return 42 },
		tickInterval:       time.Hour,
		transportGrace:     3 * time.Second,
		recoveryWindow:     30 * time.Second,
		checkpointInterval: time.Hour,
		contextIdleTimeout: time.Hour,
		limitsInterval:     time.Hour,
		inactivityTimeout:  10 * time.Minute,
		logger:             slog.Default(),
	}
}

// ownersReleasing returns an owner releaser that may be asked once per epoch
// to release sessionUUID: a controller releases it when its main goroutine
// ends, which a test does not always wait for.
func ownersReleasing(t *testing.T, sessionUUID string, epochs ...int64) *MockownerReleaser {
	t.Helper()
	owners := NewMockownerReleaser(gomock.NewController(t))
	for _, epoch := range epochs {
		owners.EXPECT().ReleaseOwner(gomock.Any(), sessionUUID, epoch).Return(nil).MaxTimes(1)
	}
	return owners
}

// newTestController returns a fresh controller whose game allows maxPlayers,
// without its main goroutine: tests run its operations directly.
func newTestController(t *testing.T, maxPlayers int) *sessionController {
	t.Helper()
	return withManifest(newSessionController(context.Background(), "session-1", 1, testDependencies()), Manifest{MaxPlayers: maxPlayers})
}

// withManifest gives controller manifest as if an admission had read it.
func withManifest(controller *sessionController, manifest Manifest) *sessionController {
	controller.lock().manifest = manifest
	controller.unlock()
	controller.manifestLoaded = true
	return controller
}

// lobbySnapshot returns the users in the lobby, in order.
func (c *sessionController) lobbySnapshot() []string {
	s := c.lock()
	defer c.unlock()
	return append([]string{}, s.lobby...)
}

func (c *sessionController) currentPhase() phase {
	s := c.lock()
	defer c.unlock()
	return s.phase
}

// latestLobby drains the lobby updates next has ready and returns the last
// one's members, or nil when none was ready.
func latestLobby(t *testing.T, next func(context.Context) (runtime.LobbyUpdate, bool)) []string {
	t.Helper()
	var members []string
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		update, ok := next(ctx)
		cancel()
		if !ok {
			return members
		}
		members = []string{}
		for _, member := range update.Members {
			members = append(members, member.UserUUID)
		}
	}
}

func endedWith(t *testing.T, ended func(context.Context) (runtime.ConnectionEnd, bool)) runtime.EndReason {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	end, ok := ended(ctx)
	if !ok {
		return ""
	}
	return end.Reason
}

// requireClosed fails unless ch is ready to receive.
func requireClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("channel was not ready")
	}
}

func TestControllerFor(t *testing.T) {
	type test struct {
		// existingEpoch, when set, is the epoch of a controller already
		// registered for the Session.
		existingEpoch int64
		epoch         int64

		expectedOK bool
		// expectedReplaced means the existing controller was retired and a
		// new one returned.
		expectedReplaced bool
		// expectedCreations is how many controller creations the call under
		// test recorded.
		expectedCreations int
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	ctx := context.Background()

	tests := map[string]func(t *testing.T) test{
		"no_controller_creates_one_without_asking_anything": func(t *testing.T) test {
			return test{epoch: 1, expectedOK: true, expectedCreations: 1}
		},
		"existing_controller_under_the_same_epoch_is_reused": func(t *testing.T) test {
			return test{existingEpoch: 1, epoch: 1, expectedOK: true}
		},
		"older_epoch_than_the_existing_controller_is_refused": func(t *testing.T) test {
			return test{existingEpoch: 2, epoch: 1}
		},
		"newer_epoch_retires_and_replaces_the_existing_controller": func(t *testing.T) test {
			return test{existingEpoch: 1, epoch: 2, expectedOK: true, expectedReplaced: true, expectedCreations: 1}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			// Nothing to call: finding or creating a controller asks no one.
			resolver := New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, ownersReleasing(t, sessionUUID, 1, 2))
			t.Cleanup(resolver.Close)
			var existing *sessionController
			var existingAdmin runtime.AdminConnection
			var existingPlayer runtime.PlayerConnection
			if tc.existingEpoch != 0 {
				existing, _ = resolver.controllerFor(ctx, sessionUUID, tc.existingEpoch)
				withManifest(existing, Manifest{MaxPlayers: 6})
				existingAdmin = existing.registerAdmin("creator")
				existingPlayer, _, _ = existing.joinLobby(ctx, "player")
			}

			logCtx := logging.Start(ctx)
			controller, ok := resolver.controllerFor(logCtx, sessionUUID, tc.epoch)

			require.Equal(t, tc.expectedCreations, recordedCreations(t, logCtx))
			require.Equal(t, tc.expectedOK, ok)
			if !tc.expectedOK {
				require.Nil(t, controller)
				return
			}
			require.NotNil(t, controller)
			if existing == nil {
				return
			}
			if !tc.expectedReplaced {
				require.Same(t, existing, controller)
				return
			}
			require.NotSame(t, existing, controller)
			require.Equal(t, 0, existing.AdminCount())
			require.Equal(t, 0, existing.PlayerCount())
			for _, ended := range []func(context.Context) (runtime.ConnectionEnd, bool){existingAdmin.Ended, existingPlayer.Ended} {
				end, ok := ended(ctx)
				require.True(t, ok)
				require.Equal(t, runtime.ConnectionEnd{Reason: runtime.EndOwnerChanged}, end)
			}
			requireClosed(t, existing.exited)
			again, _ := resolver.controllerFor(ctx, sessionUUID, tc.epoch)
			require.Same(t, controller, again)
		})
	}
}

// recordedCreations counts the controller creations ctx's request log holds.
func recordedCreations(t *testing.T, ctx context.Context) int {
	t.Helper()
	var output bytes.Buffer
	logging.FinishRequestLog(ctx, slog.New(slog.NewJSONHandler(&output, nil)), "test")
	var entry struct {
		Metrics []struct {
			Metric string `json:"metric"`
		} `json:"metrics"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
	creations := 0
	for _, measurement := range entry.Metrics {
		if measurement.Metric == "play_runtime_controllers_created_total" {
			creations++
		}
	}
	return creations
}

func TestRun(t *testing.T) {
	// started returns a running controller with its main goroutine and its
	// one player, whose game runs in context-1.
	started := func(t *testing.T, sandboxMock *MockgameSandbox, owners *MockownerReleaser) (*sessionController, runtime.PlayerConnection, context.CancelFunc) {
		t.Helper()
		deps := testDependencies()
		deps.sandbox, deps.owners = sandboxMock, owners
		root, cancel := context.WithCancel(context.Background())
		controller := withManifest(newSessionController(root, "session-1", 1, deps), Manifest{MaxPlayers: 4})
		player, _, admitted := controller.joinLobby(context.Background(), "user-a")
		require.True(t, admitted)
		participants, _, ok := controller.beginStart()
		require.True(t, ok)
		controller.contextID = "context-1"
		controller.completeStart(participants, Game{ContextRevision: 1, State: []byte(`{}`)}, runtime.GameFrontend{}, map[string][]byte{"P1": []byte(`{}`)}, nil)
		_, ok = player.NextView(context.Background())
		require.True(t, ok, "the first FULL view")
		go controller.run()
		return controller, player, cancel
	}

	t.Run("flush_runs_a_boundary_without_waiting_for_the_tick", func(t *testing.T) {
		controller, player, cancel := started(t, NewMockgameSandbox(gomock.NewController(t)), ownersReleasing(t, "session-1", 1))
		defer cancel()
		require.NoError(t, player.Sample(context.Background(), runtime.Sample{Sequence: 1, Inputs: []byte(`{}`)}))

		controller.requestFlush()

		ctx, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		view, ok := player.NextView(ctx)
		require.True(t, ok)
		require.Equal(t, runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P1", Revision: 1, State: []byte(`{}`)}, ConsumedThroughSampleSequence: 1}, view)
	})
	t.Run("operations_run_in_order_on_the_main_goroutine", func(t *testing.T) {
		controller, _, cancel := started(t, NewMockgameSandbox(gomock.NewController(t)), ownersReleasing(t, "session-1", 1))
		defer cancel()
		var ran []int
		for i := range 3 {
			require.True(t, controller.do(context.Background(), func() { ran = append(ran, i) }))
		}
		require.Equal(t, []int{0, 1, 2}, ran)
	})
	t.Run("ended_session_releases_its_context_and_ownership", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		sandboxMock, owners := NewMockgameSandbox(ctrl), NewMockownerReleaser(ctrl)
		controller, _, cancel := started(t, sandboxMock, owners)
		defer cancel()
		gomock.InOrder(
			sandboxMock.EXPECT().DiscardContext(gomock.Any(), "context-1").Return(nil),
			owners.EXPECT().ReleaseOwner(gomock.Any(), "session-1", int64(1)).Return(nil),
		)

		controller.retire()

		requireClosed(t, controller.exited)
		require.False(t, controller.do(context.Background(), func() {}), "a stopped controller runs nothing")
	})
	t.Run("ended_session_stops_its_replay_recording", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		sandboxMock, owners, replayMock := NewMockgameSandbox(ctrl), NewMockownerReleaser(ctrl), NewMockreplayRecorder(ctrl)
		controller, _, cancel := started(t, sandboxMock, owners)
		defer cancel()
		controller.deps.replay = replayMock
		require.True(t, controller.do(context.Background(), func() { controller.recordingID = testRecordingID }))
		gomock.InOrder(
			sandboxMock.EXPECT().DiscardContext(gomock.Any(), "context-1").Return(nil),
			replayMock.EXPECT().StopRecording(testRecordingID),
			owners.EXPECT().ReleaseOwner(gomock.Any(), "session-1", int64(1)).Return(nil),
		)

		controller.retire()

		requireClosed(t, controller.exited)
	})
	t.Run("shutting_down_keeps_the_context_and_releases_ownership_for_the_next_owner", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		owners := NewMockownerReleaser(ctrl)
		controller, _, cancel := started(t, NewMockgameSandbox(ctrl), owners)
		owners.EXPECT().ReleaseOwner(gomock.Any(), "session-1", int64(1)).Return(nil)

		cancel()

		requireClosed(t, controller.exited)
	})
	t.Run("start_after_the_session_ended_is_not_in_lobby", func(t *testing.T) {
		controller := newTestController(t, 4)
		controller.deps.owners = ownersReleasing(t, "session-1", 1)
		go controller.run()
		controller.retire()
		requireClosed(t, controller.exited)

		result, err := controller.requestStart(context.Background(), "creator")

		require.NoError(t, err)
		require.Equal(t, runtime.StartSessionResult{Outcome: runtime.StartNotInLobby}, result)
	})
	t.Run("start_runs_on_the_main_goroutine_and_the_tick_begins_with_the_game", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		deps := testDependencies()
		starts := NewMocksessionStarter(ctrl)
		frontends := NewMockfrontendSigner(ctrl)
		sandboxMock := NewMockgameSandbox(ctrl)
		replayMock := NewMockreplayRecorder(ctrl)
		deps.starts, deps.frontends, deps.sandbox, deps.replay = starts, frontends, sandboxMock, replayMock
		deps.owners = ownersReleasing(t, "session-1", 1)
		// The recording stops with the main goroutine, which a test does not
		// always wait for.
		replayMock.EXPECT().StopRecording(testRecordingID).MaxTimes(1)
		deps.tickInterval = time.Millisecond
		root, cancel := context.WithCancel(context.Background())
		defer cancel()
		controller := withManifest(newSessionController(root, "session-1", 1, deps), Manifest{MaxPlayers: 4})
		player, _, _ := controller.joinLobby(context.Background(), "user-a")
		go controller.run()
		anyCtx := gomock.Any()
		script := []byte(`function start() {}`)
		encoded, err := checkpoint.Encode(checkpoint.Checkpoint{GameVersionUUID: "game-version", ContextRevision: 1, State: []byte(`{}`), Participants: []string{"P1"}})
		require.NoError(t, err)
		gomock.InOrder(
			starts.EXPECT().CheckStartEligibility(anyCtx, "session-1", "creator", 1).
				Return(sessionpkg.StartEligibility{Outcome: sessionpkg.StartEligible, GameVersionUUID: "game-version", Script: script}, nil),
			frontends.EXPECT().FrontendAccess(anyCtx, "game-version").Return(sessionpkg.FrontendAccess{}, nil),
			sandboxMock.EXPECT().CreateContext(anyCtx, sandbox.ContextSpec{GameVersionUUID: "game-version", Script: script}).Return("context-1", nil),
			sandboxMock.EXPECT().Start(anyCtx, "context-1", sandbox.StartInput{Participants: []string{"P1"}, RandomSeed: 42}).
				Return(sandbox.StartResult{Outcome: sandbox.ScriptSucceeded, State: []byte(`{}`), ContextRevision: 1}, nil),
			sandboxMock.EXPECT().Project(anyCtx, "context-1", "P1").Return(sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: []byte(`{}`)}, nil),
			starts.EXPECT().CommitStart(anyCtx, "session-1", "creator", sessionpkg.StartCommit{
				Participants: []sessionpkg.StartParticipant{{UserUUID: "user-a", ParticipantID: "P1"}},
				Checkpoint:   sessionpkg.Checkpoint{ContextRevision: 1, Content: encoded},
			}).Return(sessionpkg.CommitStartCommitted, nil),
			replayMock.EXPECT().StartRecording("session-1").Return(testRecordingID),
			replayMock.EXPECT().Offer(anyCtx, testRecordingID, replay.Capture{
				SessionUUID: "session-1", GameVersionUUID: "game-version", ContextRevision: 1, State: []byte(`{}`), Participants: []string{"P1"},
			}),
		)

		result, err := controller.registerAdmin("creator").StartSession(context.Background())

		require.NoError(t, err)
		require.Equal(t, runtime.StartSessionResult{Outcome: runtime.SessionStarted}, result)
		ctx, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_, ok := player.NextView(ctx)
		require.True(t, ok, "the first FULL view")
		require.NoError(t, player.Sample(ctx, runtime.Sample{Sequence: 1, Inputs: []byte(`{}`)}))
		view, ok := player.NextView(ctx)
		require.True(t, ok, "a tick acknowledges the SAMPLE without a flush")
		require.Equal(t, int64(1), view.ConsumedThroughSampleSequence)
	})
}
