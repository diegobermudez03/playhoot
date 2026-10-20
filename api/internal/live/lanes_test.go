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
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

func TestDispatch(t *testing.T) {
	type test struct {
		clientSends      []string
		connectionLimits ConnectionLimits

		expectedCloseStatus websocket.StatusCode
		// expectedHandled is skipped when nil: a connection closed by its
		// reader may stop its lanes before or after they handle a message.
		expectedHandled map[string][]int
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
		"more_messages_than_the_connection_allows_end_it": func(t *testing.T) test {
			return test{
				clientSends:         []string{`{"type":"fast","n":1}`, `{"type":"fast","n":2}`, `{"type":"fast","n":3}`},
				connectionLimits:    ConnectionLimits{Messages: []ratelimit.Limit{{Count: 2, Per: time.Hour}}},
				expectedCloseStatus: websocket.StatusPolicyViolation,
			}
		},
		"more_bytes_than_the_connection_allows_end_it": func(t *testing.T) test {
			return test{
				clientSends:         []string{`{"type":"fast","n":1}`, `{"type":"fast","n":2}`},
				connectionLimits:    ConnectionLimits{Bytes: []ratelimit.Limit{{Count: 30, Per: time.Hour}}},
				expectedCloseStatus: websocket.StatusPolicyViolation,
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
					select {
					case fastHandled <- struct{}{}:
					default:
					}
					return End{}, false
				}, LaneLimits{Capacity: 8})
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
				}, LaneLimits{Capacity: 8})
				lanes := map[string]*Lane{"fast": fast, "slow": slow}
				serve(context.WithoutCancel(r.Context()), context.Background(), conn, func() {}, fast.Run, slow.Run, Dispatch(conn, lanes, tc.connectionLimits))
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
			if tc.expectedHandled == nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, tc.expectedHandled, handled)
		})
	}
}

func TestLaneOffer(t *testing.T) {
	type test struct {
		limits LaneLimits
		offers int
		// excessEnds makes the lane's Excess end the connection.
		excessEnds bool

		expectedPending int
		expectedExcess  int
		expectedEnded   bool
	}

	tests := map[string]func(t *testing.T) test{
		"within_its_limits_every_message_waits_in_order": func(t *testing.T) test {
			return test{
				limits:          LaneLimits{Rate: []ratelimit.Limit{{Count: 3, Per: time.Hour}}, Capacity: 3},
				offers:          3,
				expectedPending: 3,
			}
		},
		"beyond_its_rate_a_message_is_dropped_without_excess": func(t *testing.T) test {
			return test{
				limits:          LaneLimits{Rate: []ratelimit.Limit{{Count: 2, Per: time.Hour}}, Capacity: 10},
				offers:          4,
				expectedPending: 2,
			}
		},
		"beyond_its_capacity_a_message_goes_to_excess": func(t *testing.T) test {
			return test{
				limits:          LaneLimits{Capacity: 1},
				offers:          3,
				expectedPending: 1,
				expectedExcess:  2,
			}
		},
		"an_excess_that_ends_the_connection_ends_it": func(t *testing.T) test {
			return test{
				limits:          LaneLimits{Rate: []ratelimit.Limit{{Count: 1, Per: time.Hour}}, Capacity: 10},
				offers:          2,
				excessEnds:      true,
				expectedPending: 1,
				expectedExcess:  1,
				expectedEnded:   true,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			excess := 0
			limits := tc.limits
			if tc.expectedExcess > 0 {
				limits.Excess = func(context.Context, []byte) (End, bool) {
					excess++
					return End{Cause: CauseRateLimited}, tc.excessEnds
				}
			}
			lane := NewLane(func(context.Context, []byte) (End, bool) { return End{}, false }, limits)

			ended := false
			now := time.Now()
			for i := range tc.offers {
				if _, ended = lane.offer(context.Background(), []byte{byte(i)}, now); ended {
					break
				}
			}

			require.Equal(t, tc.expectedEnded, ended)
			require.Equal(t, tc.expectedExcess, excess)
			require.Len(t, lane.pending, tc.expectedPending)
			for i, data := range lane.pending {
				require.Equal(t, []byte{byte(i)}, data)
			}
		})
	}
}
