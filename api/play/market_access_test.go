package play

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/api/geolocation"
	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/live"
	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/orchestrator/marketgate"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// Every interactive entry passes the market-and-age gate before anything
// names a Session, so a refusal reveals nothing about the Session asked for.
func TestJoinPassesTheMarketGate(t *testing.T) {
	type test struct {
		decision          compliance.AccessDecision
		routed            bool
		expectedRejection string
	}

	const caller = "0f8fad5b-d9cb-469f-a165-70867728950e"
	sessionUUID := uuid.NewString()
	tests := map[string]func(t *testing.T) test{
		"a_blocked_market_refuses_before_the_session_is_named": func(t *testing.T) test {
			return test{
				decision:          compliance.AccessDecision{Outcome: compliance.AccessBlocked, Reason: compliance.BlockMarketUnavailable},
				expectedRejection: `{"type":"rejected","reason":"market_restricted"}`,
			}
		},
		"a_declaration_that_would_be_owed_is_not_asked_for_at_entry": func(t *testing.T) test {
			return test{
				decision:          compliance.AccessDecision{Outcome: compliance.AccessNeedsDeclaration},
				routed:            true,
				expectedRejection: `{"type":"rejected","reason":"invalid_session"}`,
			}
		},
		"a_known_minority_refuses_like_a_restriction": func(t *testing.T) test {
			return test{
				decision:          compliance.AccessDecision{Outcome: compliance.AccessBlocked, Reason: compliance.BlockAgeRequirement},
				expectedRejection: `{"type":"rejected","reason":"market_restricted"}`,
			}
		},
		"an_unknown_location_admits_and_the_Session_decides": func(t *testing.T) test {
			return test{
				decision:          compliance.AccessDecision{Outcome: compliance.AccessAllowed},
				routed:            true,
				expectedRejection: `{"type":"rejected","reason":"invalid_session"}`,
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			locator := NewMockownerLocator(gomock.NewController(t))
			if tc.routed {
				locator.EXPECT().LocateOrClaimOwner(gomock.Any(), sessionUUID).
					Return(runtime.OwnerPlacement{Outcome: runtime.OwnerSessionNotFound}, nil)
			}
			// The local locator never knows a country: the gate is live and,
			// where nothing is known, open.
			gate := marketgate.New(&marketStub{decision: tc.decision})
			handler := NewHandler(nil, nil, locator, nil, nil, nil, nil, live.New(0), gate, geolocation.Resolver{Locator: geolocation.Unknown{}}, nil)
			serve := routeHandler(t, handler.WebSocketRoutes(), "GET /v1/play/join/sessions/{sessionUUID}/player")
			mux := http.NewServeMux()
			mux.HandleFunc("GET /v1/play/join/sessions/{sessionUUID}/player", func(w http.ResponseWriter, r *http.Request) {
				serve(w, r.WithContext(auth.WithCaller(r.Context(), identity.User{UUID: caller, Level: identity.LevelGuest})))
			})
			server := httptest.NewServer(mux)
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, server.URL+"/v1/play/join/sessions/"+sessionUUID+"/player", &websocket.DialOptions{
				Subprotocols: []string{"playhoot.v1"},
			})
			require.NoError(t, err, "the refusal is read on an upgraded connection")
			_, data, err := conn.Read(ctx)
			require.NoError(t, err)
			require.JSONEq(t, tc.expectedRejection, string(data))
		})
	}
}
