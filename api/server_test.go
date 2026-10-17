package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/api/internal/wsx"
)

func TestRestObservability(t *testing.T) {
	type test struct {
		handler         http.HandlerFunc
		requestDuration time.Duration
		expectedCode    int
		expectedBody    string
		expectedMetrics []map[string]any
	}

	tests := map[string]func(t *testing.T) test{
		"handler_status_and_duration_are_measured_and_passed_through": func(t *testing.T) test {
			return test{
				handler: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte("missing"))
				},
				requestDuration: 120 * time.Millisecond,
				expectedCode:    http.StatusNotFound,
				expectedBody:    "missing",
				expectedMetrics: requestMeasurements(metricsTestPattern, "404", 0.12),
			}
		},
		"handler_that_writes_nothing_is_counted_as_ok": func(t *testing.T) test {
			return test{
				handler:         func(w http.ResponseWriter, r *http.Request) {},
				requestDuration: 5 * time.Millisecond,
				expectedCode:    http.StatusOK,
				expectedMetrics: requestMeasurements(metricsTestPattern, "200", 0.005),
			}
		},
		"unexpected_status_is_counted_as_other": func(t *testing.T) test {
			return test{
				handler:         func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) },
				requestDuration: time.Second,
				expectedCode:    http.StatusTeapot,
				expectedMetrics: requestMeasurements(metricsTestPattern, "other", 1),
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

			// The clock answers the start of the request, then its end.
			start := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
			clock := []time.Time{start, start.Add(tc.requestDuration)}
			requestMetrics := newRESTMetrics([]string{metricsTestPattern})
			requestMetrics.now = func() time.Time {
				now := clock[0]
				clock = clock[1:]
				return now
			}

			wrapped := restObservability(metricsTestPattern, requestMetrics)(tc.handler)
			recorder := httptest.NewRecorder()

			wrapped(recorder, httptest.NewRequest(http.MethodPost, "/sessions", nil))

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.Equal(t, tc.expectedBody, recorder.Body.String())

			var entry struct {
				Message string           `json:"msg"`
				Metrics []map[string]any `json:"metrics"`
			}
			require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
			require.Equal(t, metricsTestPattern, entry.Message)
			require.Equal(t, tc.expectedMetrics, entry.Metrics)
		})
	}
}

// wsMetricsTestPattern must match the WebSocket route set the real server
// declares, for the same reason as metricsTestPattern.
const wsMetricsTestPattern = "GET /sessions/{sessionUUID}/admin"

func TestWSObservability(t *testing.T) {
	type test struct {
		handler           http.HandlerFunc
		handshakeDuration time.Duration
		expectedUpgrade   bool
		expectedMetrics   []map[string]any
	}

	handshakeMeasurements := func(status string, seconds float64) []map[string]any {
		return []map[string]any{
			{
				"metric":    "api_websocket_handshakes_total",
				"operation": "add",
				"value":     float64(1),
				"labels":    map[string]any{"route": wsMetricsTestPattern, "status": status},
			},
			{
				"metric":    "api_websocket_handshake_duration_seconds",
				"operation": "observe",
				"value":     seconds,
				"labels":    map[string]any{"route": wsMetricsTestPattern},
			},
		}
	}

	tests := map[string]func(t *testing.T) test{
		"upgraded_handshake_is_counted_as_101": func(t *testing.T) test {
			return test{
				handler: func(w http.ResponseWriter, r *http.Request) {
					conn, err := wsx.Accept(w, r, nil)
					require.NoError(t, err)
					_ = conn.Close(websocket.StatusNormalClosure, "")
				},
				handshakeDuration: 30 * time.Millisecond,
				expectedUpgrade:   true,
				expectedMetrics:   handshakeMeasurements("101", 0.03),
			}
		},
		"refused_handshake_is_counted_with_its_status": func(t *testing.T) test {
			return test{
				handler: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
				},
				handshakeDuration: 5 * time.Millisecond,
				expectedMetrics:   handshakeMeasurements("401", 0.005),
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

			start := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
			clock := []time.Time{start, start.Add(tc.handshakeDuration)}
			handshakeMetrics := newHandshakeMetrics([]string{wsMetricsTestPattern})
			handshakeMetrics.now = func() time.Time {
				now := clock[0]
				clock = clock[1:]
				return now
			}

			done := make(chan struct{})
			wrapped := wsObservability(wsMetricsTestPattern, handshakeMetrics)(tc.handler)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(done)
				wrapped(w, r)
			}))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, server.URL, &websocket.DialOptions{Subprotocols: []string{wsx.Subprotocol}})
			if tc.expectedUpgrade {
				require.NoError(t, err)
				_, _, _ = conn.Read(ctx)
			} else {
				require.Error(t, err)
			}
			<-done

			var entry struct {
				Message string           `json:"msg"`
				Metrics []map[string]any `json:"metrics"`
			}
			require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
			require.Equal(t, wsMetricsTestPattern, entry.Message)
			require.Equal(t, tc.expectedMetrics, entry.Metrics)
		})
	}
}
