package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/logging"
)

// metricsTestPattern must match the route set the real server declares: the
// request metric's route label is shared process-wide, so declaring it with a
// different set in the same test binary would conflict.
const metricsTestPattern = "POST /sessions"

// flushedMetrics ends the request log carried by ctx and returns the metric
// measurements it recorded.
func flushedMetrics(t *testing.T, ctx context.Context) []map[string]any {
	t.Helper()

	var output bytes.Buffer
	logging.FinishRequestLog(ctx, slog.New(slog.NewJSONHandler(&output, nil)), "test")

	var entry struct {
		Metrics []map[string]any `json:"metrics"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
	return entry.Metrics
}

func requestMetric(route string, status string) map[string]any {
	return map[string]any{
		"metric":    "api_http_requests_total",
		"operation": "add",
		"value":     float64(1),
		"labels":    map[string]any{"route": route, "status": status},
	}
}

func TestRESTMetricsRecord(t *testing.T) {
	type test struct {
		pattern         string
		status          int
		expectedMetrics []map[string]any
		expectPanic     bool
	}

	tests := map[string]func(t *testing.T) test{
		"tracked_status": func(t *testing.T) test {
			return test{
				pattern:         metricsTestPattern,
				status:          http.StatusCreated,
				expectedMetrics: []map[string]any{requestMetric(metricsTestPattern, "201")},
			}
		},
		"every_tracked_status_keeps_its_own_value": func(t *testing.T) test {
			return test{
				pattern:         metricsTestPattern,
				status:          http.StatusGatewayTimeout,
				expectedMetrics: []map[string]any{requestMetric(metricsTestPattern, "504")},
			}
		},
		"untracked_status_is_counted_as_other": func(t *testing.T) test {
			return test{
				pattern:         metricsTestPattern,
				status:          http.StatusTeapot,
				expectedMetrics: []map[string]any{requestMetric(metricsTestPattern, "other")},
			}
		},
		"status_outside_http_range_is_counted_as_other": func(t *testing.T) test {
			return test{
				pattern:         metricsTestPattern,
				status:          999,
				expectedMetrics: []map[string]any{requestMetric(metricsTestPattern, "other")},
			}
		},
		"route_that_was_not_declared": func(t *testing.T) test {
			return test{pattern: "GET /not-declared", status: http.StatusOK, expectPanic: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			requestMetrics := newRESTMetrics([]string{metricsTestPattern})
			ctx := logging.Start(context.Background())

			if tc.expectPanic {
				require.Panics(t, func() { requestMetrics.record(ctx, tc.pattern, tc.status) })
				return
			}
			requestMetrics.record(ctx, tc.pattern, tc.status)

			require.Equal(t, tc.expectedMetrics, flushedMetrics(t, ctx))
		})
	}
}

func TestNewRESTMetrics(t *testing.T) {
	type test struct {
		patterns    []string
		expectPanic bool
	}

	tests := map[string]func(t *testing.T) test{
		"declared_route": func(t *testing.T) test {
			return test{patterns: []string{metricsTestPattern}}
		},
		"same_route_set_again": func(t *testing.T) test {
			return test{patterns: []string{metricsTestPattern}}
		},
		"no_routes": func(t *testing.T) test {
			return test{expectPanic: true}
		},
		"duplicated_route": func(t *testing.T) test {
			return test{patterns: []string{metricsTestPattern, metricsTestPattern}, expectPanic: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			if tc.expectPanic {
				require.Panics(t, func() { newRESTMetrics(tc.patterns) })
				return
			}
			require.NotNil(t, newRESTMetrics(tc.patterns))
		})
	}
}

func TestStatusRecorder(t *testing.T) {
	type test struct {
		write          func(w http.ResponseWriter)
		expectedStatus int
		expectedCode   int
		expectedBody   string
	}

	tests := map[string]func(t *testing.T) test{
		"nothing_written_defaults_to_ok": func(t *testing.T) test {
			return test{
				write:          func(w http.ResponseWriter) {},
				expectedStatus: http.StatusOK,
				expectedCode:   http.StatusOK,
			}
		},
		"body_without_explicit_status_is_ok": func(t *testing.T) test {
			return test{
				write:          func(w http.ResponseWriter) { _, _ = w.Write([]byte("hello")) },
				expectedStatus: http.StatusOK,
				expectedCode:   http.StatusOK,
				expectedBody:   "hello",
			}
		},
		"explicit_status": func(t *testing.T) test {
			return test{
				write: func(w http.ResponseWriter) {
					w.WriteHeader(http.StatusConflict)
					_, _ = w.Write([]byte("conflict"))
				},
				expectedStatus: http.StatusConflict,
				expectedCode:   http.StatusConflict,
				expectedBody:   "conflict",
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			underlying := httptest.NewRecorder()
			recorder := newStatusRecorder(underlying)

			tc.write(recorder)

			require.Equal(t, tc.expectedStatus, recorder.status)
			require.Equal(t, tc.expectedCode, underlying.Code)
			require.Equal(t, tc.expectedBody, underlying.Body.String())
		})
	}
}
