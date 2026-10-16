package wsx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestServe(t *testing.T) {
	type clientMessage struct {
		messageType websocket.MessageType
		data        string
	}
	type test struct {
		// outbound is what the Runtime has written before the client
		// connects; closeOutbound then closes the channel.
		outbound      []runtime.ServerMessage
		closeOutbound bool
		clientSends   []clientMessage
		// clientCloses ends the connection from the client once its
		// messages are sent; otherwise the client reads until the server
		// ends it.
		clientCloses bool

		expectedReceived    []string
		expectedCloseStatus websocket.StatusCode
	}

	tests := map[string]func(t *testing.T) test{
		"messages_are_delivered_in_order_then_close_ends_the_connection": func(t *testing.T) test {
			return test{
				outbound: []runtime.ServerMessage{
					runtime.Rejected{Reason: runtime.RejectNotCreator},
					runtime.Rejected{Reason: runtime.RejectOwnerChanged},
					runtime.Close{Reason: runtime.CloseAdmissionRejected},
				},
				closeOutbound: true,
				expectedReceived: []string{
					`{"type":"rejected","reason":"not_creator"}`,
					`{"type":"rejected","reason":"owner_changed"}`,
				},
				expectedCloseStatus: websocket.StatusPolicyViolation,
			}
		},
		"close_with_an_unknown_reason_is_a_server_error": func(t *testing.T) test {
			return test{
				outbound:            []runtime.ServerMessage{runtime.Close{Reason: "something_new"}},
				expectedCloseStatus: websocket.StatusInternalError,
			}
		},
		"outbound_closed_without_close_while_connected_is_a_server_error": func(t *testing.T) test {
			return test{
				closeOutbound:       true,
				expectedCloseStatus: websocket.StatusInternalError,
			}
		},
		"unencodable_message_is_a_server_error": func(t *testing.T) test {
			return test{
				outbound:            []runtime.ServerMessage{nil},
				expectedCloseStatus: websocket.StatusInternalError,
			}
		},
		"client_closing_ends_the_connection": func(t *testing.T) test {
			return test{clientCloses: true}
		},
		"unknown_client_message_type_ends_the_connection": func(t *testing.T) test {
			return test{
				clientSends:         []clientMessage{{messageType: websocket.MessageText, data: `{"type":"teleport"}`}},
				expectedCloseStatus: websocket.StatusPolicyViolation,
			}
		},
		"malformed_client_message_ends_the_connection": func(t *testing.T) test {
			return test{
				clientSends:         []clientMessage{{messageType: websocket.MessageText, data: `not json`}},
				expectedCloseStatus: websocket.StatusInvalidFramePayloadData,
			}
		},
		"oversized_client_message_ends_the_connection": func(t *testing.T) test {
			return test{
				clientSends:         []clientMessage{{messageType: websocket.MessageText, data: `{"type":"` + strings.Repeat("x", maxMessageBytes) + `"}`}},
				expectedCloseStatus: websocket.StatusMessageTooBig,
			}
		},
		"binary_client_message_ends_the_connection": func(t *testing.T) test {
			return test{
				clientSends:         []clientMessage{{messageType: websocket.MessageBinary, data: "\x01\x02"}},
				expectedCloseStatus: websocket.StatusUnsupportedData,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			inbound := make(chan runtime.ClientMessage, 1)
			outbound := make(chan runtime.ServerMessage, len(tc.outbound)+1)
			for _, message := range tc.outbound {
				outbound <- message
			}
			if tc.closeOutbound {
				close(outbound)
			}

			served := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(served)
				conn, err := Accept(w, r, nil)
				if err != nil {
					return
				}
				Serve(r.Context(), conn, runtime.Connection{Inbound: inbound, Outbound: outbound})
			}))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, server.URL, &websocket.DialOptions{Subprotocols: []string{Subprotocol}})
			require.NoError(t, err)

			for _, message := range tc.clientSends {
				require.NoError(t, conn.Write(ctx, message.messageType, []byte(message.data)))
			}
			var received []string
			if tc.clientCloses {
				require.NoError(t, conn.Close(websocket.StatusNormalClosure, ""))
			} else {
				for {
					_, data, err := conn.Read(ctx)
					if err != nil {
						require.Equal(t, tc.expectedCloseStatus, websocket.CloseStatus(err))
						break
					}
					received = append(received, string(data))
				}
			}
			require.Equal(t, tc.expectedReceived, received)

			select {
			case <-served:
			case <-ctx.Done():
				t.Fatal("Serve did not return")
			}
			_, open := <-inbound
			require.False(t, open, "Inbound must be closed once Serve returns")
		})
	}
}
