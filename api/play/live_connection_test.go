package play

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/api/internal/wsx"
)

func TestServeLive(t *testing.T) {
	type clientMessage struct {
		messageType websocket.MessageType
		data        string
	}
	type test struct {
		// loops are the connection's loops, built once conn exists.
		loops        func(conn *websocket.Conn) []liveLoop
		shuttingDown bool
		clientSends  []clientMessage
		// clientCloses ends the connection from the client; otherwise the
		// client reads until the server ends it.
		clientCloses bool

		expectedCloseStatus websocket.StatusCode
		expectedCloseCause  closeCause
		expectedCloseDetail string
	}

	waitForCancel := func(ctx context.Context) (connectionEnd, bool) {
		<-ctx.Done()
		return connectionEnd{}, false
	}
	stopUndecided := func(context.Context) (connectionEnd, bool) {
		return connectionEnd{}, false
	}
	endAfter := func(conn *websocket.Conn, delay time.Duration) liveLoop {
		return func(context.Context) (connectionEnd, bool) {
			time.Sleep(delay)
			_ = conn.Close(websocket.StatusNormalClosure, "session_ended")
			return connectionEnd{cause: causeRuntimeEnded, detail: "session_ended"}, true
		}
	}
	readUntilEnd := func(conn *websocket.Conn) liveLoop {
		return func(ctx context.Context) (connectionEnd, bool) {
			for {
				messageType, _, end, ended := readMessage(ctx, conn)
				if ended || messageType == "" {
					return end, ended
				}
			}
		}
	}

	tests := map[string]func(t *testing.T) test{
		"first_loop_to_end_the_connection_stops_the_rest": func(t *testing.T) test {
			return test{
				loops: func(conn *websocket.Conn) []liveLoop {
					return []liveLoop{waitForCancel, endAfter(conn, 0)}
				},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedCloseCause:  causeRuntimeEnded,
				expectedCloseDetail: "session_ended",
			}
		},
		"loop_stopping_without_ending_does_not_end_the_connection": func(t *testing.T) test {
			return test{
				loops: func(conn *websocket.Conn) []liveLoop {
					return []liveLoop{stopUndecided, endAfter(conn, 20*time.Millisecond)}
				},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedCloseCause:  causeRuntimeEnded,
				expectedCloseDetail: "session_ended",
			}
		},
		"every_loop_stopping_without_ending_is_a_server_failure": func(t *testing.T) test {
			return test{
				loops:               func(*websocket.Conn) []liveLoop { return []liveLoop{stopUndecided} },
				expectedCloseStatus: websocket.StatusInternalError,
				expectedCloseCause:  causeServerFailure,
				expectedCloseDetail: "every loop stopped without ending the connection",
			}
		},
		"shutdown_ends_the_connection_as_going_away": func(t *testing.T) test {
			return test{
				loops:               func(*websocket.Conn) []liveLoop { return []liveLoop{waitForCancel} },
				shuttingDown:        true,
				expectedCloseStatus: websocket.StatusGoingAway,
				expectedCloseCause:  causeServerShutdown,
			}
		},
		"client_closing_ends_the_connection": func(t *testing.T) test {
			return test{
				loops:              func(conn *websocket.Conn) []liveLoop { return []liveLoop{readUntilEnd(conn), waitForCancel} },
				clientCloses:       true,
				expectedCloseCause: causeClientClosed,
			}
		},
		"malformed_client_message_ends_the_connection": func(t *testing.T) test {
			return test{
				loops:               func(conn *websocket.Conn) []liveLoop { return []liveLoop{readUntilEnd(conn)} },
				clientSends:         []clientMessage{{messageType: websocket.MessageText, data: `not json`}},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
				expectedCloseCause:  causeMalformedMessage,
			}
		},
		"message_without_a_type_is_malformed": func(t *testing.T) test {
			return test{
				loops:               func(conn *websocket.Conn) []liveLoop { return []liveLoop{readUntilEnd(conn)} },
				clientSends:         []clientMessage{{messageType: websocket.MessageText, data: `{"participant_id":"P1"}`}},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
				expectedCloseCause:  causeMalformedMessage,
			}
		},
		"binary_client_message_ends_the_connection": func(t *testing.T) test {
			return test{
				loops:               func(conn *websocket.Conn) []liveLoop { return []liveLoop{readUntilEnd(conn)} },
				clientSends:         []clientMessage{{messageType: websocket.MessageBinary, data: "\x01\x02"}},
				expectedCloseStatus: websocket.StatusUnsupportedData,
				expectedCloseCause:  causeBinaryFrame,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

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
				serveLive(context.WithoutCancel(r.Context()), shutdown, conn, func() { disconnects.Add(1) }, tc.loops(conn)...)
			}))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, server.URL, &websocket.DialOptions{Subprotocols: []string{wsx.Subprotocol}})
			require.NoError(t, err)

			for _, message := range tc.clientSends {
				require.NoError(t, conn.Write(ctx, message.messageType, []byte(message.data)))
			}
			if tc.clientCloses {
				require.NoError(t, conn.Close(websocket.StatusNormalClosure, ""))
			} else {
				_, _, err := conn.Read(ctx)
				require.Equal(t, tc.expectedCloseStatus, websocket.CloseStatus(err))
			}

			select {
			case <-served:
			case <-ctx.Done():
				t.Fatal("serveLive did not return")
			}
			require.Equal(t, int32(1), disconnects.Load())

			var record struct {
				Msg         string `json:"msg"`
				CloseCause  string `json:"close_cause"`
				CloseDetail string `json:"close_detail"`
			}
			require.NoError(t, json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record))
			require.Equal(t, "live connection closed", record.Msg)
			require.Equal(t, string(tc.expectedCloseCause), record.CloseCause)
			require.Equal(t, tc.expectedCloseDetail, record.CloseDetail)
		})
	}
}
