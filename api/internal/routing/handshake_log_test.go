package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/logging"
)

func TestFinishHandshakeLog(t *testing.T) {
	type test struct {
		ctx              context.Context
		finishCalls      int
		expectedMessages []string
	}

	tests := map[string]func(t *testing.T) test{
		"first_call_flushes_the_handshake_log": func(t *testing.T) test {
			return test{
				ctx:              StartHandshakeLog(logging.Start(context.Background()), "GET /sessions/{sessionUUID}/admin"),
				finishCalls:      1,
				expectedMessages: []string{"GET /sessions/{sessionUUID}/admin handshake"},
			}
		},
		"later_calls_do_not_flush_it_again": func(t *testing.T) test {
			return test{
				ctx:              StartHandshakeLog(logging.Start(context.Background()), "GET /sessions/{sessionUUID}/admin"),
				finishCalls:      3,
				expectedMessages: []string{"GET /sessions/{sessionUUID}/admin handshake"},
			}
		},
		"context_without_a_handshake_flushes_nothing": func(t *testing.T) test {
			return test{
				ctx:         logging.Start(context.Background()),
				finishCalls: 2,
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

			for range tc.finishCalls {
				FinishHandshakeLog(tc.ctx)
			}

			var messages []string
			for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
				if line == "" {
					continue
				}
				var record struct {
					Msg string `json:"msg"`
				}
				require.NoError(t, json.Unmarshal([]byte(line), &record))
				messages = append(messages, record.Msg)
			}
			require.Equal(t, tc.expectedMessages, messages)
		})
	}
}
