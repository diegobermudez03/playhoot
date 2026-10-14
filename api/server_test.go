package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRestObservability(t *testing.T) {
	type test struct {
		handler         http.HandlerFunc
		expectedCode    int
		expectedBody    string
		expectedMetrics []map[string]any
	}

	tests := map[string]func(t *testing.T) test{
		"handler_status_is_counted_and_passed_through": func(t *testing.T) test {
			return test{
				handler: func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte("missing"))
				},
				expectedCode:    http.StatusNotFound,
				expectedBody:    "missing",
				expectedMetrics: []map[string]any{requestMetric(metricsTestPattern, "404")},
			}
		},
		"handler_that_writes_nothing_is_counted_as_ok": func(t *testing.T) test {
			return test{
				handler:         func(w http.ResponseWriter, r *http.Request) {},
				expectedCode:    http.StatusOK,
				expectedMetrics: []map[string]any{requestMetric(metricsTestPattern, "200")},
			}
		},
		"unexpected_status_is_counted_as_other": func(t *testing.T) test {
			return test{
				handler:         func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) },
				expectedCode:    http.StatusTeapot,
				expectedMetrics: []map[string]any{requestMetric(metricsTestPattern, "other")},
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

			wrapped := restObservability(metricsTestPattern, newRESTMetrics([]string{metricsTestPattern}))(tc.handler)
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
