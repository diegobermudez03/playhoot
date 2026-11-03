package liveconnection

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const (
	testPreviewUUID = "3fb3fd56-cf18-4026-84b8-92900bfb90ce"
	testOwnerUUID   = "0f8fad5b-d9cb-469f-a165-70867728950e"
)

// tapGame is a sandbox running a tiny game: every "tap" adds one to its
// player, update() records the Session time it ran at, inputs are kept per
// player, and finishAt taps finish it.
type tapGame struct {
	mu       sync.Mutex
	state    tapState
	finishAt int
	spec     sandbox.ContextSpec
	revision int64
}

type tapState struct {
	Taps       map[string]int             `json:"taps"`
	Inputs     map[string]json.RawMessage `json:"inputs"`
	LastUpdate int64                      `json:"last_update"`
	Commands   []int64                    `json:"commands"`
}

func (g *tapGame) CreateContext(_ context.Context, spec sandbox.ContextSpec) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.spec = spec
	return "context-1", nil
}

func (g *tapGame) Start(_ context.Context, _ string, input sandbox.StartInput) (sandbox.StartResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.state = tapState{Taps: map[string]int{}, Inputs: map[string]json.RawMessage{}}
	for _, participant := range input.Participants {
		g.state.Taps[participant] = 0
	}
	g.revision = 1
	return sandbox.StartResult{Outcome: sandbox.ScriptSucceeded, State: g.encoded(), ContextRevision: g.revision}, nil
}

func (g *tapGame) Restore(context.Context, string, sandbox.Snapshot) (sandbox.RestoreResult, error) {
	return sandbox.RestoreResult{Outcome: sandbox.ScriptSucceeded}, nil
}

func (g *tapGame) Execute(_ context.Context, _ string, batch sandbox.Batch) (sandbox.ExecutionResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	result := sandbox.ExecutionResult{Outcome: sandbox.ScriptSucceeded}
	for i, operation := range batch.Operations {
		switch {
		case operation.Command != nil:
			g.state.Taps[operation.Command.ParticipantID]++
			g.state.Commands = append(g.state.Commands, operation.SessionTimeMs)
			result.CommandResponses = append(result.CommandResponses, []byte(`{"ok":true}`))
			if g.finishAt > 0 && g.state.Taps[operation.Command.ParticipantID] >= g.finishAt {
				result.Finished, result.FinishedOperation = true, i
			}
		case operation.Update:
			g.state.LastUpdate = operation.SessionTimeMs
		default:
			for _, transition := range operation.Transitions {
				g.state.Inputs[transition.ParticipantID] = json.RawMessage(transition.Inputs)
			}
		}
		if result.Finished {
			break
		}
	}
	g.revision++
	result.State, result.ContextRevision = g.encoded(), g.revision
	return result, nil
}

func (g *tapGame) Project(_ context.Context, _ string, participantID string) (sandbox.ProjectResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	view, _ := json.Marshal(map[string]any{"me": participantID, "taps": g.state.Taps, "inputs": g.state.Inputs[participantID]})
	return sandbox.ProjectResult{Outcome: sandbox.ScriptSucceeded, View: view}, nil
}

func (g *tapGame) DiscardContext(context.Context, string) error { return nil }

func (g *tapGame) encoded() []byte {
	encoded, _ := json.Marshal(g.state)
	return encoded
}

func (g *tapGame) snapshot() tapState {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.state
}

// recordedPreviews is a preview store answering opening and remembering how
// previews ended.
type recordedPreviews struct {
	mu      sync.Mutex
	opening sessionpkg.PreviewOpening
	preview sessionpkg.Preview
	ends    []sessionpkg.PreviewEndReason
}

func (r *recordedPreviews) OpenPreview(context.Context, string, string, int64, int) (sessionpkg.PreviewOpening, error) {
	return r.opening, nil
}

func (r *recordedPreviews) EndPreview(_ context.Context, _ string, reason sessionpkg.PreviewEndReason) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ends = append(r.ends, reason)
	return true, nil
}

func (r *recordedPreviews) FindPreview(context.Context, string) (sessionpkg.Preview, error) {
	return r.preview, nil
}

func (r *recordedPreviews) endedWith() []sessionpkg.PreviewEndReason {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]sessionpkg.PreviewEndReason(nil), r.ends...)
}

// manualClock is a clock the test moves.
type manualClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *manualClock) advance(by time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(by)
}

