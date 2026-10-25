package play

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/live"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/identity"
)

// limitRefusalCaller is the caller of every limit refusal test.
const limitRefusalCaller = "0f8fad5b-d9cb-469f-a165-70867728950e"

func TestLimitRefusals(t *testing.T) {
	type test struct {
		// maxConnections bounds the instance; 0 takes the default.
		maxConnections int
		// otherConnections are other callers' connections, and
		// callerConnections the caller's own, reserved first.
		otherConnections  int
		callerConnections int
		// callerRefusals are the caller's refusals already in progress.
		callerRefusals int
		// refuse is what answers the request: a refusal by the rate limit
		// handed to the route's refuser, or the upgrade's own room check.
		refuse func(h *Handler, w http.ResponseWriter, r *http.Request)

		// expectedRefusal is the status refusing the upgrade, if refused
		// before it, and expectedCode its error code.
		expectedRefusal    int
		expectedCode       string
		expectedRetryAfter string
		expectedReceived   string
	}

	rateLimited := func(h *Handler, w http.ResponseWriter, r *http.Request) {
		if !h.refuseLimit(w, r, routing.LimitRefusal{Reason: routing.RefusedRateLimited, RetryAfter: 2500 * time.Millisecond}) {
			// What the rate limit answers when the route cannot.
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
		}
	}
	upgrade := func(h *Handler, w http.ResponseWriter, r *http.Request) {
		if conn, release, ok := h.upgrade(w, r, limitRefusalCaller); ok {
			release()
			_ = conn.Close(websocket.StatusNormalClosure, "")
		}
	}

	tests := map[string]func(t *testing.T) test{
		"rate_limited_is_told_with_when_to_retry": func(t *testing.T) test {
			return test{
				refuse:           rateLimited,
				expectedReceived: `{"type":"rejected","reason":"rate_limited","retry_after_seconds":3}`,
			}
		},
		"caller_with_as_many_connections_as_allowed_is_told_too_many_connections": func(t *testing.T) test {
			return test{
				callerConnections: live.MaxConnectionsPerCaller,
				refuse:            upgrade,
				expectedReceived:  `{"type":"rejected","reason":"too_many_connections"}`,
			}
		},
		"full_instance_is_told_server_busy": func(t *testing.T) test {
			return test{
				maxConnections:   1,
				otherConnections: 1,
				refuse:           upgrade,
				expectedReceived: `{"type":"rejected","reason":"server_busy"}`,
			}
		},
		"rate_limited_without_room_to_refuse_is_refused_before_the_upgrade": func(t *testing.T) test {
			return test{
				callerRefusals:     live.MaxRefusalsPerCaller,
				refuse:             rateLimited,
				expectedRefusal:    http.StatusTooManyRequests,
				expectedRetryAfter: "3",
			}
		},
		"too_many_connections_without_room_to_refuse_is_refused_before_the_upgrade": func(t *testing.T) test {
			return test{
				callerConnections: live.MaxConnectionsPerCaller,
				callerRefusals:    live.MaxRefusalsPerCaller,
				refuse:            upgrade,
				expectedRefusal:   http.StatusTooManyRequests,
				expectedCode:      "too_many_connections",
			}
		},
		"server_busy_without_room_to_refuse_is_refused_before_the_upgrade": func(t *testing.T) test {
			return test{
				maxConnections:   1,
				otherConnections: 1,
				callerRefusals:   live.MaxRefusalsPerCaller,
				refuse:           upgrade,
				expectedRefusal:  http.StatusServiceUnavailable,
				expectedCode:     "server_busy",
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			connections := live.New(tc.maxConnections)
			for range tc.otherConnections {
				_, err := connections.Reserve(context.Background(), "other")
				require.NoError(t, err)
			}
			for range tc.callerConnections {
				_, err := connections.Reserve(context.Background(), limitRefusalCaller)
				require.NoError(t, err)
			}
			for range tc.callerRefusals {
				_, ok := connections.ReserveRefusal(context.Background(), limitRefusalCaller)
				require.True(t, ok)
			}
			handler := NewHandler(nil, nil, nil, nil, nil, nil, connections, nil)
			handlerDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(handlerDone)
				tc.refuse(handler, w, r.WithContext(auth.WithCaller(r.Context(), identity.User{UUID: limitRefusalCaller})))
			}))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, response, err := websocket.Dial(ctx, server.URL, &websocket.DialOptions{Subprotocols: []string{"playhoot.v1"}})
			if tc.expectedRefusal != 0 {
				require.Error(t, err)
				require.Equal(t, tc.expectedRefusal, response.StatusCode)
				require.Equal(t, tc.expectedRetryAfter, response.Header.Get("Retry-After"))
				if tc.expectedCode != "" {
					var body struct {
						Code string `json:"code"`
					}
					require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
					require.Equal(t, tc.expectedCode, body.Code)
				}
				return
			}
			require.NoError(t, err)
			_, data, err := conn.Read(ctx)
			require.NoError(t, err)
			require.JSONEq(t, tc.expectedReceived, string(data))
			_, _, err = conn.Read(ctx)
			var closed websocket.CloseError
			require.ErrorAs(t, err, &closed)
			require.Equal(t, websocket.StatusPolicyViolation, closed.Code)
			require.Equal(t, "admission rejected", closed.Reason)

			select {
			case <-handlerDone:
			case <-ctx.Done():
				t.Fatal("handler did not return")
			}
			for range live.MaxRefusalsPerCaller - tc.callerRefusals {
				_, ok := connections.ReserveRefusal(context.Background(), limitRefusalCaller)
				require.True(t, ok, "the refusal gave its room back")
			}
			for range live.MaxConnectionsPerCaller - tc.callerConnections {
				_, err := connections.Reserve(context.Background(), limitRefusalCaller)
				if tc.otherConnections == 0 {
					require.NoError(t, err, "the refusal holds no connection room")
				}
			}
		})
	}
}

