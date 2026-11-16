package notifications

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/notifications"
)

// stubAuthorizer: "admin" is the administrator, "guest" a guest, anything
// else unauthenticated. Guests may only read notifications.
type stubAuthorizer struct{}

func (stubAuthorizer) Authorize(_ context.Context, token string, op identity.Operation) (identity.User, identity.AccessDecision, error) {
	switch token {
	case "admin":
		return identity.User{UUID: "admin-uuid", Level: identity.LevelAdmin}, identity.AccessAllowed, nil
	case "guest":
		if op == identity.OperationReadNotifications {
			return identity.User{UUID: "guest-uuid", Level: identity.LevelGuest}, identity.AccessAllowed, nil
		}
		return identity.User{UUID: "guest-uuid", Level: identity.LevelGuest}, identity.AccessForbidden, nil
	}
	return identity.User{}, identity.AccessUnauthenticated, nil
}

type stub struct {
	filter   notifications.EntryFilter
	user     string
	webhook  error
	modeSeen notifications.Mode
}

func (s *stub) ListEntries(_ context.Context, user string, f notifications.EntryFilter) (notifications.EntryPage, error) {
	s.user, s.filter = user, f
	return notifications.EntryPage{Unread: 2}, nil
}

func (s *stub) MarkRead(_ context.Context, _ string, entry string) (notifications.MarkReadOutcome, error) {
	if entry == "missing" {
		return notifications.UnknownEntry, nil
	}
	return notifications.MarkedRead, nil
}

func (s *stub) ReadPreferences(context.Context, string) (notifications.Preferences, error) {
	return notifications.DefaultPreferences, nil
}

func (s *stub) SetPreferences(_ context.Context, _ string, p notifications.Preferences) (notifications.Preferences, error) {
	return p, nil
}

func (s *stub) Unsubscribe(_ context.Context, token string) (notifications.SuppressionOutcome, error) {
	if token == "bad" {
		return notifications.SuppressionInvalid, nil
	}
	return notifications.SuppressionRecorded, nil
}

func (s *stub) ProviderWebhook(context.Context, notifications.Provider, map[string][]string, []byte) (int, error) {
	return 1, s.webhook
}

func (s *stub) AdminEmailStatus(context.Context) (notifications.EmailStatus, error) {
	return notifications.EmailStatus{Mode: notifications.ModeAutomatic}, nil
}

func (s *stub) AdminSetEmailMode(_ context.Context, _ string, mode notifications.Mode, _ string) (notifications.SetModeOutcome, error) {
	s.modeSeen = mode
	if !notifications.ValidMode(mode) {
		return notifications.ModeInvalid, nil
	}
	return notifications.ModeChanged, nil
}

func TestHandler(t *testing.T) {
	serve := func(s *stub, method, path, token, body string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		for _, route := range NewHandler(stubAuthorizer{}, Dependencies{Entries: s, Reads: s, Preferences: s, Suppression: s, Panel: s}).RESTRoutes() {
			handler := route.Handler
			if route.Authorization != nil {
				handler = route.Authorization(handler)
			}
			mux.HandleFunc(route.Pattern, handler)
		}
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder
	}
	tests := map[string]func(t *testing.T){
		"a guest lists their notifications with filters": func(t *testing.T) {
			s := &stub{}
			res := serve(s, http.MethodGet, "/v1/notifications?domain=budget&limit=5", "guest", "")
			require.Equal(t, http.StatusOK, res.Code)
			require.Equal(t, "guest-uuid", s.user)
			require.Equal(t, "budget", s.filter.SourceDomain)
			require.Equal(t, 5, s.filter.Limit)
			require.Contains(t, res.Body.String(), `"unread":2`)
		},
		"marking an unknown entry is not found": func(t *testing.T) {
			require.Equal(t, http.StatusNotFound, serve(&stub{}, http.MethodPost, "/v1/notifications/missing/read", "guest", "").Code)
			require.Equal(t, http.StatusOK, serve(&stub{}, http.MethodPost, "/v1/notifications/e/read", "guest", "").Code)
		},
		"preferences need an account": func(t *testing.T) {
			require.Equal(t, http.StatusForbidden, serve(&stub{}, http.MethodPut, "/v1/notifications/preferences", "guest", `{}`).Code)
			res := serve(&stub{}, http.MethodPut, "/v1/notifications/preferences", "admin", `{"ai_email":true}`)
			require.Equal(t, http.StatusOK, res.Code)
			require.Contains(t, res.Body.String(), `"ai_email":true`)
		},
		"unsubscribe needs no token and refuses unknown links": func(t *testing.T) {
			require.Equal(t, http.StatusOK, serve(&stub{}, http.MethodGet, "/v1/email/unsubscribe/good", "", "").Code)
			require.Equal(t, http.StatusNotFound, serve(&stub{}, http.MethodGet, "/v1/email/unsubscribe/bad", "", "").Code)
		},
		"a rejected webhook is unauthorized and an unknown provider not found": func(t *testing.T) {
			require.Equal(t, http.StatusOK, serve(&stub{}, http.MethodPost, "/v1/webhooks/email/ses", "", `{}`).Code)
			require.Equal(t, http.StatusUnauthorized, serve(&stub{webhook: notifications.ErrWebhookRejected}, http.MethodPost, "/v1/webhooks/email/ses", "", `{}`).Code)
			require.Equal(t, http.StatusServiceUnavailable, serve(&stub{webhook: errors.New("db down")}, http.MethodPost, "/v1/webhooks/email/ses", "", `{}`).Code)
			require.Equal(t, http.StatusNotFound, serve(&stub{}, http.MethodPost, "/v1/webhooks/email/mailgun", "", `{}`).Code)
		},
		"the admin panel reads and switches the mode": func(t *testing.T) {
			require.Equal(t, http.StatusForbidden, serve(&stub{}, http.MethodGet, "/admin/v1/email", "guest", "").Code)
			require.Equal(t, http.StatusOK, serve(&stub{}, http.MethodGet, "/admin/v1/email", "admin", "").Code)
			s := &stub{}
			require.Equal(t, http.StatusOK, serve(s, http.MethodPut, "/admin/v1/email/mode", "admin", `{"mode":"ses-only","reason":"incident"}`).Code)
			require.Equal(t, notifications.ModeSESOnly, s.modeSeen)
			require.Equal(t, http.StatusBadRequest, serve(&stub{}, http.MethodPut, "/admin/v1/email/mode", "admin", `{"mode":"x","reason":"y"}`).Code)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
