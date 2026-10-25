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

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/cors"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/api/internal/throttle"
	"github.com/diegobermudez03/playhoot/api/internal/wsx"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
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

	handshakeMeasurements := func(status string, limitRefusal string, seconds float64) []map[string]any {
		return []map[string]any{
			{
				"metric":    "api_websocket_handshakes_total",
				"operation": "add",
				"value":     float64(1),
				"labels":    map[string]any{"route": wsMetricsTestPattern, "status": status, "limit_refusal": limitRefusal},
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
				expectedMetrics:   handshakeMeasurements("101", "none", 0.03),
			}
		},
		"limit_refusal_told_after_the_upgrade_is_counted_with_its_reason": func(t *testing.T) test {
			return test{
				handler: func(w http.ResponseWriter, r *http.Request) {
					routing.NoteLimitRefusal(r.Context(), routing.RefusedRateLimited)
					conn, err := wsx.Accept(w, r, nil)
					require.NoError(t, err)
					_ = conn.Close(websocket.StatusPolicyViolation, "admission rejected")
				},
				handshakeDuration: 2 * time.Millisecond,
				expectedUpgrade:   true,
				expectedMetrics:   handshakeMeasurements("101", "rate_limited", 0.002),
			}
		},
		"limit_refusal_before_the_upgrade_is_counted_with_its_status_and_reason": func(t *testing.T) test {
			return test{
				handler: func(w http.ResponseWriter, r *http.Request) {
					routing.NoteLimitRefusal(r.Context(), routing.RefusedServerBusy)
					w.WriteHeader(http.StatusServiceUnavailable)
				},
				handshakeDuration: 3 * time.Millisecond,
				expectedMetrics:   handshakeMeasurements("503", "server_busy", 0.003),
			}
		},
		"refused_handshake_is_counted_with_its_status": func(t *testing.T) test {
			return test{
				handler: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
				},
				handshakeDuration: 5 * time.Millisecond,
				expectedMetrics:   handshakeMeasurements("401", "none", 0.005),
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

func TestRouteMiddlewares(t *testing.T) {
	type test struct {
		route    routing.Route
		requests int

		expectedStatuses []int
		// expectedPassed is what each request passed through, in order.
		expectedPassed [][]string
	}

	limitedToOne := &routing.RequestLimits{
		PerUser:   []ratelimit.Limit{{Count: 1, Per: time.Hour}},
		PerClient: []ratelimit.Limit{{Count: 1, Per: time.Hour}},
	}

	tests := map[string]func(t *testing.T, passed *[]string) test{
		"an_authorized_caller_is_limited_after_authorization_and_before_the_route": func(t *testing.T, passed *[]string) test {
			return test{
				route: routing.Route{
					Authorization: func(next http.HandlerFunc) http.HandlerFunc {
						return func(w http.ResponseWriter, r *http.Request) {
							*passed = append(*passed, "authorization")
							next(w, r.WithContext(auth.WithCaller(r.Context(), identity.User{UUID: "u1"})))
						}
					},
					Limits: limitedToOne,
					Middlewares: []routing.Middleware{func(next http.HandlerFunc) http.HandlerFunc {
						return func(w http.ResponseWriter, r *http.Request) {
							*passed = append(*passed, "route")
							next(w, r)
						}
					}},
				},
				requests:         2,
				expectedStatuses: []int{http.StatusOK, http.StatusTooManyRequests},
				expectedPassed:   [][]string{{"observability", "authorization", "route", "handler"}, {"observability", "authorization"}},
			}
		},
		"a_route_without_authorization_is_limited_by_client": func(t *testing.T, passed *[]string) test {
			return test{
				route:            routing.Route{Limits: limitedToOne},
				requests:         2,
				expectedStatuses: []int{http.StatusOK, http.StatusTooManyRequests},
				expectedPassed:   [][]string{{"observability", "handler"}, {"observability"}},
			}
		},
		"a_route_without_its_own_limits_takes_the_defaults": func(t *testing.T, passed *[]string) test {
			statuses := make([]int, throttle.DefaultLimits.PerClient[0].Count+1)
			passes := make([][]string, len(statuses))
			for i := range statuses {
				statuses[i] = http.StatusOK
				passes[i] = []string{"observability", "handler"}
			}
			statuses[len(statuses)-1] = http.StatusTooManyRequests
			passes[len(passes)-1] = []string{"observability"}
			return test{
				route:            routing.Route{},
				requests:         len(statuses),
				expectedStatuses: statuses,
				expectedPassed:   passes,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			var passed []string
			tc := setup(t, &passed)
			observability := func(next http.HandlerFunc) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					passed = append(passed, "observability")
					next(w, r)
				}
			}
			handler := chain(routeMiddlewares(tc.route, throttle.ClientAddress{}, observability), func(w http.ResponseWriter, r *http.Request) {
				passed = append(passed, "handler")
			})

			for i := range tc.requests {
				passed = nil
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				w := httptest.NewRecorder()
				handler(w, r)
				require.Equal(t, tc.expectedStatuses[i], w.Code, "request %d", i)
				require.Equal(t, tc.expectedPassed[i], passed, "request %d", i)
			}
		})
	}
}