func previewOpening(players ...string) sessionpkg.PreviewOpening {
	return sessionpkg.PreviewOpening{
		Outcome: sessionpkg.PreviewOpened, PreviewUUID: testPreviewUUID, Revision: 7, Participants: players,
		GameVersionUUID: "version-1", Script: []byte("built script"), ScriptProfile: "sdk-v1",
		Manifest: sessionpkg.RuntimeManifest{
			MinPlayers: 1, MaxPlayers: 4, Update: true,
			Inputs:   []sessionpkg.RuntimeInput{{Name: "boost", Type: "boolean"}},
			Commands: []sessionpkg.RuntimeCommand{{Name: "tap"}},
		},
		Frontend:  sessionpkg.FrontendAccess{URL: "https://storage/client.html", SHA256: "abc", Viewport: sessionpkg.Viewport{AspectRatio: "16:9", Width: 1920, Height: 1080}},
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

// startedPreview starts a preview of game played by players and attaches
// the owner's connection.
func startedPreview(t *testing.T, game *tapGame, store *recordedPreviews, clock *manualClock, players ...string) (*Previews, runtime.AttachPreviewResult) {
	t.Helper()
	ctx := context.Background()
	store.opening = previewOpening(players...)
	store.preview = sessionpkg.Preview{UUID: testPreviewUUID, OwnerUserUUID: testOwnerUUID, ExpiresAt: clock.Now().Add(time.Hour)}
	previews := NewPreviews(0, game, store)
	previews.resolver.deps.now = clock.Now
	previews.resolver.deps.tickInterval = 5 * time.Millisecond
	t.Cleanup(previews.Close)

	started, err := previews.StartPreview(ctx, "version-1", testOwnerUUID, 0, len(players))
	require.NoError(t, err)
	require.Equal(t, runtime.PreviewStarted, started.Outcome)
	attached, err := previews.AttachPreview(ctx, testPreviewUUID, testOwnerUUID)
	require.NoError(t, err)
	require.Equal(t, runtime.PreviewAttached, attached.Outcome)
	return previews, attached
}

// nextViewOf returns the next view the connection gets for participantID,
// skipping others; there must be one within a second.
func nextViewOf(t *testing.T, connection runtime.PreviewConnection, participantID string, full bool) runtime.OwnView {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for {
		view, ok := connection.NextView(ctx)
		require.True(t, ok, "no view of %s", participantID)
		if view.ParticipantID == participantID && (!full || view.Full) {
			return view
		}
	}
}

func command(t *testing.T, connection runtime.PreviewConnection, token string) runtime.CommandResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := connection.Command(ctx, runtime.Command{Token: token, Name: "tap"})
	require.NoError(t, err)
	return result
}

func TestPreviewStartsPinnedToItsBuild(t *testing.T) {
	game, store, clock := &tapGame{}, &recordedPreviews{}, &manualClock{now: time.Now()}
	_, attached := startedPreview(t, game, store, clock, "P1", "P2")

	require.Equal(t, sandbox.ContextSpec{GameVersionUUID: "version-1", Script: []byte("built script"), Profile: "sdk-v1"}, game.spec)
	require.Equal(t, runtime.PreviewStart{
		PreviewUUID: testPreviewUUID, Revision: 7, Participants: []string{"P1", "P2"}, ActingAs: "P1",
		Frontend: runtime.GameFrontend{URL: "https://storage/client.html", SHA256: "abc", Viewport: runtime.GameViewport{AspectRatio: "16:9", Width: 1920, Height: 1080}, Assets: []runtime.GameAsset{}},
	}, attached.Start)
	first := nextViewOf(t, attached.Connection, "P1", true)
	require.JSONEq(t, `{"me":"P1","taps":{"P1":0,"P2":0},"inputs":null}`, string(first.State))
}

func TestPreviewActsAsOneParticipantAtATime(t *testing.T) {
	game, store, clock := &tapGame{}, &recordedPreviews{}, &manualClock{now: time.Now()}
	_, attached := startedPreview(t, game, store, clock, "P1", "P2")
	owner := attached.Connection
	nextViewOf(t, owner, "P1", true)

	require.NoError(t, owner.Sample(context.Background(), runtime.Sample{Sequence: 1, Inputs: []byte(`{"boost":true}`)}))
	require.Equal(t, runtime.CommandExecuted, command(t, owner, "t1").Outcome)
	require.NoError(t, owner.ActAs(context.Background(), "P2"))
	switched := nextViewOf(t, owner, "P2", true)
	require.JSONEq(t, `{"me":"P2","taps":{"P1":1,"P2":0},"inputs":null}`, string(switched.State))

	// SAMPLE sequences start again for the new participant.
	require.NoError(t, owner.Sample(context.Background(), runtime.Sample{Sequence: 1, Inputs: []byte(`{"boost":true}`)}))
	require.Equal(t, runtime.CommandExecuted, command(t, owner, "t2").Outcome)
	require.Eventually(t, func() bool {
		state := game.snapshot()
		return state.Taps["P2"] == 1 && string(state.Inputs["P1"]) == `{"boost":false}` && string(state.Inputs["P2"]) == `{"boost":true}`
	}, time.Second, 5*time.Millisecond, "the previous participant's inputs are let go when switching")

	require.ErrorIs(t, owner.ActAs(context.Background(), "P9"), runtime.ErrUnknownParticipant)
}

func TestPreviewPauseFreezesSessionTime(t *testing.T) {
	game, store, clock := &tapGame{}, &recordedPreviews{}, &manualClock{now: time.Now()}
	_, attached := startedPreview(t, game, store, clock, "P1")
	owner := attached.Connection
	nextViewOf(t, owner, "P1", true)

	clock.advance(time.Second)
	require.Equal(t, runtime.CommandExecuted, command(t, owner, "before").Outcome)
	require.NoError(t, owner.Pause(context.Background()))
	clock.advance(5 * time.Second)
	require.Equal(t, runtime.CommandPaused, command(t, owner, "paused").Outcome)
	require.NoError(t, owner.Sample(context.Background(), runtime.Sample{Sequence: 1, Inputs: []byte(`{"boost":true}`)}))
	time.Sleep(30 * time.Millisecond)
	require.NoError(t, owner.Resume(context.Background()))
	clock.advance(time.Second)
	require.Equal(t, runtime.CommandExecuted, command(t, owner, "after").Outcome)

	state := game.snapshot()
	require.Equal(t, []int64{1000, 2000}, state.Commands, "the pause is not Session time")
	require.LessOrEqual(t, state.LastUpdate, int64(2000))
	require.Nil(t, state.Inputs["P1"], "a SAMPLE while paused is dropped")
}

func TestPreviewEnds(t *testing.T) {
	ctx := context.Background()

	t.Run("finished_after_the_final_view", func(t *testing.T) {
		game, store, clock := &tapGame{finishAt: 1}, &recordedPreviews{}, &manualClock{now: time.Now()}
		_, attached := startedPreview(t, game, store, clock, "P1")
		owner := attached.Connection
		nextViewOf(t, owner, "P1", true)
		command(t, owner, "t1")
		final := nextViewOf(t, owner, "P1", false)
		require.Contains(t, string(final.State), `"P1":1`)
		end, ok := owner.Ended(ctx)
		require.True(t, ok)
		require.Equal(t, runtime.ConnectionEnd{Reason: runtime.EndSessionEnded, SessionStatus: previewEndedStatus, EndCause: string(sessionpkg.PreviewEndFinished)}, end)
		require.Equal(t, []sessionpkg.PreviewEndReason{sessionpkg.PreviewEndFinished}, store.endedWith())
	})

	t.Run("idle_after_the_inactivity_timeout", func(t *testing.T) {
		game, store, clock := &tapGame{}, &recordedPreviews{}, &manualClock{now: time.Now()}
		store.opening = previewOpening("P1")
		store.preview = sessionpkg.Preview{UUID: testPreviewUUID, OwnerUserUUID: testOwnerUUID, ExpiresAt: clock.Now().Add(time.Hour)}
		previews := NewPreviews(0, game, store)
		previews.resolver.deps.now = clock.Now
		previews.resolver.deps.limitsInterval = 5 * time.Millisecond
		t.Cleanup(previews.Close)
		_, err := previews.StartPreview(ctx, "version-1", testOwnerUUID, 0, 1)
		require.NoError(t, err)
		attached, err := previews.AttachPreview(ctx, testPreviewUUID, testOwnerUUID)
		require.NoError(t, err)

		clock.advance(defaultInactivityTimeout)
		end, ok := attached.Connection.Ended(ctx)
		require.True(t, ok)
		require.Equal(t, string(sessionpkg.PreviewEndIdle), end.EndCause)
	})

	t.Run("by_its_owner", func(t *testing.T) {
		game, store, clock := &tapGame{}, &recordedPreviews{}, &manualClock{now: time.Now()}
		_, attached := startedPreview(t, game, store, clock, "P1")
		require.NoError(t, attached.Connection.End(ctx))
		end, _ := attached.Connection.Ended(ctx)
		require.Equal(t, string(sessionpkg.PreviewEndEndedByOwner), end.EndCause)
	})
}

func TestPreviewAttach(t *testing.T) {
	ctx := context.Background()

	t.Run("a_new_connection_replaces_the_attached_one", func(t *testing.T) {
		game, store, clock := &tapGame{}, &recordedPreviews{}, &manualClock{now: time.Now()}
		previews, first := startedPreview(t, game, store, clock, "P1", "P2")
		require.NoError(t, first.Connection.ActAs(ctx, "P2"))
		second, err := previews.AttachPreview(ctx, testPreviewUUID, testOwnerUUID)
		require.NoError(t, err)
		require.Equal(t, "P2", second.Start.ActingAs, "a reattach plays as the participant last acted as")
		end, _ := first.Connection.Ended(ctx)
		require.Equal(t, runtime.EndReplaced, end.Reason)
	})

	t.Run("someone_else_is_told_it_does_not_exist", func(t *testing.T) {
		game, store, clock := &tapGame{}, &recordedPreviews{}, &manualClock{now: time.Now()}
		previews, _ := startedPreview(t, game, store, clock, "P1")
		other, err := previews.AttachPreview(ctx, testPreviewUUID, "7c9e6679-7425-40de-944b-e07fc1f90ae7")
		require.NoError(t, err)
		require.Equal(t, runtime.PreviewNotFound, other.Outcome)
	})

	t.Run("a_preview_nothing_runs_is_told_ended_as_unavailable", func(t *testing.T) {
		store := &recordedPreviews{preview: sessionpkg.Preview{UUID: testPreviewUUID, OwnerUserUUID: testOwnerUUID}}
		previews := NewPreviews(0, &tapGame{}, store)
		t.Cleanup(previews.Close)
		result, err := previews.AttachPreview(ctx, testPreviewUUID, testOwnerUUID)
		require.NoError(t, err)
		require.Equal(t, runtime.AttachPreviewResult{Outcome: runtime.PreviewAlreadyEnded, EndReason: string(sessionpkg.PreviewEndUnavailable)}, result)
		require.Equal(t, []sessionpkg.PreviewEndReason{sessionpkg.PreviewEndUnavailable}, store.endedWith())
	})

	t.Run("an_ended_preview_tells_why", func(t *testing.T) {
		store := &recordedPreviews{preview: sessionpkg.Preview{UUID: testPreviewUUID, OwnerUserUUID: testOwnerUUID, Ended: true, EndReason: sessionpkg.PreviewEndReplaced}}
		previews := NewPreviews(0, &tapGame{}, store)
		t.Cleanup(previews.Close)
		result, err := previews.AttachPreview(ctx, testPreviewUUID, testOwnerUUID)
		require.NoError(t, err)
		require.Equal(t, string(sessionpkg.PreviewEndReplaced), result.EndReason)
	})
}

func TestStartPreviewRefusals(t *testing.T) {
	ctx := context.Background()
	store := &recordedPreviews{opening: sessionpkg.PreviewOpening{Outcome: sessionpkg.PreviewInvalidPlayers, MinPlayers: 2, MaxPlayers: 4}}
	previews := NewPreviews(0, &tapGame{}, store)
	t.Cleanup(previews.Close)

	result, err := previews.StartPreview(ctx, "version-1", testOwnerUUID, 0, 9)
	require.NoError(t, err)
	require.Equal(t, runtime.StartPreviewResult{Outcome: runtime.PreviewInvalidPlayers, MinPlayers: 2, MaxPlayers: 4}, result)

	store.opening = sessionpkg.PreviewOpening{Outcome: sessionpkg.PreviewBuildNotReady, BuildStatus: sessionpkg.DraftBuildFailed}
	result, err = previews.StartPreview(ctx, "version-1", testOwnerUUID, 0, 1)
	require.NoError(t, err)
	require.Equal(t, runtime.StartPreviewResult{Outcome: runtime.PreviewBuildNotReady, BuildStatus: string(sessionpkg.DraftBuildFailed)}, result)
}
