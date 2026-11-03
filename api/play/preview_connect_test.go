package play

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/live"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

const (
	testPreviewUUID = "3fb3fd56-cf18-4026-84b8-92900bfb90ce"
	testOwnerUUID   = "0f8fad5b-d9cb-469f-a165-70867728950e"
)

// scriptedPreview is a preview the test drives: every operation is
// recorded, and views and the end go out through channels.
type scriptedPreview struct {
	outcome   runtime.AttachPreviewOutcome
	endReason string

	mu     sync.Mutex
	calls  []string
	views  chan runtime.OwnView
	ended  chan runtime.ConnectionEnd
	closed sync.Once
}

func newScriptedPreview(outcome runtime.AttachPreviewOutcome) *scriptedPreview {
	return &scriptedPreview{outcome: outcome, views: make(chan runtime.OwnView, 8), ended: make(chan runtime.ConnectionEnd, 1)}
}

func (p *scriptedPreview) record(call string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, call)
}

func (p *scriptedPreview) end(end runtime.ConnectionEnd) {
	p.closed.Do(func() {
		close(p.views)
		p.ended <- end
	})
}

func (p *scriptedPreview) AttachPreview(_ context.Context, previewUUID string, ownerUUID string) (runtime.AttachPreviewResult, error) {
	if p.outcome != runtime.PreviewAttached {
		return runtime.AttachPreviewResult{Outcome: p.outcome, EndReason: p.endReason}, nil
	}
	p.views <- runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: "P1", Full: true, Revision: 1, State: []byte(`{"me":"P1"}`)}}
	return runtime.AttachPreviewResult{
		Outcome: runtime.PreviewAttached,
		Start:   runtime.PreviewStart{PreviewUUID: previewUUID, Revision: 4, Participants: []string{"P1", "P2"}, ActingAs: "P1", Frontend: runtime.GameFrontend{URL: "https://storage/client.html", SHA256: "abc"}},
		Connection: runtime.PreviewConnection{
			ActAs: func(_ context.Context, participantID string) error {
				p.record("act_as " + participantID)
				if participantID == "P9" {
					return fmt.Errorf("%w: P9", runtime.ErrUnknownParticipant)
				}
				p.views <- runtime.OwnView{PlayerView: runtime.PlayerView{ParticipantID: participantID, Full: true, Revision: 2, State: []byte(`{"me":"` + participantID + `"}`)}}
				return nil
			},
			Sample: func(_ context.Context, sample runtime.Sample) error {
				p.record("sample")
				return fmt.Errorf("%w: boost is undeclared", runtime.ErrInvalidSample)
			},
			Command: func(_ context.Context, command runtime.Command) (runtime.CommandResult, error) {
				p.record("command " + command.Name)
				return runtime.CommandResult{Token: command.Token, Outcome: runtime.CommandPaused}, nil
			},
			Resync: func(context.Context) error { p.record("resync"); return nil },
			Pause:  func(context.Context) error { p.record("pause"); return nil },
			Resume: func(context.Context) error { p.record("resume"); return nil },
			End: func(context.Context) error {
				p.record("end")
				p.end(runtime.ConnectionEnd{Reason: runtime.EndSessionEnded, SessionStatus: "ENDED", EndCause: "ended_by_owner"})
				return nil
			},
			NextView: func(ctx context.Context) (runtime.OwnView, bool) {
				select {
				case view, ok := <-p.views:
					return view, ok
				case <-ctx.Done():
					return runtime.OwnView{}, false
				}
			},
			Ended: func(ctx context.Context) (runtime.ConnectionEnd, bool) {
				select {
				case end := <-p.ended:
					return end, true
				case <-ctx.Done():
					return runtime.ConnectionEnd{}, false
				}
			},
			Disconnect: func() { p.record("disconnect") },
		},
	}, nil
}

