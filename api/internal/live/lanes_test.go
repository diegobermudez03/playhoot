package live

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/api/internal/wsx"
)

func TestDispatch(t *testing.T) {
	type test struct {
		clientSends []string

		expectedCloseStatus websocket.StatusCode
		expectedHandled     map[string][]int
	}

	tests := map[string]func(t *testing.T) test{
		"each_type_keeps_its_order_without_waiting_for_others": func(t *testing.T) test {
			return test{
				clientSends: []string{
					// slow 1 is handled only after both fast messages behind it.
					`{"type":"slow","n":1}`,
					`{"type":"fast","n":1}`,
					`{"type":"fast","n":2}`,
					`{"type":"slow","n":2}`,
				},
				expectedCloseStatus: websocket.StatusNormalClosure,
				expectedHandled:     map[string][]int{"fast": {1, 2}, "slow": {1, 2}},
			}
		},
		"unknown_type_ends_the_connection": func(t *testing.T) test {
			return test{
				clientSends:         []string{`{"type":"other","n":1}`},
				expectedCloseStatus: websocket.StatusPolicyViolation,
				expectedHandled:     map[string][]int{},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			var mu sync.Mutex
			handled := map[string][]int{}
			fastHandled := make(chan struct{}, 2)
			record := func(messageType string, data []byte) int {
				var message struct {
					N int `json:"n"`
				}
				require.NoError(t, json.Unmarshal(data, &message))
				mu.Lock()
				defer mu.Unlock()
				handled[messageType] = append(handled[messageType], message.N)
				return message.N
			}

			served := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(served)
				conn, err := wsx.Accept(w, r, nil)
				if err != nil {
					return
				}
				fast := NewLane(func(_ context.Context, data []byte) (End, bool) {
					record("fast", data)
					fastHandled <- struct{}{}
					return End{}, false
				})
				slow := NewLane(func(ctx context.Context, data []byte) (End, bool) {
					if record("slow", data) == 2 {
						_ = conn.Close(websocket.StatusNormalClosure, "")
						return End{Cause: CauseEndedByOwner}, true
					}
					for range 2 {
						select {
						case <-fastHandled:
						case <-ctx.Done():
							return End{}, false
						}
					}
					return End{}, false
				})
				lanes := map[string]*Lane{"fast": fast, "slow": slow}
				serve(context.WithoutCancel(r.Context()), context.Background(), conn, func() {}, fast.Run, slow.Run, Dispatch(conn, lanes))
			}))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, server.URL, &websocket.DialOptions{Subprotocols: []string{wsx.Subprotocol}})
			require.NoError(t, err)
			for _, message := range tc.clientSends {
				require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(message)))
			}
			_, _, err = conn.Read(ctx)
			require.Equal(t, tc.expectedCloseStatus, websocket.CloseStatus(err))

			select {
			case <-served:
			case <-ctx.Done():
				t.Fatal("serve did not return")
			}
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, tc.expectedHandled, handled)
		})
	}
}
