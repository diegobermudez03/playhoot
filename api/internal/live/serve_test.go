package live

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/api/internal/wsx"
)

func TestServe(t *testing.T) {
	type clientMessage struct {
		messageType websocket.MessageType
		data        string
	}
	type test struct {
		// loops are the connection's loops, built once conn exists.
		loops        func(conn *websocket.Conn) []Loop
		shuttingDown bool
		clientSends  []clientMessage
		// clientCloses ends the connection from the client; otherwise the
		// client reads until the server ends it, unless clientSilent, when it
		// never reads, so it answers no ping and takes no message.
		clientCloses bool
		clientSilent bool
		// pingEvery, pongWithin and writeWithin replace the keepalive and
		// write timings.
		pingEvery   time.Duration
		pongWithin  time.Duration
		writeWithin time.Duration

		expectedCloseStatus websocket.StatusCode
		expectedCloseCause  Cause
		expectedCloseDetail string
	}

	waitForCancel := func(ctx context.Context) (End, bool) {
		<-ctx.Done()
		return End{}, false
	}
	stopUndecided := func(context.Context) (End, bool) {
		return End{}, false
	}
	endAfter := func(conn *websocket.Conn, delay time.Duration) Loop {
		return func(context.Context) (End, bool) {
			time.Sleep(delay)
			_ = conn.Close(websocket.StatusNormalClosure, "session_ended")
			return End{Cause: CauseEndedByOwner, Detail: "session_ended"}, true
		}
	}
	// readQuietly reads, so pings are answered, without ever ending the
	// connection itself.
	readQuietly := func(conn *websocket.Conn) Loop {
		return func(ctx context.Context) (End, bool) {
			for {
				if _, _, err := conn.Read(ctx); err != nil {
					return End{}, false
				}
			}
		}
	}
	readUntilEnd := func(conn *websocket.Conn) Loop {
		return func(ctx context.Context) (End, bool) {
			for {
				messageType, _, end, ended := ReadMessage(ctx, conn)
				if ended || messageType == "" {
					return end, ended
				}
			}
		}
	}

	tests := map[string]func(t *testing.T) test{
		"first_loop_to_end_the_connection_stops_the_rest": func(t *testing.T) test {
			return test{
				loops: func(conn *websocket.Conn) []Loop {
					return []Loop{waitForCancel, endAfter(conn, 0)}
				},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedCloseCause:  CauseEndedByOwner,
				expectedCloseDetail: "session_ended",
			}
		},
		"loop_stopping_without_ending_does_not_end_the_connection": func(t *testing.T) test {
			return test{
				loops: func(conn *websocket.Conn) []Loop {
					return []Loop{stopUndecided, endAfter(conn, 20*time.Millisecond)}
				},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedCloseCause:  CauseEndedByOwner,
				expectedCloseDetail: "session_ended",
			}
		},
		"every_loop_stopping_without_ending_is_a_server_failure": func(t *testing.T) test {
			return test{
				loops:               func(*websocket.Conn) []Loop { return []Loop{stopUndecided} },
				expectedCloseStatus: websocket.StatusInternalError,
				expectedCloseCause:  CauseServerFailure,
				expectedCloseDetail: "every loop stopped without ending the connection",
			}
		},
		"shutdown_ends_the_connection_as_going_away": func(t *testing.T) test {
			return test{
				loops:               func(*websocket.Conn) []Loop { return []Loop{waitForCancel} },
				shuttingDown:        true,
				expectedCloseStatus: websocket.StatusGoingAway,
				expectedCloseCause:  CauseServerShutdown,
			}
		},
		"client_closing_ends_the_connection": func(t *testing.T) test {
			return test{
				loops:              func(conn *websocket.Conn) []Loop { return []Loop{readUntilEnd(conn), waitForCancel} },
				clientCloses:       true,
				expectedCloseCause: CauseClientClosed,
			}
		},
		"malformed_client_message_ends_the_connection": func(t *testing.T) test {
			return test{
				loops:               func(conn *websocket.Conn) []Loop { return []Loop{readUntilEnd(conn)} },
				clientSends:         []clientMessage{{messageType: websocket.MessageText, data: `not json`}},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
				expectedCloseCause:  CauseMalformedMessage,
			}
		},
		"message_without_a_type_is_malformed": func(t *testing.T) test {
			return test{
				loops:               func(conn *websocket.Conn) []Loop { return []Loop{readUntilEnd(conn)} },
				clientSends:         []clientMessage{{messageType: websocket.MessageText, data: `{"participant_id":"P1"}`}},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
				expectedCloseCause:  CauseMalformedMessage,
			}
		},
		"client_answering_pings_stays_connected": func(t *testing.T) test {
			return test{
				loops: func(conn *websocket.Conn) []Loop {
					return []Loop{readQuietly(conn), endAfter(conn, 300*time.Millisecond)}
				},
				pingEvery:           20 * time.Millisecond,
				pongWithin:          100 * time.Millisecond,
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedCloseCause:  CauseEndedByOwner,
				expectedCloseDetail: "session_ended",
			}
		},
		"client_not_answering_pings_is_unresponsive": func(t *testing.T) test {
			return test{
				loops:              func(conn *websocket.Conn) []Loop { return []Loop{readUntilEnd(conn)} },
				clientSilent:       true,
				pingEvery:          20 * time.Millisecond,
				pongWithin:         50 * time.Millisecond,
				expectedCloseCause: CauseUnresponsive,
			}
		},
		"client_not_taking_a_message_in_time_is_too_slow": func(t *testing.T) test {
			huge := strings.Repeat("x", 64<<20)
			return test{
				loops: func(conn *websocket.Conn) []Loop {
					return []Loop{readUntilEnd(conn), func(ctx context.Context) (End, bool) {
						// The socket's buffers take a few before the client is
						// seen not taking them.
						for {
							if end, ended := WriteMessage(ctx, conn, huge); ended || ctx.Err() != nil {
								return end, ended
							}
						}
					}}
				},
				clientSilent:       true,
				writeWithin:        100 * time.Millisecond,
				expectedCloseCause: CauseSlowConsumer,
			}
		},
		"binary_client_message_ends_the_connection": func(t *testing.T) test {
			return test{
				loops:               func(conn *websocket.Conn) []Loop { return []Loop{readUntilEnd(conn)} },
				clientSends:         []clientMessage{{messageType: websocket.MessageBinary, data: "\x01\x02"}},
				expectedCloseStatus: websocket.StatusUnsupportedData,
				expectedCloseCause:  CauseBinaryFrame,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			if tc.pingEvery > 0 {
				previousInterval, previousTimeout := pingInterval, pongTimeout
				pingInterval, pongTimeout = tc.pingEvery, tc.pongWithin
				t.Cleanup(func() { pingInterval, pongTimeout = previousInterval, previousTimeout })
			}
			if tc.writeWithin > 0 {
				previous := writeTimeout
				writeTimeout = tc.writeWithin
				t.Cleanup(func() { writeTimeout = previous })
			}

			var output bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })

			shutdown, signalShutdown := context.WithCancel(context.Background())
			defer signalShutdown()
			if tc.shuttingDown {
				signalShutdown()
			}
			var disconnects atomic.Int32

			served := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(served)
				conn, err := wsx.Accept(w, r, nil)
				if err != nil {
					return
				}
				serve(context.WithoutCancel(r.Context()), shutdown, conn, func() { disconnects.Add(1) }, tc.loops(conn)...)
			}))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, server.URL, &websocket.DialOptions{Subprotocols: []string{wsx.Subprotocol}})
			require.NoError(t, err)

			for _, message := range tc.clientSends {
				require.NoError(t, conn.Write(ctx, message.messageType, []byte(message.data)))
			}
			switch {
			case tc.clientCloses:
				require.NoError(t, conn.Close(websocket.StatusNormalClosure, ""))
			case tc.clientSilent:
				defer conn.CloseNow()
			default:
				_, _, err := conn.Read(ctx)
				require.Equal(t, tc.expectedCloseStatus, websocket.CloseStatus(err))
			}

			select {
			case <-served:
			case <-ctx.Done():
				t.Fatal("Serve did not return")
			}
			require.Equal(t, int32(1), disconnects.Load())

			var record struct {
				Msg         string           `json:"msg"`
				CloseCause  string           `json:"close_cause"`
				CloseDetail string           `json:"close_detail"`
				Metrics     []map[string]any `json:"metrics"`
			}
			require.NoError(t, json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record))
			require.Equal(t, "live connection closed", record.Msg)
			require.Equal(t, string(tc.expectedCloseCause), record.CloseCause)
			require.Equal(t, tc.expectedCloseDetail, record.CloseDetail)
			require.Len(t, record.Metrics, 3)
			require.Equal(t, map[string]any{"metric": "api_live_connections_active", "operation": "add", "value": float64(-1), "labels": map[string]any{}}, record.Metrics[0])
			require.Equal(t, map[string]any{
				"metric": "api_live_connections_closed_total", "operation": "add", "value": float64(1),
				"labels": map[string]any{"close_cause": string(tc.expectedCloseCause)},
			}, record.Metrics[1])
			require.Equal(t, "api_live_connection_duration_seconds", record.Metrics[2]["metric"])
		})
	}
}