// connectPreview serves the preview endpoint for preview and dials it as
// the owner.
func connectPreview(t *testing.T, preview previewAttacher) (*websocket.Conn, context.Context) {
	t.Helper()
	handler := NewHandler(nil, nil, nil, nil, nil, nil, preview, live.New(0), nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.SetPathValue("previewUUID", testPreviewUUID)
		handler.handlePreviewConnect(w, r.WithContext(auth.WithCaller(r.Context(), identity.User{UUID: testOwnerUUID})))
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, server.URL, &websocket.DialOptions{Subprotocols: []string{"playhoot.v1"}})
	require.NoError(t, err)
	return conn, ctx
}

func readJSON(t *testing.T, ctx context.Context, conn *websocket.Conn) string {
	t.Helper()
	_, data, err := conn.Read(ctx)
	require.NoError(t, err)
	return string(data)
}

func TestHandlePreviewConnect(t *testing.T) {
	t.Run("plays_switches_pauses_and_ends", func(t *testing.T) {
		preview := newScriptedPreview(runtime.PreviewAttached)
		conn, ctx := connectPreview(t, preview)

		require.JSONEq(t, `{"type":"preview_started","preview_uuid":"`+testPreviewUUID+`","revision":4,"participants":["P1","P2"],"acting_as":"P1","paused":false,
			"frontend":{"url":"https://storage/client.html","sha256":"abc","expires_at":"0001-01-01T00:00:00Z","viewport":{"aspect_ratio":"","width":0,"height":0},"assets":[]}}`, readJSON(t, ctx, conn))
		require.JSONEq(t, `{"type":"player_view","participant_id":"P1","full":true,"revision":1,"state":{"me":"P1"},"consumed_through_sample_sequence":0}`, readJSON(t, ctx, conn))

		require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(`{"type":"act_as","participant_id":"P2"}`)))
		require.JSONEq(t, `{"type":"acting_as","participant_id":"P2"}`, readJSON(t, ctx, conn))
		require.JSONEq(t, `{"type":"player_view","participant_id":"P2","full":true,"revision":2,"state":{"me":"P2"},"consumed_through_sample_sequence":0}`, readJSON(t, ctx, conn))

		require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(`{"type":"act_as","participant_id":"P9"}`)))
		require.JSONEq(t, `{"type":"error","code":"unknown_participant","detail":"P9"}`, readJSON(t, ctx, conn))
		require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(`{"type":"pause"}`)))
		require.JSONEq(t, `{"type":"paused"}`, readJSON(t, ctx, conn))
		require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(`{"type":"command","token":"t1","name":"tap"}`)))
		require.JSONEq(t, `{"type":"command_result","token":"t1","outcome":"paused"}`, readJSON(t, ctx, conn))
		require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(`{"type":"sample","sequence":1,"inputs":{"boost":true}}`)))
		require.JSONEq(t, `{"type":"error","code":"invalid_game_data","detail":"invalid sample: boost is undeclared"}`, readJSON(t, ctx, conn), "a preview is told, not closed")
		require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(`{"type":"resume"}`)))
		require.JSONEq(t, `{"type":"resumed"}`, readJSON(t, ctx, conn))

		require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(`{"type":"end"}`)))
		require.JSONEq(t, `{"type":"preview_ended","reason":"ended_by_owner"}`, readJSON(t, ctx, conn))
		_, _, err := conn.Read(ctx)
		require.Equal(t, websocket.StatusNormalClosure, websocket.CloseStatus(err))
	})

	t.Run("anyone_but_the_owner_is_told_it_does_not_exist", func(t *testing.T) {
		conn, ctx := connectPreview(t, newScriptedPreview(runtime.PreviewNotFound))
		require.JSONEq(t, `{"type":"rejected","reason":"not_found"}`, readJSON(t, ctx, conn))
		_, _, err := conn.Read(ctx)
		require.Equal(t, websocket.StatusPolicyViolation, websocket.CloseStatus(err))
	})

	t.Run("an_ended_preview_tells_why", func(t *testing.T) {
		preview := newScriptedPreview(runtime.PreviewAlreadyEnded)
		preview.endReason = "replaced"
		conn, ctx := connectPreview(t, preview)
		require.JSONEq(t, `{"type":"preview_ended","reason":"replaced"}`, readJSON(t, ctx, conn))
		_, _, err := conn.Read(ctx)
		require.Equal(t, websocket.StatusNormalClosure, websocket.CloseStatus(err))
	})
}