func TestPreflightRoutes(t *testing.T) {
	noop := func(w http.ResponseWriter, r *http.Request) {}
	routes := []routing.Route{
		{Pattern: "POST /v1/sessions", Handler: noop},
		{Pattern: "POST /v1/guests", Handler: noop},
		{Pattern: "GET /v1/sessions", Handler: noop},
	}

	preflights := preflightRoutes(routes, cors.NewPolicy([]string{"https://play.example.com"}))

	patterns := make([]string, len(preflights))
	allowed := make(map[string]string, len(preflights))
	for i, preflight := range preflights {
		patterns[i] = preflight.Pattern
		r := httptest.NewRequest(http.MethodOptions, "/", nil)
		r.Header.Set("Origin", "https://play.example.com")
		w := httptest.NewRecorder()
		preflight.Handler(w, r)
		require.Equal(t, http.StatusNoContent, w.Code)
		allowed[preflight.Pattern] = w.Header().Get("Access-Control-Allow-Methods")
	}
	require.Equal(t, []string{"OPTIONS /v1/sessions", "OPTIONS /v1/guests"}, patterns)
	require.Equal(t, map[string]string{"OPTIONS /v1/sessions": "POST, GET", "OPTIONS /v1/guests": "POST"}, allowed)
}

func TestRESTRefusalsAreReadableCrossOrigin(t *testing.T) {
	type test struct {
		origin string

		expectedOrigin string
	}

	tests := map[string]func(t *testing.T) test{
		"allowed_origin_reads_the_refusal": func(t *testing.T) test {
			return test{origin: "https://play.example.com", expectedOrigin: "https://play.example.com"}
		},
		"disallowed_origin_is_refused_the_same_without_permission": func(t *testing.T) test {
			return test{origin: "https://evil.example.com"}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			route := routing.Route{Limits: &routing.RequestLimits{PerClient: []ratelimit.Limit{{Count: 1, Per: time.Hour}}}}
			policy := cors.NewPolicy([]string{"https://play.example.com"})
			handler := chain(routeMiddlewares(route, throttle.ClientAddress{}, policy.Responses()), func(w http.ResponseWriter, r *http.Request) {})

			var w *httptest.ResponseRecorder
			for range 2 {
				r := httptest.NewRequest(http.MethodPost, "/v1/guests", nil)
				r.Header.Set("Origin", tc.origin)
				w = httptest.NewRecorder()
				handler(w, r)
			}

			require.Equal(t, http.StatusTooManyRequests, w.Code, "REST keeps its HTTP refusal")
			require.NotEmpty(t, w.Header().Get("Retry-After"))
			require.Equal(t, tc.expectedOrigin, w.Header().Get("Access-Control-Allow-Origin"))
		})
	}
}
