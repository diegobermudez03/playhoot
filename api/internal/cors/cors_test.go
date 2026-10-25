package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseOrigins(t *testing.T) {
	type test struct {
		raw string

		expected  []string
		expectErr bool
	}

	tests := map[string]func(t *testing.T) test{
		"origins_are_canonical": func(t *testing.T) test {
			return test{raw: " https://Play.Example.com , http://localhost:5173,,", expected: []string{"https://play.example.com", "http://localhost:5173"}}
		},
		"star_allows_any_origin": func(t *testing.T) test {
			return test{raw: "*", expected: []string{"*"}}
		},
		"empty_allows_none": func(t *testing.T) test {
			return test{raw: ""}
		},
		"ipv6_host": func(t *testing.T) test {
			return test{raw: "http://[::1]:5173", expected: []string{"http://[::1]:5173"}}
		},
		"star_with_other_origins": func(t *testing.T) test {
			return test{raw: "*,https://play.example.com", expectErr: true}
		},
		"host_without_scheme": func(t *testing.T) test {
			return test{raw: "play.example.com", expectErr: true}
		},
		"host_pattern": func(t *testing.T) test {
			return test{raw: "https://*.example.com", expectErr: true}
		},
		"origin_with_a_path": func(t *testing.T) test {
			return test{raw: "https://play.example.com/", expectErr: true}
		},
		"scheme_other_than_http": func(t *testing.T) test {
			return test{raw: "ftp://play.example.com", expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			origins, err := ParseOrigins(tc.raw)
			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, origins)
		})
	}
}

func TestPolicy(t *testing.T) {
	type test struct {
		allowed []string
		// origin is the request's Origin header; empty sends none.
		origin string

		// expectedOrigin is Access-Control-Allow-Origin; empty when the
		// origin may not read the response.
		expectedOrigin string
	}

	tests := map[string]func(t *testing.T) test{
		"allowed_origin_is_echoed": func(t *testing.T) test {
			return test{allowed: []string{"https://play.example.com"}, origin: "https://play.example.com", expectedOrigin: "https://play.example.com"}
		},
		"disallowed_origin_gets_no_permission": func(t *testing.T) test {
			return test{allowed: []string{"https://play.example.com"}, origin: "https://evil.example.com"}
		},
		"any_origin_echoes_the_request_origin": func(t *testing.T) test {
			return test{allowed: []string{AnyOrigin}, origin: "http://localhost:5173", expectedOrigin: "http://localhost:5173"}
		},
		"same_scheme_and_host_on_another_port_is_another_origin": func(t *testing.T) test {
			return test{allowed: []string{"http://localhost:5173"}, origin: "http://localhost:3000"}
		},
		"request_without_origin_gets_no_permission": func(t *testing.T) test {
			return test{allowed: []string{AnyOrigin}}
		},
		"nothing_allowed": func(t *testing.T) test {
			return test{origin: "https://play.example.com"}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			policy := NewPolicy(tc.allowed)
			allowed := tc.expectedOrigin != ""

			t.Run("response", func(t *testing.T) {
				r := httptest.NewRequest(http.MethodPost, "/v1/guests", nil)
				if tc.origin != "" {
					r.Header.Set("Origin", tc.origin)
				}
				w := httptest.NewRecorder()
				served := false
				policy.Responses()(func(w http.ResponseWriter, r *http.Request) {
					served = true
					w.Header().Set("Retry-After", "3")
					w.WriteHeader(http.StatusTooManyRequests)
				})(w, r)

				require.True(t, served, "the request is served whatever its origin")
				require.Equal(t, http.StatusTooManyRequests, w.Code)
				require.Equal(t, tc.expectedOrigin, w.Header().Get("Access-Control-Allow-Origin"))
				require.Equal(t, []string{"Origin"}, w.Header().Values("Vary"))
				require.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
				if allowed {
					require.Equal(t, "Retry-After", w.Header().Get("Access-Control-Expose-Headers"))
				} else {
					require.Empty(t, w.Header().Get("Access-Control-Expose-Headers"))
				}
			})

			t.Run("preflight", func(t *testing.T) {
				r := httptest.NewRequest(http.MethodOptions, "/v1/sessions", nil)
				if tc.origin != "" {
					r.Header.Set("Origin", tc.origin)
				}
				r.Header.Set("Access-Control-Request-Method", http.MethodPost)
				r.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
				w := httptest.NewRecorder()
				policy.Preflight([]string{http.MethodPost})(w, r)

				require.Equal(t, http.StatusNoContent, w.Code)
				require.Equal(t, tc.expectedOrigin, w.Header().Get("Access-Control-Allow-Origin"))
				require.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
				if allowed {
					require.Equal(t, "POST", w.Header().Get("Access-Control-Allow-Methods"))
					require.Equal(t, "Authorization, Content-Type", w.Header().Get("Access-Control-Allow-Headers"))
					require.Equal(t, "3600", w.Header().Get("Access-Control-Max-Age"))
				} else {
					require.Empty(t, w.Header().Get("Access-Control-Allow-Methods"))
					require.Empty(t, w.Header().Get("Access-Control-Allow-Headers"))
					require.Empty(t, w.Header().Get("Access-Control-Max-Age"))
				}
			})
		})
	}
}