func TestRefusalOfASilentClientEndsAtItsLinger(t *testing.T) {
	previous := refusalLinger
	refusalLinger = 200 * time.Millisecond
	t.Cleanup(func() { refusalLinger = previous })

	connections := live.New(0)
	handler := NewHandler(nil, nil, nil, nil, nil, nil, connections, nil)
	handlerDone := make(chan time.Time, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.refuseLimit(w, r.WithContext(auth.WithCaller(r.Context(), identity.User{UUID: limitRefusalCaller})), routing.LimitRefusal{Reason: routing.RefusedServerBusy})
		handlerDone <- time.Now()
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := time.Now()
	conn, _, err := websocket.Dial(ctx, server.URL, &websocket.DialOptions{Subprotocols: []string{"playhoot.v1"}})
	require.NoError(t, err)
	defer conn.CloseNow()
	_, data, err := conn.Read(ctx)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"rejected","reason":"server_busy"}`, string(data))
	// The client never reads again, so it never answers the server's close.

	select {
	case ended := <-handlerDone:
		require.Less(t, ended.Sub(started), time.Second, "the refusal ends at its linger, not at the close handshake's own timeouts")
	case <-ctx.Done():
		t.Fatal("refusal did not end")
	}
	for range live.MaxRefusalsPerCaller {
		_, ok := connections.ReserveRefusal(context.Background(), limitRefusalCaller)
		require.True(t, ok, "the refusal gave its room back")
	}
}

func TestOnlyLiveRoutesRefuseLimitsOnTheConnection(t *testing.T) {
	handler := NewHandler(nil, nil, nil, nil, nil, nil, live.New(0), nil)
	for _, route := range handler.WebSocketRoutes() {
		require.NotNil(t, route.RefuseLimit, route.Pattern)
	}
	for _, route := range handler.RESTRoutes() {
		require.Nil(t, route.RefuseLimit, route.Pattern)
	}
}
