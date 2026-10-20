package throttle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

func TestRequests(t *testing.T) {
	type call struct {
		// userUUID is the caller the route authorized; empty for none.
		userUUID   string
		remoteAddr string
		after      time.Duration

		expectedStatus     int
		expectedRetryAfter string
		// expectedCheck are the labels the limit check was measured with;
		// nil when no limit applies.
		expectedCheck map[string]any
	}
	type test struct {
		limits routing.RequestLimits
		calls  []call
	}

	limits := routing.RequestLimits{
		PerUser:   []ratelimit.Limit{{Count: 2, Per: time.Second}, {Count: 3, Per: time.Minute}},
		PerClient: []ratelimit.Limit{{Count: 1, Per: time.Second}},
	}

	user := func(swept bool) map[string]any { return map[string]any{"key": "user", "swept": fmt.Sprint(swept)} }
	client := func(swept bool) map[string]any { return map[string]any{"key": "client", "swept": fmt.Sprint(swept)} }

	tests := map[string]func(t *testing.T) test{
		"a_user_is_limited_per_second_and_per_minute": func(t *testing.T) test {
			return test{
				limits: limits,
				calls: []call{
					{userUUID: "u1", remoteAddr: "10.0.0.1:1", expectedStatus: http.StatusOK, expectedCheck: user(true)},
					{userUUID: "u1", remoteAddr: "10.0.0.1:1", expectedStatus: http.StatusOK, expectedCheck: user(false)},
					{userUUID: "u1", remoteAddr: "10.0.0.1:1", expectedStatus: http.StatusTooManyRequests, expectedRetryAfter: "1", expectedCheck: user(false)},
					{userUUID: "u1", remoteAddr: "10.0.0.1:1", after: time.Second, expectedStatus: http.StatusOK, expectedCheck: user(false)},
					{userUUID: "u1", remoteAddr: "10.0.0.1:1", after: time.Second, expectedStatus: http.StatusTooManyRequests, expectedRetryAfter: "18", expectedCheck: user(false)},
					// A minute after the last sweep, a check sweeps again.
					{userUUID: "u1", remoteAddr: "10.0.0.1:1", after: time.Minute, expectedStatus: http.StatusOK, expectedCheck: user(true)},
				},
			}
		},
		"users_behind_one_address_do_not_limit_each_other": func(t *testing.T) test {
			return test{
				limits: limits,
				calls: []call{
					{userUUID: "u1", remoteAddr: "10.0.0.1:1", expectedStatus: http.StatusOK, expectedCheck: user(true)},
					{userUUID: "u2", remoteAddr: "10.0.0.1:2", expectedStatus: http.StatusOK, expectedCheck: user(false)},
					{userUUID: "u3", remoteAddr: "10.0.0.1:3", expectedStatus: http.StatusOK, expectedCheck: user(false)},
				},
			}
		},
		"a_caller_without_a_user_is_limited_by_address": func(t *testing.T) test {
			return test{
				limits: limits,
				calls: []call{
					{remoteAddr: "10.0.0.1:1", expectedStatus: http.StatusOK, expectedCheck: client(true)},
					{remoteAddr: "10.0.0.1:2", expectedStatus: http.StatusTooManyRequests, expectedRetryAfter: "1", expectedCheck: client(false)},
					{remoteAddr: "10.0.0.2:1", expectedStatus: http.StatusOK, expectedCheck: client(false)},
				},
			}
		},
		"no_limits_let_everything_through": func(t *testing.T) test {
			return test{
				calls: []call{
					{remoteAddr: "10.0.0.1:1", expectedStatus: http.StatusOK},
					{remoteAddr: "10.0.0.1:1", expectedStatus: http.StatusOK},
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
			handler := Requests(tc.limits, ClientAddress{}, func() time.Time { return now })(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			for i, c := range tc.calls {
				now = now.Add(c.after)
				r := httptest.NewRequest(http.MethodGet, "/", nil)
				r.RemoteAddr = c.remoteAddr
				ctx := logging.Start(r.Context())
				if c.userUUID != "" {
					ctx = auth.WithUserUUID(ctx, c.userUUID)
				}
				w := httptest.NewRecorder()
				handler(w, r.WithContext(ctx))

				require.Equal(t, c.expectedCheck, checkLabels(t, ctx), "call %d", i)

				require.Equal(t, c.expectedStatus, w.Code, "call %d", i)
				require.Equal(t, c.expectedRetryAfter, w.Header().Get("Retry-After"), "call %d", i)
				if c.expectedStatus == http.StatusTooManyRequests {
					var body httpx.ErrorResponse
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
					require.Equal(t, codeRateLimited, body.Code)
				}
			}
		})
	}
}

// checkLabels are the labels ctx's request log measured the limit check
// with; nil when it was not measured.
func checkLabels(t *testing.T, ctx context.Context) map[string]any {
	t.Helper()
	var output bytes.Buffer
	logging.FinishRequestLog(ctx, slog.New(slog.NewJSONHandler(&output, nil)), "test")
	var entry struct {
		Metrics []struct {
			Metric string         `json:"metric"`
			Labels map[string]any `json:"labels"`
		} `json:"metrics"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
	for _, measurement := range entry.Metrics {
		if measurement.Metric == "api_http_rate_limit_check_duration_seconds" {
			return measurement.Labels
		}
	}
	return nil
}

func TestClientAddressOf(t *testing.T) {
	type test struct {
		trustedProxies int
		forwardedFor   []string
		remoteAddr     string

		expected string
	}

	tests := map[string]func(t *testing.T) test{
		"without_trusted_proxies_the_connection_address_counts": func(t *testing.T) test {
			return test{
				forwardedFor: []string{"203.0.113.9"},
				remoteAddr:   "10.0.0.1:5000",
				expected:     "10.0.0.1",
			}
		},
		"one_trusted_proxy_appends_the_client_address": func(t *testing.T) test {
			return test{
				trustedProxies: 1,
				forwardedFor:   []string{"198.51.100.7, 203.0.113.9"},
				remoteAddr:     "10.0.0.1:5000",
				expected:       "203.0.113.9",
			}
		},
		"two_trusted_proxies_skip_the_one_in_front": func(t *testing.T) test {
			return test{
				trustedProxies: 2,
				forwardedFor:   []string{"198.51.100.7", "203.0.113.9, 192.0.2.1"},
				remoteAddr:     "10.0.0.1:5000",
				expected:       "203.0.113.9",
			}
		},
		"fewer_entries_than_proxies_take_the_first": func(t *testing.T) test {
			return test{
				trustedProxies: 3,
				forwardedFor:   []string{"203.0.113.9"},
				remoteAddr:     "10.0.0.1:5000",
				expected:       "203.0.113.9",
			}
		},
		"without_the_header_the_connection_address_counts": func(t *testing.T) test {
			return test{
				trustedProxies: 1,
				remoteAddr:     "10.0.0.1:5000",
				expected:       "10.0.0.1",
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remoteAddr
			for _, value := range tc.forwardedFor {
				r.Header.Add("X-Forwarded-For", value)
			}

			require.Equal(t, tc.expected, ClientAddress{TrustedProxies: tc.trustedProxies}.Of(r))
		})
	}
}
