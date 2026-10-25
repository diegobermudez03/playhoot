package wsx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestAcceptOrigins(t *testing.T) {
	type test struct {
		allowed []string
		origin  string

		expectedUpgrade bool
	}

	tests := map[string]func(t *testing.T) test{
		"listed_origin_is_accepted": func(t *testing.T) test {
			return test{allowed: []string{"https://play.example.com"}, origin: "https://play.example.com", expectedUpgrade: true}
		},
		"same_host_with_another_scheme_is_refused": func(t *testing.T) test {
			return test{allowed: []string{"https://play.example.com"}, origin: "http://play.example.com"}
		},
		"unlisted_origin_is_refused": func(t *testing.T) test {
			return test{allowed: []string{"https://play.example.com"}, origin: "https://evil.example.com"}
		},
		"any_origin": func(t *testing.T) test {
			return test{allowed: []string{"*"}, origin: "http://localhost:5173", expectedUpgrade: true}
		},
		"ipv6_origin_is_matched_literally": func(t *testing.T) test {
			return test{allowed: []string{"http://[::1]:5173"}, origin: "http://[::1]:5173", expectedUpgrade: true}
		},
		"nothing_listed_refuses_other_origins": func(t *testing.T) test {
			return test{origin: "https://play.example.com"}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			patterns := OriginPatterns(tc.allowed)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := Accept(w, r, patterns)
				if err == nil {
					_ = conn.Close(websocket.StatusNormalClosure, "")
				}
			}))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, response, err := websocket.Dial(ctx, server.URL, &websocket.DialOptions{
				Subprotocols: []string{Subprotocol},
				HTTPHeader:   http.Header{"Origin": []string{tc.origin}},
			})
			if !tc.expectedUpgrade {
				require.Error(t, err)
				require.Equal(t, http.StatusForbidden, response.StatusCode)
				return
			}
			require.NoError(t, err)
			_ = conn.CloseNow()
		})
	}
}
