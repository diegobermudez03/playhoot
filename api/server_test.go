package api_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/api"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// syncBuffer is a bytes.Buffer safe for one goroutine to read (via
// String) while another writes (via Write) - slog's built-in handlers
// serialize concurrent writes against each other, but a plain
// bytes.Buffer still races if a test goroutine reads it directly while a
// connection's own background goroutine (still tearing down
// asynchronously) is writing through the same handler.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// TestCreateSession_NotImplemented proves the current transport-skeleton
// contract for POST /sessions: the wire request is validated, but nothing
// behind it exists yet - see api/session's doc comment for why.
func TestCreateSession_NotImplemented(t *testing.T) {
	srv := api.NewServer()
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/sessions", "application/json", strings.NewReader(`{"game_uuid":"g","host_user_uuid":"u","idempotency_key":"k"}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotImplemented, resp.StatusCode)
}

// TestWebSocket_UpgradesAndRejectsUnknownMessages proves the current
// transport-skeleton contract for GET /ws: the connection-upgrade
// mechanics are real and working (a client really gets a live socket),
// but no message type has a real implementation behind it yet - every
// message answers ERROR - see api/session's doc comment for why.
func TestWebSocket_UpgradesAndRejectsUnknownMessages(t *testing.T) {
	srv := api.NewServer()
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	wsQuery := url.Values{
		"join_code":       []string{"1"},
		"user_uuid":       []string{"user-1"},
		"display_name":    []string{"Alice"},
		"idempotency_key": []string{"join-1"},
	}
	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?" + wsQuery.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	joinResult := readWSMessage(t, conn)
	require.Equal(t, "JOIN_RESULT", joinResult["type"])

	require.NoError(t, conn.WriteJSON(map[string]any{"type": "ANSWER_INTERACTION"}))

	reply := readWSMessage(t, conn)
	require.Equal(t, "ERROR", reply["type"])
	require.Equal(t, "unknown message type", reply["message"])
}

// TestWebSocket_ObservabilityLogsShareOneTraceID proves the WebSocket
// observability wrapper's actual contract: it logs the connection opening
// immediately and its closing once the handler returns, and every log
// line this connection produces in between (the join handshake, each
// inbound message) shares that same trace id - the mechanism that lets
// them all be correlated later even though each is its own independently
// flushed structured log line.
func TestWebSocket_ObservabilityLogsShareOneTraceID(t *testing.T) {
	var buf syncBuffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	srv := api.NewServer()
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	wsQuery := url.Values{
		"join_code":       []string{"1"},
		"user_uuid":       []string{"user-1"},
		"display_name":    []string{"Alice"},
		"idempotency_key": []string{"join-1"},
	}
	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?" + wsQuery.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)

	readWSMessage(t, conn) // JOIN_RESULT
	require.NoError(t, conn.WriteJSON(map[string]any{"type": "ANSWER_INTERACTION"}))
	readWSMessage(t, conn) // ERROR reply
	require.NoError(t, conn.Close())

	// A WebSocket upgrade hijacks its connection away from net/http's own
	// bookkeeping, so a previous test's connection can still be tearing
	// down (and logging through this same process-global slog default)
	// after that test itself has already returned - grouping by trace id
	// means a stray leftover line from a different connection can never
	// be mistaken for one of this connection's own, rather than assuming
	// the buffer only ever contains this test's output.
	//
	// Eventually's own condition function runs on a goroutine testify
	// spawns internally, not this test's own goroutine - t.Fatal/require
	// must never be called from it (testing.T documents this), so
	// groupLogLinesByTraceID below skips anything it cannot parse instead
	// of failing; the real assertions happen afterward, on this test's
	// own goroutine, against whatever Eventually last captured.
	var found []map[string]any
	require.Eventually(t, func() bool {
		byTraceID := groupLogLinesByTraceID(buf.String())
		for _, entries := range byTraceID {
			if hasWSLifecycleEvent(entries, "opened") && hasWSLifecycleEvent(entries, "closed") {
				found = entries
				return true
			}
		}
		return false
	}, 2*time.Second, 10*time.Millisecond, "expected a trace id with both an opened and closed GET /ws log line; captured so far: %s", buf.String())

	require.Len(t, found, 4, "expected exactly opened, join, message, and closed log lines to share this connection's trace id; got: %#v", found)
}

// groupLogLinesByTraceID parses output (one JSON object per line) and
// groups entries by their trace_id field. A line that fails to parse, or
// carries no trace_id, is silently skipped rather than failing the
// caller - see this function's only caller for why.
func groupLogLinesByTraceID(output string) map[string][]map[string]any {
	byTraceID := map[string][]map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		traceID, ok := entry["trace_id"].(string)
		if !ok || traceID == "" {
			continue
		}
		byTraceID[traceID] = append(byTraceID[traceID], entry)
	}
	return byTraceID
}

func hasWSLifecycleEvent(entries []map[string]any, event string) bool {
	for _, entry := range entries {
		if entry["msg"] == "GET /ws" && entry["event"] == event {
			return true
		}
	}
	return false
}

func readWSMessage(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	var msg map[string]any
	require.NoError(t, conn.ReadJSON(&msg))
	return msg
}
