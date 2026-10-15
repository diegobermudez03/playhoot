package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
